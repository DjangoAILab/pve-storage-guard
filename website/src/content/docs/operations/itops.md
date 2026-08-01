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
service described below remains uninvoked and approval-gated.

This is a local single-writer handoff only. It has no network exporter, ITOps
callback, rotation manager, actuator, or public-trace sanitization. An approved
transport, runtime invocation, and incident-linking adapter remain future work.

The internal ITOps draft now contains the matching pure event mapper and an
uninvoked persistence service. It requires an already-authorized target plus
reviewed domain, policy revision, and storage/disk resource bindings. The
repository rechecks target ownership and kind, then atomically persists the
private audit row and `estimated`, low-cardinality metrics. Deterministic IDs
make identical retries no-ops; a canonical SHA-256 metric-projection digest
rejects altered retries. One call is bounded to 64 events, 10,000 metrics, and
1 MiB of private audit details per event.

This does not create a live ingestion path. The public reader has no transport
or persistence, and the internal importer has no approved runtime registration,
route, scheduler, alert evaluation, incident creation, notification, or
actuation side effect.

## Current draft boundary

The internal integration draft includes the restricted read-only storage probe,
multi-signal detector, disabled alert seeds, dashboard, and a pure replay-trace
builder. The builder accepts only already-authorized metric samples and emits
relative offsets, numeric evidence, and coarse classes. Its diskstats semantics
are fixed to `average` / `derived`; callers cannot label them `p95`.

There is no export route, transport, importer runtime registration, actuator,
probe installation, alert enablement, or production deployment. The public
batch reader is local, digest-bound, and persistence-free. All 1,252 backend
tests across 148 files pass locally,
including four export tests and 16 focused mapper/importer/repository tests.
Internal CI run 153 completed both its quality gate in 4m35s and dependent
linux/amd64 image build in 4m57s for the importer commit while the PR stayed
Draft. Production rollout remains an explicit approval gate.
