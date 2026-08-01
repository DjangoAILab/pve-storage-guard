---
title: Performance evidence
description: Reproducible microbenchmarks and their limits.
---

The core path has repeatable Go benchmarks for one policy observation,
allocation across 100 disks, and a verified no-change safety-gate pass with
in-memory fakes.

Five 500 ms runs on an Apple M4 Pro with Go 1.26.1 produced:

| Benchmark | Range | Heap allocation |
| --- | ---: | ---: |
| Policy observation | 25.55–25.81 ns/op | 0 B, 0 allocs/op |
| 100-disk allocation | 12.50–12.93 µs/op | 11,856 B, 9 allocs/op |
| Verified safety gate | 76.74–78.65 ns/op | 0 B, 0 allocs/op |

These are local computation measurements, not end-to-end production claims.
They exclude collection, durable state, network latency, the PVE API, QEMU
configuration, and host read-back. CI runs a short benchmark smoke test but
does not impose unstable nanosecond thresholds on shared runners.

```sh
go test -run '^$' -bench . -benchtime=500ms -benchmem -count=5 \
  ./internal/policy ./internal/allocator ./internal/safety
```

Controlled-load and end-to-end canary latency remain promotion gates.
