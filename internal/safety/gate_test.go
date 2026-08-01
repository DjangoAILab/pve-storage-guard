package safety

import (
	"context"
	"errors"
	"testing"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

func TestGateAppliesOnlyAfterLeaseAndEffectiveStateMatch(t *testing.T) {
	gate, actuator, _, _, now := newTestGate(t)
	result, err := gate.Apply(context.Background(), validAttempt(now))
	if err != nil {
		t.Fatal(err)
	}
	if result.Code != "applied" || !result.Applied || result.Frozen || result.PreviousEffectiveLimitMiBPS != 20 || result.ReadBackLimitMiBPS != 15 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if actuator.readCalls != 1 || actuator.applyCalls != 1 || actuator.effective.WriteLimitMiBPS != 15 {
		t.Fatalf("unexpected actuator calls/state: %+v", actuator)
	}
}

func TestGateRejectsConflictingLeaseBeforeActuator(t *testing.T) {
	gate, actuator, leases, _, now := newTestGate(t)
	attempt := validAttempt(now)
	leases.current.HolderID = "other-controller"

	result, err := gate.Apply(context.Background(), attempt)
	assertRejection(t, err, CodeLeaseConflict)
	if result.Code != CodeLeaseConflict || result.Frozen || actuator.readCalls != 0 || actuator.applyCalls != 0 {
		t.Fatalf("conflicting lease reached actuator: result=%+v actuator=%+v", result, actuator)
	}
}

func TestGateRejectsExpiredLeaseBeforeActuator(t *testing.T) {
	gate, actuator, leases, _, now := newTestGate(t)
	attempt := validAttempt(now)
	attempt.Lease.ExpiresAt = now
	leases.current = attempt.Lease

	result, err := gate.Apply(context.Background(), attempt)
	assertRejection(t, err, CodeLeaseExpired)
	if result.Code != CodeLeaseExpired || actuator.readCalls != 0 || actuator.applyCalls != 0 {
		t.Fatalf("expired lease reached actuator: result=%+v actuator=%+v", result, actuator)
	}
}

func TestGateRejectsUnavailableAuthoritativeLeaseBeforeActuator(t *testing.T) {
	gate, actuator, leases, _, now := newTestGate(t)
	leases.err = errors.New("injected lease store failure")

	result, err := gate.Apply(context.Background(), validAttempt(now))
	assertRejection(t, err, CodeLeaseUnavailable)
	if result.Code != CodeLeaseUnavailable || result.Frozen || actuator.readCalls != 0 || actuator.applyCalls != 0 {
		t.Fatalf("unverified lease reached actuator: result=%+v actuator=%+v", result, actuator)
	}
}

func TestGateRejectsApprovalOutsideAuthoritativeEnvelope(t *testing.T) {
	gate, actuator, _, approvals, now := newTestGate(t)
	approvals.approval.MaximumMiBPS = 10

	result, err := gate.Apply(context.Background(), validAttempt(now))
	assertRejection(t, err, CodeApprovalMismatch)
	if result.Code != CodeApprovalMismatch || result.Frozen || actuator.readCalls != 0 || actuator.applyCalls != 0 {
		t.Fatalf("unapproved limit reached actuator: result=%+v actuator=%+v", result, actuator)
	}
}

func TestGateRejectsUnavailableAuthoritativeApproval(t *testing.T) {
	gate, actuator, _, approvals, now := newTestGate(t)
	approvals.err = errors.New("injected approval store failure")

	result, err := gate.Apply(context.Background(), validAttempt(now))
	assertRejection(t, err, CodeApprovalUnavailable)
	if result.Code != CodeApprovalUnavailable || result.Frozen || actuator.readCalls != 0 || actuator.applyCalls != 0 {
		t.Fatalf("unverified approval reached actuator: result=%+v actuator=%+v", result, actuator)
	}
}

func TestGateRejectsExpiredAuthoritativeApproval(t *testing.T) {
	gate, actuator, _, approvals, now := newTestGate(t)
	approvals.approval.ExpiresAt = now

	result, err := gate.Apply(context.Background(), validAttempt(now))
	assertRejection(t, err, CodeApprovalExpired)
	if result.Code != CodeApprovalExpired || result.Frozen || actuator.readCalls != 0 || actuator.applyCalls != 0 {
		t.Fatalf("expired approval reached actuator: result=%+v actuator=%+v", result, actuator)
	}
}

func TestGateFreezesResourceAfterActuatorFault(t *testing.T) {
	gate, actuator, _, _, now := newTestGate(t)
	actuator.applyErr = errors.New("injected privileged adapter failure")

	result, err := gate.Apply(context.Background(), validAttempt(now))
	assertRejection(t, err, CodeApplyFailed)
	if result.Code != CodeApplyFailed || !result.Frozen || result.Applied || actuator.applyCalls != 1 {
		t.Fatalf("actuator failure did not fail closed: result=%+v actuator=%+v", result, actuator)
	}

	result, err = gate.Apply(context.Background(), validAttempt(now))
	assertRejection(t, err, CodeResourceFrozen)
	if !result.Frozen || actuator.readCalls != 1 || actuator.applyCalls != 1 {
		t.Fatalf("frozen resource reached actuator again: result=%+v actuator=%+v", result, actuator)
	}
}

func TestGateFreezesOnEffectiveStateDrift(t *testing.T) {
	gate, actuator, _, _, now := newTestGate(t)
	actuator.effective.WriteLimitMiBPS = 19

	result, err := gate.Apply(context.Background(), validAttempt(now))
	assertRejection(t, err, CodeEffectiveDrift)
	if !result.Frozen || actuator.applyCalls != 0 {
		t.Fatalf("drift did not block apply: result=%+v actuator=%+v", result, actuator)
	}
}

func TestGateFreezesOnReadBackMismatch(t *testing.T) {
	gate, actuator, _, _, now := newTestGate(t)
	actuator.readBackOverride = 14

	result, err := gate.Apply(context.Background(), validAttempt(now))
	assertRejection(t, err, CodeReadBackMismatch)
	if !result.Frozen || result.Applied || actuator.applyCalls != 1 {
		t.Fatalf("read-back mismatch did not fail closed: result=%+v actuator=%+v", result, actuator)
	}
}

func newTestGate(t *testing.T) (*Gate, *fakeActuator, *fakeLeaseVerifier, *fakeApprovalVerifier, time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	actuator := &fakeActuator{effective: EffectiveLimit{ResourceKey: "resource-a", WriteLimitMiBPS: 20}}
	leases := &fakeLeaseVerifier{current: validAttempt(now).Lease}
	approvals := &fakeApprovalVerifier{approval: Approval{
		ID: "approval-a", DomainKey: "domain-a", PolicyVersion: "policy-v1", ResourceKey: "resource-a",
		MinimumMiBPS: 5, MaximumMiBPS: 25, ExpiresAt: now.Add(10 * time.Minute),
	}}
	gate, err := NewGate("domain-a", "controller-a", "policy-v1", []Envelope{{
		ResourceKey: "resource-a", MinimumMiBPS: 5, MaximumMiBPS: 25,
	}}, actuator, leases, approvals, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return gate, actuator, leases, approvals, now
}

func validAttempt(now time.Time) Attempt {
	return Attempt{
		Request: v1.ApplyRequest{
			SchemaVersion: v1.SchemaVersion, ProposalID: "proposal-a", ApprovalID: "approval-a",
			PolicyVersion: "policy-v1", DomainKey: "domain-a", LeaseHolderID: "controller-a",
			LeaseGeneration: 7, ResourceKey: "resource-a", WriteLimitMiBPS: 15,
			ExpiresAt: now.Add(5 * time.Minute),
		},
		Lease: Lease{
			DomainKey: "domain-a", HolderID: "controller-a", Generation: 7,
			ExpiresAt: now.Add(time.Minute),
		},
		ExpectedEffectiveLimitMiBPS: 20,
	}
}

type fakeLeaseVerifier struct {
	current Lease
	err     error
	calls   int
}

type fakeApprovalVerifier struct {
	approval Approval
	err      error
	calls    int
}

func (f *fakeApprovalVerifier) Get(_ context.Context, _ string) (Approval, error) {
	f.calls++
	return f.approval, f.err
}

func (f *fakeLeaseVerifier) Current(_ context.Context, _ string) (Lease, error) {
	f.calls++
	return f.current, f.err
}

func assertRejection(t *testing.T, err error, code string) {
	t.Helper()
	var rejection *Rejection
	if !errors.As(err, &rejection) || rejection.Code != code {
		t.Fatalf("expected rejection %q, got %v", code, err)
	}
}

type fakeActuator struct {
	effective        EffectiveLimit
	readBackOverride float64
	applyErr         error
	readCalls        int
	applyCalls       int
}

func (f *fakeActuator) ReadEffective(_ context.Context, _ string) (EffectiveLimit, error) {
	f.readCalls++
	return f.effective, nil
}

func (f *fakeActuator) ApplyApproved(_ context.Context, request v1.ApplyRequest) (EffectiveLimit, error) {
	f.applyCalls++
	if f.applyErr != nil {
		return EffectiveLimit{}, f.applyErr
	}
	f.effective = EffectiveLimit{ResourceKey: request.ResourceKey, WriteLimitMiBPS: request.WriteLimitMiBPS}
	if f.readBackOverride != 0 {
		f.effective.WriteLimitMiBPS = f.readBackOverride
	}
	return f.effective, nil
}
