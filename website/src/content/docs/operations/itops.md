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
journal failure is fail-closed for that proposal.

This is a local single-writer handoff only. It has no network exporter, ITOps
callback, rotation manager, actuator, or public-trace sanitization. An approved
ITOps ingestion/linking adapter remains future work.

## Current draft boundary

The internal integration draft includes the restricted read-only storage probe,
multi-signal detector, disabled alert seeds, dashboard, and a pure replay-trace
builder. The builder accepts only already-authorized metric samples and emits
relative offsets, numeric evidence, and coarse classes. Its diskstats semantics
are fixed to `average` / `derived`; callers cannot label them `p95`.

There is no export route, file writer, actuator, probe installation, alert
enablement, or production deployment. All 1,236 backend tests across 145 files,
including four export tests, pass locally. The latest internal CI quality gate
and its dependent linux/amd64 image build both passed. Production rollout
remains an explicit approval gate.
