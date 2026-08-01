# Community context and prior art

PVE Storage Guard builds on existing enforcement and control ideas; it does not
claim to invent I/O throttling.

## Platform primitives

- [Proxmox VE `qm` manual](https://pve.proxmox.com/pve-docs/qm.1.html)
  documents per-disk bandwidth and IOPS limits. These are the intended low-level
  enforcement primitive, not a complete feedback controller.
- [Linux cgroup v2 I/O controller](https://docs.kernel.org/admin-guide/cgroup-v2.html#io)
  provides `io.max`, latency, and cost-oriented mechanisms. It demonstrates the
  value of protecting latency-sensitive work at the shared device boundary.
- [QEMU block I/O throttling](https://www.qemu.org/docs/master/system/qemu-block-drivers.html)
  is the virtualization-layer mechanism underlying many disk limits.

## Control and orchestration projects

- [resctl-demo](https://github.com/facebookexperimental/resctl-demo) explores
  resource control using cgroup2 and pressure signals, including I/O control.
- [Koordinator](https://github.com/koordinator-sh/koordinator) coordinates
  quality-of-service and interference management for Kubernetes workloads.
  Its scope and platform are broader than a PVE storage-domain guard.
- [pve-io-guard](https://github.com/mikysal78/pve-io-guard) is a small community
  PVE-focused shell project that watches disk utilization and applies a fixed
  throttle. It is useful evidence that operators encounter this need. PVE
  Storage Guard takes a different architecture: storage-domain latency/SLO
  feedback, exact disk enrollment, bounded allocation, observer/shadow modes,
  desired/effective read-back, audit, and ITOps integration.

## Gap addressed here

The primitives can impose a limit, and broader control systems show useful
patterns, but PVE operators still need a narrowly packaged safety system that:

- maps PVE workloads and disks to the correct shared storage domain;
- detects latency/pressure plus management-plane degradation;
- chooses bounded, per-domain and per-disk proposals without online ML;
- defaults to observer/shadow and explains every decision;
- applies only pre-authorized structured changes and verifies effective state;
- ships replay evidence, Prometheus metrics, runbooks, and a PVE-specific
  distribution.

No code from the referenced projects is copied into this repository. Their
licenses and designs must be reviewed independently before any future code reuse.
