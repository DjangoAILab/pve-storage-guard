// Package config decodes versioned user configuration into core policy types.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strings"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	"github.com/DjangoAILab/pve-storage-guard/internal/allocator"
	"github.com/DjangoAILab/pve-storage-guard/internal/policy"
)

var (
	opaqueKeyPattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	privateIDPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	workloadIDPattern  = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)
	qemuDiskKeyPattern = regexp.MustCompile(`^(?:ide|sata|scsi|virtio)[0-9]{1,2}$`)
)

// StorageDomainPolicy is the v1alpha1 JSON policy document.
type StorageDomainPolicy struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"metadata"`
	Spec struct {
		Mode                   string `json:"mode"`
		ControlIntervalSeconds int    `json:"controlIntervalSeconds"`
		CooldownSeconds        int    `json:"cooldownSeconds"`
		TelemetryMaxAgeSeconds int    `json:"telemetryMaxAgeSeconds"`
		Latency                struct {
			HealthyP95Milliseconds float64 `json:"healthyP95Milliseconds"`
			TargetP95Milliseconds  float64 `json:"targetP95Milliseconds"`
			EmergencyMilliseconds  float64 `json:"emergencyMilliseconds"`
		} `json:"latency"`
		Budget struct {
			MinimumMiBPS float64 `json:"minimumMiBPS"`
			InitialMiBPS float64 `json:"initialMiBPS"`
			MaximumMiBPS float64 `json:"maximumMiBPS"`
		} `json:"budget"`
		AIMD struct {
			AdditiveIncreaseMiBPS  float64 `json:"additiveIncreaseMiBPS"`
			MultiplicativeDecrease float64 `json:"multiplicativeDecrease"`
			HealthyWindows         int     `json:"healthyWindows"`
			BreachWindows          int     `json:"breachWindows"`
		} `json:"aimd"`
	} `json:"spec"`
}

// DiskEnrollment is the v1alpha1 JSON enrollment document.
type DiskEnrollment struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		StorageDomain string `json:"storageDomain"`
		ResourceKey   string `json:"resourceKey"`
		WorkloadClass string `json:"workloadClass"`
		Criticality   string `json:"criticality"`
		Envelope      struct {
			MinimumMiBPS float64 `json:"minimumMiBPS"`
			MaximumMiBPS float64 `json:"maximumMiBPS"`
			Weight       float64 `json:"weight"`
		} `json:"envelope"`
	} `json:"spec"`
}

// PVEAgentConfig is the strict v1alpha1 local read-only agent document.
type PVEAgentConfig struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Spec       struct {
		DomainKey                 string  `json:"domainKey"`
		Node                      string  `json:"node"`
		Storage                   string  `json:"storage"`
		ZPool                     string  `json:"zpool"`
		SampleIntervalSeconds     int     `json:"sampleIntervalSeconds"`
		CommandTimeoutSeconds     int     `json:"commandTimeoutSeconds"`
		EmergencyWaitMilliseconds float64 `json:"emergencyWaitMilliseconds"`
		Resources                 []struct {
			ResourceKey  string `json:"resourceKey"`
			KernelDevice string `json:"kernelDevice"`
			Root         bool   `json:"root"`
			Critical     bool   `json:"critical"`
		} `json:"resources"`
	} `json:"spec"`
}

// PVECanaryPreflightConfig binds one private, read-only QEMU disk eligibility
// check. It contains no credential, command, lifecycle action, or apply mode.
type PVECanaryPreflightConfig struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Spec       struct {
		DomainKey             string   `json:"domainKey"`
		ResourceKey           string   `json:"resourceKey"`
		Node                  string   `json:"node"`
		Storage               string   `json:"storage"`
		ZPool                 string   `json:"zpool"`
		WorkloadKind          string   `json:"workloadKind"`
		WorkloadID            string   `json:"workloadId"`
		DiskKey               string   `json:"diskKey"`
		RequiredTags          []string `json:"requiredTags"`
		CommandTimeoutSeconds int      `json:"commandTimeoutSeconds"`
		Envelope              struct {
			MinimumMiBPS  float64 `json:"minimumMiBPS"`
			MaximumMiBPS  float64 `json:"maximumMiBPS"`
			RollbackMiBPS float64 `json:"rollbackMiBPS"`
		} `json:"envelope"`
	} `json:"spec"`
}

// ReadPVECanaryPreflightConfig strictly decodes an owner-only private binding.
func ReadPVECanaryPreflightConfig(path string) (PVECanaryPreflightConfig, error) {
	var document PVECanaryPreflightConfig
	if err := readStrictPrivateJSON(path, &document); err != nil {
		return document, fmt.Errorf("read PVE canary preflight config: %w", err)
	}
	if err := document.Validate(); err != nil {
		return document, err
	}
	return document, nil
}

// Validate enforces one exact QEMU data disk, explicit non-critical tags, and
// a static rollback limit inside the immutable envelope.
func (d PVECanaryPreflightConfig) Validate() error {
	if d.APIVersion != v1.SchemaVersion || d.Kind != "PVECanaryPreflightConfig" {
		return errors.New("PVE canary preflight apiVersion or kind is unsupported")
	}
	if !opaqueKeyPattern.MatchString(d.Spec.DomainKey) || !opaqueKeyPattern.MatchString(d.Spec.ResourceKey) || d.Spec.WorkloadKind != "qemu" ||
		!workloadIDPattern.MatchString(d.Spec.WorkloadID) || !qemuDiskKeyPattern.MatchString(d.Spec.DiskKey) {
		return errors.New("PVE canary domain or workload binding is invalid")
	}
	for _, value := range []string{d.Spec.Node, d.Spec.Storage, d.Spec.ZPool} {
		if !privateIDPattern.MatchString(value) || value == "." || value == ".." || strings.Contains(value, "..") {
			return errors.New("PVE canary private binding is invalid")
		}
	}
	if d.Spec.CommandTimeoutSeconds < 1 || d.Spec.CommandTimeoutSeconds > 30 {
		return errors.New("PVE canary command timeout must be between 1 and 30 seconds")
	}
	tags := make(map[string]struct{}, len(d.Spec.RequiredTags))
	for _, tag := range d.Spec.RequiredTags {
		if !opaqueKeyPattern.MatchString(tag) {
			return errors.New("PVE canary required tag is invalid")
		}
		tags[tag] = struct{}{}
	}
	if len(tags) != len(d.Spec.RequiredTags) {
		return errors.New("PVE canary required tags must be unique")
	}
	for _, required := range []string{"non-critical", "pve-storage-guard"} {
		if _, ok := tags[required]; !ok {
			return errors.New("PVE canary requires explicit non-critical and pve-storage-guard tags")
		}
	}
	minimum, maximum, rollback := d.Spec.Envelope.MinimumMiBPS, d.Spec.Envelope.MaximumMiBPS, d.Spec.Envelope.RollbackMiBPS
	if math.IsNaN(minimum) || math.IsInf(minimum, 0) || minimum <= 0 ||
		math.IsNaN(maximum) || math.IsInf(maximum, 0) || maximum < minimum ||
		math.IsNaN(rollback) || math.IsInf(rollback, 0) || rollback < minimum || rollback > maximum ||
		!exactMiBPSToBytes(rollback) {
		return errors.New("PVE canary envelope or rollback limit is invalid")
	}
	return nil
}

func exactMiBPSToBytes(value float64) bool {
	bytes := value * 1024 * 1024
	return bytes > 0 && bytes <= float64(math.MaxUint64) && math.Abs(bytes-math.Round(bytes)) <= 1e-6
}

// ReadPVEAgentConfig strictly decodes and validates a local agent file.
func ReadPVEAgentConfig(path string) (PVEAgentConfig, error) {
	var document PVEAgentConfig
	if err := readStrictPrivateJSON(path, &document); err != nil {
		return document, fmt.Errorf("read PVE agent config: %w", err)
	}
	if err := document.Validate(); err != nil {
		return document, err
	}
	return document, nil
}

func readStrictPrivateJSON(path string, value any) error {
	before, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0o077 != 0 || before.Size() > 64*1024 {
		return errors.New("private config must be a regular owner-only file no larger than 64 KiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) {
		return errors.New("private config changed while opening")
	}
	return decodeStrictJSON(file, value)
}

// Validate checks command-injection, identity, and resource bounds.
func (d PVEAgentConfig) Validate() error {
	if d.APIVersion != v1.SchemaVersion || d.Kind != "PVEAgentConfig" {
		return errors.New("PVE agent apiVersion or kind is unsupported")
	}
	if !opaqueKeyPattern.MatchString(d.Spec.DomainKey) {
		return errors.New("PVE agent domainKey must be a lowercase opaque key")
	}
	for _, value := range []string{d.Spec.Node, d.Spec.Storage, d.Spec.ZPool} {
		if !privateIDPattern.MatchString(value) || value == "." || value == ".." || strings.Contains(value, "..") {
			return errors.New("PVE agent private binding is invalid")
		}
	}
	if d.Spec.SampleIntervalSeconds < 1 || d.Spec.SampleIntervalSeconds > 60 {
		return errors.New("PVE agent sample interval must be between 1 and 60 seconds")
	}
	if d.Spec.CommandTimeoutSeconds < d.Spec.SampleIntervalSeconds+1 || d.Spec.CommandTimeoutSeconds > 120 {
		return errors.New("PVE agent command timeout must exceed the sample interval and be at most 120 seconds")
	}
	if d.Spec.EmergencyWaitMilliseconds <= 0 || len(d.Spec.Resources) == 0 || len(d.Spec.Resources) > 64 {
		return errors.New("PVE agent emergency threshold and 1-64 resources are required")
	}
	keys := make(map[string]struct{}, len(d.Spec.Resources))
	devices := make(map[string]struct{}, len(d.Spec.Resources))
	for _, resource := range d.Spec.Resources {
		if !opaqueKeyPattern.MatchString(resource.ResourceKey) || !privateIDPattern.MatchString(resource.KernelDevice) || strings.Contains(resource.KernelDevice, "..") {
			return errors.New("PVE agent resource binding is invalid")
		}
		if _, exists := keys[resource.ResourceKey]; exists {
			return errors.New("PVE agent resourceKey values must be unique")
		}
		if _, exists := devices[resource.KernelDevice]; exists {
			return errors.New("PVE agent kernelDevice values must be unique")
		}
		keys[resource.ResourceKey] = struct{}{}
		devices[resource.KernelDevice] = struct{}{}
	}
	return nil
}

// ReadPolicy strictly decodes and validates a policy file.
func ReadPolicy(path string) (StorageDomainPolicy, error) {
	var document StorageDomainPolicy
	if err := readStrictJSON(path, &document); err != nil {
		return document, fmt.Errorf("read policy: %w", err)
	}
	if document.APIVersion != v1.SchemaVersion || document.Kind != "StorageDomainPolicy" {
		return document, errors.New("policy apiVersion or kind is unsupported")
	}
	if document.Metadata.Name == "" || document.Metadata.Version == "" {
		return document, errors.New("policy metadata name and version are required")
	}
	if document.Spec.Mode != "shadow" {
		return document, errors.New("this command accepts shadow policies only")
	}
	if document.Spec.ControlIntervalSeconds < 1 || document.Spec.TelemetryMaxAgeSeconds < 1 {
		return document, errors.New("control interval and telemetry max age must be positive")
	}
	if _, err := document.CorePolicy(); err != nil {
		return document, err
	}
	return document, nil
}

// CorePolicy converts the versioned document to a platform-neutral policy.
func (d StorageDomainPolicy) CorePolicy() (policy.PoolPolicy, error) {
	core := policy.PoolPolicy{
		MinimumBudgetMiBPS:      d.Spec.Budget.MinimumMiBPS,
		InitialBudgetMiBPS:      d.Spec.Budget.InitialMiBPS,
		MaximumBudgetMiBPS:      d.Spec.Budget.MaximumMiBPS,
		HealthyWaitMilliseconds: d.Spec.Latency.HealthyP95Milliseconds,
		TargetWaitMilliseconds:  d.Spec.Latency.TargetP95Milliseconds,
		EmergencyMilliseconds:   d.Spec.Latency.EmergencyMilliseconds,
		AdditiveIncreaseMiBPS:   d.Spec.AIMD.AdditiveIncreaseMiBPS,
		MultiplicativeDecrease:  d.Spec.AIMD.MultiplicativeDecrease,
		HealthyWindows:          d.Spec.AIMD.HealthyWindows,
		BreachWindows:           d.Spec.AIMD.BreachWindows,
		Cooldown:                time.Duration(d.Spec.CooldownSeconds) * time.Second,
	}
	if err := core.Validate(); err != nil {
		return policy.PoolPolicy{}, fmt.Errorf("validate policy: %w", err)
	}
	return core, nil
}

// ReadEnrollment strictly decodes one explicitly enrolled resource.
func ReadEnrollment(path string) (DiskEnrollment, error) {
	var document DiskEnrollment
	if err := readStrictJSON(path, &document); err != nil {
		return document, fmt.Errorf("read enrollment: %w", err)
	}
	if document.APIVersion != v1.SchemaVersion || document.Kind != "DiskEnrollment" {
		return document, errors.New("enrollment apiVersion or kind is unsupported")
	}
	if document.Metadata.Name == "" || document.Spec.StorageDomain == "" || document.Spec.ResourceKey == "" {
		return document, errors.New("enrollment name, storage domain, and resource key are required")
	}
	if document.Spec.Criticality != "non-critical" {
		return document, errors.New("only explicitly non-critical resources can be enrolled")
	}
	if _, err := allocator.Allocate(document.Spec.Envelope.MaximumMiBPS, []allocator.DiskEnvelope{document.Envelope()}); err != nil {
		return document, fmt.Errorf("validate enrollment envelope: %w", err)
	}
	return document, nil
}

// Envelope converts an enrollment to a generic allocation boundary.
func (d DiskEnrollment) Envelope() allocator.DiskEnvelope {
	return allocator.DiskEnvelope{
		ResourceKey:  d.Spec.ResourceKey,
		MinimumMiBPS: d.Spec.Envelope.MinimumMiBPS,
		MaximumMiBPS: d.Spec.Envelope.MaximumMiBPS,
		Weight:       d.Spec.Envelope.Weight,
	}
}

func readStrictJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return decodeStrictJSON(file, value)
}

func decodeStrictJSON(reader io.Reader, value any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}
