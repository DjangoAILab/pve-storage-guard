package telemetry

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

func TestJournalCreatesPrivateFileAndAppendsCompleteJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	journal, err := OpenJournal(path)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	first := testDecisionEvent("proposal-0123456789abcdef01234567")
	second := testDecisionEvent("proposal-abcdef0123456789abcdef01")
	if err := journal.Append(first); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if err := journal.Append(second); err != nil {
		t.Fatalf("append second: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatalf("close journal: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat journal: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("journal mode = %o, want 600", got)
	}
	assertJournalEvents(t, path, []string{first.EventID, second.EventID})
}

func TestJournalAppendsAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	first, err := OpenJournal(path)
	if err != nil {
		t.Fatalf("open first journal: %v", err)
	}
	firstEvent := testDecisionEvent("proposal-0123456789abcdef01234567")
	if err := first.Append(firstEvent); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first journal: %v", err)
	}

	second, err := OpenJournal(path)
	if err != nil {
		t.Fatalf("reopen journal: %v", err)
	}
	secondEvent := testDecisionEvent("proposal-abcdef0123456789abcdef01")
	if err := second.Append(secondEvent); err != nil {
		t.Fatalf("append second: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second journal: %v", err)
	}
	assertJournalEvents(t, path, []string{firstEvent.EventID, secondEvent.EventID})
}

func TestOpenJournalRejectsUnsafeTargets(t *testing.T) {
	directory := t.TempDir()
	insecure := filepath.Join(directory, "insecure.jsonl")
	if err := os.WriteFile(insecure, nil, 0o644); err != nil {
		t.Fatalf("create insecure file: %v", err)
	}
	private := filepath.Join(directory, "private.jsonl")
	if err := os.WriteFile(private, nil, 0o600); err != nil {
		t.Fatalf("create private file: %v", err)
	}
	symlink := filepath.Join(directory, "link.jsonl")
	if err := os.Symlink(private, symlink); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "empty", path: ""},
		{name: "directory", path: directory},
		{name: "group or other accessible", path: insecure},
		{name: "symlink", path: symlink},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			journal, err := OpenJournal(test.path)
			if err == nil {
				_ = journal.Close()
				t.Fatal("expected unsafe target to be rejected")
			}
		})
	}
}

func TestJournalRejectsNonShadowEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	journal, err := OpenJournal(path)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	defer func() { _ = journal.Close() }()
	event := testDecisionEvent("proposal-0123456789abcdef01234567")
	event.Safety.ActuationAllowed = true
	if err := journal.Append(event); err == nil {
		t.Fatal("expected actuating event to be rejected")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat journal: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("invalid event wrote %d bytes", info.Size())
	}
}

func TestJournalRejectsForgedLinkage(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*v1.DecisionEvent)
	}{
		{name: "event id", mutate: func(event *v1.DecisionEvent) {
			event.EventID = "event-0123456789abcdef01234567"
		}},
		{name: "observation age", mutate: func(event *v1.DecisionEvent) {
			event.Observation.AgeSeconds = 9
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "decisions.jsonl")
			journal, err := OpenJournal(path)
			if err != nil {
				t.Fatalf("open journal: %v", err)
			}
			defer func() { _ = journal.Close() }()
			event := testDecisionEvent("proposal-0123456789abcdef01234567")
			test.mutate(&event)
			if err := journal.Append(event); err == nil {
				t.Fatal("expected forged linkage to be rejected")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat journal: %v", err)
			}
			if info.Size() != 0 {
				t.Fatalf("invalid event wrote %d bytes", info.Size())
			}
		})
	}
}

func TestOpenJournalRejectsSecondWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	first, err := OpenJournal(path)
	if err != nil {
		t.Fatalf("open first journal: %v", err)
	}
	defer func() { _ = first.Close() }()
	second, err := OpenJournal(path)
	if err == nil {
		_ = second.Close()
		t.Fatal("expected second writer to be rejected")
	}
}

func TestJournalStopsBeforeConfiguredSizeLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	journal, err := openJournal(path, 1)
	if err != nil {
		t.Fatalf("open bounded journal: %v", err)
	}
	defer func() { _ = journal.Close() }()
	if err := journal.Append(testDecisionEvent("proposal-0123456789abcdef01234567")); err == nil {
		t.Fatal("expected size limit to reject event")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat journal: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("bounded journal wrote %d bytes", info.Size())
	}
}

func testDecisionEvent(proposalID string) v1.DecisionEvent {
	now := time.Date(2026, 8, 2, 1, 2, 3, 0, time.UTC)
	return v1.DecisionEvent{
		SchemaVersion: v1.SchemaVersion,
		EventID:       eventID(proposalID),
		EventType:     v1.DecisionEventType,
		RecordedAt:    now,
		DomainKey:     "domain-opaque-1",
		Mode:          "shadow",
		PolicyVersion: "policy-opaque-1",
		Observation: v1.DecisionEventObservation{
			ID:                       "observation-opaque-1",
			ObservedAt:               now.Add(-time.Second),
			AgeSeconds:               1,
			WriteWaitP95Milliseconds: 42,
			WaitValid:                true,
			ManagementPlaneHealthy:   true,
		},
		Decision: v1.DecisionEventDecision{
			ProposalID:          proposalID,
			Reason:              "hold",
			PreviousBudgetMiBPS: 10,
			DesiredBudgetMiBPS:  10,
			Allocations:         map[string]float64{"resource-opaque-1": 10},
			AllocationFeasible:  true,
		},
		Safety: v1.DecisionEventSafety{
			AllocationFeasible:   true,
			LeaseStatus:          v1.SafetyNotEvaluated,
			ApprovalStatus:       v1.SafetyNotEvaluated,
			EffectiveStateStatus: v1.SafetyNotEvaluated,
		},
		Outcome: v1.DecisionOutcomeShadowEvaluated,
	}
}

func assertJournalEvents(t *testing.T, path string, wantIDs []string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open journal for assertion: %v", err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	var gotIDs []string
	for scanner.Scan() {
		var event v1.DecisionEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("decode journal line: %v", err)
		}
		gotIDs = append(gotIDs, event.EventID)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan journal: %v", err)
	}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("event count = %d, want %d (%v)", len(gotIDs), len(wantIDs), gotIDs)
	}
	for index := range wantIDs {
		if gotIDs[index] != wantIDs[index] {
			t.Fatalf("event %d = %q, want %q", index, gotIDs[index], wantIDs[index])
		}
	}
}
