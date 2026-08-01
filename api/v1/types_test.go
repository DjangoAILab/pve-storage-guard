package v1

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecisionJournalVerificationSerializesContentDigest(t *testing.T) {
	summary := DecisionJournalVerification{
		SchemaVersion: SchemaVersion,
		Kind:          DecisionJournalVerificationKind,
		ContentDigest: "sha256:" + strings.Repeat("a", 64),
		EventCount:    2,
	}
	payload, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal verification: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode verification: %v", err)
	}
	if decoded["contentDigest"] != summary.ContentDigest {
		t.Fatalf("missing content digest: %s", payload)
	}
}

func TestDecisionJournalBatchSerializesPrivatePageContract(t *testing.T) {
	next := uint64(2)
	batch := DecisionJournalBatch{
		SchemaVersion: SchemaVersion,
		Kind:          DecisionJournalBatchKind,
		Verification: DecisionJournalVerification{
			SchemaVersion: SchemaVersion,
			Kind:          DecisionJournalVerificationKind,
			ContentDigest: "sha256:" + strings.Repeat("b", 64),
			EventCount:    3,
		},
		Offset:     0,
		NextOffset: &next,
		Complete:   false,
		Events:     make([]DecisionEvent, 2),
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("marshal batch: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	if decoded["kind"] != DecisionJournalBatchKind || decoded["offset"] != float64(0) || decoded["nextOffset"] != float64(2) {
		t.Fatalf("unexpected batch contract: %s", payload)
	}
	if _, exists := decoded["events"]; !exists {
		t.Fatalf("batch omitted events: %s", payload)
	}
}
