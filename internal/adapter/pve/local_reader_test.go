package pve

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DjangoAILab/pve-storage-guard/internal/config"
)

type fakeRunner struct {
	output map[operation][]byte
	err    map[operation]error
}

func (f fakeRunner) Run(_ context.Context, op operation, _ commandRequest) ([]byte, error) {
	if err := f.err[op]; err != nil {
		return nil, err
	}
	return f.output[op], nil
}

type fakeProc map[string][]byte

func (f fakeProc) ReadFile(path string) ([]byte, error) {
	payload, ok := f[path]
	if !ok {
		return nil, errors.New("missing fixture")
	}
	return payload, nil
}

func TestLocalReaderEmitsTypedIdentitySafeObservation(t *testing.T) {
	document := validAgentConfig()
	reader := fixtureReader(document)
	observation, err := reader.Observe(context.Background(), "reference-pool", time.Time{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if !observation.WaitValid || observation.WriteWaitP95Milliseconds != 2.097151 || observation.WaitEvidence == nil || observation.WaitEvidence.Statistic != "p95-upper-bound" || observation.WaitEvidence.Source != "openzfs-total-wait-histogram" {
		t.Fatalf("unexpected wait evidence: %+v", observation)
	}
	if !observation.ManagementPlaneHealthy || observation.IOPressure == nil || len(observation.DiskSignals) != 1 || observation.DiskSignals[0].ResourceKey != "resource-a" {
		t.Fatalf("unexpected supporting signals: %+v", observation)
	}
	payload, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(payload)
	for _, private := range []string{"private-node", "private-storage", "privatepool", "private-disk"} {
		if strings.Contains(serialized, private) {
			t.Fatalf("observation leaked private identity %q: %s", private, serialized)
		}
	}
}

func TestLocalReaderZeroWritesEmitsInvalidWait(t *testing.T) {
	document := validAgentConfig()
	reader := fixtureReader(document)
	reader.runner = fakeRunner{output: map[operation][]byte{
		opClusterStatus:          []byte(`[{"type":"node","name":"private-node","online":true}]`),
		opStorageConfig:          []byte(`{"type":"zfspool","pool":"privatepool/dataset"}`),
		opStorageStatus:          []byte(`{"active":true,"enabled":true,"type":"zfspool"}`),
		opZFSWaitHistogramLayout: []byte("privatepool total_wait disk_wait syncq_wait asyncq_wait\nlatency read write read write read write read write scrub trim rebuild\n"),
		opZFSWaitHistogram:       []byte("privatepool\n1048575 1 0 0 0 0 0 0 0\n"),
	}, err: map[operation]error{}}
	observation, err := reader.Observe(context.Background(), "reference-pool", time.Time{})
	if err != nil || observation.WaitValid || observation.WaitEvidence != nil || observation.WriteWaitP95Milliseconds != 0 {
		t.Fatalf("observation=%+v err=%v", observation, err)
	}
}

func TestLocalReaderFailsClosedOnStorageMismatch(t *testing.T) {
	reader := fixtureReader(validAgentConfig())
	runner := reader.runner.(fakeRunner)
	runner.output[opStorageConfig] = []byte(`{"type":"zfspool","pool":"different"}`)
	reader.runner = runner
	if _, err := reader.Observe(context.Background(), "reference-pool", time.Time{}); err == nil || strings.Contains(err.Error(), "different") || strings.Contains(err.Error(), "privatepool") {
		t.Fatalf("expected sanitized mismatch error, got %v", err)
	}
}

func TestLocalReaderInventoryIsExplicitAndIdentitySafe(t *testing.T) {
	reader := fixtureReader(validAgentConfig())
	inventory, err := reader.InventorySnapshot(context.Background())
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	if inventory.DomainKey != "reference-pool" || inventory.StorageType != "zfspool" || len(inventory.Resources) != 1 || inventory.Resources[0].ResourceKey != "resource-a" {
		t.Fatalf("inventory=%+v", inventory)
	}
	payload, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-node", "private-storage", "privatepool", "private-disk"} {
		if strings.Contains(string(payload), private) {
			t.Fatalf("inventory leaked private identity %q: %s", private, payload)
		}
	}
}

func fixtureReader(document config.PVEAgentConfig) *LocalReader {
	return &LocalReader{
		config: document,
		runner: fakeRunner{output: map[operation][]byte{
			opClusterStatus:          []byte(`[{"type":"cluster","quorate":true},{"type":"node","name":"private-node","online":true}]`),
			opStorageConfig:          []byte(`{"type":"zfspool","pool":"privatepool/dataset","storage":"private-storage"}`),
			opStorageStatus:          []byte(`{"active":true,"enabled":true,"type":"zfspool"}`),
			opZFSWaitHistogramLayout: []byte("privatepool total_wait disk_wait syncq_wait asyncq_wait\nlatency read write read write read write read write scrub trim rebuild\n"),
			opZFSWaitHistogram:       []byte("privatepool\n1048575 1 94 0 0 0 0 0 0\n2097151 1 1 0 0 0 0 0 0\n4194303 1 5 0 0 0 0 0 0\n"),
		}, err: map[operation]error{}},
		proc: fakeProc{
			"/proc/pressure/io": []byte("some avg10=2.50 avg60=1 avg300=1 total=1\nfull avg10=0.25 avg60=1 avg300=1 total=1\n"),
			"/proc/diskstats":   []byte("8 16 private-disk 1 2 3 4 5 6 7 8 9 10 11 0 0 0\n"),
		},
		now: func() time.Time { return time.Date(2026, 8, 2, 1, 2, 3, 0, time.UTC) },
		id:  func() (string, error) { return "observation-opaque", nil },
	}
}

func validAgentConfig() config.PVEAgentConfig {
	var document config.PVEAgentConfig
	document.APIVersion = "guard.storage-slo.io/v1alpha1"
	document.Kind = "PVEAgentConfig"
	document.Spec.DomainKey = "reference-pool"
	document.Spec.Node = "private-node"
	document.Spec.Storage = "private-storage"
	document.Spec.ZPool = "privatepool"
	document.Spec.SampleIntervalSeconds = 1
	document.Spec.CommandTimeoutSeconds = 5
	document.Spec.EmergencyWaitMilliseconds = 100
	document.Spec.Resources = append(document.Spec.Resources, struct {
		ResourceKey  string `json:"resourceKey"`
		KernelDevice string `json:"kernelDevice"`
		Root         bool   `json:"root"`
		Critical     bool   `json:"critical"`
	}{ResourceKey: "resource-a", KernelDevice: "private-disk"})
	return document
}
