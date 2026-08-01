# ITOps integration

## Objective

ITOps provides durable observability, alert evaluation, incident linkage, and
approved task execution. PVE Storage Guard owns storage-control state and
decisions. The PVE probe remains read-only; mutation, if later enabled, uses a
separate structured capability.

## Ownership boundary

| Capability | Owner |
| --- | --- |
| Raw/normalized observations | ITOps observability + PVE collector |
| PVE inventory and disk/storage mapping | PVE Adapter |
| Policy resolution and pool actor | PVE Storage Guard |
| Approval envelope and task handoff | ITOps |
| Structured apply and effective read-back | PVE actuator |
| Decision journal | PVE Storage Guard, linked from ITOps |
| Alerts, incidents, escalation | ITOps |

ITOps does not reproduce the AIMD state machine in its generic threshold engine.
Generic thresholds are stateless sample evaluation; the controller needs durable
cooldown, desired/effective state, leases, and policy versions.

## Metrics contract

Recommended Prometheus-style names (labels are bounded and use opaque IDs):

### Storage and management plane

- `pve_storage_guard_pool_write_wait_seconds{domain,quantile}`
- `pve_storage_guard_pool_iops{domain,operation}`
- `pve_storage_guard_pool_throughput_bytes_per_second{domain,operation}`
- `pve_storage_guard_pool_queue_depth{domain}`
- `pve_storage_guard_io_psi_ratio{node,window}`
- `pve_storage_guard_management_probe_success{node,probe}`
- `pve_storage_guard_management_probe_duration_seconds{node,probe}`
- `pve_storage_guard_disk_throughput_bytes_per_second{domain,resource,operation}`
- `pve_storage_guard_disk_demand_share_ratio{domain,resource}`

### Controller and actuation

- `pve_storage_guard_observation_age_seconds{domain}`
- `pve_storage_guard_controller_up{domain}`
- `pve_storage_guard_controller_lease{domain}`
- `pve_storage_guard_budget_bytes_per_second{domain,state}` where `state` is
  `previous`, `desired`, or `effective`.
- `pve_storage_guard_allocation_bytes_per_second{domain,resource,state}`
- `pve_storage_guard_decisions_total{domain,mode,reason,outcome}`
- `pve_storage_guard_actuation_duration_seconds{domain,outcome}`
- `pve_storage_guard_readback_mismatch{domain,resource}`
- `pve_storage_guard_policy_info{domain,version,mode}` with constant value 1.

Do not use VM names, guest names, paths, task IDs, event IDs, or raw device names
as metric labels. Those belong in structured events to avoid sensitive exposure
and cardinality growth.

## Structured decision event

```json
{
  "schemaVersion": "guard.storage-slo.io/v1alpha1",
  "eventId": "event-0123456789abcdef01234567",
  "eventType": "storage_guard.decision",
  "recordedAt": "2026-08-02T01:02:03Z",
  "domainKey": "opaque-domain-key",
  "mode": "shadow",
  "policyVersion": "opaque-policy-version",
  "observation": {
    "id": "opaque-observation-id",
    "observedAt": "2026-08-02T01:02:01.8Z",
    "ageSeconds": 1.2,
    "writeWaitP95Milliseconds": 42.0,
    "waitValid": true,
    "emergency": false,
    "managementPlaneHealthy": true
  },
  "decision": {
    "proposalId": "proposal-0123456789abcdef01234567",
    "reason": "multiplicative_decrease",
    "previousBudgetMiBps": 20,
    "desiredBudgetMiBps": 10,
    "changed": true,
    "allocations": {"opaque-resource-key": 10},
    "allocationFeasible": true
  },
  "safety": {
    "allocationFeasible": true,
    "actuationAllowed": false,
    "leaseStatus": "not_evaluated",
    "approvalStatus": "not_evaluated",
    "effectiveStateStatus": "not_evaluated"
  },
  "outcome": "shadow_evaluated"
}
```

The public controller now implements this shadow event as an opt-in local JSONL
journal. It validates the event, appends it to a private regular file, syncs the
file, and only then emits the proposal. Journal failure suppresses the matching
proposal. This is a single-writer local handoff, not an ITOps ingestion route or
a production durability claim. Absolute event times and opaque internal keys
make the private journal unsuitable for public release without a separate
sanitization/export review.

## Alert model

### Informational

- Bulk disk share is high but pool latency and management probes remain healthy.
- Shadow controller proposes a change. Route to dashboard/event stream; no page.

### Warning

Trigger when storage latency breaches the target for two windows **and** at least
one corroborating signal is present: elevated PSI/queue, dominant enrolled disk,
or degraded management probe. Also warn on stale telemetry, lease loss, policy
infeasibility, or repeated shadow churn.

### Critical

