package policy

import (
	"math"
	"testing"
	"time"
)

func testPolicy() PoolPolicy {
	return PoolPolicy{
		MinimumBudgetMiBPS: 5, InitialBudgetMiBPS: 20, MaximumBudgetMiBPS: 25,
		HealthyWaitMilliseconds: 15, TargetWaitMilliseconds: 25, EmergencyMilliseconds: 100,
		AdditiveIncreaseMiBPS: 0.5, MultiplicativeDecrease: 0.5,
		HealthyWindows: 2, BreachWindows: 2, Cooldown: 60 * time.Second,
	}
}

func healthy(at time.Time, wait float64) Feedback {
	return Feedback{At: at, WaitMilliseconds: wait, WaitValid: true, ManagementPlaneHealthy: true}
}

func TestInvalidAndStaleFeedbackNeverIncrease(t *testing.T) {
	c, err := NewController(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	for _, feedback := range []Feedback{
		{At: time.Unix(1, 0), Stale: true},
		{At: time.Unix(2, 0), WaitMilliseconds: math.NaN(), WaitValid: true},
		{At: time.Unix(3, 0), WaitMilliseconds: 1, WaitValid: false},
	} {
		decision := c.Observe(feedback)
		if decision.Changed || decision.BudgetMiBPS != 20 {
			t.Fatalf("unsafe decision: %+v", decision)
		}
	}
}

func TestEmergencyBypassesCooldownAndClamps(t *testing.T) {
	c, err := NewController(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Unix(100, 0)
	c.Observe(healthy(base, 1))
	c.Observe(healthy(base.Add(time.Minute), 1))
	decision := c.Observe(Feedback{At: base.Add(time.Minute + time.Second), WaitMilliseconds: 120, WaitValid: true, Emergency: true})
	if !decision.Changed || decision.BudgetMiBPS != 5 || decision.Reason != "emergency" {
		t.Fatalf("unexpected emergency: %+v", decision)
	}
}

func TestHealthyIncreaseIsSlowAndPressureDecreaseIsBounded(t *testing.T) {
	c, err := NewController(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Unix(100, 0)
	if d := c.Observe(healthy(base, 10)); d.Changed {
		t.Fatalf("first healthy window changed: %+v", d)
	}
	if d := c.Observe(healthy(base.Add(time.Minute), 10)); d.BudgetMiBPS != 20.5 {
		t.Fatalf("increase: %+v", d)
	}
	c.Observe(healthy(base.Add(2*time.Minute), 30))
	if d := c.Observe(healthy(base.Add(3*time.Minute), 30)); d.BudgetMiBPS != 10.25 {
		t.Fatalf("decrease: %+v", d)
	}
}

func TestManagementFailureCannotIncrease(t *testing.T) {
	c, err := NewController(testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	d := c.Observe(Feedback{At: time.Unix(100, 0), WaitMilliseconds: 1, WaitValid: true, ManagementPlaneHealthy: false})
	if !d.Changed || d.BudgetMiBPS != 10 || d.Reason != "management_plane_decrease" {
		t.Fatalf("unexpected decision: %+v", d)
	}
}

func TestRestoreRejectsOutOfBoundsState(t *testing.T) {
	_, err := RestoreController(testPolicy(), State{BudgetMiBPS: 100})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRestoreRejectsImpossibleCounterState(t *testing.T) {
	for _, state := range []State{
		{BudgetMiBPS: 20, ConsecutiveHealthy: 1, ConsecutiveBreaches: 1},
		{BudgetMiBPS: 20, ConsecutiveHealthy: testPolicy().HealthyWindows},
		{BudgetMiBPS: 20, ConsecutiveBreaches: testPolicy().BreachWindows},
	} {
		if _, err := RestoreController(testPolicy(), state); err == nil {
			t.Fatalf("accepted impossible restored state: %+v", state)
		}
	}
}

func TestRestorePreservesCooldownAndEmergencyBypass(t *testing.T) {
	base := time.Unix(100, 0)
	state := State{
		BudgetMiBPS:        20.5,
		ConsecutiveHealthy: 1,
		LastChangeAt:       base,
	}
	c, err := RestoreController(testPolicy(), state)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.State(); got != state {
		t.Fatalf("restore changed durable state: got %+v want %+v", got, state)
	}
	if decision := c.Observe(healthy(base.Add(time.Second), 1)); decision.Changed || decision.Reason != "cooldown_hold" {
		t.Fatalf("restart bypassed cooldown: %+v", decision)
	}
	decision := c.Observe(Feedback{
		At: base.Add(2 * time.Second), WaitMilliseconds: 120, WaitValid: true,
		Emergency: true, ManagementPlaneHealthy: true,
	})
	if !decision.Changed || decision.BudgetMiBPS != testPolicy().MinimumBudgetMiBPS || decision.Reason != "emergency" {
		t.Fatalf("emergency did not bypass restored cooldown: %+v", decision)
	}
}

func FuzzControllerBudgetNeverLeavesBounds(f *testing.F) {
	f.Add(float64(10), true, false, true, false, uint8(5))
	f.Add(float64(120), true, false, false, true, uint8(2))
	f.Add(math.NaN(), true, false, true, false, uint8(3))
	f.Fuzz(func(t *testing.T, wait float64, valid, stale, managementHealthy, emergency bool, steps uint8) {
		controller, err := NewController(testPolicy())
		if err != nil {
			t.Fatal(err)
		}
		base := time.Unix(100, 0)
		for i := 0; i < int(steps%32)+1; i++ {
			decision := controller.Observe(Feedback{
				At:                     base.Add(time.Duration(i) * time.Minute),
				WaitMilliseconds:       wait,
				WaitValid:              valid,
				Stale:                  stale,
				Emergency:              emergency,
				ManagementPlaneHealthy: managementHealthy,
			})
			if decision.BudgetMiBPS < 5 || decision.BudgetMiBPS > 25 {
				t.Fatalf("budget escaped bounds: %+v", decision)
			}
		}
	})
}

func FuzzHomogeneousFeedbackIsMonotonic(f *testing.F) {
	f.Add(uint8(8), uint8(2), uint8(2), float64(10), float64(30))
	f.Fuzz(func(t *testing.T, steps, healthyWindows, breachWindows uint8, healthyWait, pressureWait float64) {
		p := testPolicy()
		p.Cooldown = 0
		p.HealthyWindows = int(healthyWindows%8) + 1
		p.BreachWindows = int(breachWindows%8) + 1
		healthyWait = boundedFuzzValue(healthyWait, p.HealthyWaitMilliseconds)
		pressureWait = p.TargetWaitMilliseconds + 1 + boundedFuzzValue(
			pressureWait,
			p.EmergencyMilliseconds-p.TargetWaitMilliseconds-1,
		)

		healthyController, err := NewController(p)
		if err != nil {
			t.Fatal(err)
		}
		pressureController, err := NewController(p)
		if err != nil {
			t.Fatal(err)
		}
		previousHealthy, previousPressure := p.InitialBudgetMiBPS, p.InitialBudgetMiBPS
		for i := 0; i < int(steps%64)+1; i++ {
			at := time.Unix(int64(i+1), 0)
			healthyDecision := healthyController.Observe(healthy(at, healthyWait))
			if healthyDecision.BudgetMiBPS < previousHealthy-1e-9 {
				t.Fatalf("healthy sequence decreased: previous=%f decision=%+v", previousHealthy, healthyDecision)
			}
			pressureDecision := pressureController.Observe(healthy(at, pressureWait))
			if pressureDecision.BudgetMiBPS > previousPressure+1e-9 {
				t.Fatalf("pressure sequence increased: previous=%f decision=%+v", previousPressure, pressureDecision)
			}
			previousHealthy = healthyDecision.BudgetMiBPS
			previousPressure = pressureDecision.BudgetMiBPS
		}
	})
}

func boundedFuzzValue(value, upper float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Mod(math.Abs(value), upper)
}
