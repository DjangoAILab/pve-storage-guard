# Performance evidence

## Scope

The controller runs far below storage and management-plane time scales, but its
local paths still need reproducible performance smoke tests. Go benchmarks cover
one policy observation, deterministic allocation across 100 disks, one fully
verified no-change safety-gate pass using in-memory fakes, and the local shadow
command with and without its opt-in private journal.

The shadow-command benchmark includes real example policy/enrollment reads,
JSONL decoding and proposal encoding, policy evaluation, allocation, and command
startup/close overhead for each 32-observation batch. The journal case also
reopens the private file and performs one real append plus `fsync` per event.
Proposal bytes are encoded to `io.Discard`; terminal, pipe, and network output
costs are excluded.

These benchmarks do not include metric collection, ITOps durable stores,
network I/O, PVE API latency, QEMU configuration, or read-back from a host. They
are not end-to-end production performance claims.

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

### Local shadow command

Five 500 ms runs on the same machine and runtime used a fixed batch of 32 valid
observations:

| Path | Observed time per observation | Heap allocation per 32-observation batch |
| --- | ---: | ---: |
| Default shadow, no journal | 4.048–4.155 µs | 158,413–158,421 B; 1,037 allocs |
| Shadow with private append + `fsync` journal | 3.770–3.827 ms | 222,704–223,171 B; 1,429–1,431 allocs |

The journal result measures this healthy local APFS filesystem. It does not
predict sync latency on a PVE host, especially during storage pressure. The
rough three-order-of-magnitude difference is expected because the stronger
audit mode deliberately syncs every event before emitting its proposal. The
journal remains opt-in; it must not be placed on the guarded storage domain.
Batching or asynchronous persistence would change failure and durability
semantics and is not part of this result.

## Reproduction

```sh
go test -run '^$' -bench . -benchtime=500ms -benchmem -count=5 \
  ./internal/policy ./internal/allocator ./internal/safety \
  ./cmd/pve-storage-guard
```

CI runs shorter smoke benchmarks to ensure all benchmark paths compile and
execute. It does not enforce absolute nanosecond thresholds on shared runners.
A regression gate should be added only with a stable runner, repeated samples,
and a reviewed tolerance. Collector, adapter, persistent ITOps, and controlled
host latency remain canary prerequisites.
