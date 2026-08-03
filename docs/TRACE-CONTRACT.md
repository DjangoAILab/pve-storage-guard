# Replay trace contract

## Purpose

The replay contract prevents synthetic, modeled, aggregated, or same-incident
data from being presented as independent observed evidence. Its machine-readable
schemas are `poc/schema/replay-trace.schema.json` for v1alpha1 and
`poc/schema/replay-trace-v1alpha2.schema.json` for v1alpha2; the dependency-free
validator and evidence assessor is `poc/trace_contract.py`.

Structural validity, policy-signal compatibility, and evidence independence are
three separate results. A trace may be valid and useful for tests without being
eligible to validate a production policy.

## Required semantics

Every trace declares:

- an opaque name and independence group;
- `observed`, `synthetic`, or `modeled` provenance;
- storage and workload classes;
- sample interval, full window duration, and sanitized status;
- whether write-wait is a real p95, an average, or ZFS total-wait;
- whether write-wait was measured at the storage-domain, block-device,
  virtual-disk, or application layer;
- whether another p95 aggregation is applied across the control window;
- monotonically increasing relative offsets, not host timestamps or identities.

New exports use v1alpha2, whose management status is `healthy`, `unhealthy`, or
`unknown`. The v1alpha1 boolean remains readable for diagnostics but cannot pass
the machine promotion gate. Structural
sample completeness, valid-write-wait completeness, and known-management
completeness are reported separately; invalid placeholders and unknown
management data cannot satisfy the promotion gate.

The current controller API consumes a storage-domain p95 observation. Only a
v1alpha2 trace declaring `writeWaitStatistic=p95`,
`writeWaitMeasurementLayer=storage-domain`, and
`controlWindowAggregation=none` is signal-compatible with that API. Average
diskstats service time and ZFS total-wait remain valuable
detection evidence, but they must not be relabeled as an I/O latency p95.

## Independent-trace qualification

The current automated gate requires all of the following:

- source kind is `observed` and sanitization is affirmed;
- independence group differs from the reference incident;
- at least 600 declared window seconds with at least 95% structural,
  valid-write-wait, and known-management completeness, including leading and
  trailing gaps;
- a known storage class and workload class;
- a policy-compatible storage-domain p95 signal;
- no structural validation error.

This is a minimum evidence gate, not proof of generality. Promotion still needs
coverage across quiet and busy windows, more than one workload shape, and the
storage classes for which defaults will be published.

Storage-only public traces may be processed in a separate research lane. They
must declare management status `unknown` and remain ineligible for active-control
promotion. See [external trace research](EXTERNAL-TRACE-RESEARCH.md).

Observed sources with no portable latency field use the separate
`WorkloadShapeTrace` contract instead of inventing a wait statistic. That
contract retains only interval IOPS and throughput and is categorically
ineligible for active control. Its v1alpha2 form distinguishes source issue
time from service-arrival time and requires a storage class: `unknown` when the
source has no supported claim, or an explicit logical class such as
`network-block`. A known class can close only the separate storage-class
research gate. The accepted UMass/SPC and Alibaba research artifacts and their
attribution are documented in
[external trace research](EXTERNAL-TRACE-RESEARCH.md).

## Privacy boundary

Public traces may contain relative offsets, numeric metrics, coarse storage and
workload classes, and derivation notes. They must not contain hostnames, VM/CT
IDs, IP/MAC addresses, pool/device names, serials, UUIDs, tenant names, raw log
messages, credentials, or guest content. The private source and sanitized export
must have separate hashes and a human redaction review before publication.

## ITOps export mapping

The internal ITOps draft contains a pure builder for mapping a reviewed,
already-authorized metric window into this contract. It fixes diskstats-derived
wait to `average`, provenance to `derived`, and window aggregation to `none`;
its measurement layer is fixed to `block-device`. Callers cannot configure
those fields as a storage-domain `p95`. A real histogram/telemetry
percentile would require a separate, explicit source mapping. Missing wait
samples remain gaps; a present wait sample without a matching management
observation is retained with v1alpha2 status `unknown`. Neither condition is
forward-filled. Offsets are relative to the declared window start; completeness
is never recomputed from only the first and last surviving samples. The builder has no route, file
writer, or publication side effect, so human source review and export approval
remain separate gates.

Validate a candidate with:

```sh
python3 poc/trace_contract.py candidate.json \
  --reference-group reference-incident
```

An exit code of zero means the document is structurally valid. The JSON output's
`meets_machine_independence_gate` field is a necessary machine check, not proof
that the declared provenance is true. Source hashing, redaction review, and
human evidence approval are still required before a Checklist item can close.
