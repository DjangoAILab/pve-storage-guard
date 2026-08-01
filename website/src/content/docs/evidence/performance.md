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

```sh
go test -run '^$' -bench . -benchtime=500ms -benchmem -count=5 \
  ./internal/policy ./internal/allocator ./internal/safety \
  ./cmd/pve-storage-guard
```

Collector, adapter, persistent ITOps, and controlled-host latency remain
promotion gates.
