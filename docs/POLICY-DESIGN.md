# Policy design

## Goals

The policy protects a storage-domain latency SLO while admitting as much bulk
work as the evidence safely permits. It must be simple enough to replay,
explain, bound, and operate without an online optimizer.

## Policy scopes and precedence

Policy uses three scopes:

1. exact disk override;
2. workload-class policy;
3. storage-domain default.

The resolved policy is immutable for a decision and includes its version. A
resource is controlled only when its exact adapter identity is enrolled. Names
and heuristics never implicitly enroll a disk. Root, boot, quorum, database,
and other critical disks default to excluded.

## Storage-domain policy

```yaml
apiVersion: guard.storage-slo.io/v1alpha1
kind: StorageDomainPolicy
metadata:
  name: bulk-pool-default
spec:
  mode: shadow
  controlInterval: 10s
  cooldown: 60s
  telemetryMaxAge: 5s
  latency:
    healthyP95: 15ms
    targetP95: 25ms
    emergency: 100ms
  budget:
    minimum: 5MiB/s
    initial: 20MiB/s
    maximum: 25MiB/s
  aimd:
    additiveIncrease: 0.5MiB/s
    multiplicativeDecrease: 0.5
    healthyWindows: 12
    breachWindows: 2
```

These values are the current PoC shadow candidate, not a universal production
default. A storage-domain policy must record storage class, observed baseline,
benchmark evidence, and owner before it may enter canary mode.

## Disk enrollment

```yaml
apiVersion: guard.storage-slo.io/v1alpha1
kind: DiskEnrollment
metadata:
  name: bulk-import-data
spec:
  storageDomain: reference-node/bulk-pool
  resource: adapter-opaque-disk-key
  workloadClass: bulk-hdd
  criticality: non-critical
  envelope:
    minimum: 5MiB/s
    maximum: 40MiB/s
    weight: 1
```

The storage-domain budget is authoritative for shared safety. Disk envelopes
protect individual workloads and express allocation policy. A disk maximum does
not reserve or force throughput.

PVE enrollment has a separate live evidence gate before controlled load. The
owner-only binding names one exact QEMU disk, but the public assessment contains
no PVE identity. The read-only preflight requires both `non-critical` and
`pve-storage-guard` tags on the live workload, an unlocked guest, an active
bound ZFS storage, a writable data disk excluded from the explicit boot order,
and a static rollback value inside the approved envelope. A name, a manually
typed `criticality` field, or a passing historical replay cannot replace these
checks. Passing preflight still reports `activeControlEligible=false`.

## Allocator

Given aggregate budget `B` and enrolled active disks `i`:

1. Grant each disk its minimum `min_i`.
2. If `sum(min_i) > B`, return `infeasible`; retain explicit minima in the
   proposal, block new bulk admissions, and alert. Never invent an unsafe split.
3. Distribute remaining budget by positive weight among unsaturated disks.
4. Clamp each allocation at `max_i` and redistribute unused shares.
5. Sort by stable opaque disk key so results are deterministic.

## Bounded AIMD state machine

Every control interval uses the p95 write-wait observation for the storage
domain plus metric freshness and management-plane health:

This is a typed statistic contract, not a generic name for any latency-like
signal. Average service time or ZFS total-wait requires a separately calibrated
policy and cannot be substituted for p95. See ADR-0003 and the replay trace
contract.

- `emergency` or latency >= 100 ms: immediately propose the minimum budget;
- latency > 25 ms for two consecutive windows: multiply budget by 0.5;
- latency < 15 ms for twelve consecutive windows: add 0.5 MiB/s;
- between thresholds: hold;
- stale, missing, invalid, or conflicting observations: hold and prohibit an
  increase;
- management-plane critical signal: decrease or hold according to the reviewed
  policy, never increase;
- apply at most once per 60-second cooldown, except emergency decrease;
- clamp every result to the storage-domain and disk envelopes.

The 120-second healthy period is deliberately asymmetric: increases are slow,
decreases are fast. A minimum material change avoids write churn caused by unit
rounding.

## Modes

- `observer`: collect and evaluate detection signals; no desired limits.
- `shadow`: emit desired budgets/allocations and explanations; no mutation.
- `canary`: apply only to exact approved non-critical enrollments and policy
  version, with expiry and rollback state.
- `enforce`: future mode, gated per storage domain after canary evidence.

Mode cannot be promoted by the adaptive algorithm. Promotion is an operator
configuration change with review and audit.

## Safety invariants

1. No proposal exceeds resolved hard bounds.
2. Stale or invalid telemetry never causes an increase.
3. At most one authoritative lease owner and fencing generation writes a
   storage domain; a caller-presented lease is verified against the current
   lease before the actuator boundary.
4. A request is bounded by an authoritative, unexpired approval for the exact
   domain, policy version, resource, and limit envelope; a non-empty approval
   ID alone is never authorization.
5. Unknown effective state blocks actuation.
6. Policy/inventory drift freezes the affected disk.
7. Apply/read-back mismatch attempts only a pre-authorized restoration of the
   last verified state, then stops the resource controller.
8. Controller failure does not interrupt host management or remove the last
   verified throttle.
9. The actuator cannot change VM lifecycle, storage topology, or filesystems.
10. Decisions are deterministic for the same input, state, and policy version.

## Decision record

Each evaluation emits:

- event ID, time, controller and lease identity;
- storage-domain and anonymizable opaque resource keys;
- mode and immutable policy version;
- input observation IDs, values, freshness, and management health;
- previous budget, proposed budget, allocations, and reason code;
- safety gate outcomes;
- previous effective, desired, read-back effective, and rollback state;
- outcome, latency, and linked ITOps incident if any.

Raw guest data, secrets, storage serials, network addresses, and unrestricted
command output are never decision fields.

## Calibration and defaults

The first version uses operator-defined storage classes and bounded offline
search. It does not learn online. A proposed default must:

- match or beat the fixed-20 unsafe-second count in every validation model;
- not increase severe latency or management-plane failures;
- show tolerable limit-change churn;
- remain stable under parameter-neighbor sensitivity;
- pass an independent trace and controlled load test.

The 20 MiB/s comparator is not a production fallback: a same-episode observed
natural-load check exceeded the latency gate in 22 of 60 samples and was rolled
back. If promotion gates fail, remain in shadow mode and derive a reviewed
storage-domain-specific static rollback limit; do not promote a visually
appealing throughput result or assume that tightening one disk alone protects a
shared domain.
