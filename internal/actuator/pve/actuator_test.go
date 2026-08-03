package pve

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	"github.com/DjangoAILab/pve-storage-guard/internal/config"
	"github.com/DjangoAILab/pve-storage-guard/internal/safety"
)

const (
	digestA = "1111111111111111111111111111111111111111"
	digestB = "2222222222222222222222222222222222222222"
)

type fakeBackend struct {
	reads     [][]byte
	readErr   error
	updateErr error
	updates   []updateCall
}

type updateCall struct {
	node, workloadID, diskKey, diskValue, digest string
}

type fixedLeaseVerifier struct{ lease safety.Lease }

func (f fixedLeaseVerifier) Current(context.Context, string) (safety.Lease, error) {
	return f.lease, nil
}

type fixedApprovalVerifier struct{ approval safety.Approval }

func (f fixedApprovalVerifier) Get(context.Context, string) (safety.Approval, error) {
	return f.approval, nil
}

func (f *fakeBackend) ReadQEMUConfig(context.Context, string, string) ([]byte, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	if len(f.reads) == 0 {
		return nil, errors.New("private backend detail")
	}
	payload := f.reads[0]
	f.reads = f.reads[1:]
	return payload, nil
}

func (f *fakeBackend) UpdateQEMUDisk(_ context.Context, node, workloadID, diskKey, diskValue, digest string) error {
	f.updates = append(f.updates, updateCall{node: node, workloadID: workloadID, diskKey: diskKey, diskValue: diskValue, digest: digest})
	return f.updateErr
}

func TestActuatorReadsExistingBoundedWriteLimit(t *testing.T) {
	backend := &fakeBackend{reads: [][]byte{qemuPayload(digestA, "bps_wr=33554432")}}
	actuator := newFixtureActuator(t, backend)
	effective, err := actuator.ReadEffective(context.Background(), "resource-a")
	if err != nil || effective.ResourceKey != "resource-a" || effective.WriteLimitMiBPS != 32 {
		t.Fatalf("effective=%+v err=%v", effective, err)
	}
	if _, err := actuator.ReadEffective(context.Background(), "resource-b"); err == nil {
		t.Fatal("expected unenrolled resource rejection")
	}
}

func TestActuatorAppliesOnlyBPSWriteWithDigestAndReadsBack(t *testing.T) {
	backend := &fakeBackend{reads: [][]byte{
		qemuPayload(digestA, "cache=none,bps_wr=33554432,discard=on"),
		qemuPayload(digestB, "cache=none,bps_wr=67108864,discard=on"),
	}}
	actuator := newFixtureActuator(t, backend)
	effective, err := actuator.ApplyApproved(context.Background(), validApplyRequest(64))
	if err != nil || effective.WriteLimitMiBPS != 64 || len(backend.updates) != 1 {
		t.Fatalf("effective=%+v updates=%+v err=%v", effective, backend.updates, err)
	}
	want := updateCall{
		node: "private-node", workloadID: "101", diskKey: "scsi1",
		diskValue: "private-storage:disk,cache=none,bps_wr=67108864,discard=on", digest: digestA,
	}
	if backend.updates[0] != want {
		t.Fatalf("update=%+v want=%+v", backend.updates[0], want)
	}
}

func TestActuatorReturnsActualReadBackForSafetyGateMismatch(t *testing.T) {
	backend := &fakeBackend{reads: [][]byte{
		qemuPayload(digestA, "bps_wr=33554432"),
		qemuPayload(digestB, "bps_wr=50331648"),
	}}
	actuator := newFixtureActuator(t, backend)
	effective, err := actuator.ApplyApproved(context.Background(), validApplyRequest(64))
	if err != nil || effective.WriteLimitMiBPS != 48 {
		t.Fatalf("effective=%+v err=%v", effective, err)
	}
}

