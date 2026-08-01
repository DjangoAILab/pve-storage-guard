// Package allocator distributes a storage-domain budget within disk envelopes.
package allocator

import (
	"errors"
	"math"
	"sort"
)

// DiskEnvelope is an explicitly enrolled disk allocation boundary.
type DiskEnvelope struct {
	ResourceKey  string
	MinimumMiBPS float64
	MaximumMiBPS float64
	Weight       float64
}

// Result explains both feasible and infeasible allocations.
type Result struct {
	Allocations          map[string]float64
	Feasible             bool
	RequiredMinimumMiBPS float64
	UnallocatedMiBPS     float64
}

// Allocate grants minima, then weighted shares, and never exceeds maxima.
func Allocate(budgetMiBPS float64, disks []DiskEnvelope) (Result, error) {
	if invalidNumber(budgetMiBPS) || budgetMiBPS < 0 {
		return Result{}, errors.New("budget must be finite and non-negative")
	}
	ordered := append([]DiskEnvelope(nil), disks...)
	for _, disk := range ordered {
		if disk.ResourceKey == "" {
			return Result{}, errors.New("resource key is required")
		}
		if invalidNumber(disk.MinimumMiBPS) || invalidNumber(disk.MaximumMiBPS) || disk.MinimumMiBPS < 0 || disk.MinimumMiBPS > disk.MaximumMiBPS {
			return Result{}, errors.New("disk bounds must satisfy finite 0 <= minimum <= maximum")
		}
		if invalidNumber(disk.Weight) || disk.Weight <= 0 {
			return Result{}, errors.New("disk weight must be finite and positive")
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ResourceKey < ordered[j].ResourceKey })
	for i := 1; i < len(ordered); i++ {
		if ordered[i-1].ResourceKey == ordered[i].ResourceKey {
			return Result{}, errors.New("resource keys must be unique")
		}
	}

	allocations := make(map[string]float64, len(ordered))
	required := 0.0
	for _, disk := range ordered {
		allocations[disk.ResourceKey] = disk.MinimumMiBPS
		required += disk.MinimumMiBPS
	}
	if required > budgetMiBPS+1e-9 {
		return Result{Allocations: allocations, Feasible: false, RequiredMinimumMiBPS: required}, nil
	}
	remaining := budgetMiBPS - required
	eligible := append([]DiskEnvelope(nil), ordered...)
	for remaining > 1e-9 && len(eligible) > 0 {
		totalWeight := 0.0
		for _, disk := range eligible {
			if allocations[disk.ResourceKey] < disk.MaximumMiBPS-1e-9 {
				totalWeight += disk.Weight
			}
		}
		if totalWeight <= 0 {
			break
		}
		distributed := 0.0
		next := eligible[:0]
		for _, disk := range eligible {
			room := disk.MaximumMiBPS - allocations[disk.ResourceKey]
			if room <= 1e-9 {
				continue
			}
			grant := math.Min(remaining*disk.Weight/totalWeight, room)
			allocations[disk.ResourceKey] += grant
			distributed += grant
			if room-grant > 1e-9 {
				next = append(next, disk)
			}
		}
		remaining -= distributed
		eligible = next
		if distributed <= 1e-12 {
			break
		}
	}
	for key, value := range allocations {
		allocations[key] = math.Round(value*1_000_000) / 1_000_000
	}
	return Result{Allocations: allocations, Feasible: true, RequiredMinimumMiBPS: required, UnallocatedMiBPS: math.Max(0, remaining)}, nil
}

func invalidNumber(value float64) bool { return math.IsNaN(value) || math.IsInf(value, 0) }
