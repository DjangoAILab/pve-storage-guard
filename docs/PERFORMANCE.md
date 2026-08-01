# Performance evidence

## Scope

The controller runs far below storage and management-plane time scales, but the
core path still needs a reproducible performance smoke test. Go benchmarks cover
one policy observation, deterministic allocation across 100 disks, and one
fully verified no-change safety-gate pass using in-memory fakes.

These benchmarks do not include metric collection, durable stores, network I/O,
PVE API latency, QEMU configuration, or read-back from a host. They are not
end-to-end production performance claims.

## Baseline

Five 500 ms runs on 2026-08-02 used Go 1.26.1 on Darwin/arm64, Apple M4 Pro:

| Benchmark | Observed range | Heap allocation |
| --- | ---: | ---: |
| Policy `Observe` | 25.55–25.81 ns/op | 0 B, 0 allocs/op |
| Allocate 100 disks | 12.50–12.93 µs/op | 11,856 B, 9 allocs/op |
| Verified no-change safety gate | 76.74–78.65 ns/op | 0 B, 0 allocs/op |

The 100-disk allocator case is deliberately larger than the initial one-disk
canary. The numbers show only that local computation is not the expected control
loop bottleneck on this machine. Hosted CI and other architectures will differ.

## Reproduction

```sh
go test -run '^$' -bench . -benchtime=500ms -benchmem -count=5 \
  ./internal/policy ./internal/allocator ./internal/safety
```

CI runs shorter smoke benchmarks to ensure all benchmark paths compile and
execute. It does not enforce absolute nanosecond thresholds on shared runners.
A regression gate should be added only with a stable runner, repeated samples,
and a reviewed tolerance. End-to-end latency remains a canary prerequisite.
