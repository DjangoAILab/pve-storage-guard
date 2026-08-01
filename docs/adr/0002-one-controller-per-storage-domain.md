# ADR-0002: One bounded controller per storage domain

## Status

Accepted — 2026-08-01

## Context

Several virtual disks can share one physical storage bottleneck. Independent
controllers see the same pressure but do not coordinate their increases and
decreases. A global host limit is also incorrect when independent pools have
different media, capacity, and workloads.

## Decision

Use one single-writer controller actor per storage domain. It adjusts a bounded
aggregate budget from storage-domain feedback. A deterministic allocator grants
explicitly enrolled disks their minima, then distributes remaining capacity by
weight without exceeding disk maxima.

Policy resolution order is exact disk override, workload class, then storage
domain default. Root and critical disks are excluded unless explicitly enrolled.

## Consequences

- Each shared bottleneck has one feedback loop and independent SLO/limits.
- Per-disk differences are envelopes and weights, not separate algorithms.
- Inventory must correctly map disks to storage domains.
- Infeasible minimums are a hard configuration error and block new bulk
  admissions; the allocator does not silently violate them.

## Alternatives

- **One controller per disk:** rejected due to uncoordinated oscillation.
- **One controller per host:** rejected because storage domains may be
  independent.
- **Static limits only:** retained as a baseline and fallback, but not selected
  as the sole policy.
- **PID or predictive ML:** deferred; current evidence is sparse and bounded
  AIMD is easier to explain, validate, and fail safely.
