---
title: Offline PoC
description: Historical evidence, counterfactual models, strategy results, and limits.
---

## Evidence lanes

**Observed shadow replay** consumes the exact retained write-wait series and
records controller decisions without changing history. The incident series is
ZFS write total-wait; taking p95 across its samples does not turn it into a true
I/O latency p95. It is pressure-proxy evidence, not production p95 calibration.

**Counterfactual sensitivity replay** caps derived demand by each proposed
budget and evaluates conservative, nominal, and optimistic monotonic pool
models. These outcomes are estimates, not production measurements.

**Incident evidence assessment** keeps event ordering and aggregate field
checks outside the replay lane. Retained write-wait collection started 53
seconds after the management-failure marker. The pressure signature is visible
one second after collection starts, but advance warning is not proven.

## Result

| Scenario | Fixed 20 admission | Selected AIMD admission | Unsafe seconds (both) |
| --- | ---: | ---: | ---: |
| Conservative | 59.26% | 60.11% | 1 |
| Nominal | 59.26% | 63.73% | 0 |
| Optimistic | 59.26% | 63.88% | 0 |

The threshold-step controller caused 158 unsafe seconds in the conservative
model and is rejected. Of 972 searched AIMD policies, 198 passed the cross-model
safety constraint.

## Evidence gaps

The historical store did not retain one-second PSI, queue, IOPS, or SSH/API
probes. The twelve write-wait samples are an incident window, not a baseline.
One incident cannot establish defaults for SSD, HDD, Ceph, or network storage.
Two other local windows survive only as same-episode aggregate summaries, so
they cannot be replayed or counted as an independent trace. The replay trace
contract keeps source kind, statistic, completeness, and independence explicit.

A later fixed-20 natural-load check had 22 of 60 samples above 25 ms and a
234.065464 ms p99. Controlled load was not started; the cap was rejected and
rolled back. The recommendation is **shadow-only**. Fixed 20 MiB/s is a model
comparator, not a validated production fallback.
