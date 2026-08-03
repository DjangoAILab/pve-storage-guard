package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	"github.com/DjangoAILab/pve-storage-guard/internal/config"
	"github.com/DjangoAILab/pve-storage-guard/internal/telemetry"
)

type fakeAgentReader struct {
	mu           sync.Mutex
	observations []v1.Observation
	observeErr   error
	started      chan struct{}
	release      chan struct{}
	calls        int
}

func (f *fakeAgentReader) InventorySnapshot(context.Context) (v1.PVEInventory, error) {
	return v1.PVEInventory{}, nil
}

func (f *fakeAgentReader) Observe(ctx context.Context, _ string, _ time.Time) (v1.Observation, error) {
	f.mu.Lock()
	f.calls++
	index := f.calls - 1
	f.mu.Unlock()
	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
	}
	if f.release != nil {
		select {
		case <-ctx.Done():
			return v1.Observation{}, ctx.Err()
		case <-f.release:
		}
	}
	if f.observeErr != nil {
		return v1.Observation{}, f.observeErr
	}
	if len(f.observations) == 0 {
		return v1.Observation{SchemaVersion: v1.SchemaVersion, ID: "observation-watch", DomainKey: "reference-pool"}, nil
	}
	return f.observations[index%len(f.observations)], nil
}

