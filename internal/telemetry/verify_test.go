package telemetry

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

func TestVerifyJournalSummarizesWithoutIdentifiers(t *testing.T) {
	first := testDecisionEvent("proposal-0123456789abcdef01234567")
	second := testDecisionEvent("proposal-abcdef0123456789abcdef01")
	second.RecordedAt = second.RecordedAt.Add(time.Second)
	second.Observation.ObservedAt = second.Observation.ObservedAt.Add(time.Second)
	second.PolicyVersion = "policy-opaque-2"
	second.Decision.Changed = true
	path := writePrivateJournal(t, first, second)

	summary, err := VerifyJournal(path)
	if err != nil {
		t.Fatalf("verify journal: %v", err)
	}
	if summary.EventCount != 2 || summary.ChangedCount != 1 || summary.PolicyVersionCount != 2 || summary.DuplicateEventCount != 0 || summary.TimestampRegressionCount != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	payloadBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	expectedDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(payloadBytes))
	if summary.ContentDigest != expectedDigest {
		t.Fatalf("content digest=%q want %q", summary.ContentDigest, expectedDigest)
	}
	if summary.EarliestRecordedAt == nil || !summary.EarliestRecordedAt.Equal(first.RecordedAt) || summary.LatestRecordedAt == nil || !summary.LatestRecordedAt.Equal(second.RecordedAt) {
		t.Fatalf("unexpected time bounds: %+v", summary)
	}
	payload, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	for _, privateValue := range []string{first.DomainKey, first.Observation.ID, first.Decision.ProposalID, first.EventID, "resource-opaque-1"} {
		if bytes.Contains(payload, []byte(privateValue)) {
			t.Fatalf("summary leaked private value %q: %s", privateValue, payload)
		}
	}
}

func TestReadVerifiedJournalBatchReturnsOnlyTheRequestedPage(t *testing.T) {
	first := testDecisionEvent("proposal-0123456789abcdef01234567")
	second := testDecisionEvent("proposal-abcdef0123456789abcdef01")
	third := testDecisionEvent("proposal-111111111111111111111111")
	second.RecordedAt = second.RecordedAt.Add(time.Second)
	second.Observation.ObservedAt = second.Observation.ObservedAt.Add(time.Second)
	third.RecordedAt = third.RecordedAt.Add(2 * time.Second)
	third.Observation.ObservedAt = third.Observation.ObservedAt.Add(2 * time.Second)
	path := writePrivateJournal(t, first, second, third)
	summary, err := VerifyJournal(path)
	if err != nil {
		t.Fatalf("verify journal: %v", err)
	}

	page, err := ReadVerifiedJournalBatch(path, summary.ContentDigest, 0, 2)
	if err != nil {
		t.Fatalf("read first page: %v", err)
	}
	if page.Offset != 0 || page.Complete || page.NextOffset == nil || *page.NextOffset != 2 {
		t.Fatalf("unexpected first page cursor: %+v", page)
	}
	if len(page.Events) != 2 || page.Events[0].EventID != first.EventID || page.Events[1].EventID != second.EventID {
		t.Fatalf("unexpected first page events: %+v", page.Events)
	}
	if page.Verification.ContentDigest != summary.ContentDigest || page.Verification.EventCount != 3 {
		t.Fatalf("unexpected verification: %+v", page.Verification)
	}

	last, err := ReadVerifiedJournalBatch(path, summary.ContentDigest, 2, 2)
	if err != nil {
		t.Fatalf("read last page: %v", err)
	}
	if !last.Complete || last.NextOffset != nil || len(last.Events) != 1 || last.Events[0].EventID != third.EventID {
		t.Fatalf("unexpected last page: %+v", last)
	}
}

func TestReadVerifiedJournalBatchRejectsDigestAndCursorMismatch(t *testing.T) {
	path := writePrivateJournal(t, testDecisionEvent("proposal-0123456789abcdef01234567"))
	summary, err := VerifyJournal(path)
	if err != nil {
		t.Fatalf("verify journal: %v", err)
	}
	for _, test := range []struct {
		name   string
		digest string
		offset uint64
		limit  uint64
	}{
		{name: "empty digest", digest: "", limit: 1},
		{name: "wrong digest", digest: "sha256:" + strings.Repeat("0", 64), limit: 1},
		{name: "offset beyond end", digest: summary.ContentDigest, offset: 2, limit: 1},
		{name: "zero limit", digest: summary.ContentDigest, limit: 0},
		{name: "oversized limit", digest: summary.ContentDigest, limit: 65},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ReadVerifiedJournalBatch(path, test.digest, test.offset, test.limit); err == nil {
				t.Fatal("expected batch read to fail")
			}
		})
	}
}

func TestReadVerifiedJournalBatchAcceptsAnEmptySealedJournal(t *testing.T) {
	path := writePrivateJournal(t)
	summary, err := VerifyJournal(path)
	if err != nil {
		t.Fatalf("verify journal: %v", err)
	}
	batch, err := ReadVerifiedJournalBatch(path, summary.ContentDigest, 0, 64)
	if err != nil {
		t.Fatalf("read empty batch: %v", err)
	}
	if !batch.Complete || batch.NextOffset != nil || len(batch.Events) != 0 {
		t.Fatalf("unexpected empty batch: %+v", batch)
	}
}

