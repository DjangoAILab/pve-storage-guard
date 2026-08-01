// Package policy implements the platform-neutral storage-slo-guard engine.
package policy

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// PoolPolicy is the immutable configuration for one storage-domain controller.
type PoolPolicy struct {
	MinimumBudgetMiBPS      float64
	InitialBudgetMiBPS      float64
	MaximumBudgetMiBPS      float64
	HealthyWaitMilliseconds float64
	TargetWaitMilliseconds  float64
	EmergencyMilliseconds   float64
	AdditiveIncreaseMiBPS   float64
	MultiplicativeDecrease  float64
	HealthyWindows          int
	BreachWindows           int
	Cooldown                time.Duration
}

// Validate rejects policies that cannot satisfy the controller's hard bounds.
func (p PoolPolicy) Validate() error {
	switch {
	case !finitePositive(p.MinimumBudgetMiBPS):
		return errors.New("minimum budget must be finite and positive")
	case !finitePositive(p.InitialBudgetMiBPS), !finitePositive(p.MaximumBudgetMiBPS):
		return errors.New("initial and maximum budgets must be finite and positive")
	case p.MinimumBudgetMiBPS > p.InitialBudgetMiBPS || p.InitialBudgetMiBPS > p.MaximumBudgetMiBPS:
		return errors.New("budget bounds must satisfy minimum <= initial <= maximum")
	case !finiteNonNegative(p.HealthyWaitMilliseconds):
		return errors.New("healthy wait must be finite and non-negative")
	case !finitePositive(p.TargetWaitMilliseconds), !finitePositive(p.EmergencyMilliseconds):
		return errors.New("target and emergency waits must be finite and positive")
	case p.HealthyWaitMilliseconds >= p.TargetWaitMilliseconds || p.TargetWaitMilliseconds >= p.EmergencyMilliseconds:
		return errors.New("wait thresholds must be strictly increasing")
	case !finitePositive(p.AdditiveIncreaseMiBPS):
		return errors.New("additive increase must be finite and positive")
	case !finitePositive(p.MultiplicativeDecrease) || p.MultiplicativeDecrease >= 1:
		return errors.New("multiplicative decrease must be between zero and one")
	case p.HealthyWindows < 1 || p.BreachWindows < 1:
		return errors.New("window counts must be positive")
	case p.Cooldown < 0:
		return errors.New("cooldown must be non-negative")
	default:
		return nil
	}
}

// Feedback is one aggregated storage-domain observation.
type Feedback struct {
	At                     time.Time
	WaitMilliseconds       float64
	WaitValid              bool
	Stale                  bool
	Emergency              bool
	ManagementPlaneHealthy bool
}

// State is durable controller state. Desired and effective actuation state live
// outside this pure policy package.
type State struct {
	BudgetMiBPS         float64
	ConsecutiveHealthy  int
	ConsecutiveBreaches int
	LastChangeAt        time.Time
}

// Decision explains a proposed aggregate budget.
type Decision struct {
	At                  time.Time
	PreviousBudgetMiBPS float64
	BudgetMiBPS         float64
	Changed             bool
	Reason              string
}

// Controller evaluates a single storage domain deterministically.
type Controller struct {
	policy PoolPolicy
	state  State
}

// NewController validates policy and returns a controller at its initial budget.
func NewController(p PoolPolicy) (*Controller, error) {
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("validate pool policy: %w", err)
	}
	return &Controller{policy: p, state: State{BudgetMiBPS: p.InitialBudgetMiBPS}}, nil
}

// RestoreController validates durable state before resuming a controller.
func RestoreController(p PoolPolicy, state State) (*Controller, error) {
	controller, err := NewController(p)
	if err != nil {
		return nil, err
	}
	if !finitePositive(state.BudgetMiBPS) || state.BudgetMiBPS < p.MinimumBudgetMiBPS || state.BudgetMiBPS > p.MaximumBudgetMiBPS {
		return nil, errors.New("restored budget is outside policy bounds")
	}
	if state.ConsecutiveHealthy < 0 || state.ConsecutiveBreaches < 0 {
		return nil, errors.New("restored counters must be non-negative")
	}
	if state.ConsecutiveHealthy > 0 && state.ConsecutiveBreaches > 0 {
		return nil, errors.New("restored healthy and breach counters are mutually exclusive")
	}
	if state.ConsecutiveHealthy >= p.HealthyWindows || state.ConsecutiveBreaches >= p.BreachWindows {
		return nil, errors.New("restored counters must be below their decision windows")
	}
	controller.state = state
	return controller, nil
}

