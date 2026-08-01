package safety

import (
	"context"
	"testing"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

func BenchmarkGateApplyVerifiedNoChange(b *testing.B) {
	now := time.Unix(0, 0).UTC()
	lease := Lease{DomainKey: "domain-a", HolderID: "controller-a", Generation: 1, ExpiresAt: now.Add(time.Hour)}
	actuator := &benchmarkActuator{effective: EffectiveLimit{ResourceKey: "resource-a", WriteLimitMiBPS: 15}}
	gate, err := NewGate(
		"domain-a", "controller-a", "policy-v1",
		[]Envelope{{ResourceKey: "resource-a", MinimumMiBPS: 5, MaximumMiBPS: 25}},
		actuator,
		benchmarkLeaseVerifier{lease: lease},
		benchmarkApprovalVerifier{approval: Approval{
			ID: "approval-a", DomainKey: "domain-a", PolicyVersion: "policy-v1", ResourceKey: "resource-a",
			MinimumMiBPS: 5, MaximumMiBPS: 25, ExpiresAt: now.Add(time.Hour),
		}},
		func() time.Time { return now },
	)
	if err != nil {
		b.Fatal(err)
	}
	attempt := Attempt{
		Request: v1.ApplyRequest{
			SchemaVersion: v1.SchemaVersion, ProposalID: "proposal-a", ApprovalID: "approval-a",
			PolicyVersion: "policy-v1", DomainKey: "domain-a", LeaseHolderID: "controller-a",
			LeaseGeneration: 1, ResourceKey: "resource-a", WriteLimitMiBPS: 15,
			ExpiresAt: now.Add(30 * time.Minute),
		},
		Lease: lease, ExpectedEffectiveLimitMiBPS: 15,
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := gate.Apply(ctx, attempt); err != nil {
			b.Fatal(err)
		}
	}
}

type benchmarkLeaseVerifier struct{ lease Lease }

func (v benchmarkLeaseVerifier) Current(context.Context, string) (Lease, error) { return v.lease, nil }

type benchmarkApprovalVerifier struct{ approval Approval }

func (v benchmarkApprovalVerifier) Get(context.Context, string) (Approval, error) {
	return v.approval, nil
}

type benchmarkActuator struct{ effective EffectiveLimit }

func (a *benchmarkActuator) ReadEffective(context.Context, string) (EffectiveLimit, error) {
	return a.effective, nil
}

func (a *benchmarkActuator) ApplyApproved(_ context.Context, request v1.ApplyRequest) (EffectiveLimit, error) {
	a.effective = EffectiveLimit{ResourceKey: request.ResourceKey, WriteLimitMiBPS: request.WriteLimitMiBPS}
	return a.effective, nil
}
