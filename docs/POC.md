# Offline proof of concept

## Purpose

The PoC answers two questions before any production mutation:

1. Which simple policy best balances storage safety, admitted bulk work, job
   completion, and configuration churn?
2. Can the decision process be reproduced without PVE, SSH, a database, or an
   actuator?

## Available historical evidence

The reference incident currently provides:

| Signal | Resolution | Window | Treatment |
| --- | ---: | ---: | --- |
| VM write-counter-derived rates | about 60 s | 25 min | observed |
| attributed bulk-disk burst | 18 s | one burst | observed |
| ZFS pool write-wait | 1 s | 12 samples | observed |
| durable-service sync latency | event samples | two events | qualitative correlation only |
| incident ordering markers | event timestamps | three markers | observed summary |
| fixed-20 natural-load check | aggregate | 60 samples | observed summary; non-replayable |
| historical PSI, queue depth, IOPS | unavailable | unavailable | evidence gap |
| historical SSH/API probes | unavailable | unavailable | evidence gap |

The demand trace expands to 1,500 one-second samples and inserts the exact
18-second attributed burst. Interpolation retains each counter-derived rate for
its preceding interval. It does not pretend that one-minute counters were
originally sampled at one second.

### Evidence audit and statistic compatibility

A 2026-08-02 local evidence audit found two other retained storage windows, but
only aggregate summaries survived: one 30-sample quiet-period gate and one
60-sample post-change check. In the latter, a fixed 20 MiB/s cap still had 22
samples above 25 ms and a 234.065464 ms p99 under natural load. Controlled load
was not started; the cap was rejected and rolled back. The per-sample series are
unavailable, both windows are from the same operational episode, and neither
may be replayed or counted as an independent trace.

The twelve incident values are ZFS write total-wait samples. The PoC computes a
p95 across those sampled values for controller windows; that is not a true p95
of individual I/O latency. It is valid as an explicitly labeled pressure-proxy
replay, but it cannot calibrate the online API's p95 thresholds. ADR-0003 and
the [replay trace contract](TRACE-CONTRACT.md) make this distinction a
machine-tested promotion gate.

## Sanitization

Public fixtures replace node, pool, VM, disk, and workload names with semantic
aliases. They contain numeric samples, UTC-relative timestamps, units, and
derivations only. Raw logs, internal addresses, serials, UUIDs, MAC addresses,
credentials, guest content, and internal domains are excluded.

## Validation lanes

### Observed shadow replay

Policies consume the exact twelve observed wait values. Decisions do not modify
the captured series. This lane measures detection time, proposed limits, reason
codes, and churn only. Its signal statistic is ZFS total-wait, not a production
I/O p95.

The retained write-wait collection began 53 seconds after the retained
management-failure marker. A two-consecutive-sample 25 ms pressure signature is
present one second after collection begins, and the first 100 ms sample appears
at offset two seconds. Therefore the history proves rapid recognition once
telemetry exists, but it does **not** prove advance warning. Historical PSI,
queue, and management-probe series are missing, so multi-signal corroboration
is also not measurable.

### Counterfactual sensitivity replay

Historical controlled demand is capped by each proposed budget and passed
through a documented monotonic pool model. Three scenarios vary safe capacity,
baseline latency, and overload slope:

| Model | Base wait | Safe capacity | Overload slope |
| --- | ---: | ---: | ---: |
| conservative | 12 ms | 15 MiB/s | 1.2 ms per MiB/s |
| nominal | 10 ms | 20 MiB/s | 0.9 ms per MiB/s |
| optimistic | 8 ms | 30 MiB/s | 0.6 ms per MiB/s |

These are sensitivity models, not fitted storage physics or production
measurements.

## Strategies

- `no_limit`: admission baseline, effectively unbounded.
- `fixed_20`: model comparator; field evidence rejects treating it as a
  validated production fallback.
- `step_5_10_40`: threshold table with cooldown.
- bounded AIMD variants, including a safety-constrained grid-search candidate.

The bounded search evaluates 972 policies. A candidate is eligible only if its
unsafe seconds are no greater than `fixed_20` in every model. Among eligible
policies, aggregate admitted bytes are maximized; severe time, recovery, churn,
and conservative bounds break ties.

## Evaluation metrics

- seconds above 25 ms (`unsafe_seconds`);
- seconds at/above 100 ms (`severe_seconds`);
- recovery time to a stable safe window;
- admitted and demanded MiB, admission percentage;
- estimated completion time for controlled demand;
- limit changes and change rate;
- observed/shadow decision latency;
- when available in future traces: p50/p95/p99 latency, IOPS, throughput, queue,
  PSI, SSH/API probe success and latency, and task completion.

Ranking is safety-first and lexicographic. More throughput cannot compensate for
a safety regression.

## Current result

The original offline run evaluated 972 AIMD policies; 198 passed the cross-model
safety gate. The selected shadow candidate is:

| Parameter | Value |
| --- | ---: |
| Minimum / initial / maximum | 5 / 20 / 25 MiB/s |
| Healthy / target / emergency wait | 15 / 25 / 100 ms |
| Additive increase | 0.5 MiB/s |
| Multiplicative decrease | 0.5 |
| Healthy / breach windows | 12 / 2 |
| Control interval / cooldown | 10 / 60 s |

Compared with fixed 20 in the models, admitted demand changed from 59.26% to
60.11% (conservative), 63.73% (nominal), and 63.88% (optimistic). Unsafe seconds
were respectively 1, 0, and 0 for both the selected candidate and fixed-20.
The threshold-step baseline produced 158 unsafe seconds in the conservative
model and is rejected.

These are model-assisted estimates from one incident. They support only this
conclusion: slow bounded adaptation is worth further shadow validation. They do
not prove active production prevention or a universal performance gain.

The separate observed fixed-cap check overrides any stronger operational claim:
20 MiB/s was insufficient in a later natural-load baseline on the same storage
episode and was rolled back. This does not invalidate its use as a deterministic
counterfactual comparator; it does block using it as a universal fallback or
promotion gate by itself.

### Parameter sensitivity

The reproducible report also varies six parameters one at a time around the
selected policy: maximum budget, additive increase, multiplicative decrease,
healthy windows, breach windows, and cooldown. Each neighbor is evaluated across
all three pool models with the same fixed-20 safety gate. This detects whether a
recommendation is isolated and brittle; it does not replace independent traces
or storage-class validation.

## Reproduction

```sh
python3 -m unittest discover -s poc -p 'test_*.py' -v
python3 poc/simulate.py --format markdown
python3 poc/simulate.py --format json
python3 poc/trace_contract.py candidate.json --reference-group reference-incident
```

Reviewed output snapshots will be stored under `poc/results/`. CI must regenerate
them and fail on unexplained differences.

## Promotion gates

- Reproduced tests and reports from anonymized fixtures.
- Parameter-neighbor sensitivity and at least one independent trace that passes
  the trace contract's provenance, completeness, independence, and statistic
  compatibility gates.
- Several weeks of 1 Hz latency/PSI/queue and management probe history.
- Controlled non-critical disk load test.
- Restart, stale metric, lease conflict, apply/read-back mismatch, and rollback
  fault injection.
- Existing storage soak gate and operator review.

Until all gates pass, `aimd_poc_tuned` stays shadow-only. `fixed_20` remains a
model comparator only; any canary fallback must be calibrated and approved for
the specific storage domain.
