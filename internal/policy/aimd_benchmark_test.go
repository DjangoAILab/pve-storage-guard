package policy

import (
	"testing"
	"time"
)

func BenchmarkControllerObserve(b *testing.B) {
	controller, err := NewController(PoolPolicy{
		MinimumBudgetMiBPS: 5, InitialBudgetMiBPS: 20, MaximumBudgetMiBPS: 25,
		HealthyWaitMilliseconds: 15, TargetWaitMilliseconds: 25, EmergencyMilliseconds: 100,
		AdditiveIncreaseMiBPS: 0.5, MultiplicativeDecrease: 0.5,
		HealthyWindows: 12, BreachWindows: 2, Cooldown: 60 * time.Second,
	})
	if err != nil {
		b.Fatal(err)
	}
	base := time.Unix(0, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		wait := 10.0
		if index%20 >= 18 {
			wait = 30
		}
		controller.Observe(Feedback{
			At: base.Add(time.Duration(index) * time.Second), WaitMilliseconds: wait,
			WaitValid: true, ManagementPlaneHealthy: true,
		})
	}
}