// State returns a copy suitable for durable persistence.
func (c *Controller) State() State { return c.state }

// Observe evaluates one feedback sample. Invalid, stale, or management-plane
// unhealthy input can never increase the budget.
func (c *Controller) Observe(feedback Feedback) Decision {
	if feedback.Stale || !feedback.WaitValid || !finiteNonNegative(feedback.WaitMilliseconds) {
		c.resetCounters()
		return c.decide(feedback.At, c.state.BudgetMiBPS, "stale_or_invalid_hold", false)
	}

	if feedback.Emergency || feedback.WaitMilliseconds >= c.policy.EmergencyMilliseconds {
		c.resetCounters()
		return c.decide(feedback.At, c.policy.MinimumBudgetMiBPS, "emergency", true)
	}

	if !feedback.ManagementPlaneHealthy {
		c.resetCounters()
		desired := c.state.BudgetMiBPS * c.policy.MultiplicativeDecrease
		return c.decide(feedback.At, desired, "management_plane_decrease", true)
	}

	if feedback.WaitMilliseconds > c.policy.TargetWaitMilliseconds {
		c.state.ConsecutiveBreaches++
		c.state.ConsecutiveHealthy = 0
		if c.state.ConsecutiveBreaches >= c.policy.BreachWindows {
			c.state.ConsecutiveBreaches = 0
			return c.decide(feedback.At, c.state.BudgetMiBPS*c.policy.MultiplicativeDecrease, "multiplicative_decrease", false)
		}
		return c.decide(feedback.At, c.state.BudgetMiBPS, "breach_pending", false)
	}

	if feedback.WaitMilliseconds < c.policy.HealthyWaitMilliseconds {
		c.state.ConsecutiveHealthy++
		c.state.ConsecutiveBreaches = 0
		if c.state.ConsecutiveHealthy >= c.policy.HealthyWindows {
			c.state.ConsecutiveHealthy = 0
			return c.decide(feedback.At, c.state.BudgetMiBPS+c.policy.AdditiveIncreaseMiBPS, "additive_increase", false)
		}
		return c.decide(feedback.At, c.state.BudgetMiBPS, "healthy_pending", false)
	}

	c.resetCounters()
	return c.decide(feedback.At, c.state.BudgetMiBPS, "hysteresis_hold", false)
}

func (c *Controller) decide(at time.Time, desired float64, reason string, bypassCooldown bool) Decision {
	previous := c.state.BudgetMiBPS
	desired = math.Min(c.policy.MaximumBudgetMiBPS, math.Max(c.policy.MinimumBudgetMiBPS, desired))
	desired = math.Round(desired*1_000_000) / 1_000_000
	cooldownElapsed := c.state.LastChangeAt.IsZero() || at.Sub(c.state.LastChangeAt) >= c.policy.Cooldown
	changed := math.Abs(desired-previous) > 1e-9 && (bypassCooldown || cooldownElapsed)
	if changed {
		c.state.BudgetMiBPS = desired
		c.state.LastChangeAt = at
	} else if math.Abs(desired-previous) > 1e-9 && !cooldownElapsed {
		reason = "cooldown_hold"
	}
	return Decision{At: at, PreviousBudgetMiBPS: previous, BudgetMiBPS: c.state.BudgetMiBPS, Changed: changed, Reason: reason}
}

func (c *Controller) resetCounters() {
	c.state.ConsecutiveHealthy = 0
	c.state.ConsecutiveBreaches = 0
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}

func finiteNonNegative(value float64) bool {
	return value >= 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}
