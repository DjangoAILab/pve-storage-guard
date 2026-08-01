---
title: Offline PoC
description: Historical evidence, counterfactual models, strategy results, and limits.
---

## Evidence lanes

**Observed shadow replay** consumes the exact retained write-wait series and
records controller decisions without changing history.

**Counterfactual sensitivity replay** caps derived demand by each proposed
budget and evaluates conservative, nominal, and optimistic monotonic pool
models. These outcomes are estimates, not production measurements.

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

The recommendation is **shadow-only**. Fixed 20 MiB/s remains the fallback.
