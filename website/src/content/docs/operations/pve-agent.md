---
title: Read-only PVE agent
description: Collect identity-safe PVE and OpenZFS evidence without actuation.
---

The v0.1 agent is a same-host observer for an explicitly bound Proxmox VE
`zfspool`. It supports one-shot and serial observation, but has no listener,
credential input, network exporter, or actuator. Production installation
remains a separate safety checkpoint.

The source tree also contains a fake-backed QEMU write-limit actuator core for
offline review. It is not integrated with this agent or any runtime. The core
binds one opaque resource to one owner-configured disk, requires an existing
bounded `bps_wr`, rejects conflicting bandwidth/IOPS options, fences the update
with the PVE config digest, and verifies full unmanaged state on read-back. A
live backend, controlled-load rehearsal, rollback proof, and production write
still require separate reviews and approval.

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
pve-storage-guard agent watch --config /private/path/agent.json --period 10s
```

Inventory must prove the node is healthy, the PVE storage is active ZFS, its
dataset belongs to the configured top-level pool, and every explicitly enrolled
device exists. Observation then emits one JSON sample. An interval with no
writes emits an invalid wait rather than a false zero-latency percentile.
Watch emits JSONL, never overlaps collectors, and exits cleanly when SIGTERM
cancels a sample or its bounded inter-sample wait.

## Fixed local surface

The binary compiles in five read-only commands: PVE cluster status, PVE storage
configuration, node storage status, a ZFS histogram-layout probe, and the
interval `zpool iostat -wpH -y` sample. It reads only
`/proc/pressure/io` and `/proc/diskstats`. It never invokes a shell or accepts
an executable path or free-form argv from configuration. Each command has a
deadline, fixed environment, and bounded output; unknown formats fail closed.

The current distroless controller image does not contain host `pvesh` or
`zpool` tools and is not a drop-in PVE collector. A proposed observer-only
systemd unit runs under a static non-root account with no capabilities, network
namespace, or writable filesystem path. Repository tests and Ubuntu 24.04
`systemd-analyze` validate its static contract (0.8 exposure). An ephemeral
Ubuntu PID-1 job also validates non-root initial start, supervised restart,
candidate cold start, and exact binary/config rollback with synthetic fixtures.
Neither check proves real PVE ACL, `/dev/zfs`, or sustained runtime behavior.
The `v0.1.0-rc.2` binary passed the bounded
production read-only compatibility check, but remains evidence-only until
those host gates pass.

## Non-production evidence gate

After an approved binary, owner-only config, and repository are staged on a
non-production PVE node, run the external validator as the intended non-root
observer account:

```sh
python3 scripts/validate_nonprod_observer.py \
  --binary /absolute/path/pve-storage-guard \
  --config /absolute/private/path/agent.json \
  --expected-sha256 sha256:APPROVED_RELEASE_DIGEST
```

Use a previously reviewed release digest, not a newly trusted local checksum.
The gate re-hashes the binary before each fixed command, validates inventory,
one observation, two watch records, private-identity absence, and a zero-exit
SIGTERM. Raw child output stays bounded in memory. Failure has zero stdout;
success is one identity-free versioned summary with no host, path, domain,
resource, timestamp, or metric.

This validates a short observer execution only. The separate CI lifecycle job
validates portable restart/rollback mechanics, but neither job installs on PVE,
proves 24-hour stability, or authorizes production. Synthetic CI passes never
count as PVE host permission evidence.

## Production read-only compatibility

Where no non-production PVE host exists, the separately approved production
dry-run may use `scripts/validate_prod_observer_compatibility.py` with the same
binary, private config, and approved digest arguments. A random owner-only
`/dev/shm` directory can hold those inputs for the duration of the command and
must be removed afterward. The procedure installs no service and must not
create an account, ACL, journal, cgroup, or I/O limit.

Root execution requires the additional explicit `--allow-root` acknowledgement
and is always surfaced as a failed hardening boundary in the result.

The result has the distinct kind `PVEHostObserverCompatibility`, records root
execution as a limitation, and always sets `promotionEligible: false`. It can
prove only compiled read compatibility, bounded identity-safe output, repeated
sampling, and clean SIGTERM cancellation. It cannot replace non-root
permissions, systemd, controlled-load, sustained-sampling, rollback, alert, or
actuation evidence.

## Compatibility evidence

| PVE | OpenZFS | Proven | Still gated |
| --- | --- | --- | --- |
| 9.2 | 2.4 | Source-bound and `v0.1.0-rc.2` inventory/observe/two-record watch, SIGTERM zero exit, release checksum/provenance, PVE/ZFS/PSI/diskstats formats, portable cancellation, static unit analysis, ephemeral Ubuntu PID-1 restart/cold-start/exact rollback | non-root PVE ACL/device/unit behavior, sustained sampling, controlled load, policy thresholds, actuation |

The fixture retains observed field and histogram shape but uses fixed aliases,
synthetic topology/cardinality, and synthetic operational values. It is
explicitly ineligible for performance or policy claims. The approved
compatibility run used only a short-lived owner-only `/dev/shm` directory and
left no process, account, service unit, or RAM artifact. Its anonymous result
is permanently promotion-ineligible.
Read the full
[compatibility evidence](https://github.com/DjangoAILab/pve-storage-guard/blob/main/docs/COMPATIBILITY.md).

Read the repository's full [agent runbook](https://github.com/DjangoAILab/pve-storage-guard/blob/main/docs/PVE-AGENT.md)
and [ADR-0007](https://github.com/DjangoAILab/pve-storage-guard/blob/main/docs/adr/0007-local-read-only-pve-agent.md).
