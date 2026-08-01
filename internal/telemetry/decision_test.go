package telemetry

import (
	"reflect"
	"testing"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

func TestNewShadowDecisionEventMapsEvidenceAndProposal(t *testing.T) {
	observedAt := time.Date(2026, 8, 2, 1, 2, 3, 0, time.UTC)
	generatedAt := observedAt.Add(1200 * time.Millisecond)
	observation := v1.Observation{
		SchemaVersion:            v1.SchemaVersion,
		ID:                       "observation-opaque-1",
		ObservedAt:               observedAt,
		DomainKey:                "domain-opaque-1",
		WriteWaitP95Milliseconds: 42,
		WaitValid:                true,
		Emergency:                false,
		ManagementPlaneHealthy:   true,
	}
	proposal := v1.Proposal{
		SchemaVersion:       v1.SchemaVersion,
		ID:                  "proposal-0123456789abcdef01234567",
		GeneratedAt:         generatedAt,
		ObservationID:       observation.ID,
		PolicyVersion:       "policy-sha256-opaque",
		DomainKey:           observation.DomainKey,
		Mode:                "shadow",
		Reason:              "multiplicative_decrease",
		PreviousBudgetMiBPS: 20,
		BudgetMiBPS:         10,
		Changed:             true,
		Allocations:         map[string]float64{"resource-opaque-1": 10},
		AllocationFeasible:  true,
		ActuationAllowed:    false,
	}

	event, err := NewShadowDecisionEvent(observation, proposal)
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	if event.SchemaVersion != v1.SchemaVersion || event.EventType != v1.DecisionEventType || event.EventID != "event-5e5a2e4da13e7bb6eea0709e" {
		t.Fatalf("unexpected envelope: %+v", event)
	}
	if !event.RecordedAt.Equal(generatedAt) || event.DomainKey != observation.DomainKey || event.PolicyVersion != proposal.PolicyVersion || event.Mode != "shadow" {
		t.Fatalf("unexpected identity fields: %+v", event)
	}
	if event.Observation.ID != observation.ID || !event.Observation.ObservedAt.Equal(observedAt) || event.Observation.AgeSeconds != 1.2 || event.Observation.WriteWaitP95Milliseconds != 42 || !event.Observation.WaitValid || !event.Observation.ManagementPlaneHealthy {
		t.Fatalf("unexpected observation evidence: %+v", event.Observation)
	}
	if event.Decision.ProposalID != proposal.ID || event.Decision.Reason != proposal.Reason || event.Decision.PreviousBudgetMiBPS != 20 || event.Decision.DesiredBudgetMiBPS != 10 || !event.Decision.Changed || !reflect.DeepEqual(event.Decision.Allocations, proposal.Allocations) {
		t.Fatalf("unexpected decision: %+v", event.Decision)
	}
	if !event.Safety.AllocationFeasible || event.Safety.ActuationAllowed || event.Safety.LeaseStatus != v1.SafetyNotEvaluated || event.Safety.ApprovalStatus != v1.SafetyNotEvaluated || event.Safety.EffectiveStateStatus != v1.SafetyNotEvaluated {
		t.Fatalf("unexpected shadow safety result: %+v", event.Safety)
	}
	if event.Outcome != v1.DecisionOutcomeShadowEvaluated {
		t.Fatalf("unexpected outcome: %q", event.Outcome)
	}

	again, err := NewShadowDecisionEvent(observation, proposal)
	if err != nil || again.EventID != event.EventID {
		t.Fatalf("event id is not deterministic: first=%q second=%q err=%v", event.EventID, again.EventID, err)
	}
}

func TestNewShadowDecisionEventRejectsInconsistentInputs(t *testing.T) {
	now := time.Now().UTC()
	observation := v1.Observation{SchemaVersion: v1.SchemaVersion, ID: "observation-1", ObservedAt: now, DomainKey: "domain-1", WaitValid: true}
	proposal := v1.Proposal{
		SchemaVersion:      v1.SchemaVersion,
		ID:                 "proposal-0123456789abcdef01234567",
		GeneratedAt:        now,
		ObservationID:      observation.ID,
		PolicyVersion:      "policy-1",
		DomainKey:          observation.DomainKey,
		Mode:               "shadow",
		Allocations:        map[string]float64{"resource-1": 5},
		AllocationFeasible: true,
	}

	tests := []struct {
		name   string
		mutate func(*v1.Observation, *v1.Proposal)
	}{
		{name: "schema", mutate: func(o *v1.Observation, _ *v1.Proposal) { o.SchemaVersion = "other" }},
		{name: "observation link", mutate: func(_ *v1.Observation, p *v1.Proposal) { p.ObservationID = "other" }},
		{name: "domain", mutate: func(_ *v1.Observation, p *v1.Proposal) { p.DomainKey = "other" }},
		{name: "mode", mutate: func(_ *v1.Observation, p *v1.Proposal) { p.Mode = "canary" }},
		{name: "actuation", mutate: func(_ *v1.Observation, p *v1.Proposal) { p.ActuationAllowed = true }},
		{name: "negative wait", mutate: func(o *v1.Observation, _ *v1.Proposal) { o.WriteWaitP95Milliseconds = -1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidateObservation := observation
			candidateProposal := proposal
			test.mutate(&candidateObservation, &candidateProposal)
			if _, err := NewShadowDecisionEvent(candidateObservation, candidateProposal); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