func TestVerifyJournalAcceptsEmptyFile(t *testing.T) {
	path := writePrivateJournal(t)
	summary, err := VerifyJournal(path)
	if err != nil {
		t.Fatalf("verify empty journal: %v", err)
	}
	if summary.EventCount != 0 || summary.EarliestRecordedAt != nil || summary.LatestRecordedAt != nil {
		t.Fatalf("unexpected empty summary: %+v", summary)
	}
}

func TestVerifyJournalReportsDuplicateAndTimestampRegression(t *testing.T) {
	first := testDecisionEvent("proposal-0123456789abcdef01234567")
	second := testDecisionEvent("proposal-abcdef0123456789abcdef01")
	second.RecordedAt = second.RecordedAt.Add(-time.Second)
	second.Observation.ObservedAt = second.Observation.ObservedAt.Add(-time.Second)
	path := writePrivateJournal(t, first, first, second)

	summary, err := VerifyJournal(path)
	if err != nil {
		t.Fatalf("verify journal: %v", err)
	}
	if summary.DuplicateEventCount != 1 || summary.TimestampRegressionCount != 1 {
		t.Fatalf("unexpected anomalies: %+v", summary)
	}
}

func TestVerifyJournalRejectsMultipleDomains(t *testing.T) {
	first := testDecisionEvent("proposal-0123456789abcdef01234567")
	second := testDecisionEvent("proposal-abcdef0123456789abcdef01")
	second.DomainKey = "different-domain"
	path := writePrivateJournal(t, first, second)
	if _, err := VerifyJournal(path); err == nil || !strings.Contains(err.Error(), "multiple storage domains") {
		t.Fatalf("expected domain rejection, got %v", err)
	}
}

func TestVerifyJournalRejectsActiveWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	journal, err := OpenJournal(path)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer func() { _ = journal.Close() }()
	if _, err := VerifyJournal(path); err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("expected active writer rejection, got %v", err)
	}
	if _, err := ReadVerifiedJournalBatch(path, "sha256:"+strings.Repeat("0", 64), 0, 1); err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("expected active writer batch rejection, got %v", err)
	}
}

func TestVerifyJournalRejectsUnsafeTargets(t *testing.T) {
	directory := t.TempDir()
	private := writePrivateJournal(t, testDecisionEvent("proposal-0123456789abcdef01234567"))
	insecure := filepath.Join(directory, "insecure.jsonl")
	if err := os.WriteFile(insecure, nil, 0o644); err != nil {
		t.Fatalf("create insecure file: %v", err)
	}
	symlink := filepath.Join(directory, "link.jsonl")
	if err := os.Symlink(private, symlink); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	for _, path := range []string{"", directory, insecure, symlink} {
		if _, err := VerifyJournal(path); err == nil {
			t.Fatalf("expected unsafe path %q to fail", path)
		}
	}
}

func TestVerifyJournalRejectsMalformedRecords(t *testing.T) {
	valid := testDecisionEvent("proposal-0123456789abcdef01234567")
	forged := valid
	forged.EventID = "event-0123456789abcdef01234567"
	badAge := valid
	badAge.Observation.AgeSeconds = 9
	validJSON, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("marshal valid event: %v", err)
	}
	unknown := append(append([]byte(nil), validJSON[:len(validJSON)-1]...), []byte(`,"unknown":true}`)...)

	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "truncated", payload: []byte(`{"schemaVersion":`)},
		{name: "unknown field", payload: unknown},
		{name: "two objects", payload: append(append([]byte(nil), validJSON...), []byte(` {}`)...)},
		{name: "forged event id", payload: mustMarshalEvent(t, forged)},
		{name: "inconsistent age", payload: mustMarshalEvent(t, badAge)},
		{name: "oversized line", payload: []byte(strings.Repeat("x", 1024*1024+1))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := append(append([]byte(nil), test.payload...), '\n')
			path := writePrivatePayload(t, payload)
			if _, err := VerifyJournal(path); err == nil {
				t.Fatal("expected malformed journal to fail")
			}
		})
	}
}

func TestVerifyJournalEnforcesFileSizeBound(t *testing.T) {
	path := writePrivateJournal(t, testDecisionEvent("proposal-0123456789abcdef01234567"))
	if _, err := verifyJournal(path, 1, 1024*1024); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("expected file size rejection, got %v", err)
	}
}

func writePrivateJournal(t *testing.T, events ...v1.DecisionEvent) string {
	t.Helper()
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			t.Fatalf("encode event: %v", err)
		}
	}
	return writePrivatePayload(t, payload.Bytes())
}

func writePrivatePayload(t *testing.T, payload []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	return path
}

func mustMarshalEvent(t *testing.T, event v1.DecisionEvent) []byte {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return payload
}