func (f *fakeAgentReader) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestAgentWatchEmitsSerialJSONLAndStopsWhileWaiting(t *testing.T) {
	reader := &fakeAgentReader{observations: []v1.Observation{
		{SchemaVersion: v1.SchemaVersion, ID: "observation-watch-1", DomainKey: "reference-pool"},
		{SchemaVersion: v1.SchemaVersion, ID: "observation-watch-2", DomainKey: "reference-pool"},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	var stdout bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- watchAgent(ctx, reader, "reference-pool", 20*time.Millisecond, &stdout)
	}()
	deadline := time.Now().Add(time.Second)
	for reader.callCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("watch: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"id":"observation-watch-1"`) || !strings.Contains(lines[1], `"id":"observation-watch-2"`) {
		t.Fatalf("unexpected JSONL: %q", stdout.String())
	}
}

func TestAgentWatchCancelsAnInFlightObservation(t *testing.T) {
	reader := &fakeAgentReader{started: make(chan struct{}, 1), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	var stdout bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- watchAgent(ctx, reader, "reference-pool", time.Second, &stdout)
	}()
	select {
	case <-reader.started:
	case <-time.After(time.Second):
		t.Fatal("observation did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || stdout.Len() != 0 {
			t.Fatalf("err=%v stdout=%q", err, stdout.String())
		}
	case <-time.After(time.Second):
		t.Fatal("watch did not stop after cancellation")
	}
}

func TestAgentWatchFailsClosedOnObservationOrOutputError(t *testing.T) {
	reader := &fakeAgentReader{observeErr: errors.New("unavailable")}
	if err := watchAgent(context.Background(), reader, "reference-pool", time.Second, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "observe") {
		t.Fatalf("unexpected observation error: %v", err)
	}
	reader = &fakeAgentReader{}
	if err := watchAgent(context.Background(), reader, "reference-pool", time.Second, failingWriter{}); err == nil || !strings.Contains(err.Error(), "write observation") {
		t.Fatalf("unexpected output error: %v", err)
	}
}

func TestAgentWatchCLIValidatesCadenceBeforeReaderCreation(t *testing.T) {
	for _, period := range []string{"999ms", "1h1s", "invalid"} {
		var stdout, stderr bytes.Buffer
		factoryCalled := false
		err := runAgentWithContext(context.Background(), []string{"watch", "--config", "unused", "--period", period}, &stdout, &stderr, func(config.PVEAgentConfig) (pveAgentReader, error) {
			factoryCalled = true
			return &fakeAgentReader{}, nil
		})
		if err == nil || factoryCalled || stdout.Len() != 0 {
			t.Fatalf("period=%q err=%v factoryCalled=%v stdout=%q", period, err, factoryCalled, stdout.String())
		}
	}
}

func TestAgentWatchCLIWiresPrivateConfigAndCancellation(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "agent.json")
	payload := []byte(`{"apiVersion":"guard.storage-slo.io/v1alpha1","kind":"PVEAgentConfig","spec":{"domainKey":"reference-pool","node":"node-a","storage":"storage-a","zpool":"pool-a","sampleIntervalSeconds":1,"commandTimeoutSeconds":5,"emergencyWaitMilliseconds":100,"resources":[{"resourceKey":"resource-a","kernelDevice":"sdb","root":false,"critical":false}]}}`)
	if err := os.WriteFile(configPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	reader := &fakeAgentReader{started: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runAgentWithContext(ctx, []string{"watch", "--config", configPath, "--period", "1s"}, &stdout, &stderr, func(document config.PVEAgentConfig) (pveAgentReader, error) {
			if document.Spec.DomainKey != "reference-pool" {
				return nil, fmt.Errorf("unexpected domain %q", document.Spec.DomainKey)
			}
			return reader, nil
		})
	}()
	select {
	case <-reader.started:
	case <-time.After(time.Second):
		t.Fatal("watch did not start")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("watch: %v; stderr=%q", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id":"observation-watch"`) {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

type fakeCanaryAssessor struct {
	assessment v1.PVECanaryPreflightAssessment
}

func (f fakeCanaryAssessor) Assess(context.Context) v1.PVECanaryPreflightAssessment {
	return f.assessment
}

func TestCanaryPreflightCLIEmitsIdentityFreeNonActuatingAssessment(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "canary.json")
	payload := []byte(`{"apiVersion":"guard.storage-slo.io/v1alpha1","kind":"PVECanaryPreflightConfig","spec":{"domainKey":"reference-pool","node":"private-node","storage":"private-storage","zpool":"privatepool","workloadKind":"qemu","workloadId":"101","diskKey":"scsi1","requiredTags":["non-critical","pve-storage-guard"],"commandTimeoutSeconds":5,"envelope":{"minimumMiBPS":16,"maximumMiBPS":128,"rollbackMiBPS":32}}}`)
	if err := os.WriteFile(configPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	assessment := v1.PVECanaryPreflightAssessment{
		SchemaVersion: v1.SchemaVersion, Kind: v1.PVECanaryPreflightAssessmentKind,
		ShadowOnly: true, RequestedMutations: 0, ControlledLoadEligible: true, ActiveControlEligible: false,
	}
	var stdout, stderr bytes.Buffer
	err := runCanaryPreflight([]string{"--config", configPath}, &stdout, &stderr, func(document config.PVECanaryPreflightConfig) (canaryPreflightAssessor, error) {
		if document.Spec.WorkloadID != "101" || document.Spec.DiskKey != "scsi1" {
			return nil, errors.New("unexpected private binding")
		}
		return fakeCanaryAssessor{assessment: assessment}, nil
	})
	if err != nil {
		t.Fatalf("preflight: %v; stderr=%q", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"activeControlEligible":false`) || !strings.Contains(stdout.String(), `"requestedMutations":0`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
	for _, private := range []string{"private-node", "private-storage", "101", "scsi1"} {
		if strings.Contains(stdout.String(), private) {
			t.Fatalf("output leaked %q: %s", private, stdout.String())
		}
	}
}

func TestCanaryPreflightCLIRejectsUnsafeConfigBeforeFactory(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "canary.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	var stdout, stderr bytes.Buffer
	err := runCanaryPreflight([]string{"--config", configPath}, &stdout, &stderr, func(config.PVECanaryPreflightConfig) (canaryPreflightAssessor, error) {
		called = true
		return nil, nil
	})
	if err == nil || called || stdout.Len() != 0 {
		t.Fatalf("err=%v called=%v stdout=%q", err, called, stdout.String())
	}
}

func TestShadowCommandEmitsNonActuatingProposal(t *testing.T) {
	root := filepath.Join("..", "..")
	now := time.Now().UTC()
	input := fmt.Sprintf(`{"schemaVersion":"guard.storage-slo.io/v1alpha1","id":"observation-cli-1","observedAt":%q,"domainKey":"reference-bulk-pool","writeWaitP95Milliseconds":120,"waitValid":true,"emergency":false,"managementPlaneHealthy":true}`+"\n", now.Format(time.RFC3339Nano))
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"shadow",
		"--policy", filepath.Join(root, "configs", "examples", "reference-shadow-policy.json"),
		"--enrollment", filepath.Join(root, "configs", "examples", "reference-enrollment.json"),
	}, strings.NewReader(input), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"actuationAllowed":false`) || !strings.Contains(stdout.String(), `"reason":"emergency"`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestShadowCommandRejectsMissingEnrollment(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"shadow", "--policy", "unused.json"}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "at least one --enrollment") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestShadowCommandWritesOptInJournalBeforeProposal(t *testing.T) {
	root := filepath.Join("..", "..")
	now := time.Now().UTC()
	input := fmt.Sprintf(`{"schemaVersion":"guard.storage-slo.io/v1alpha1","id":"observation-cli-journal-1","observedAt":%q,"domainKey":"reference-bulk-pool","writeWaitP95Milliseconds":120,"waitValid":true,"emergency":false,"managementPlaneHealthy":true}`+"\n", now.Format(time.RFC3339Nano))
	journalPath := filepath.Join(t.TempDir(), "decisions.jsonl")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"shadow",
		"--policy", filepath.Join(root, "configs", "examples", "reference-shadow-policy.json"),
		"--enrollment", filepath.Join(root, "configs", "examples", "reference-enrollment.json"),
		"--journal", journalPath,
	}, strings.NewReader(input), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	var proposal v1.Proposal
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &proposal); err != nil {
		t.Fatalf("decode stdout proposal: %v", err)
	}
	payload, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var event v1.DecisionEvent
	if err := json.Unmarshal(bytes.TrimSpace(payload), &event); err != nil {
		t.Fatalf("decode journal: %v", err)
	}
	if event.Decision.ProposalID != proposal.ID || event.Observation.ID != proposal.ObservationID || event.Safety.ActuationAllowed || proposal.ActuationAllowed {
		t.Fatalf("journal and proposal mismatch: event=%+v proposal=%+v", event, proposal)
	}
	info, err := os.Stat(journalPath)
	if err != nil {
		t.Fatalf("stat journal: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode = %o, want 600", info.Mode().Perm())
	}
}

func TestShadowCommandJournalAppendsAcrossRuns(t *testing.T) {
	root := filepath.Join("..", "..")
	journalPath := filepath.Join(t.TempDir(), "decisions.jsonl")
	for index := 1; index <= 2; index++ {
		now := time.Now().UTC()
		input := fmt.Sprintf(`{"schemaVersion":"guard.storage-slo.io/v1alpha1","id":"observation-cli-append-%d","observedAt":%q,"domainKey":"reference-bulk-pool","writeWaitP95Milliseconds":120,"waitValid":true,"emergency":false,"managementPlaneHealthy":true}`+"\n", index, now.Format(time.RFC3339Nano))
		var stdout, stderr bytes.Buffer
		code := run([]string{
			"shadow",
			"--policy", filepath.Join(root, "configs", "examples", "reference-shadow-policy.json"),
			"--enrollment", filepath.Join(root, "configs", "examples", "reference-enrollment.json"),
			"--journal", journalPath,
		}, strings.NewReader(input), &stdout, &stderr)
		if code != 0 {
			t.Fatalf("run %d exit %d: %s", index, code, stderr.String())
		}
	}
	payload, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(payload)), "\n")
	if len(lines) != 2 {
		t.Fatalf("journal lines = %d, want 2: %q", len(lines), payload)
	}
}

func TestShadowCommandRejectsUnsafeJournalBeforeOutput(t *testing.T) {
	root := filepath.Join("..", "..")
	now := time.Now().UTC()
	input := fmt.Sprintf(`{"schemaVersion":"guard.storage-slo.io/v1alpha1","id":"observation-cli-unsafe-1","observedAt":%q,"domainKey":"reference-bulk-pool","writeWaitP95Milliseconds":120,"waitValid":true,"emergency":false,"managementPlaneHealthy":true}`+"\n", now.Format(time.RFC3339Nano))
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"shadow",
		"--policy", filepath.Join(root, "configs", "examples", "reference-shadow-policy.json"),
		"--enrollment", filepath.Join(root, "configs", "examples", "reference-enrollment.json"),
		"--journal", t.TempDir(),
	}, strings.NewReader(input), &stdout, &stderr)
	if code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "journal") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestShadowCommandDoesNotWriteJournalByDefault(t *testing.T) {
	root := filepath.Join("..", "..")
	now := time.Now().UTC()
	input := fmt.Sprintf(`{"schemaVersion":"guard.storage-slo.io/v1alpha1","id":"observation-cli-default-1","observedAt":%q,"domainKey":"reference-bulk-pool","writeWaitP95Milliseconds":120,"waitValid":true,"emergency":false,"managementPlaneHealthy":true}`+"\n", now.Format(time.RFC3339Nano))
	directory := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"shadow",
		"--policy", filepath.Join(root, "configs", "examples", "reference-shadow-policy.json"),
		"--enrollment", filepath.Join(root, "configs", "examples", "reference-enrollment.json"),
	}, strings.NewReader(input), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read temp directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("default shadow created files: %v", entries)
	}
}

