package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	"github.com/DjangoAILab/pve-storage-guard/internal/telemetry"
)

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
