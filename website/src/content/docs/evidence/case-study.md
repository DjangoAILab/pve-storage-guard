---
title: Incident case study
description: An anonymized production PVE management-plane starvation incident.
---

A bulk virtual disk dominated shared ZFS writes. The pool remained structurally
healthy, yet synchronous operations and host management became severely delayed.
Ending an initiating session did not immediately restore SSH because the actual
workload and queued I/O outlived that session.

## Retained numeric evidence

- 2,177,396,736 attributed bytes over approximately 18 seconds: 115.36 MiB/s.
- Eleven of twelve write-wait samples exceeded 50 ms.
- Two exceeded 100 ms; the peak was approximately 348.87 ms.
- Durability-sensitive events took approximately 4.4 and 9.3 seconds.
- Collection began after the retained management-failure marker, so the
  historical data does not prove advance warning.
- A later fixed-20 natural-load check had 22/60 unsafe samples and a 234.065464
  ms p99; the cap was rejected and rolled back. Only aggregate evidence remains.

## Root cause

Ungoverned bulk write demand shared a storage failure domain with the management
and durability-sensitive paths. Monitoring covered capacity and structural
health but not a storage latency SLO plus workload attribution.

The fixed-20 result is not replayable or independent, but it is sufficient to
reject claims that one static per-disk cap is a validated universal fallback.

All node, pool, VM/disk, network, workload, session, and application identities
are removed. Raw logs and guest data are not published.

## Rollout safety evidence

The first observer rollout used separate restricted-probe and application
checkpoints. The read-only probe checkpoint completed after an exact rollback
and retry, with every allowed operation validated. The application checkpoint
then failed closed on a protected host-state invariant and restored the previous
source and image selector before any container, database, credential, alert, or
registry change.

Review separated a candidate allowlist defect from one bounded pre-existing
directory-mode drift. The former now has regression coverage; the latter still
requires its own exact, non-recursive approval. No application deployment,
alert, notification, journal import, or actuation followed. This is evidence
that the rollout safety boundary worked, not evidence that active control is
ready.