func TestJournalVerifyCommandEmitsIdentityFreeSummary(t *testing.T) {
	root := filepath.Join("..", "..")
	now := time.Now().UTC()
	input := fmt.Sprintf(`{"schemaVersion":"guard.storage-slo.io/v1alpha1","id":"private-observation-cli-verify","observedAt":%q,"domainKey":"reference-bulk-pool","writeWaitP95Milliseconds":120,"waitValid":true,"emergency":false,"managementPlaneHealthy":true}`+"\n", now.Format(time.RFC3339Nano))
	journalPath := filepath.Join(t.TempDir(), "decisions.jsonl")
	var shadowOut, shadowErr bytes.Buffer
	if code := run([]string{
		"shadow",
		"--policy", filepath.Join(root, "configs", "examples", "reference-shadow-policy.json"),
		"--enrollment", filepath.Join(root, "configs", "examples", "reference-enrollment.json"),
		"--journal", journalPath,
	}, strings.NewReader(input), &shadowOut, &shadowErr); code != 0 {
		t.Fatalf("shadow exit %d: %s", code, shadowErr.String())
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"journal", "verify", "--journal", journalPath}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("verify exit %d: %s", code, stderr.String())
	}
	var summary v1.DecisionJournalVerification
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.SchemaVersion != v1.SchemaVersion || summary.Kind != v1.DecisionJournalVerificationKind || summary.EventCount != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	for _, privateValue := range []string{"reference-bulk-pool", "resource-a", "private-observation-cli-verify", "proposal-", "event-"} {
		if strings.Contains(stdout.String(), privateValue) {
			t.Fatalf("summary leaked %q: %s", privateValue, stdout.String())
		}
	}
}

