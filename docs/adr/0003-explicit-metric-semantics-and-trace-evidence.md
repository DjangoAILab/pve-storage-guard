# ADR-0003: Preserve metric semantics and qualify replay evidence

## Status

Accepted — 2026-08-02

## Context

The reference incident retained one-second ZFS write total-wait samples. The
offline simulator takes a p95 across a control window of those samples. This is
not the same statistic as a true p95 over individual I/O latency observations.
ITOps can currently derive average service time from diskstats, but diskstats
also cannot produce a p95.

Without an explicit contract, a useful pressure proxy could be accidentally
renamed as p95 telemetry and its thresholds promoted beyond what the evidence
supports. Similarly, synthetic data or another window from the same incident
could be described as independent validation.

## Decision

- Every replay trace declares source kind, independence group, storage/workload
  class, sampling interval, full window duration, wait statistic, window
  aggregation, and provenance. Completeness includes leading/trailing gaps.
- The current controller observation remains a true p95 contract. Only matching
  p95 traces are eligible to validate its thresholds.
- Average and total-wait signals may support detection and proxy replay, but
  must retain their labels and cannot qualify as production p95 calibration.
- Independent evidence must be observed, from a different independence group,
  sufficiently complete, sanitized, machine-assessed, and supported by a human
  provenance/redaction review.
- Synthetic and modeled traces remain first-class test inputs but never count
  as observed production evidence.

## Consequences

- The current historical PoC supports a shadow-policy direction, not direct
  production threshold calibration.
- ITOps must preserve metric statistic and provenance during export.
- A future policy may support additional explicitly typed pressure signals, but
  doing so requires separate thresholds and validation rather than an alias.
- Independent trace collection remains an open promotion gate.

## Alternatives

- **Treat window-p95 total-wait as I/O p95:** rejected as statistically false.
- **Use diskstats average as p95:** rejected because cumulative counters cannot
  recover a latency distribution.
- **Accept any second trace as independent:** rejected because same-source or
  synthetic traces do not test generalization.
