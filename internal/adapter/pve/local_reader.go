package pve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	"github.com/DjangoAILab/pve-storage-guard/internal/config"
)

type procReader interface {
	ReadFile(string) ([]byte, error)
}

type osProcReader struct{}

func (osProcReader) ReadFile(path string) ([]byte, error) {
	if path != "/proc/pressure/io" && path != "/proc/diskstats" {
		return nil, errors.New("procfs path is not allowlisted")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	payload, err := io.ReadAll(io.LimitReader(file, maxCommandOutput+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > maxCommandOutput {
		return nil, errors.New("procfs output exceeded safety limit")
	}
	return payload, nil
}

// LocalReader is the concrete same-host, read-only PVE/OpenZFS adapter.
type LocalReader struct {
	config config.PVEAgentConfig
	runner commandRunner
	proc   procReader
	now    func() time.Time
	id     func() (string, error)
}

var _ Reader = (*LocalReader)(nil)

// NewLocalReader constructs a reader with fixed local commands and procfs paths.
func NewLocalReader(document config.PVEAgentConfig) (*LocalReader, error) {
	if err := document.Validate(); err != nil {
		return nil, err
	}
	return &LocalReader{
		config: document,
		runner: localCommandRunner{timeout: time.Duration(document.Spec.CommandTimeoutSeconds) * time.Second},
		proc:   osProcReader{},
		now:    func() time.Time { return time.Now().UTC() },
		id:     newObservationID,
	}, nil
}

func (r *LocalReader) request() commandRequest {
	return commandRequest{Node: r.config.Spec.Node, Storage: r.config.Spec.Storage, ZPool: r.config.Spec.ZPool, IntervalSeconds: r.config.Spec.SampleIntervalSeconds}
}

// Inventory returns only explicitly enrolled resources after verifying binding.
func (r *LocalReader) Inventory(ctx context.Context) ([]InventoryDisk, error) {
	healthy, err := r.managementHealthy(ctx)
	if err != nil || !healthy {
		return nil, errors.New("PVE management plane is unavailable")
	}
	if err := r.verifyStorage(ctx); err != nil {
		return nil, err
	}
	if _, err := r.readDiskSignals(); err != nil {
		return nil, errors.New("configured device inventory is unavailable")
	}
	result := make([]InventoryDisk, 0, len(r.config.Spec.Resources))
	for _, resource := range r.config.Spec.Resources {
		result = append(result, InventoryDisk{ResourceKey: resource.ResourceKey, DomainKey: r.config.Spec.DomainKey, Root: resource.Root, Critical: resource.Critical})
	}
	return result, nil
}

// InventorySnapshot creates an identity-safe public wire snapshot.
func (r *LocalReader) InventorySnapshot(ctx context.Context) (v1.PVEInventory, error) {
	resources, err := r.Inventory(ctx)
	if err != nil {
		return v1.PVEInventory{}, err
	}
	result := v1.PVEInventory{SchemaVersion: v1.SchemaVersion, Kind: v1.PVEInventoryKind, ObservedAt: r.now(), DomainKey: r.config.Spec.DomainKey, StorageType: "zfspool", Resources: make([]v1.PVEInventoryDisk, 0, len(resources))}
	for _, resource := range resources {
		result.Resources = append(result.Resources, v1.PVEInventoryDisk{ResourceKey: resource.ResourceKey, Root: resource.Root, Critical: resource.Critical})
	}
	return result, nil
}

// Observe returns one normalized storage-domain sample.
func (r *LocalReader) Observe(ctx context.Context, domainKey string, _ time.Time) (v1.Observation, error) {
	if domainKey != r.config.Spec.DomainKey {
		return v1.Observation{}, errors.New("requested storage domain is not configured")
	}
	id, err := r.id()
	if err != nil {
		return v1.Observation{}, errors.New("create observation identifier")
	}
	observation := v1.Observation{SchemaVersion: v1.SchemaVersion, ID: id, DomainKey: r.config.Spec.DomainKey}
	managementHealthy, _ := r.managementHealthy(ctx)
	observation.ManagementPlaneHealthy = managementHealthy
	if err := r.verifyStorage(ctx); err != nil {
		return v1.Observation{}, err
	}
	payload, err := r.runner.Run(ctx, opZFSWaitHistogram, r.request())
	if err != nil {
		return v1.Observation{}, errors.New("write-wait histogram is unavailable")
	}
	waitMilliseconds, sampleWeight, bucketUpper, err := parseHistogramP95(payload, r.config.Spec.ZPool)
	if err != nil && !errors.Is(err, errNoWriteSamples) {
		return v1.Observation{}, errors.New("write-wait histogram is invalid")
	}
	if err == nil {
		observation.WriteWaitP95Milliseconds = waitMilliseconds
		observation.WaitValid = true
		observation.Emergency = waitMilliseconds >= r.config.Spec.EmergencyWaitMilliseconds
		observation.WaitEvidence = &v1.WaitEvidence{
			MeasurementLayer: "storage-domain", Statistic: "p95-upper-bound", Source: "openzfs-total-wait-histogram", Provenance: "observed",
			SampleIntervalSeconds: r.config.Spec.SampleIntervalSeconds, SampleWeight: sampleWeight, BucketUpperBoundNanoseconds: bucketUpper,
		}
	}
	if pressurePayload, readErr := r.proc.ReadFile("/proc/pressure/io"); readErr == nil {
		if pressure, parseErr := parsePSI(pressurePayload); parseErr == nil {
			observation.IOPressure = &pressure
		}
	}
	if signals, readErr := r.readDiskSignals(); readErr == nil {
		observation.DiskSignals = signals
	}
	observation.ObservedAt = r.now()
	return observation, nil
}

func (r *LocalReader) managementHealthy(ctx context.Context) (bool, error) {
	payload, err := r.runner.Run(ctx, opClusterStatus, r.request())
	if err != nil {
		return false, err
	}
	return parseClusterHealthy(payload, r.config.Spec.Node)
}

func (r *LocalReader) verifyStorage(ctx context.Context) error {
	configPayload, err := r.runner.Run(ctx, opStorageConfig, r.request())
	if err != nil {
		return errors.New("PVE storage binding is unavailable")
	}
	binding, err := parseStorageConfig(configPayload)
	if err != nil || binding.StorageType != "zfspool" || validatePoolBinding(r.config.Spec.ZPool, binding.Pool) != nil {
		return errors.New("PVE storage binding is invalid")
	}
	statusPayload, err := r.runner.Run(ctx, opStorageStatus, r.request())
	if err != nil {
		return errors.New("PVE storage status is unavailable")
	}
	active, storageType, err := parseStorageActive(statusPayload)
	if err != nil || !active || storageType != "zfspool" {
		return errors.New("PVE storage is not active zfspool storage")
	}
	layoutPayload, err := r.runner.Run(ctx, opZFSWaitHistogramLayout, r.request())
	if err != nil || parseHistogramLayout(layoutPayload, r.config.Spec.ZPool) != nil {
		return errors.New("OpenZFS wait histogram layout is unsupported")
	}
	return nil
}

func (r *LocalReader) readDiskSignals() ([]v1.DiskSignal, error) {
	payload, err := r.proc.ReadFile("/proc/diskstats")
	if err != nil {
		return nil, err
	}
	devices := make(map[string]string, len(r.config.Spec.Resources))
	for _, resource := range r.config.Spec.Resources {
		devices[resource.KernelDevice] = resource.ResourceKey
	}
	return parseDiskstats(payload, devices)
}

func newObservationID() (string, error) {
	data := make([]byte, 12)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("random identifier: %w", err)
	}
	return "observation-" + hex.EncodeToString(data), nil
}
