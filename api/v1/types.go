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
	// DecisionJournalVerificationKind identifies an identity-free verification summary.
	DecisionJournalVerificationKind = "DecisionJournalVerification"
	// DecisionJournalBatchKind identifies one private, content-addressed journal page.
	DecisionJournalBatchKind = "DecisionJournalBatch"
	// PVEInventoryKind identifies an identity-safe PVE inventory snapshot.
	PVEInventoryKind = "PVEInventory"
	// PVECanaryPreflightAssessmentKind identifies a read-only canary eligibility result.
	PVECanaryPreflightAssessmentKind = "PVECanaryPreflightAssessment"
)

// Observation is a normalized, read-only storage-domain sample.
type Observation struct {
	SchemaVersion            string        `json:"schemaVersion"`
	ID                       string        `json:"id"`
	ObservedAt               time.Time     `json:"observedAt"`
	DomainKey                string        `json:"domainKey"`
	WriteWaitP95Milliseconds float64       `json:"writeWaitP95Milliseconds"`
	WaitValid                bool          `json:"waitValid"`
	Emergency                bool          `json:"emergency"`
	ManagementPlaneHealthy   bool          `json:"managementPlaneHealthy"`
	WaitEvidence             *WaitEvidence `json:"waitEvidence,omitempty"`
	IOPressure               *IOPressure   `json:"ioPressure,omitempty"`
	DiskSignals              []DiskSignal  `json:"diskSignals,omitempty"`
}

// WaitEvidence preserves the layer, statistic, and provenance of a wait value.
type WaitEvidence struct {
	MeasurementLayer            string  `json:"measurementLayer"`
	Statistic                   string  `json:"statistic"`
	Source                      string  `json:"source"`
	Provenance                  string  `json:"provenance"`
	SampleIntervalSeconds       int     `json:"sampleIntervalSeconds"`
	SampleWeight                float64 `json:"sampleWeight"`
	BucketUpperBoundNanoseconds uint64  `json:"bucketUpperBoundNanoseconds"`
}

// IOPressure is a typed Linux PSI snapshot. Values are percentages.
type IOPressure struct {
	SomeAvg10 float64 `json:"someAvg10Percent"`
	FullAvg10 float64 `json:"fullAvg10Percent"`
}

// DiskSignal exposes identity-safe cumulative diskstats corroboration.
type DiskSignal struct {
	ResourceKey                 string `json:"resourceKey"`
	ReadsCompletedTotal         uint64 `json:"readsCompletedTotal"`
	WritesCompletedTotal        uint64 `json:"writesCompletedTotal"`
	ReadSectorsTotal            uint64 `json:"readSectorsTotal"`
	WrittenSectorsTotal         uint64 `json:"writtenSectorsTotal"`
	InFlightIO                  uint64 `json:"inFlightIo"`
	IOTimeMillisecondsTotal     uint64 `json:"ioTimeMillisecondsTotal"`
	WeightedIOMillisecondsTotal uint64 `json:"weightedIoMillisecondsTotal"`
}

// PVEInventory is a normalized inventory that omits private PVE identifiers.
type PVEInventory struct {
	SchemaVersion string             `json:"schemaVersion"`
	Kind          string             `json:"kind"`
	ObservedAt    time.Time          `json:"observedAt"`
	DomainKey     string             `json:"domainKey"`
	StorageType   string             `json:"storageType"`
	Resources     []PVEInventoryDisk `json:"resources"`
}

// PVEInventoryDisk is one explicitly enrolled opaque resource.
type PVEInventoryDisk struct {
	ResourceKey string `json:"resourceKey"`
	Root        bool   `json:"root"`
	Critical    bool   `json:"critical"`
}

// PVECanaryPreflightChecks contains only identity-free, read-only checks.
type PVECanaryPreflightChecks struct {
	ManagementHealthy     bool `json:"managementHealthy"`
	StorageBound          bool `json:"storageBound"`
	ExplicitlyNonCritical bool `json:"explicitlyNonCritical"`
	WorkloadUnlocked      bool `json:"workloadUnlocked"`
	DiskExists            bool `json:"diskExists"`
	DiskOnStorage         bool `json:"diskOnStorage"`
	DiskIsData            bool `json:"diskIsData"`
	DiskIsNonBoot         bool `json:"diskIsNonBoot"`
	DiskIsWritable        bool `json:"diskIsWritable"`
	RollbackWithinBounds  bool `json:"rollbackWithinBounds"`
}

// PVECanaryPreflightAssessment never grants actuation. It proves only whether
// one exact private enrollment is structurally eligible for a controlled-load
// rehearsal after separate operator approval.
type PVECanaryPreflightAssessment struct {
	SchemaVersion          string                   `json:"schemaVersion"`
	Kind                   string                   `json:"kind"`
	ShadowOnly             bool                     `json:"shadowOnly"`
	RequestedMutations     int                      `json:"requestedMutations"`
	ControlledLoadEligible bool                     `json:"controlledLoadEligible"`
	ActiveControlEligible  bool                     `json:"activeControlEligible"`
	Checks                 PVECanaryPreflightChecks `json:"checks"`
	Gaps                   []string                 `json:"gaps"`
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
	ID                       string        `json:"id"`
	ObservedAt               time.Time     `json:"observedAt"`
	AgeSeconds               float64       `json:"ageSeconds"`
	WriteWaitP95Milliseconds float64       `json:"writeWaitP95Milliseconds"`
	WaitValid                bool          `json:"waitValid"`
	Emergency                bool          `json:"emergency"`
	ManagementPlaneHealthy   bool          `json:"managementPlaneHealthy"`
	WaitEvidence             *WaitEvidence `json:"waitEvidence,omitempty"`
	IOPressure               *IOPressure   `json:"ioPressure,omitempty"`
	DiskSignals              []DiskSignal  `json:"diskSignals,omitempty"`
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

// DecisionJournalVerification summarizes one structurally valid sealed journal
// without returning its domain, resource, observation, proposal, or event IDs.
type DecisionJournalVerification struct {
	SchemaVersion            string     `json:"schemaVersion"`
	Kind                     string     `json:"kind"`
	ContentDigest            string     `json:"contentDigest"`
	EventCount               uint64     `json:"eventCount"`
	ChangedCount             uint64     `json:"changedCount"`
	PolicyVersionCount       uint64     `json:"policyVersionCount"`
	DuplicateEventCount      uint64     `json:"duplicateEventCount"`
	TimestampRegressionCount uint64     `json:"timestampRegressionCount"`
	EarliestRecordedAt       *time.Time `json:"earliestRecordedAt,omitempty"`
	LatestRecordedAt         *time.Time `json:"latestRecordedAt,omitempty"`
}

// DecisionJournalBatch contains one bounded private page from an exact sealed
// journal. Unlike DecisionJournalVerification, it is not safe to publish.
type DecisionJournalBatch struct {
	SchemaVersion string                      `json:"schemaVersion"`
	Kind          string                      `json:"kind"`
	Verification  DecisionJournalVerification `json:"verification"`
	Offset        uint64                      `json:"offset"`
	NextOffset    *uint64                     `json:"nextOffset,omitempty"`
	Complete      bool                        `json:"complete"`
	Events        []DecisionEvent             `json:"events"`
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
