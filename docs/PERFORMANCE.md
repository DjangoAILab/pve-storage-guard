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

### ITOps replay export window

The internal ITOps Draft integration includes a pure, route-free builder that
joins already-authorized diskstats and management samples into a sanitized
ReplayTrace. A performance audit found that its first implementation repeatedly
scanned the complete caller array for every interval. It now builds
request-scoped, first-match indexes in one pass and sorts only the qualifying
write-wait series. A regression test rejects any return to global `find` scans,
while a separate compatibility test preserves the previous duplicate-input
semantics.

Five local runs on the same Apple M4 Pro, using Node.js 22.21.1 on
Darwin/arm64, generated deterministic 60-second samples before timing only the
in-memory export:

| Window | Input samples | Output intervals | Export time range | GC-retained heap delta |
| --- | ---: | ---: | ---: | ---: |
| 1 day | 10,080 | 1,440 | 7.164–7.563 ms | 354,400–355,776 B |
| 7 days | 70,560 | 10,080 | 42.294–46.537 ms | 1,838,536–1,838,680 B |
| 14 days | 141,120 | 20,160 | 80.472–82.093 ms | 3,583,840 B |

Every repeated case produced the same output SHA-256 for its scale and passed
interval, audit-count, coverage, and fixed `average` / `derived` block-device
semantic checks. The heap figure is measured after explicit garbage collection
with the returned trace still live; it is retained output memory, not peak
allocation.

Before internal PR #37 merged, internal CI run 161
independently executed the one-day smoke inside the complete Node quality gate;
that gate passed in 4m34s and its dependent linux/amd64 image build passed in
4m45s. The integration later merged after explicit approval but remains
undeployed.

This is not a collector or host benchmark. It excludes fixture generation,
probe execution, SSH, PVE REST, SQLite, network delivery, dashboard queries,
and storage-device behavior. It only shows that the pure export stage is no
longer the expected bottleneck for bounded deployment validation or a later
representative 7–14-day alert-calibration window. Deployment acceptance itself
has no fixed 24-hour waiting requirement.

## Reproduction

```sh
go test -run '^$' -bench . -benchtime=500ms -benchmem -count=5 \
  ./internal/policy ./internal/allocator ./internal/safety \
  ./cmd/pve-storage-guard
```

The internal ITOps Draft reproduces its identity-free export cases with:

```sh
npm --prefix backend run benchmark:storage-replay
```

CI runs shorter smoke benchmarks to ensure all benchmark paths compile and
execute. It does not enforce absolute nanosecond thresholds on shared runners.
A regression gate should be added only with a stable runner, repeated samples,
and a reviewed tolerance. Restricted-probe/SSH execution, PVE REST adapter,
persistent ITOps, real host storage, and controlled-load latency remain canary
prerequisites.
