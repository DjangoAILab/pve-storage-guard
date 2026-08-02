---
title: ITOps integration
description: Metrics, decision events, alerts, dashboards, and runbook ownership.
---

ITOps owns durable observability, alert evaluation, incident linkage, and
approved task handoff. PVE Storage Guard owns policy state and decisions. The
existing PVE probe remains read-only.

## Minimum signals

- storage-domain write-wait p95/p99, IOPS, throughput, and queue;
- node I/O PSI;
- per-disk write rate and dominant share;
- SSH and PVE API probe success/latency;
- observation age, controller lease, mode, policy version;
- previous, desired, and effective budgets;
- decision reason and apply/read-back outcome.

## Alert semantics

A throughput spike alone is informational. Warning requires a latency target
breach plus pressure, dominance, or management degradation. Emergency latency
plus management failure, read-back mismatch, conflicting writers, rollback
failure, or continued pressure at minimum is critical.

Group by storage domain and use clear/hold durations plus reason-aware
deduplication.

## Decision-journal handoff

The public shadow CLI now supports an opt-in private JSONL decision journal. It
records normalized observation evidence, the bounded proposal, explicit
`not_evaluated` authority/effective-state fields, and a stable event ID. Each
event is appended and synced before the matching proposal reaches stdout;
journal failure is fail-closed for that proposal. A non-blocking exclusive lock
enforces one writer, and a 256 MiB hard cap requires operator rotation before
output can resume.

The journal must not share the storage domain being guarded. Slow or failed
sync intentionally stops shadow output; batching or asynchronous persistence
would weaken the current persist-before-output boundary and is not enabled.

For audit handoff, first stop or detach the writer, rotate the file under the
site's approved procedure, and run `pve-storage-guard journal verify --journal
SEALED.jsonl`. The read-only verifier refuses an active writer, unsafe file,
oversized or malformed event, forged linkage, inconsistent age, and journals
containing multiple storage domains. Its versioned output contains the exact
raw-file SHA-256 digest, counts, and time bounds but no resource identities.

Approval must bind that digest and expected summary, target, domain, policy
version, expiry, and exact resource mappings. An authorized local consumer may
then invoke `journal batch` with the approved digest, offset, and a limit of at
most 64. The command revalidates the entire file before emitting the requested
private page. A mutable path is never authority, and batch stdout must never be
captured in general task logs.

Verification identifies structurally valid exact bytes. It is not a signature,
sanitization result, publication approval, or permission to ingest. The public
code performs no rotation, transport, or import; the internal persistence
service described below remains runtime-uninvoked and approval-gated.

This is a local single-writer handoff only. It has no network exporter, ITOps
callback, rotation manager, actuator, or public-trace sanitization. An approved
transport, runtime invocation, and incident-linking adapter remain future work.

The internal ITOps draft now contains the matching pure event mapper and a
runtime-uninvoked persistence service. It requires an already-authorized target plus
reviewed domain, policy revision, and storage/disk resource bindings. The
repository rechecks target ownership and kind, then atomically persists the
private audit row and `estimated`, low-cardinality metrics. Deterministic IDs
make identical retries no-ops; a canonical SHA-256 metric-projection digest
rejects altered retries. One call is bounded to 64 events, 10,000 metrics, and
1 MiB of private audit details per event.

The draft also contains an unregistered, idempotent-write handoff capability
and exact-argv reader. The immutable approval binds the digest and summary,
target, domain/policy expectation, storage and disk resources, optional review
group, and batch size. Persistence rechecks the current running proposal,
envelope hash, approval, and expiry in the audit/metric transaction. Only
digest/count reconciliation reaches task evidence.
An optional review group must already exist; historical import never creates or
changes an alert, group, or incident.

A cross-repository local PoC built the public binary and passed one synthetic
private event through the compiled ITOps exact-argv reader. The identity-free
result confirmed digest match, one event, and complete pagination; event content
was not printed.

This does not create a live ingestion path. The public reader has no transport
or persistence, and the internal importer has no approved runtime registration,
route, scheduler, alert evaluation, incident creation, notification, or
actuation side effect.

## Current draft boundary

The internal integration draft includes the restricted read-only storage probe,
multi-signal detector, disabled alert seeds, dashboard, and a pure replay-trace
builder. The builder accepts only already-authorized metric samples and emits
relative offsets, numeric evidence, and coarse classes. Its diskstats semantics
are fixed to `average` / `derived`; callers cannot label them `p95`. It emits
ReplayTrace v1alpha2 with a fixed `block-device` measurement layer and preserves a storage sample with absent management
evidence as `managementPlaneStatus=unknown`, so the public assessor can measure
the two coverage dimensions separately.

The builder now creates request-scoped metric indexes once rather than scanning
the full caller array per output interval. A regression test forbids global
rescans and another preserves first-match compatibility. Five local Node.js
22.21.1 runs exported a deterministic 14-day, 60-second fixture (141,120 input
metrics and 20,160 output intervals) in 80.472–82.093 ms with identical output
checksums. This is pure in-memory evidence only; it excludes the probe, SSH,
PVE REST, SQLite, network, dashboard, and real storage.

The storage probe now optionally runs the fixed argv `zpool iostat -lpH 1 2`,
discards the since-boot row, and maps only a complete one-second interval.
ZFS pool IOPS, throughput, `total_wait`, and `disk_wait` are labeled
`statistic=interval_mean` and remain separate from diskstats-derived device
averages. Missing OpenZFS or inactive optional queue columns degrade safely.
These ZFS series are shadow calibration telemetry: detector v1 and its disabled
thresholds do not consume them.

The dashboard mirrors this boundary with separate ZFS-pool and member-disk
tables and states that ZFS `total_wait` is not I/O p95. A live stdin-only probe
check succeeded without installing or registering anything on the host.

There is no export route, transport, importer runtime registration, actuator,
probe installation, alert enablement, or production deployment. The public
batch reader is local, digest-bound, and persistence-free. All 1,268 backend
tests across 151 files and all 101 frontend tests pass locally; build, lint, and
dependency boundaries are also clean. Internal CI run 154 validated capability
commit `51cc834` with a 4m31s quality gate and 4m47s image build. Run 155
validated the existing-review-group refinement `e7e7997` with a 4m34s quality
gate and 4m45s image build. Final [run 156](https://gitea.wj2015.com/PEM/itops-agent-platform/actions/runs/156)
validated typed ZFS shadow telemetry `ccbbabd` with a 4m33s quality gate and
5m10s linux/amd64 image build while PR #37 stays Draft. Production rollout
remains an explicit approval gate.

Replay-export semantic commits `60b8359` and `8104bb2` were validated by
[run 160](https://gitea.wj2015.com/PEM/itops-agent-platform/actions/runs/160).
The indexed exporter and one-day CI smoke in `324422f` were then validated by
[run 161](https://gitea.wj2015.com/PEM/itops-agent-platform/actions/runs/161):
the Node quality gate passed in 4m34s and the linux/amd64 image build in 4m45s.
PR #37 remains Draft and undeployed.
