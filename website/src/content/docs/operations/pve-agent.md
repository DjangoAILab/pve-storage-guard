---
title: Read-only PVE agent
description: Collect identity-safe PVE and OpenZFS evidence without actuation.
---

The v0.1 agent is a one-shot, same-host observer for an explicitly bound
Proxmox VE `zfspool`. It has no listener, credential input, network exporter,
or actuator. Production installation remains a separate safety checkpoint.

## Evidence semantics

The controller's wait input is a percentile contract. Linux diskstats cannot
recover a percentile from cumulative counters, so the agent only sets
`waitValid: true` from an OpenZFS interval `total_wait` write-latency
histogram. The emitted evidence calls the value a `p95-upper-bound`: the
nearest-rank p95 bucket's upper edge.

PSI and diskstats are corroborating signals. Diskstats I/O counts, sectors, and
queue-time fields remain cumulative totals; downstream collectors must
difference consecutive snapshots before labeling IOPS or throughput rates.

## Configure private bindings

Copy `configs/examples/reference-pve-agent.json` outside the repository,
replace the placeholders, and restrict it to the owner:

```sh
install -m 0600 configs/examples/reference-pve-agent.json /private/path/agent.json
$EDITOR /private/path/agent.json
```

Real node, storage, pool, and kernel-device names stay in this file. Opaque
`domainKey` and `resourceKey` values are the only identities emitted on stdout.
The agent rejects symlinks, non-regular files, files readable by group/other,
files over 64 KiB, duplicate bindings, path-like values, and unknown fields.

## Observe safely

On a reviewed PVE test host:

```sh
pve-storage-guard agent inventory --config /private/path/agent.json
pve-storage-guard agent observe --config /private/path/agent.json
```

Inventory must prove the node is healthy, the PVE storage is active ZFS, its
dataset belongs to the configured top-level pool, and every explicitly enrolled
device exists. Observation then emits one JSON sample. An interval with no
writes emits an invalid wait rather than a false zero-latency percentile.

## Fixed local surface

The binary compiles in five read-only commands: PVE cluster status, PVE storage
configuration, node storage status, a ZFS histogram-layout probe, and the
interval `zpool iostat -wpH -y` sample. It reads only
`/proc/pressure/io` and `/proc/diskstats`. It never invokes a shell or accepts
an executable path or free-form argv from configuration. Each command has a
deadline, fixed environment, and bounded output; unknown formats fail closed.

The current distroless controller image does not contain host `pvesh` or
`zpool` tools and is not a drop-in PVE collector. Use a locally built binary in
a test environment until host packaging, hardening, and a PVE/OpenZFS version
compatibility matrix pass their gates.

Read the repository's full [agent runbook](https://github.com/DjangoAILab/pve-storage-guard/blob/main/docs/PVE-AGENT.md)
and [ADR-0007](https://github.com/DjangoAILab/pve-storage-guard/blob/main/docs/adr/0007-local-read-only-pve-agent.md).
