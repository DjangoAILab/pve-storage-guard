// Package safety owns the platform-neutral boundary between policy proposals
// and any privileged actuator.
package safety

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	v1 "github.com/DjangoAILab/pve-storage-guard/api/v1"
)

const limitEpsilon = 1e-6

// Rejection codes are stable decision-journal values. They intentionally do
// not include actuator error text, which could contain environment details.
const (
	CodeInvalidRequest      = "invalid_request"
	CodeLeaseUnavailable    = "lease_unavailable"
	CodeLeaseConflict       = "lease_conflict"
	CodeLeaseExpired        = "lease_expired"
	CodeApprovalUnavailable = "approval_unavailable"
	CodeApprovalMismatch    = "approval_mismatch"
	CodeApprovalExpired     = "approval_expired"
	CodeResourceFrozen      = "resource_frozen"
	CodeEffectiveReadFailed = "effective_read_failed"
	CodeEffectiveDrift      = "effective_state_drift"
	CodeApplyFailed         = "apply_failed"
	CodeReadBackMismatch    = "readback_mismatch"
)

// Lease proves that one controller generation owns a storage domain until an
// explicit deadline. Lease persistence and renewal live outside this package.
type Lease struct {
	DomainKey  string
	HolderID   string
	Generation uint64
	ExpiresAt  time.Time
}

// Envelope is the hard write-limit boundary for one explicitly enrolled
// resource.
type Envelope struct {
	ResourceKey  string
	MinimumMiBPS float64
	MaximumMiBPS float64
}

// EffectiveLimit is the constrained state returned by an actuator. The
// actuator interface contains no command, argument, or lifecycle operation.
type EffectiveLimit struct {
	ResourceKey     string
	WriteLimitMiBPS float64
}

// Actuator is implemented by a platform adapter after least-privilege review.
type Actuator interface {
	ReadEffective(context.Context, string) (EffectiveLimit, error)
	ApplyApproved(context.Context, v1.ApplyRequest) (EffectiveLimit, error)
}

// LeaseVerifier returns the authoritative current lease. The safety gate never
// treats caller-supplied lease fields as proof of ownership.
type LeaseVerifier interface {
	Current(context.Context, string) (Lease, error)
}

// Approval is the immutable operator-reviewed envelope for one resource.
type Approval struct {
	ID            string
	DomainKey     string
	PolicyVersion string
	ResourceKey   string
	MinimumMiBPS  float64
	MaximumMiBPS  float64
	ExpiresAt     time.Time
}

// ApprovalVerifier returns approvals from an authoritative store. A non-empty
// ID in a request is not sufficient authorization.
type ApprovalVerifier interface {
	Get(context.Context, string) (Approval, error)
}

// Attempt contains the durable state that must match immediately before an
// approved request can cross the privileged boundary.
type Attempt struct {
	Request                     v1.ApplyRequest
	Lease                       Lease
	ExpectedEffectiveLimitMiBPS float64
}

// Result is safe to journal. Detailed adapter errors belong in restricted
// local logs and are deliberately excluded.
type Result struct {
	Code                        string
	DomainKey                   string
	ResourceKey                 string
	PolicyVersion               string
	LeaseGeneration             uint64
	PreviousEffectiveLimitMiBPS float64
	DesiredLimitMiBPS           float64
	ReadBackLimitMiBPS          float64
	Applied                     bool
	Frozen                      bool
}

// Rejection is an explainable fail-closed result.
type Rejection struct {
	Code string
	err  error
}

func (r *Rejection) Error() string { return fmt.Sprintf("safety gate rejected request: %s", r.Code) }
func (r *Rejection) Unwrap() error { return r.err }

// Gate serializes one storage domain and freezes resources after ambiguous or
// failed privileged operations. A frozen resource requires external,
// operator-reviewed reconciliation; there is intentionally no automatic reset.
type Gate struct {
	domainKey     string
	holderID      string
	policyVersion string
	actuator      Actuator
	leases        LeaseVerifier
	approvals     ApprovalVerifier
	now           func() time.Time
	envelopes     map[string]Envelope

	mu     sync.Mutex
	frozen map[string]string
}

