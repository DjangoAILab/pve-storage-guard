package controller

import (
	"testing"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
	"github.com/DjangoAILab/pve-storage-guard/internal/allocator"
	"github.com/DjangoAILab/pve-storage-guard/internal/policy"
)

func TestShadowProposesButNeverAuthorizes(t *testing.T) {
	shadow := newTestShadow(t)
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	observation := v1.Observation{
		SchemaVersion: v1.SchemaVersion, ID: "observation-1", ObservedAt: now,
		DomainKey: "reference-domain", WriteWaitP95Milliseconds: 120,
		WaitValid: true, ManagementPlaneHealthy: true,
	}
	proposal, err := shadow.Process(observation, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !proposal.Changed || proposal.BudgetMiBPS != 5 || proposal.Reason != "emergency" {
		t.Fatalf("unexpected proposal: %+v", proposal)
	}
	if proposal.ActuationAllowed || !proposal.AllocationFeasible || proposal.Allocations["resource-a"] != 5 {
		t.Fatalf("unsafe proposal: %+v", proposal)
	}
}

func TestShadowStaleSampleCannotIncrease(t *testing.T) {
	shadow := newTestShadow(t)
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	proposal, err := shadow.Process(v1.Observation{
		SchemaVersion: v1.SchemaVersion, ID: "stale-1", ObservedAt: now.Add(-time.Minute),
		DomainKey: "reference-domain", WriteWaitP95Milliseconds: 1,
		WaitValid: true, ManagementPlaneHealthy: true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Changed || proposal.BudgetMiBPS != 20 || proposal.Reason != "stale_or_invalid_hold" {
		t.Fatalf("stale input changed state: %+v", proposal)
	}
}

func TestShadowRejectsWrongDomain(t *testing.T) {
	shadow := newTestShadow(t)
	now := time.Now()
	_, err := shadow.Process(v1.Observation{SchemaVersion: v1.SchemaVersion, ID: "wrong", ObservedAt: now, DomainKey: "other"}, now)
	if err == nil {
		t.Fatal("expected domain mismatch error")
	}
}

func newTestShadow(t *testing.T) *Shadow {
	t.Helper()
	shadow, err := NewShadow("reference-domain", "test-1", 5*time.Second, policy.PoolPolicy{
		MinimumBudgetMiBPS: 5, InitialBudgetMiBPS: 20, MaximumBudgetMiBPS: 25,
		HealthyWaitMilliseconds: 15, TargetWaitMilliseconds: 25, EmergencyMilliseconds: 100,
		AdditiveIncreaseMiBPS: 0.5, MultiplicativeDecrease: 0.5,
		HealthyWindows: 12, BreachWindows: 2, Cooldown: time.Minute,
	}, []allocator.DiskEnvelope{{ResourceKey: "resource-a", MinimumMiBPS: 5, MaximumMiBPS: 25, Weight: 1}})
	if err != nil {
		t.Fatal(err)
	}
	return shadow
}
