---
title: Performance evidence
description: Reproducible microbenchmarks and their limits.
---

The local paths have repeatable Go benchmarks for one policy observation,
allocation across 100 disks, a verified no-change safety-gate pass with
in-memory fakes, and the real shadow command with and without its opt-in private
journal.

Five 500 ms runs on an Apple M4 Pro with Go 1.26.1 produced:

| Benchmark | Range | Heap allocation |
| --- | ---: | ---: |
| Policy observation | 25.55–25.81 ns/op | 0 B, 0 allocs/op |
| 100-disk allocation | 12.50–12.93 µs/op | 11,856 B, 9 allocs/op |
| Verified safety gate | 76.74–78.65 ns/op | 0 B, 0 allocs/op |

The command benchmark reads the real example policy/enrollment and processes 32
valid JSONL observations per batch. It includes proposal encoding; output goes
to `io.Discard`. The journal case additionally reopens its private file and
performs a real append plus `fsync` for every event.

| Command path | Time per observation | Heap allocation per 32-observation batch |
| --- | ---: | ---: |
| Default shadow | 4.048–4.155 µs | 158,413–158,421 B; 1,037 allocs |
| Private append + `fsync` journal | 3.770–3.827 ms | 222,704–223,171 B; 1,429–1,431 allocs |

These are local computation measurements, not end-to-end production claims.
They exclude collection, durable state, network latency, the PVE API, QEMU
configuration, and host read-back. CI runs a short benchmark smoke test but
does not impose unstable nanosecond thresholds on shared runners.

The journal measurement reflects one healthy local APFS filesystem. Sync
latency can be much worse during host storage pressure, so the journal stays
disabled by default and must not share the guarded storage domain. Batching or
asynchronous writes would weaken its current persist-before-output semantics and
are not represented here.

## ITOps replay export window

The internal ITOps Draft also has a pure builder that joins authorized
diskstats and management samples into a sanitized ReplayTrace. Its first
implementation repeatedly scanned the complete caller array for each interval;
the audited implementation now builds request-scoped, first-match indexes in
one pass and sorts only the qualifying wait series. Tests forbid global array
rescans and preserve the previous first-match behavior.

Five Node.js 22.21.1 Darwin/arm64 runs on the same local machine timed only the
in-memory export after deterministic fixture generation:

| Window | Input samples | Output intervals | Export range | Retained heap delta |
| --- | ---: | ---: | ---: | ---: |
| 1 day | 10,080 | 1,440 | 7.164–7.563 ms | 354,400–355,776 B |
| 7 days | 70,560 | 10,080 | 42.294–46.537 ms | 1,838,536–1,838,680 B |
| 14 days | 141,120 | 20,160 | 80.472–82.093 ms | 3,583,840 B |

Each repeated scale produced an identical SHA-256 output checksum and retained
the fixed average/derived block-device semantics. The heap value is measured
after explicit garbage collection while the output remains live; it is not a
peak-allocation number. These figures exclude sample generation, the probe,
SSH, PVE REST, SQLite, network, dashboards, and real storage behavior.

Before internal PR #37 merged, internal CI run 161
independently ran the one-day smoke: the complete Node quality gate passed in
4m34s and the dependent linux/amd64 image build passed in 4m45s. Nothing was
deployed by that run; the integration later merged after explicit approval and
remains undeployed.

```sh
# In the reviewed internal ITOps Draft branch
npm --prefix backend run benchmark:storage-replay
```

```sh
go test -run '^$' -bench . -benchtime=500ms -benchmem -count=5 \
  ./internal/policy ./internal/allocator ./internal/safety \
  ./cmd/pve-storage-guard
```

Restricted-probe/SSH execution, the PVE REST adapter, persistent ITOps, real
host storage, and controlled-load latency remain promotion gates.