Trigger immediately on emergency latency plus management-plane failure/critical
PSI, or on apply/read-back mismatch, rollback failure, conflicting writers, or
continued pressure at minimum budget. Critical response stops new bulk admission
and pages an operator; it does not stop guests automatically.

Use separate clear and hold durations, incident grouping by storage domain, and
reason-aware deduplication. A single throughput spike is not a page.

## Dashboard

One storage-domain view should align:

1. write-wait p50/p95/p99, queue, PSI, IOPS, throughput;
2. management probe success/latency;
3. per-disk demand rate and share;
4. previous/desired/effective budgets and allocations;
5. decisions, policy/mode changes, actuation outcomes;
6. workload task start/end and incident annotations.

The dashboard must label observed measurements separately from modeled replay.

## Runbook

### Diagnose

1. Confirm collector freshness and clock health.
2. Confirm affected storage domain and management probes.
3. Attribute dominant disk demand using the PVE Adapter mapping.
4. Inspect current policy, mode, lease owner, desired/effective state, and recent
   decisions.
5. Determine whether pressure persists at the minimum/fixed fallback.

### Stabilize

- Observer/shadow: stop or slow new bulk admission through the owning workload
  system; do not mutate PVE from the controller.
- Approved canary: use only the structured exact-resource limit inside the
  approved envelope, then verify effective state.
- On mismatch or drift: restore the last verified state if that restoration was
  pre-authorized, freeze the resource, and escalate.

### Recover and review

Verify management probes and latency remain healthy for a stable window, close
the incident with linked decision events, preserve sanitized derived evidence,
and update policy only through versioned review.

## Rollout

1. Add/retain 1 Hz metrics and management probes.
2. Offline replay in CI.
3. Local observer, then shadow with ITOps event ingestion.
4. Alert tuning across quiet and busy periods.
5. Non-critical controlled load and rollback rehearsal.
6. Explicitly approved, expiring one-disk canary.
7. Evidence review before any broader enforcement.

## Current implementation status

The internal ITOps draft PR adds one restricted, read-only operation:
`pve.storage-pressure`. It reads Linux PSI and diskstats pseudo-files only and
maps bounded node/disk metrics for the existing fast collection scope. Safe
counter deltas derive IOPS, throughput, average wait, queue depth, and
utilization; the PVE REST cluster-status probe supplies management success and
duration. It does not execute `zpool iostat`, expose hardware identifiers,
install the probe, arm alerts, or deploy any service.

ITOps currently permits a minimum fast interval of 10 seconds. This is suitable
for durable monitoring and alert correlation, but not for a one-second control
loop. The local PVE Storage Guard agent must own high-frequency sampling and
downsample observations/events into ITOps; ITOps must not become the actuator or
the source of controller timing.

Diskstats provides cumulative operation and time counters, not a p95 latency.
Average read/write service time is derived from successive deltas. A real p95
still requires an appropriate storage telemetry source and must not be
fabricated from these counters.

The draft integration also contains a pure replay-trace builder for reviewed,
already-authorized metric samples. It emits only relative offsets, numeric
evidence, and coarse storage/workload classes; internal target, resource, node,
and absolute sample-time fields are selection inputs and never become public
trace fields. Its wait semantics are fixed to `average`, aggregation to `none`,
and provenance to `derived`, so a caller cannot relabel diskstats evidence as
`p95`. Gaps remain gaps against the declared window. This builder has no route,
API, file writer, scheduler, or deployment path and therefore does not publish
data by itself.

The v1 detector emits a per-disk level only after wait telemetry exists.
Warning requires target wait plus PSI, queue, or management failure;
critical requires emergency wait plus full PSI or management failure. The
recommended warning/critical rules are seeded disabled. They remain disabled
until the integration is merged and a shadow baseline supports their thresholds.
The persisted SQLite integration test now proves that a two-sample warning
debounce survives evaluator restart and produces one firing followed by one
recovery, with the detector evidence retained. Real notification delivery is
still intentionally disabled.

As of 2026-08-02, the integration remains an internal draft PR and has not been
merged or deployed. Verification covers all 1,236 backend tests across 145
files, including 16 focused PVE/runtime tests, 11 restricted-probe tests, 15
focused threshold-evaluator tests, and four replay-export tests. TypeScript
build/lint and the dependency-boundary check also pass. The latest internal CI
quality-gate job passed in 4m32s, followed by its dependent linux/amd64
image-build job in 4m48s. Production probe installation, trace export,
notification delivery, and alert enablement remain explicit approval gates.

The draft also adds a PVE-only storage-pressure dashboard that combines PSI,
management-probe health, per-disk average wait, queue depth, utilization,
throughput, detector level, and alert-gate state. All 101 frontend tests plus
the production frontend build and lint pass. The UI labels detector output as
evidence rather than a controller decision and does not expose an actuation
action. A screenshot must come from a real shadow baseline, not fixture data.
