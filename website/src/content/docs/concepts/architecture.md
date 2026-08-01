---
title: Architecture
description: PVE product boundary, generic policy kernel, and failure isolation.
---

## One control loop per shared bottleneck

The primary key is `(adapter, node, storage-domain)`, not VM or disk. A single
actor adjusts an aggregate budget. A deterministic allocator distributes that
budget only to explicitly enrolled disks within their envelopes.

```text
PVE Adapter + Metrics Collector
              │ normalized observations
              ▼
      storage-slo-guard
      pool actor · policy · allocator
              │ bounded proposal
              ▼
       Safety Controller ── shadow ──> decision journal / ITOps
              │ approved canary only
              ▼
      constrained PVE Actuator ──> effective-state read-back
```

## Components

- **PVE Adapter** owns node, storage, VM/CT, task, and disk relationships.
- **Metrics Collector** owns timestamped latency, IOPS, throughput, queue, PSI,
  and management probes.
- **Policy Engine** owns immutable policy resolution and bounded AIMD.
- **Safety Controller** owns leases, freshness, bounds, cooldown, modes,
  desired/effective reconciliation, and rollback.
- **Actuator** accepts structured exact-resource operations, never commands.
- **Event/Telemetry** exports bounded metrics and append-only explanations.

## Failure-domain placement

Production topology places the controller and journal outside the PVE storage
failure domain. A small local agent separates read-only collection from a
disabled-by-default privileged actuator. Network loss or controller failure can
never cause a limit increase.

See the complete [architecture document on GitHub](https://github.com/DjangoAILab/pve-storage-guard/blob/main/docs/ARCHITECTURE.md).
