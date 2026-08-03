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

## Observed production shadow view

![Identity-free observed ITOps storage-pressure shadow baseline](/pve-storage-guard/itops-storage-pressure-shadow-baseline.png)

This is a deterministic crop from the authenticated production dashboard, not
fixture data. The target header and per-pool/per-disk tables were excluded; the
retained cells contain only detector state, aggregate IO PSI,
management-probe state, and the disabled alert-gate count. It is evidence of
the observer boundary, not of prevention or actuation.

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

The merged code also contains an unregistered, idempotent-write handoff capability
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

## Current rollout boundary

The internal integration includes the restricted read-only storage probe,
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
alert enablement, or application deployment. The public
batch reader is local, digest-bound, and persistence-free. All 1,268 backend
tests across 151 files and all 101 frontend tests pass locally; build, lint, and
dependency boundaries are also clean. Internal CI run 154 validated capability
commit `51cc834` with a 4m31s quality gate and 4m47s image build. Run 155
validated the existing-review-group refinement `e7e7997` with a 4m34s quality
gate and 4m45s image build. Final internal run 156
validated typed ZFS shadow telemetry `ccbbabd` with a 4m33s quality gate and
5m10s linux/amd64 image build. PR #37 later merged after explicit approval, and
post-merge run 164 passed both jobs. This merge did not deploy or register the
integration.

Replay-export semantic commits `60b8359` and `8104bb2` were validated by
internal run 160.
The indexed exporter and one-day CI smoke in `324422f` were then validated by
internal run 161:
the Node quality gate passed in 4m34s and the linux/amd64 image build in 4m45s.

The restricted read-only probe checkpoint was later explicitly approved and
completed after an exact rollback-and-retry rehearsal. The separately approved
application checkpoint failed closed during protected host-state validation and
restored the prior source tree and image selector before container, database,
credential, registry, alert, or notification changes. A reviewed successor fix
passed the complete quality and isolated linux/amd64 image-smoke gates in
internal run 201
and again at the rebased PR head in
internal run 204.
After a separate narrow repair and renewed approval, the exact-digest successor
deployment completed. Two bounded read-only checkpoints verified fresh
collectors, the exact recommended-rule set, healthy zero-restart containers,
absent deployment journals, and the retained predecessor rollback pair. The
operator then closed deployment acceptance with an explicit evidence
limitation instead of waiting for a fixed 24-hour timer. This proves the
observer deployment boundary; it is not statistical threshold calibration.

## Calibration remains shadow-only

The first identity-free production replay contained 203 complete cycles and
812 disk detector samples. A second contained 240 complete cycles and 960 disk
detector samples. Detector-v1 recomputation reported zero mismatches in both.
The second window would have produced four warning firing/recovery lifecycles,
but no critical lifecycle and only one quiet cycle. The exact warning/critical
rules remained disabled, so the v1 evidence gate correctly rejected combined
arming.

No production pressure was generated to manufacture the missing evidence.
Alerts, notifications, journal registration, and actuation remain disabled.

An internal Draft at exact head `786308f` now proposes an identity-free v2
assessment that grades warning and critical evidence separately. Shared gates
still require structural coverage, exact detector recomputation, the exact two
disabled rules, and fail-closed rule-set review. Each severity needs its own
exact contract, exposure and complete firing/recovery lifecycle; every resource
that actually fired must also have at least ten severity-appropriate baseline
samples. Whole-domain quiet coverage becomes diagnostic rather than a shared
gate. The combined result remains the logical AND of both severities and the
only proposed action remains keeping both rules disabled.

The Draft passed local focused, read-only SQLite, state-machine, architecture,
lint, build, coverage, replay, and secret-scan gates. It is not merged or
deployed. The exact evaluator was then streamed to the active backend as stdin,
not installed, and opened production SQLite read-only/query-only. Across 240
complete cycles and 960 detector samples it found zero mismatches, two
warning-firing resources with adequate individual baselines, and four complete
warning recoveries. Warning review evidence passed. Critical had no firing or
recovery, so critical and combined eligibility remained false. Both rules stayed
disabled and no notification or control action followed. Critical production
pressure will not be manufactured; synthetic critical tests prove evaluator
logic only, not production readiness.

## Online release-backup follow-up

The verified offline archive exposed a long maintenance pause. An internal
repository-only Draft now creates a consistent SQLite online backup and copies
stable ordinary non-database files while the active pair remains healthy. It
checks capacity before staging and again before compression using actual staged
bytes plus headroom. Helper containers are networkless, capability- and
resource-bounded, time-bounded, and release-labeled; cleanup may remove only an
exact current-release match, while a stale helper blocks the next attempt for
operator inspection.

The hardened Draft passed its complete quality gate and isolated linux/amd64
image smoke. A read-only host probe verified the exact timeout CLI contract,
but did not create a helper or prove cgroup-v2 I/O weighting. The mitigation
remains unmerged and undeployed. An exact candidate, a separately approved
synthetic production-host rehearsal, effective I/O-weight evidence, and
explicit deployment approval remain required. Available event history does not
support an exact downtime claim.