// NewGate validates immutable control-plane inputs. It does not acquire a
// lease, open a listener, or enable actuation.
func NewGate(domainKey, holderID, policyVersion string, envelopes []Envelope, actuator Actuator, leases LeaseVerifier, approvals ApprovalVerifier, now func() time.Time) (*Gate, error) {
	if domainKey == "" || holderID == "" || policyVersion == "" {
		return nil, errors.New("domain key, holder id, and policy version are required")
	}
	if actuator == nil || leases == nil || approvals == nil || now == nil {
		return nil, errors.New("actuator, lease verifier, approval verifier, and clock are required")
	}
	if len(envelopes) == 0 {
		return nil, errors.New("at least one enrollment envelope is required")
	}
	resolved := make(map[string]Envelope, len(envelopes))
	for _, envelope := range envelopes {
		if envelope.ResourceKey == "" || !finitePositive(envelope.MinimumMiBPS) || !finitePositive(envelope.MaximumMiBPS) || envelope.MinimumMiBPS > envelope.MaximumMiBPS {
			return nil, errors.New("enrollment envelope is invalid")
		}
		if _, exists := resolved[envelope.ResourceKey]; exists {
			return nil, errors.New("duplicate enrollment resource key")
		}
		resolved[envelope.ResourceKey] = envelope
	}
	return &Gate{
		domainKey: domainKey, holderID: holderID, policyVersion: policyVersion,
		actuator: actuator, leases: leases, approvals: approvals, now: now, envelopes: resolved,
		frozen: make(map[string]string),
	}, nil
}

// Apply validates lease ownership, immutable approval fields, enrollment,
// bounds, expiry, and effective-state continuity before calling the actuator.
func (g *Gate) Apply(ctx context.Context, attempt Attempt) (Result, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	request := attempt.Request
	result := Result{
		DomainKey: g.domainKey, ResourceKey: request.ResourceKey,
		PolicyVersion: request.PolicyVersion, LeaseGeneration: attempt.Lease.Generation,
		DesiredLimitMiBPS: request.WriteLimitMiBPS,
	}
	now := g.now()
	if code, err := g.validateRequest(attempt, now); err != nil {
		result.Code = code
		return result, reject(code, err)
	}
	if code, err := g.verifyLease(ctx, attempt.Lease, now); err != nil {
		result.Code = code
		return result, reject(code, err)
	}
	if code, err := g.verifyApproval(ctx, request, now); err != nil {
		result.Code = code
		return result, reject(code, err)
	}
	if reason, frozen := g.frozen[request.ResourceKey]; frozen {
		result.Code, result.Frozen = CodeResourceFrozen, true
		return result, reject(CodeResourceFrozen, errors.New(reason))
	}

	previous, err := g.actuator.ReadEffective(ctx, request.ResourceKey)
	if err != nil || !g.validEffective(previous, request.ResourceKey) {
		return g.freeze(result, CodeEffectiveReadFailed, errors.New("effective state is unavailable or invalid"))
	}
	result.PreviousEffectiveLimitMiBPS = previous.WriteLimitMiBPS
	if !sameLimit(previous.WriteLimitMiBPS, attempt.ExpectedEffectiveLimitMiBPS) {
		return g.freeze(result, CodeEffectiveDrift, errors.New("effective state differs from durable expected state"))
	}

	readBack, err := g.actuator.ApplyApproved(ctx, request)
	if err != nil {
		return g.freeze(result, CodeApplyFailed, errors.New("actuator returned an error"))
	}
	result.ReadBackLimitMiBPS = readBack.WriteLimitMiBPS
	if !g.validEffective(readBack, request.ResourceKey) || !sameLimit(readBack.WriteLimitMiBPS, request.WriteLimitMiBPS) {
		return g.freeze(result, CodeReadBackMismatch, errors.New("read-back does not match approved desired state"))
	}

	result.Code = "applied"
	result.Applied = true
	return result, nil
}

