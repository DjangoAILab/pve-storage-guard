package pve

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	"github.com/DjangoAILab/pve-storage-guard/internal/config"
)

func TestCanaryPreflightRequiresExplicitTaggedNonBootDataDisk(t *testing.T) {
	reader := fixtureCanaryReader(validCanaryConfig(), `{
  "tags":"non-critical;pve-storage-guard",
  "boot":"order=scsi0;ide2;net0",
  "scsi0":"private-storage:vm-101-disk-0,size=32G",
  "scsi1":"private-storage:vm-101-disk-1,size=64G"
}`)
	assessment := reader.Assess(context.Background())
	if !assessment.ControlledLoadEligible || assessment.ActiveControlEligible || assessment.RequestedMutations != 0 || len(assessment.Gaps) != 0 {
		t.Fatalf("unexpected assessment: %+v", assessment)
	}
	payload, err := json.Marshal(assessment)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-node", "private-storage", "101", "scsi1"} {
		if strings.Contains(string(payload), private) {
			t.Fatalf("assessment leaked private binding %q: %s", private, payload)
		}
	}
}

func TestCanaryPreflightFailsClosedForBootLockedReadOnlyOrUntaggedDisk(t *testing.T) {
	for name, qemuConfig := range map[string]string{
		"boot":       `{"tags":"non-critical;pve-storage-guard","boot":"order=scsi1","scsi1":"private-storage:disk"}`,
		"locked":     `{"tags":"non-critical;pve-storage-guard","lock":"backup","boot":"order=scsi0","scsi1":"private-storage:disk"}`,
		"readonly":   `{"tags":"non-critical;pve-storage-guard","boot":"order=scsi0","scsi1":"private-storage:disk,readonly=1"}`,
		"untagged":   `{"tags":"non-critical","boot":"order=scsi0","scsi1":"private-storage:disk"}`,
		"wrong-pool": `{"tags":"non-critical;pve-storage-guard","boot":"order=scsi0","scsi1":"other-storage:disk"}`,
	} {
		t.Run(name, func(t *testing.T) {
			assessment := fixtureCanaryReader(validCanaryConfig(), qemuConfig).Assess(context.Background())
			if assessment.ControlledLoadEligible || assessment.ActiveControlEligible || len(assessment.Gaps) == 0 {
				t.Fatalf("unexpected assessment: %+v", assessment)
			}
		})
	}
}

func TestCanaryPreflightFailsClosedWhenReadOnlyCommandsFail(t *testing.T) {
	reader := fixtureCanaryReader(validCanaryConfig(), `{}`)
	reader.runner = fakeRunner{output: map[operation][]byte{}, err: map[operation]error{
		opClusterStatus: context.DeadlineExceeded,
		opStorageConfig: context.DeadlineExceeded,
		opQEMUConfig:    context.DeadlineExceeded,
	}}
	assessment := reader.Assess(context.Background())
	if assessment.ControlledLoadEligible || assessment.ActiveControlEligible || assessment.RequestedMutations != 0 || len(assessment.Gaps) < 3 {
		t.Fatalf("unexpected assessment: %+v", assessment)
	}
}

func TestCanaryPreflightRejectsStorageDomainPoolMismatch(t *testing.T) {
	reader := fixtureCanaryReader(validCanaryConfig(), `{"tags":"non-critical;pve-storage-guard","boot":"order=scsi0","scsi1":"private-storage:disk"}`)
	runner := reader.runner.(fakeRunner)
	runner.output[opStorageConfig] = []byte(`{"type":"zfspool","pool":"different-pool","storage":"private-storage"}`)
	reader.runner = runner
	assessment := reader.Assess(context.Background())
	if assessment.ControlledLoadEligible || assessment.Checks.StorageBound {
		t.Fatalf("unexpected assessment: %+v", assessment)
	}
}

func fixtureCanaryReader(document config.PVECanaryPreflightConfig, qemuConfig string) *CanaryPreflightReader {
	return &CanaryPreflightReader{config: document, runner: fakeRunner{output: map[operation][]byte{
		opClusterStatus: []byte(`[{"type":"node","name":"private-node","online":true}]`),
		opStorageConfig: []byte(`{"type":"zfspool","pool":"privatepool","storage":"private-storage"}`),
		opStorageStatus: []byte(`{"active":true,"enabled":true,"type":"zfspool"}`),
		opQEMUConfig:    []byte(qemuConfig),
	}, err: map[operation]error{}}}
}

func validCanaryConfig() config.PVECanaryPreflightConfig {
	var document config.PVECanaryPreflightConfig
	document.APIVersion = v1.SchemaVersion
	document.Kind = "PVECanaryPreflightConfig"
	document.Spec.DomainKey = "reference-pool"
	document.Spec.Node = "private-node"
	document.Spec.Storage = "private-storage"
	document.Spec.ZPool = "privatepool"
	document.Spec.WorkloadKind = "qemu"
	document.Spec.WorkloadID = "101"
	document.Spec.DiskKey = "scsi1"
	document.Spec.RequiredTags = []string{"non-critical", "pve-storage-guard"}
	document.Spec.CommandTimeoutSeconds = 5
	document.Spec.Envelope.MinimumMiBPS = 16
	document.Spec.Envelope.MaximumMiBPS = 128
	document.Spec.Envelope.RollbackMiBPS = 32
	return document
}
