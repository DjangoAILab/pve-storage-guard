// Package controller composes policy decisions and bounded allocation.
package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	"github.com/DjangoAILab/pve-storage-guard/internal/allocator"
	"github.com/DjangoAILab/pve-storage-guard/internal/policy"
)

// Shadow evaluates observations but can never authorize actuation.
type Shadow struct {
	domainKey     string
	policyVersion string
	maxAge        time.Duration
	policy        *policy.Controller
	envelopes     []allocator.DiskEnvelope
}

// NewShadow creates one stateful controller for one storage domain.
func NewShadow(domainKey, policyVersion string, maxAge time.Duration, poolPolicy policy.PoolPolicy, envelopes []allocator.DiskEnvelope) (*Shadow, error) {
	if domainKey == "" || policyVersion == "" {
		return nil, errors.New("domain key and policy version are required")
	}
	if maxAge <= 0 {
		return nil, errors.New("telemetry max age must be positive")
	}
	core, err := policy.NewController(poolPolicy)
	if err != nil {
		return nil, err
	}
	if _, err := allocator.Allocate(poolPolicy.InitialBudgetMiBPS, envelopes); err != nil {
		return nil, fmt.Errorf("validate envelopes: %w", err)
	}
	return &Shadow{domainKey: domainKey, policyVersion: policyVersion, maxAge: maxAge, policy: core, envelopes: append([]allocator.DiskEnvelope(nil), envelopes...)}, nil
}

// Process emits an explainable, non-mutating proposal.
func (s *Shadow) Process(observation v1.Observation, receivedAt time.Time) (v1.Proposal, error) {
	if observation.SchemaVersion != v1.SchemaVersion {
		return v1.Proposal{}, errors.New("unsupported observation schema version")
	}
	if observation.ID == "" || observation.DomainKey != s.domainKey || observation.ObservedAt.IsZero() || receivedAt.IsZero() {
		return v1.Proposal{}, errors.New("observation id, timestamps, and matching domain are required")
	}
	age := receivedAt.Sub(observation.ObservedAt)
	stale := age > s.maxAge || age < -s.maxAge
	decision := s.policy.Observe(policy.Feedback{
		At:                     receivedAt,
		WaitMilliseconds:       observation.WriteWaitP95Milliseconds,
		WaitValid:              observation.WaitValid && finiteNonNegative(observation.WriteWaitP95Milliseconds),
		Stale:                  stale,
		Emergency:              observation.Emergency,
		ManagementPlaneHealthy: observation.ManagementPlaneHealthy,
	})
	allocation, err := allocator.Allocate(decision.BudgetMiBPS, s.envelopes)
	if err != nil {
		return v1.Proposal{}, fmt.Errorf("allocate proposal: %w", err)
	}
	return v1.Proposal{
		SchemaVersion:       v1.SchemaVersion,
		ID:                  proposalID(observation.ID, s.policyVersion, decision),
		GeneratedAt:         receivedAt,
		ObservationID:       observation.ID,
		PolicyVersion:       s.policyVersion,
		DomainKey:           s.domainKey,
		Mode:                "shadow",
		Reason:              decision.Reason,
		PreviousBudgetMiBPS: decision.PreviousBudgetMiBPS,
		BudgetMiBPS:         decision.BudgetMiBPS,
		Changed:             decision.Changed,
		Allocations:         allocation.Allocations,
		AllocationFeasible:  allocation.Feasible,
		ActuationAllowed:    false,
	}, nil
}

func proposalID(observationID, policyVersion string, decision policy.Decision) string {
	payload := fmt.Sprintf("%s\x00%s\x00%.6f\x00%s", observationID, policyVersion, decision.BudgetMiBPS, decision.Reason)
	sum := sha256.Sum256([]byte(payload))
	return "proposal-" + hex.EncodeToString(sum[:12])
}

func finiteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
