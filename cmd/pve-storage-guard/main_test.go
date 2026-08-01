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
