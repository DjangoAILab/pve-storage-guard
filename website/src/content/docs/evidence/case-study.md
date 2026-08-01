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

## Root cause

Ungoverned bulk write demand shared a storage failure domain with the management
and durability-sensitive paths. Monitoring covered capacity and structural
health but not a storage latency SLO plus workload attribution.

All node, pool, VM/disk, network, workload, session, and application identities
are removed. Raw logs and guest data are not published.
