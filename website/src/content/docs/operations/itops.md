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
containing multiple storage domains. Its versioned output contains counts and
time bounds but no resource identities.

Verification is structural only. It is not a signature, sanitization result,
publication approval, or permission to ingest. The current code performs no
rotation or import; an ITOps persistence adapter remains an explicit future
approval gate.

This is a local single-writer handoff only. It has no network exporter, ITOps
callback, rotation manager, actuator, or public-trace sanitization. An approved
ITOps ingestion/linking adapter remains future work.

The internal ITOps draft now contains the matching pure event mapper. It
requires an already-authorized target and expected domain, strictly validates
the non-actuating contract and bounded reason/resource cardinality, and returns
an inert audit handoff plus decision-derived metrics marked `estimated`. It has
no journal reader, repository write, metric processor, route, scheduler, or
incident side effect.

## Current draft boundary

The internal integration draft includes the restricted read-only storage probe,
multi-signal detector, disabled alert seeds, dashboard, and a pure replay-trace
builder. The builder accepts only already-authorized metric samples and emits
relative offsets, numeric evidence, and coarse classes. Its diskstats semantics
are fixed to `average` / `derived`; callers cannot label them `p95`.

There is no export route, file writer, journal reader, repository write,
actuator, probe installation, alert enablement, or production deployment. All
1,240 backend tests across 146 files, including four export tests and four
decision-mapper tests, pass locally. The latest internal CI quality gate passed
in 4m31s; the dependent linux/amd64 image job remained blocked by required
conditions while the PR stayed Draft, so no image result is claimed. Production
rollout remains an explicit approval gate.
