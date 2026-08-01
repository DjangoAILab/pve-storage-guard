# Architecture

## Architectural intent

PVE Storage Guard is a PVE-first product with a platform-neutral control core.
It protects a shared storage bottleneck without making the controller part of
the bottleneck or granting policy code host-level command execution.

```text
                 read-only                                      privileged
PVE inventory ─────────────┐                                  ┌──────────────┐
pool/disk metrics ─────────┼─> PVE Adapter / Collector        │ PVE Actuator │
management health ─────────┘              │                    └──────┬───────┘
                                           v                           ^
                                normalized observations               │
                                           │                           │
                                           v                           │
                              storage-slo-guard engine                 │
                              - pool actor                             │
                              - policy resolver                        │
                              - bounded allocator                      │
                                           │                           │
                                           v                           │
                                   Safety Controller                  │
                              proposal + approval gate ────────────────┘
                                           │
                                           v
                               Event / Telemetry / ITOps
```

## Runtime shape

The intended artifact is one Go binary, `pve-storage-guard`, with isolated
runtime modes:

- `agent`: local PVE inventory and metrics collection; a separately enabled,
  tightly constrained actuator surface.
- `controller`: unprivileged policy actors, allocation, safety checks, state,
  and decision journal; preferably outside the PVE storage failure domain.
- `replay`: deterministic offline replay and strategy comparison.
- `policy validate`: schema, bounds, enrollment, and feasibility validation.

The same binary simplifies packaging, but Unix identities, systemd units,
network listeners, and permissions keep read-only and privileged roles apart.

## Component boundaries

### PVE Adapter

Owns PVE-specific node, VM/CT, task, storage, and disk relationships. Converts
platform identities into internal opaque resource keys. It does not decide
limits. Discovery is read-only; actuation is a separate capability.

### Metrics Collector

Collects timestamped pool latency, IOPS, throughput, queue pressure, PSI, disk
health, and management-plane probes. It normalizes units and reports freshness.
It preserves each metric's statistic and provenance: diskstats average service
time and ZFS total-wait cannot be emitted as an I/O p95. It does not classify a
disk as safe to control.

### Policy Engine: `storage-slo-guard`

Owns immutable policy resolution, one actor per storage domain, bounded AIMD,
and deterministic budget allocation. Its input and output contain no PVE API
calls or shell commands.

### Actuator

Accepts only structured operations for an explicitly enrolled resource and a
validated policy envelope. It snapshots prior effective state, applies an
approved desired state, reads effective state back, and emits the result. It
cannot stop/restart guests or execute arbitrary commands.

### Safety Controller

Owns leases, policy version matching, min/max clamps, cooldown, timeouts,
freshness gates, desired/effective reconciliation, rollback, and failure
degradation. It is the only path from a proposal to an actuator request.

The code-level apply gate verifies a caller-presented lease against an
authoritative `LeaseVerifier` and resolves the approval ID through an
authoritative `ApprovalVerifier`; caller fields alone are never proof of
ownership or approval. The verified generation is carried as a fencing token
in the structured actuator request. A conflicting, expired, or unavailable
lease/approval is rejected before any effective-state read or apply.
Effective-state drift, an actuator error, or a read-back mismatch freezes that
resource without automatic retry. The gate is currently exercised only with
injected fakes and is not wired to a PVE command, listener, service, or
production configuration.

### Event and Telemetry

Exports metrics and append-only decision events with observation references,
policy version, previous/desired/effective state, reason, mode, and outcome.
It integrates with ITOps without making ITOps metrics ingestion depend on
actuation availability.

The current shadow CLI implements the first narrow slice: an opt-in local JSONL
journal. It writes a versioned `storage_guard.decision` event to a private
regular file, calls `fsync`, and only then emits the matching proposal to
stdout. Shadow records explicitly mark lease, approval, and effective state as
`not_evaluated`; they never imply checks that did not run. The CLI has no
journal by default, network exporter, rotation manager, or ITOps callback. A
non-blocking exclusive file lock enforces the single-writer contract. The
journal stops before 256 MiB and suppresses further proposals until an operator
rotates it, preventing an unbounded audit file from consuming the filesystem.

The matching read-only verifier is deliberately a sealed-file boundary, not a
live journal consumer. It takes a non-blocking shared lock, which fails while
the exclusive writer is active, and validates file safety, size, strict JSONL,
event/proposal linkage, observation age, and single-domain ownership. Its
versioned summary contains the exact raw-file SHA-256 digest, counts, and time
bounds but no domain, resource, observation, proposal, or event identities.

