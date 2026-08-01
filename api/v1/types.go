// Package v1 defines versioned adapter and telemetry contracts.
package v1

import "time"

// SchemaVersion identifies the first pre-release wire contract.
const SchemaVersion = "guard.storage-slo.io/v1alpha1"

// Observation is a normalized, read-only storage-domain sample.
type Observation struct {
	SchemaVersion            string    `json:"schemaVersion"`
	ID                       string    `json:"id"`
	ObservedAt               time.Time `json:"observedAt"`
	DomainKey                string    `json:"domainKey"`
	WriteWaitP95Milliseconds float64   `json:"writeWaitP95Milliseconds"`
	ManagementPlaneHealthy   bool      `json:"managementPlaneHealthy"`
}

// Proposal is a non-mutating desired allocation emitted by the policy engine.
type Proposal struct {
	SchemaVersion string             `json:"schemaVersion"`
	ID            string             `json:"id"`
	ObservationID string             `json:"observationId"`
	PolicyVersion string             `json:"policyVersion"`
	DomainKey     string             `json:"domainKey"`
	Mode          string             `json:"mode"`
	Reason        string             `json:"reason"`
	BudgetMiBPS   float64            `json:"budgetMiBps"`
	Allocations   map[string]float64 `json:"allocations"`
}

// ApplyRequest is the only shape accepted by a future constrained actuator.
// It intentionally contains no command or argument fields.
type ApplyRequest struct {
	SchemaVersion   string    `json:"schemaVersion"`
	ProposalID      string    `json:"proposalId"`
	ApprovalID      string    `json:"approvalId"`
	PolicyVersion   string    `json:"policyVersion"`
	ResourceKey     string    `json:"resourceKey"`
	WriteLimitMiBPS float64   `json:"writeLimitMiBps"`
	ExpiresAt       time.Time `json:"expiresAt"`
}
