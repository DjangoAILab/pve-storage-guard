// Package v1 defines versioned adapter and telemetry contracts.
package v1

import "time"

// SchemaVersion identifies the first pre-release wire contract.
const SchemaVersion = "guard.storage-slo.io/v1alpha1"

const (
	// DecisionEventType identifies a controller decision journal record.
	DecisionEventType = "storage_guard.decision"
	// DecisionOutcomeShadowEvaluated records an evaluation that cannot actuate.
	DecisionOutcomeShadowEvaluated = "shadow_evaluated"
	// SafetyNotEvaluated prevents shadow records from implying authority checks.
	SafetyNotEvaluated = "not_evaluated"
)

// Observation is a normalized, read-only storage-domain sample.
type Observation struct {
	SchemaVersion            string    `json:"schemaVersion"`
	ID                       string    `json:"id"`
	ObservedAt               time.Time `json:"observedAt"`
	DomainKey                string    `json:"domainKey"`
	WriteWaitP95Milliseconds float64   `json:"writeWaitP95Milliseconds"`
	WaitValid                bool      `json:"waitValid"`
	Emergency                bool      `json:"emergency"`
	ManagementPlaneHealthy   bool      `json:"managementPlaneHealthy"`
}

// Proposal is a non-mutating desired allocation emitted by the policy engine.
type Proposal struct {
	SchemaVersion       string             `json:"schemaVersion"`
	ID                  string             `json:"id"`
	GeneratedAt         time.Time          `json:"generatedAt"`
	ObservationID       string             `json:"observationId"`
	PolicyVersion       string             `json:"policyVersion"`
	DomainKey           string             `json:"domainKey"`
	Mode                string             `json:"mode"`
	Reason              string             `json:"reason"`
	PreviousBudgetMiBPS float64            `json:"previousBudgetMiBps"`
	BudgetMiBPS         float64            `json:"budgetMiBps"`
	Changed             bool               `json:"changed"`
	Allocations         map[string]float64 `json:"allocations"`
	AllocationFeasible  bool               `json:"allocationFeasible"`
	ActuationAllowed    bool               `json:"actuationAllowed"`
}

// DecisionEvent is an append-only, structured audit record for one proposal.
type DecisionEvent struct {
	SchemaVersion string                   `json:"schemaVersion"`
	EventID       string                   `json:"eventId"`
	EventType     string                   `json:"eventType"`
	RecordedAt    time.Time                `json:"recordedAt"`
	DomainKey     string                   `json:"domainKey"`
	Mode          string                   `json:"mode"`
	PolicyVersion string                   `json:"policyVersion"`
	Observation   DecisionEventObservation `json:"observation"`
	Decision      DecisionEventDecision    `json:"decision"`
	Safety        DecisionEventSafety      `json:"safety"`
	Outcome       string                   `json:"outcome"`
}

// DecisionEventObservation captures the normalized evidence used by shadow.
type DecisionEventObservation struct {
	ID                       string    `json:"id"`
	ObservedAt               time.Time `json:"observedAt"`
	AgeSeconds               float64   `json:"ageSeconds"`
	WriteWaitP95Milliseconds float64   `json:"writeWaitP95Milliseconds"`
	WaitValid                bool      `json:"waitValid"`
	Emergency                bool      `json:"emergency"`
	ManagementPlaneHealthy   bool      `json:"managementPlaneHealthy"`
}

// DecisionEventDecision records the bounded desired allocation.
type DecisionEventDecision struct {
	ProposalID          string             `json:"proposalId"`
	Reason              string             `json:"reason"`
	PreviousBudgetMiBPS float64            `json:"previousBudgetMiBps"`
	DesiredBudgetMiBPS  float64            `json:"desiredBudgetMiBps"`
	Changed             bool               `json:"changed"`
	Allocations         map[string]float64 `json:"allocations"`
	AllocationFeasible  bool               `json:"allocationFeasible"`
}

// DecisionEventSafety states only checks that shadow actually performed.
type DecisionEventSafety struct {
	AllocationFeasible   bool   `json:"allocationFeasible"`
	ActuationAllowed     bool   `json:"actuationAllowed"`
	LeaseStatus          string `json:"leaseStatus"`
	ApprovalStatus       string `json:"approvalStatus"`
	EffectiveStateStatus string `json:"effectiveStateStatus"`
}

// ApplyRequest is the only shape accepted by a future constrained actuator.
// It intentionally contains no command or argument fields.
type ApplyRequest struct {
	SchemaVersion   string    `json:"schemaVersion"`
	ProposalID      string    `json:"proposalId"`
	ApprovalID      string    `json:"approvalId"`
	PolicyVersion   string    `json:"policyVersion"`
	DomainKey       string    `json:"domainKey"`
	LeaseHolderID   string    `json:"leaseHolderId"`
	LeaseGeneration uint64    `json:"leaseGeneration"`
	ResourceKey     string    `json:"resourceKey"`
	WriteLimitMiBPS float64   `json:"writeLimitMiBps"`
	ExpiresAt       time.Time `json:"expiresAt"`
}
