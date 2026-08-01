package allocator

import (
	"strconv"
	"testing"
)

func BenchmarkAllocateHundredDisks(b *testing.B) {
	disks := make([]DiskEnvelope, 100)
	for index := range disks {
		disks[index] = DiskEnvelope{
			ResourceKey:  "resource-" + strconv.Itoa(index),
			MinimumMiBPS: 1, MaximumMiBPS: 20, Weight: float64(index%5 + 1),
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		result, err := Allocate(500, disks)
		if err != nil || !result.Feasible {
			b.Fatalf("unexpected allocation: result=%+v err=%v", result, err)
		}
	}
}