The separate `journal batch` command accepts that approved digest, an offset,
and a limit of at most 64. While holding the same lock, it scans and validates
the complete file, compares the digest, rechecks file identity, and only then
emits the requested private page. Paths are location hints, never authority.
The command has no credentials, network transport, database, approval lookup,
or log sink. Verification and paging do not rotate, persist, transmit,
deduplicate, reorder, or authenticate provenance; see ADR 0005.

## Controller boundary: storage domain first

The primary control key is `(adapter, node, storage-domain)`, not VM or disk.
All controlled disks sharing the bottleneck use one aggregate budget. A
deterministic allocator distributes the budget within per-disk envelopes.

This avoids synchronized independent loops that all increase after the same
healthy sample or all decrease after the same pressure signal. Separate
physical pools or independently provisioned storage domains may use different
controllers, SLOs, capacity baselines, and limit ranges.

## Data flow and decision lifecycle

1. Collector emits a timestamped normalized observation.
2. Adapter inventory resolves explicit enrollments and storage-domain members.
3. The pool actor acquires/renews its single-writer lease and resolves the
   immutable policy version.
4. The policy engine proposes an aggregate budget and disk allocation.
5. Safety Controller validates freshness, bounds, cooldown, policy feasibility,
   enrollment, desired/effective state, and operating mode.
6. In observer/shadow mode, an explicitly configured journal is synced before
   the proposal is emitted; neither path can authorize an apply.
7. After the writer is stopped or detached, an operator may rotate and verify a
   sealed journal. ITOps approval binds its immutable digest, expected counts,
   target, domain, policy version, expiry, and resource mappings; an authorized
   local capability may then request bounded pages. Active files are never
   tailed, and mutable paths are never approval identities.
8. In approved canary mode, the constrained actuator snapshots, applies, and
   reads back effective state.
9. Telemetry records the full outcome and ITOps evaluates multi-signal alerts.

## Deployment modes

### Standalone development

Collector, controller, and replay can run locally with fixture or read-only
inputs. No actuator is enabled.

### Same-host PVE observer

Separate system users/services run collector and controller. The controller has
no mutation permission. This mode validates discovery, timing, and telemetry.

### Split production topology

The controller and journal run outside the PVE storage failure domain. A small
agent on PVE exposes read-only observations and, only when explicitly enabled,
an authenticated allowlisted actuator. Network loss never causes an increase.

## State and consistency

- One authoritative lease owner and generation per storage domain may apply
  changes; the generation is carried to the actuator as a fencing token.
- Policy versions are immutable inputs to decisions.
- Desired and effective values are distinct and survive restart.
- On restart, the controller reads effective state before calculating a new
  proposal.
- Restored controller state is accepted only when its budget remains inside the
  active policy, healthy/breach counters are mutually exclusive and below their
  trigger windows, and the prior cooldown is preserved. Invalid state fails
  closed; emergency decrease remains allowed.
- Unknown effective state, inventory drift, hot-plug ambiguity, policy
  mismatch, actuator failure, or read-back mismatch freezes that resource and
  alerts. Automatic retry is prohibited while frozen.
- The last verified effective limit remains in force if the controller fails;
  optional expiry behavior must be explicit per policy, never assumed.

## Security boundary

The agent is not a general remote shell. The actuation API accepts a resource
key, validated limit fields, policy/version/approval identifiers, and expiry.
Authentication, replay protection, least-privilege local permissions, audit,
and response read-back are mandatory before production use.

## Repository layout

```text
cmd/pve-storage-guard/      CLI entrypoint
api/v1/                    versioned observation and actuation contracts
internal/policy/           storage-slo-guard engine
internal/controller/       per-pool actor and durable state
internal/allocator/        bounded disk allocation
internal/collector/        platform-neutral collector contracts
internal/adapter/pve/      PVE discovery and identity mapping
internal/actuator/pve/     constrained PVE actuation
internal/safety/           gates, reconciliation, rollback
internal/telemetry/        metrics and decision events
poc/                       offline reference replay and fixtures
deploy/                    systemd, container, Prometheus, Grafana
integrations/itops/        ITOps contracts and examples
docs/                      product, operations, security, ADRs
website/                   GitHub Pages source
```

## Quality attributes

- **Safety:** invalid/stale input cannot increase limits; all outputs are
  bounded and reversible.
- **Availability:** collection and management probes remain independent of the
  controller and actuator.
- **Explainability:** deterministic decisions with reason codes and evidence.
- **Portability:** adapters own platform semantics; the engine owns generic
  storage-domain control.
- **Operability:** Prometheus metrics, structured logs, runbooks, and replay.
- **Performance:** control work is small and periodic; cardinality is bounded by
  explicitly enrolled resources.