func (g *Gate) validateRequest(attempt Attempt, now time.Time) (string, error) {
	request, lease := attempt.Request, attempt.Lease
	envelope, enrolled := g.envelopes[request.ResourceKey]
	if request.SchemaVersion != v1.SchemaVersion || request.ProposalID == "" || request.ApprovalID == "" ||
		request.PolicyVersion != g.policyVersion || request.DomainKey != g.domainKey ||
		request.LeaseHolderID != lease.HolderID || request.LeaseGeneration != lease.Generation ||
		request.ResourceKey == "" || !enrolled ||
		request.ExpiresAt.IsZero() || !request.ExpiresAt.After(now) ||
		!within(request.WriteLimitMiBPS, envelope) || !within(attempt.ExpectedEffectiveLimitMiBPS, envelope) {
		return CodeInvalidRequest, errors.New("request identity, approval, expiry, enrollment, or bounds are invalid")
	}
	return "", nil
}

func (g *Gate) verifyLease(ctx context.Context, presented Lease, now time.Time) (string, error) {
	current, err := g.leases.Current(ctx, g.domainKey)
	if err != nil {
		return CodeLeaseUnavailable, errors.New("authoritative lease is unavailable")
	}
	if current.DomainKey != g.domainKey || current.HolderID != g.holderID || current.Generation == 0 ||
		presented.DomainKey != current.DomainKey || presented.HolderID != current.HolderID ||
		presented.Generation != current.Generation || !presented.ExpiresAt.Equal(current.ExpiresAt) {
		return CodeLeaseConflict, errors.New("presented lease does not match the authoritative owner and generation")
	}
	if current.ExpiresAt.IsZero() || !current.ExpiresAt.After(now) {
		return CodeLeaseExpired, errors.New("authoritative lease has expired")
	}
	return "", nil
}

func (g *Gate) verifyApproval(ctx context.Context, request v1.ApplyRequest, now time.Time) (string, error) {
	approval, err := g.approvals.Get(ctx, request.ApprovalID)
	if err != nil {
		return CodeApprovalUnavailable, errors.New("authoritative approval is unavailable")
	}
	if approval.ID != request.ApprovalID || approval.DomainKey != g.domainKey ||
		approval.PolicyVersion != g.policyVersion || approval.ResourceKey != request.ResourceKey ||
		!finitePositive(approval.MinimumMiBPS) || !finitePositive(approval.MaximumMiBPS) ||
		approval.MinimumMiBPS > approval.MaximumMiBPS || request.WriteLimitMiBPS < approval.MinimumMiBPS-limitEpsilon ||
		request.WriteLimitMiBPS > approval.MaximumMiBPS+limitEpsilon {
		return CodeApprovalMismatch, errors.New("request does not match the authoritative approval envelope")
	}
	if approval.ExpiresAt.IsZero() || !approval.ExpiresAt.After(now) {
		return CodeApprovalExpired, errors.New("authoritative approval has expired")
	}
	if request.ExpiresAt.After(approval.ExpiresAt) {
		return CodeApprovalMismatch, errors.New("request expiry exceeds the authoritative approval")
	}
	return "", nil
}

func (g *Gate) validEffective(limit EffectiveLimit, resourceKey string) bool {
	envelope, enrolled := g.envelopes[resourceKey]
	return enrolled && limit.ResourceKey == resourceKey && within(limit.WriteLimitMiBPS, envelope)
}

func (g *Gate) freeze(result Result, code string, cause error) (Result, error) {
	g.frozen[result.ResourceKey] = code
	result.Code, result.Frozen = code, true
	return result, reject(code, cause)
}

func reject(code string, cause error) error { return &Rejection{Code: code, err: cause} }

func within(value float64, envelope Envelope) bool {
	return finitePositive(value) && value >= envelope.MinimumMiBPS-limitEpsilon && value <= envelope.MaximumMiBPS+limitEpsilon
}

func sameLimit(left, right float64) bool { return math.Abs(left-right) <= limitEpsilon }

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
