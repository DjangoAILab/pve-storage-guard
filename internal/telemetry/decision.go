// Package telemetry builds privacy-bounded decision records and journals them.
package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

// NewShadowDecisionEvent builds an auditable event without inventing authority
// or effective-state checks that shadow mode never performs.
func NewShadowDecisionEvent(observation v1.Observation, proposal v1.Proposal) (v1.DecisionEvent, error) {
	if observation.SchemaVersion != v1.SchemaVersion || proposal.SchemaVersion != v1.SchemaVersion {
		return v1.DecisionEvent{}, errors.New("unsupported schema version")
	}
	if observation.ID == "" || proposal.ID == "" || proposal.ObservationID != observation.ID {
		return v1.DecisionEvent{}, errors.New("observation and proposal linkage is invalid")
	}
	if observation.DomainKey == "" || proposal.DomainKey != observation.DomainKey {
		return v1.DecisionEvent{}, errors.New("observation and proposal domain is invalid")
	}
	if observation.ObservedAt.IsZero() || proposal.GeneratedAt.IsZero() || proposal.PolicyVersion == "" {
		return v1.DecisionEvent{}, errors.New("timestamps and policy version are required")
	}
	if proposal.Mode != "shadow" || proposal.ActuationAllowed {
		return v1.DecisionEvent{}, errors.New("decision event accepts non-actuating shadow proposals only")
	}
	if !finiteNonNegative(observation.WriteWaitP95Milliseconds) || !finiteNonNegative(proposal.PreviousBudgetMiBPS) || !finiteNonNegative(proposal.BudgetMiBPS) {
		return v1.DecisionEvent{}, errors.New("decision event contains a non-finite or negative metric")
	}
	allocations := make(map[string]float64, len(proposal.Allocations))
	for resourceKey, allocation := range proposal.Allocations {
		if resourceKey == "" || !finiteNonNegative(allocation) {
			return v1.DecisionEvent{}, errors.New("decision event contains an invalid allocation")
		}
		allocations[resourceKey] = allocation
	}
	diskSignals := append([]v1.DiskSignal(nil), observation.DiskSignals...)
	var waitEvidence *v1.WaitEvidence
	if observation.WaitEvidence != nil {
		copy := *observation.WaitEvidence
		waitEvidence = &copy
	}
	var ioPressure *v1.IOPressure
	if observation.IOPressure != nil {
		copy := *observation.IOPressure
		ioPressure = &copy
	}

	event := v1.DecisionEvent{
		SchemaVersion: v1.SchemaVersion,
		EventID:       eventID(proposal.ID),
		EventType:     v1.DecisionEventType,
		RecordedAt:    proposal.GeneratedAt,
		DomainKey:     proposal.DomainKey,
		Mode:          proposal.Mode,
		PolicyVersion: proposal.PolicyVersion,
		Observation: v1.DecisionEventObservation{
			ID:                       observation.ID,
			ObservedAt:               observation.ObservedAt,
			AgeSeconds:               proposal.GeneratedAt.Sub(observation.ObservedAt).Seconds(),
			WriteWaitP95Milliseconds: observation.WriteWaitP95Milliseconds,
			WaitValid:                observation.WaitValid,
			Emergency:                observation.Emergency,
			ManagementPlaneHealthy:   observation.ManagementPlaneHealthy,
			WaitEvidence:             waitEvidence,
			IOPressure:               ioPressure,
			DiskSignals:              diskSignals,
		},
		Decision: v1.DecisionEventDecision{
			ProposalID:          proposal.ID,
			Reason:              proposal.Reason,
			PreviousBudgetMiBPS: proposal.PreviousBudgetMiBPS,
			DesiredBudgetMiBPS:  proposal.BudgetMiBPS,
			Changed:             proposal.Changed,
			Allocations:         allocations,
			AllocationFeasible:  proposal.AllocationFeasible,
		},
		Safety: v1.DecisionEventSafety{
			AllocationFeasible:   proposal.AllocationFeasible,
			ActuationAllowed:     false,
			LeaseStatus:          v1.SafetyNotEvaluated,
			ApprovalStatus:       v1.SafetyNotEvaluated,
			EffectiveStateStatus: v1.SafetyNotEvaluated,
		},
		Outcome: v1.DecisionOutcomeShadowEvaluated,
	}
	if err := validateDecisionEvent(event); err != nil {
		return v1.DecisionEvent{}, err
	}
	return event, nil
}

func eventID(proposalID string) string {
	sum := sha256.Sum256([]byte(proposalID))
	return "event-" + hex.EncodeToString(sum[:12])
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finiteNonNegative(value float64) bool {
	return value >= 0 && finite(value)
}
