---
title: Community context
description: How existing PVE, QEMU, Linux, and orchestration work relates.
---

PVE Storage Guard uses existing enforcement primitives and community control
ideas. It does not claim to invent I/O throttling.

- Proxmox VE and QEMU provide per-disk bandwidth/IOPS limit mechanisms.
- Linux cgroup v2 provides `io.max`, latency, and cost-oriented controllers.
- `resctl-demo` demonstrates pressure-aware resource control.
- Koordinator coordinates workload QoS and interference on Kubernetes.
- `pve-io-guard` demonstrates operator demand for a small PVE-focused guard.

The gap addressed here is packaging these ideas as a PVE storage-domain safety
loop with exact enrollment, observer/shadow modes, desired/effective read-back,
bounded allocation, ITOps telemetry, and rollback.

[Read the sourced prior-art review](https://github.com/DjangoAILab/pve-storage-guard/blob/main/docs/PRIOR-ART.md).