func TestJournalVerifyCommandRejectsActiveWriter(t *testing.T) {
	journalPath := filepath.Join(t.TempDir(), "decisions.jsonl")
	journal, err := telemetry.OpenJournal(journalPath)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	defer func() { _ = journal.Close() }()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"journal", "verify", "--journal", journalPath}, strings.NewReader(""), &stdout, &stderr); code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "lock") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestJournalVerifyCommandRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		{"journal"},
		{"journal", "verify"},
		{"journal", "verify", "--unknown"},
		{"journal", "verify", "--journal", "unused", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, strings.NewReader(""), &stdout, &stderr); code == 0 || stdout.Len() != 0 {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestJournalBatchCommandEmitsApprovedBoundedPages(t *testing.T) {
	journalPath := createCLIJournal(t, 3)
	summary, err := telemetry.VerifyJournal(journalPath)
	if err != nil {
		t.Fatalf("verify journal: %v", err)
	}

	var firstOut, firstErr bytes.Buffer
	if code := run([]string{
		"journal", "batch",
		"--journal", journalPath,
		"--expected-digest", summary.ContentDigest,
		"--limit", "2",
	}, strings.NewReader(""), &firstOut, &firstErr); code != 0 {
		t.Fatalf("first batch exit %d: %s", code, firstErr.String())
	}
	var first v1.DecisionJournalBatch
	if err := json.Unmarshal(bytes.TrimSpace(firstOut.Bytes()), &first); err != nil {
		t.Fatalf("decode first batch: %v", err)
	}
	if len(first.Events) != 2 || first.Offset != 0 || first.NextOffset == nil || *first.NextOffset != 2 || first.Complete {
		t.Fatalf("unexpected first batch: %+v", first)
	}
	if first.Verification.ContentDigest != summary.ContentDigest || !strings.Contains(firstOut.String(), "private-observation-cli-batch-1") {
		t.Fatalf("first batch did not bind and expose expected private events: %s", firstOut.String())
	}

	var lastOut, lastErr bytes.Buffer
	if code := run([]string{
		"journal", "batch",
		"--journal", journalPath,
		"--expected-digest", summary.ContentDigest,
		"--offset", "2",
		"--limit", "2",
	}, strings.NewReader(""), &lastOut, &lastErr); code != 0 {
		t.Fatalf("last batch exit %d: %s", code, lastErr.String())
	}
	var last v1.DecisionJournalBatch
	if err := json.Unmarshal(bytes.TrimSpace(lastOut.Bytes()), &last); err != nil {
		t.Fatalf("decode last batch: %v", err)
	}
	if len(last.Events) != 1 || last.Offset != 2 || last.NextOffset != nil || !last.Complete {
		t.Fatalf("unexpected last batch: %+v", last)
	}
}

func TestJournalBatchCommandRejectsUnapprovedContentWithoutStdout(t *testing.T) {
	journalPath := createCLIJournal(t, 1)
	wrongDigest := "sha256:" + strings.Repeat("0", 64)
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"journal", "batch",
		"--journal", journalPath,
		"--expected-digest", wrongDigest,
	}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "digest") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, privateValue := range []string{"reference-bulk-pool", "private-observation-cli-batch-1"} {
		if strings.Contains(stderr.String(), privateValue) {
			t.Fatalf("error leaked %q: %s", privateValue, stderr.String())
		}
	}
}

func TestJournalBatchCommandRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		{"journal", "batch"},
		{"journal", "batch", "--journal", "unused"},
		{"journal", "batch", "--journal", "unused", "--expected-digest", "bad"},
		{"journal", "batch", "--journal", "unused", "--expected-digest", "sha256:" + strings.Repeat("0", 64), "--limit", "0"},
		{"journal", "batch", "--journal", "unused", "--expected-digest", "sha256:" + strings.Repeat("0", 64), "--limit", "65"},
		{"journal", "batch", "--unknown"},
		{"journal", "batch", "--journal", "unused", "--expected-digest", "sha256:" + strings.Repeat("0", 64), "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, strings.NewReader(""), &stdout, &stderr); code == 0 || stdout.Len() != 0 {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}
}

func createCLIJournal(t *testing.T, eventCount int) string {
	t.Helper()
	root := filepath.Join("..", "..")
	journalPath := filepath.Join(t.TempDir(), "decisions.jsonl")
	base := time.Now().UTC().Add(-time.Duration(eventCount) * time.Second)
	for index := 1; index <= eventCount; index++ {
		observedAt := base.Add(time.Duration(index) * time.Second)
		input := fmt.Sprintf(`{"schemaVersion":"guard.storage-slo.io/v1alpha1","id":"private-observation-cli-batch-%d","observedAt":%q,"domainKey":"reference-bulk-pool","writeWaitP95Milliseconds":120,"waitValid":true,"emergency":false,"managementPlaneHealthy":true}`+"\n", index, observedAt.Format(time.RFC3339Nano))
		var stdout, stderr bytes.Buffer
		if code := run([]string{
			"shadow",
			"--policy", filepath.Join(root, "configs", "examples", "reference-shadow-policy.json"),
			"--enrollment", filepath.Join(root, "configs", "examples", "reference-enrollment.json"),
			"--journal", journalPath,
		}, strings.NewReader(input), &stdout, &stderr); code != 0 {
			t.Fatalf("shadow event %d exit %d: %s", index, code, stderr.String())
		}
	}
	return journalPath
}
