package allocator

import (
	"math"
	"testing"
)

func TestAllocateRespectsBoundsAndWeights(t *testing.T) {
	result, err := Allocate(50, []DiskEnvelope{
		{ResourceKey: "b", MinimumMiBPS: 10, MaximumMiBPS: 50, Weight: 3},
		{ResourceKey: "a", MinimumMiBPS: 5, MaximumMiBPS: 20, Weight: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Feasible {
		t.Fatal("expected feasible")
	}
	if math.Abs(result.Allocations["a"]+result.Allocations["b"]-50) > 1e-6 {
		t.Fatalf("allocation: %+v", result)
	}
	if result.Allocations["b"] <= result.Allocations["a"] {
		t.Fatalf("weights ignored: %+v", result)
	}
}

func TestInfeasibleMinimumsAreExplicit(t *testing.T) {
	result, err := Allocate(15, []DiskEnvelope{
		{ResourceKey: "a", MinimumMiBPS: 10, MaximumMiBPS: 20, Weight: 1},
		{ResourceKey: "b", MinimumMiBPS: 10, MaximumMiBPS: 20, Weight: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Feasible || result.RequiredMinimumMiBPS != 20 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRejectsDuplicateAndNonFiniteInput(t *testing.T) {
	_, err := Allocate(10, []DiskEnvelope{{ResourceKey: "a", MaximumMiBPS: 10, Weight: 1}, {ResourceKey: "a", MaximumMiBPS: 10, Weight: 1}})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	_, err = Allocate(math.NaN(), nil)
	if err == nil {
		t.Fatal("expected non-finite error")
	}
}

func FuzzAllocationNeverExceedsEnvelope(f *testing.F) {
	f.Add(float64(20), float64(5), float64(15), float64(1))
	f.Fuzz(func(t *testing.T, budget, minimum, maximum, weight float64) {
		result, err := Allocate(budget, []DiskEnvelope{{
			ResourceKey: "resource-a", MinimumMiBPS: minimum,
			MaximumMiBPS: maximum, Weight: weight,
		}})
		if err != nil {
			return
		}
		allocation := result.Allocations["resource-a"]
		if allocation < minimum-1e-6 || allocation > maximum+1e-6 {
			t.Fatalf("allocation escaped envelope: %+v", result)
		}
	})
}

func FuzzAllocationFeasibilityAndOrderInvariance(f *testing.F) {
	f.Add(float64(30), float64(5), float64(15), float64(1), float64(7), float64(20), float64(3))
	f.Fuzz(func(t *testing.T, budget, minimumA, roomA, weightA, minimumB, roomB, weightB float64) {
		minimumA = boundedFuzzValue(minimumA, 20)
		minimumB = boundedFuzzValue(minimumB, 20)
		maximumA := minimumA + boundedFuzzValue(roomA, 50)
		maximumB := minimumB + boundedFuzzValue(roomB, 50)
		weightA = 0.1 + boundedFuzzValue(weightA, 10)
		weightB = 0.1 + boundedFuzzValue(weightB, 10)
		budget = boundedFuzzValue(budget, 120)
		a := DiskEnvelope{ResourceKey: "a", MinimumMiBPS: minimumA, MaximumMiBPS: maximumA, Weight: weightA}
		b := DiskEnvelope{ResourceKey: "b", MinimumMiBPS: minimumB, MaximumMiBPS: maximumB, Weight: weightB}

		forward, err := Allocate(budget, []DiskEnvelope{a, b})
		if err != nil {
			t.Fatal(err)
		}
		reverse, err := Allocate(budget, []DiskEnvelope{b, a})
		if err != nil {
			t.Fatal(err)
		}
		if forward.Feasible != reverse.Feasible ||
			math.Abs(forward.Allocations["a"]-reverse.Allocations["a"]) > 1e-9 ||
			math.Abs(forward.Allocations["b"]-reverse.Allocations["b"]) > 1e-9 {
			t.Fatalf("input order changed allocation: forward=%+v reverse=%+v", forward, reverse)
		}

		required := minimumA + minimumB
		if !forward.Feasible {
			if required <= budget+1e-9 || math.Abs(forward.RequiredMinimumMiBPS-required) > 1e-9 {
				t.Fatalf("incorrect infeasible result: budget=%f result=%+v", budget, forward)
			}
			return
		}
		allocationA, allocationB := forward.Allocations["a"], forward.Allocations["b"]
		if allocationA < minimumA-1e-6 || allocationA > maximumA+1e-6 ||
			allocationB < minimumB-1e-6 || allocationB > maximumB+1e-6 {
			t.Fatalf("allocation escaped envelope: a=%+v b=%+v result=%+v", a, b, forward)
		}
		total := allocationA + allocationB
		if total > budget+2e-6 {
			t.Fatalf("allocation exceeded budget: budget=%f result=%+v", budget, forward)
		}
		maximumTotal := maximumA + maximumB
		expectedTotal := math.Min(budget, maximumTotal)
		if math.Abs(total-expectedTotal) > 2e-6 {
			t.Fatalf("feasible budget was not fully allocated: expected=%f result=%+v", expectedTotal, forward)
		}
	})
}

func boundedFuzzValue(value, upper float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Mod(math.Abs(value), upper)
}
