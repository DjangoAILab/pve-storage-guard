# Replay trace contract

## Purpose

The replay contract prevents synthetic, modeled, aggregated, or same-incident
data from being presented as independent observed evidence. Its machine-readable
schema is `poc/schema/replay-trace.schema.json`; the dependency-free validator
and evidence assessor is `poc/trace_contract.py`.

Structural validity, policy-signal compatibility, and evidence independence are
three separate results. A trace may be valid and useful for tests without being
eligible to validate a production policy.

## Required semantics

Every trace declares:

- an opaque name and independence group;
- `observed`, `synthetic`, or `modeled` provenance;
- storage and workload classes;
- sample interval and sanitized status;
- whether write-wait is a real p95, an average, or ZFS total-wait;
- whether another p95 aggregation is applied across the control window;
- monotonically increasing relative offsets, not host timestamps or identities.

The current controller API consumes a p95 observation. Only a trace declaring
`writeWaitStatistic=p95` and `controlWindowAggregation=none` is signal-compatible
with that API. Average diskstats service time and ZFS total-wait remain valuable
detection evidence, but they must not be relabeled as an I/O latency p95.

## Independent-trace qualification

The current automated gate requires all of the following:

- source kind is `observed` and sanitization is affirmed;
- independence group differs from the reference incident;
- at least 600 seconds with at least 95% sample completeness;
- a known storage class and workload class;
- a policy-compatible p95 signal;
- no structural validation error.

This is a minimum evidence gate, not proof of generality. Promotion still needs
coverage across quiet and busy windows, more than one workload shape, and the
storage classes for which defaults will be published.

## Privacy boundary

Public traces may contain relative offsets, numeric metrics, coarse storage and
workload classes, and derivation notes. They must not contain hostnames, VM/CT
IDs, IP/MAC addresses, pool/device names, serials, UUIDs, tenant names, raw log
messages, credentials, or guest content. The private source and sanitized export
must have separate hashes and a human redaction review before publication.

## ITOps export mapping

ITOps may export a reviewed window into this contract after collection. It must
preserve the original statistic and provenance. Diskstats-derived average wait
maps to `average`; a real histogram/telemetry percentile maps to `p95`. Missing
samples remain gaps and affect completeness rather than being forward-filled.

Validate a candidate with:

```sh
python3 poc/trace_contract.py candidate.json \
  --reference-group reference-incident
```

An exit code of zero means the document is structurally valid. The JSON output's
`meets_machine_independence_gate` field is a necessary machine check, not proof
that the declared provenance is true. Source hashing, redaction review, and
human evidence approval are still required before a Checklist item can close.
