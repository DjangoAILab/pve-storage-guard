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