func TestSafetyGateFreezesPVEReadBackMismatch(t *testing.T) {
	now := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	lease := safety.Lease{DomainKey: "reference-pool", HolderID: "holder-a", Generation: 1, ExpiresAt: now.Add(time.Hour)}
	approval := safety.Approval{
		ID: "approval-a", DomainKey: "reference-pool", PolicyVersion: "policy-a", ResourceKey: "resource-a",
		MinimumMiBPS: 16, MaximumMiBPS: 128, ExpiresAt: now.Add(time.Hour),
	}
	backend := &fakeBackend{reads: [][]byte{
		qemuPayload(digestA, "bps_wr=33554432"),
		qemuPayload(digestA, "bps_wr=33554432"),
		qemuPayload(digestB, "bps_wr=50331648"),
	}}
	actuator := newFixtureActuator(t, backend)
	gate, err := safety.NewGate(
		"reference-pool", "holder-a", "policy-a",
		[]safety.Envelope{{ResourceKey: "resource-a", MinimumMiBPS: 16, MaximumMiBPS: 128}},
		actuator, fixedLeaseVerifier{lease}, fixedApprovalVerifier{approval}, func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	request := validApplyRequest(64)
	request.ExpiresAt = now.Add(30 * time.Minute)
	result, err := gate.Apply(context.Background(), safety.Attempt{
		Request: request, Lease: lease, ExpectedEffectiveLimitMiBPS: 32,
	})
	if err == nil || result.Code != safety.CodeReadBackMismatch || !result.Frozen || len(backend.updates) != 1 {
		t.Fatalf("result=%+v updates=%+v err=%v", result, backend.updates, err)
	}
	result, err = gate.Apply(context.Background(), safety.Attempt{
		Request: request, Lease: lease, ExpectedEffectiveLimitMiBPS: 48,
	})
	if err == nil || result.Code != safety.CodeResourceFrozen || !result.Frozen {
		t.Fatalf("second result=%+v err=%v", result, err)
	}
}

func TestActuatorSkipsNoOpUpdate(t *testing.T) {
	backend := &fakeBackend{reads: [][]byte{qemuPayload(digestA, "bps_wr=33554432")}}
	actuator := newFixtureActuator(t, backend)
	effective, err := actuator.ApplyApproved(context.Background(), validApplyRequest(32))
	if err != nil || effective.WriteLimitMiBPS != 32 || len(backend.updates) != 0 {
		t.Fatalf("effective=%+v updates=%+v err=%v", effective, backend.updates, err)
	}
}

func TestActuatorRejectsInvalidRequestBeforePrivilegedRead(t *testing.T) {
	for name, mutate := range map[string]func(*v1.ApplyRequest){
		"schema":        func(r *v1.ApplyRequest) { r.SchemaVersion = "v2" },
		"domain":        func(r *v1.ApplyRequest) { r.DomainKey = "other-domain" },
		"resource":      func(r *v1.ApplyRequest) { r.ResourceKey = "resource-b" },
		"generation":    func(r *v1.ApplyRequest) { r.LeaseGeneration = 0 },
		"below floor":   func(r *v1.ApplyRequest) { r.WriteLimitMiBPS = 8 },
		"above ceiling": func(r *v1.ApplyRequest) { r.WriteLimitMiBPS = 256 },
		"fraction byte": func(r *v1.ApplyRequest) { r.WriteLimitMiBPS = 32.0000001 },
	} {
		t.Run(name, func(t *testing.T) {
			backend := &fakeBackend{}
			actuator := newFixtureActuator(t, backend)
			request := validApplyRequest(32)
			mutate(&request)
			if _, err := actuator.ApplyApproved(context.Background(), request); err == nil {
				t.Fatal("expected request rejection")
			}
			if len(backend.reads) != 0 || len(backend.updates) != 0 {
				t.Fatal("invalid request crossed privileged boundary")
			}
		})
	}
}

func TestActuatorFailsClosedForUnsafeLiveDiskState(t *testing.T) {
	valid := string(qemuPayload(digestA, "bps_wr=33554432"))
	for name, payload := range map[string][]byte{
		"missing digest":    []byte(strings.Replace(valid, `"digest":"`+digestA+`",`, "", 1)),
		"uppercase digest":  []byte(strings.Replace(valid, digestA, strings.Repeat("A", 40), 1)),
		"locked":            []byte(strings.Replace(valid, `"tags":`, `"lock":"backup","tags":`, 1)),
		"untagged":          []byte(strings.Replace(valid, "non-critical;pve-storage-guard", "non-critical", 1)),
		"boot disk":         []byte(strings.Replace(valid, "order=scsi0", "order=scsi1", 1)),
		"wrong storage":     []byte(strings.Replace(valid, "private-storage:disk", "other-storage:disk", 1)),
		"cloud-init":        []byte(strings.Replace(valid, "private-storage:disk", "private-storage:cloudinit", 1)),
		"read-only":         qemuPayload(digestA, "bps_wr=33554432,ro=1"),
		"unknown read-only": qemuPayload(digestA, "bps_wr=33554432,ro=on"),
		"unknown media":     qemuPayload(digestA, "bps_wr=33554432,media=tape"),
		"unlimited":         qemuPayload(digestA, "cache=none"),
		"conflicting mbps":  qemuPayload(digestA, "bps_wr=33554432,mbps_wr=32"),
		"burst rate":        qemuPayload(digestA, "bps_wr=33554432,bps_wr_max_length=10"),
		"duplicate":         qemuPayload(digestA, "bps_wr=33554432,bps_wr=33554432"),
		"outside envelope":  qemuPayload(digestA, "bps_wr=8388608"),
		"malformed option":  qemuPayload(digestA, "bps_wr=33554432,discard"),
		"option whitespace": qemuPayload(digestA, "bps_wr =33554432"),
		"duplicate json":    []byte(`{"digest":"` + digestA + `","digest":"` + digestB + `","tags":"non-critical;pve-storage-guard","boot":"order=scsi0","scsi1":"private-storage:disk,bps_wr=33554432"}`),
		"trailing json":     []byte(string(qemuPayload(digestA, "bps_wr=33554432")) + `{}`),
	} {
		t.Run(name, func(t *testing.T) {
			actuator := newFixtureActuator(t, &fakeBackend{reads: [][]byte{payload}})
			if _, err := actuator.ReadEffective(context.Background(), "resource-a"); err == nil {
				t.Fatal("expected fail-closed result")
			}
		})
	}
}

func TestActuatorRejectsNonManagedReadBackDrift(t *testing.T) {
	backend := &fakeBackend{reads: [][]byte{
		qemuPayload(digestA, "cache=none,bps_wr=33554432"),
		qemuPayload(digestB, "cache=writeback,bps_wr=67108864"),
	}}
	actuator := newFixtureActuator(t, backend)
	if _, err := actuator.ApplyApproved(context.Background(), validApplyRequest(64)); err == nil {
		t.Fatal("expected read-back binding drift rejection")
	}
}

func TestActuatorDoesNotLeakBackendErrors(t *testing.T) {
	backend := &fakeBackend{readErr: errors.New("private-node vm 101 token secret")}
	actuator := newFixtureActuator(t, backend)
	_, err := actuator.ReadEffective(context.Background(), "resource-a")
	if err == nil || strings.Contains(err.Error(), "private-node") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe error: %v", err)
	}

	backend = &fakeBackend{reads: [][]byte{qemuPayload(digestA, "bps_wr=33554432")}, updateErr: errors.New("private update detail")}
	actuator = newFixtureActuator(t, backend)
	_, err = actuator.ApplyApproved(context.Background(), validApplyRequest(64))
	if err == nil || strings.Contains(err.Error(), "private update detail") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestActuatorKeepsRollbackAsExplicitApprovedTarget(t *testing.T) {
	actuator := newFixtureActuator(t, &fakeBackend{})
	if actuator.RollbackLimitMiBPS() != 32 {
		t.Fatalf("rollback=%v", actuator.RollbackLimitMiBPS())
	}
}

func FuzzParseQEMUStateNeverPanics(f *testing.F) {
	binding := newFixtureBinding()
	for _, seed := range [][]byte{
		qemuPayload(digestA, "bps_wr=33554432"),
		{},
		[]byte(`{}`),
		[]byte(`{"digest":"duplicate","digest":"duplicate"}`),
		{0xff, 0x00, '{', '}'},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(_ *testing.T, payload []byte) {
		_, _ = parseQEMUState(payload, binding)
	})
}

func newFixtureActuator(t *testing.T, backend Backend) *Actuator {
	t.Helper()
	document := validActuatorConfig()
	actuator, err := NewActuator(document, backend)
	if err != nil {
		t.Fatal(err)
	}
	return actuator
}

func validActuatorConfig() config.PVECanaryPreflightConfig {
	var document config.PVECanaryPreflightConfig
	document.APIVersion = v1.SchemaVersion
	document.Kind = "PVECanaryPreflightConfig"
	document.Spec.DomainKey = "reference-pool"
	document.Spec.ResourceKey = "resource-a"
	document.Spec.Node = "private-node"
	document.Spec.Storage = "private-storage"
	document.Spec.ZPool = "private-pool"
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

func newFixtureBinding() binding {
	document := validActuatorConfig()
	return binding{
		domainKey: document.Spec.DomainKey, resourceKey: document.Spec.ResourceKey,
		node: document.Spec.Node, storage: document.Spec.Storage,
		workloadID: document.Spec.WorkloadID, diskKey: document.Spec.DiskKey,
		requiredTags:  append([]string(nil), document.Spec.RequiredTags...),
		minimumMiBPS:  document.Spec.Envelope.MinimumMiBPS,
		maximumMiBPS:  document.Spec.Envelope.MaximumMiBPS,
		rollbackMiBPS: document.Spec.Envelope.RollbackMiBPS,
	}
}

func validApplyRequest(limit float64) v1.ApplyRequest {
	return v1.ApplyRequest{
		SchemaVersion: v1.SchemaVersion, ProposalID: "proposal-a", ApprovalID: "approval-a",
		PolicyVersion: "policy-a", DomainKey: "reference-pool", LeaseHolderID: "holder-a",
		LeaseGeneration: 1, ResourceKey: "resource-a", WriteLimitMiBPS: limit,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
}

func qemuPayload(digest, options string) []byte {
	disk := "private-storage:disk"
	if options != "" {
		disk += "," + options
	}
	return []byte(`{"digest":"` + digest + `","tags":"non-critical;pve-storage-guard","boot":"order=scsi0","scsi1":"` + disk + `"}`)
}
