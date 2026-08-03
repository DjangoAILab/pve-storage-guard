# Compatibility evidence

Compatibility is evidence-scoped. A parser accepting one product combination
does not imply that host packaging, permissions, service supervision,
performance, or actuation have been validated.

## Current matrix

| Proxmox VE | OpenZFS | Evidence level | Validated surfaces | Not validated |
| --- | --- | --- | --- | --- |
| 9.2 | 2.4 | Release binary production read-compatible; not promotion-eligible | cluster status JSON; ZFS storage config/status JSON; `total_wait` header semantics; 37-bucket/12-column scripted histogram; PSI; diskstats; source-bound and `v0.1.0-rc.2` inventory/observe/two-record watch; SIGTERM zero exit; release checksum/provenance; portable cancellation; static systemd checks; ephemeral Ubuntu PID-1 non-root start/restart/cold-start/exact rollback | non-root PVE Unix/ACL/device permissions; live PVE systemd behavior; sustained sampling; controlled load; alerts; policy calibration; actuation |

The fixture is under
`internal/adapter/pve/testdata/pve-9.2-openzfs-2.4/`. Its manifest labels the
shape as observed, every operational value and topology/cardinality as
synthetic, identities as fixed aliases, and policy-evidence eligibility as
false.

## Capture and privacy method

On 2026-08-02, a bounded in-memory probe ran against the reference PVE host.
The probe used only these read operations:

```text
pvesh get /cluster/status --output-format json
pvesh get /version --output-format json
pvesh get /storage --output-format json
pvesh get /storage/<validated-id> --output-format json
pvesh get /nodes/<validated-node>/storage/<validated-id>/status --output-format json
zfs version
zpool iostat -w <validated-pool>
zpool iostat -wpH -y <validated-pool> 1 1
read /proc/pressure/io
read /proc/diskstats
```

Private identifiers were validated in process and replaced before stdout.
Histogram weights, PSI values, diskstats values, health values, and operational
state, optional-row presence, dataset depth, and row counts were replaced with
deterministic synthetic values. Only field types,
column labels, column count, bucket boundaries, and product major/minor were
retained. Raw stdout/stderr, addresses, capacities, timestamps, node/storage/
pool/device names, guest data, and patch/build strings did not leave the host
and were not written locally. The probe performed no remote write.

The public test independently rejects private-address patterns, generic private
markers, local paths, and MAC-like values across every fixture file. A separate
pre-publication scan checked the fixture for the reference environment's known
identifiers without storing those identifiers in the repository.

## Production read-only compiled compatibility

No separate non-production PVE host is available. After an explicit dry-run
approval, a clean linux/amd64 build from public main `bfab0fb` ran on the
reference production PVE host through
`scripts/validate_prod_observer_compatibility.py`. The build identified itself
as `v0.1.0-dev.bfab0fb`; its SHA-256 was
`b12b3be070c70ed87685c93c6e768f04ad23576e430b809e465a56936ac7e96e`.
The binary, two validator modules, and a conservative private config existed
only in a random owner-only `/dev/shm` directory for the command duration. All
discovered leaf resources were marked root and critical.

The identity-free result validated the binary digest, inventory, one
observation, two serial watch samples, and a zero-exit SIGTERM. It reported no
private-identity leak, no raw-output persistence, and zero requested mutations.
Because the existing SSH execution identity was root, the result records
`nonRoot: false`, includes `non-root-not-validated`, and always sets
`promotionEligible: false`. No raw metric, topology, capacity, path, node,
storage, pool, or device value was retained. A post-run categorical check found
no remaining RAM staging directory, observer process, service account, or
systemd unit.

The previously published `v0.1.0-rc.1` linux/amd64 archive and checksum were
also verified, but its source revision predates the `agent` CLI. Its
compatibility attempt therefore failed closed at inventory and is not presented
as host evidence.

The successor
[`v0.1.0-rc.2` release](https://github.com/DjangoAILab/pve-storage-guard/releases/tag/v0.1.0-rc.2)
is bound to public main `412ddcf`. Its release workflow generated four archives,
four SPDX SBOMs, SHA-256 checksums, GitHub build provenance, a signed
multi-architecture image, and an automated Linux asset smoke test. All release
jobs passed in
[run 30732828939](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30732828939).
Independent download verification accepted every checksum and the GitHub
attestations for the linux/amd64 archive and SBOM.

The exact linux/amd64 release binary (`0.1.0-rc.2`, SHA-256
`5917ab568f94451ba2d125adb5633ab77288885237d41fc14c74971e6012c84a`)
then repeated the same owner-only, RAM-staged compatibility sequence on the
reference host. Inventory, one observation, two watch records, and SIGTERM
passed with no private identity leak, raw-output persistence, or requested
mutation. The result remains `nonRoot: false` and `promotionEligible: false`.
Post-run checks again found zero staging artifacts, observer processes, service
accounts, or units.

## Ephemeral systemd lifecycle evidence

Public PR #46
[run 30791350808](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30791350808)
exercised the exact production observer unit on an ephemeral Ubuntu 24.04 VM
with systemd as PID 1. Two byte-distinct builds of
the real CLI and two owner-only synthetic configs represented the baseline and
candidate. Fixed command shims accepted only the adapter's five read-only
operations and returned the public synthetic PVE 9.2/OpenZFS 2.4 fixtures.

The baseline started as the dedicated non-root account and emitted two valid
observations. SIGKILL of its main PID produced a different supervised PID after
the unit's configured 30-second restart delay. A full stop then candidate cold
start and a second full stop then baseline rollback cold start each produced a
new PID and valid sample. Rollback restored the exact initial binary and config
digests. The unit was never enabled; cleanup removed every created fixed path
and identity and restored the pre-existing `/usr/local/bin` inode/owner/group/
mode. The identity-free artifact recorded two initial and one sample for each
later phase and has SHA-256
`1c23af970801c23d00a0e7d50c02bacc128276c6c6550288dbd4df9366c10bc3`.

This closes portable systemd lifecycle and artifact-rollback mechanics only.
The synthetic shims do not exercise PVE ACLs, `/dev/zfs`, real journald policy,
sustained load, or a Proxmox kernel/systemd environment.

## What this proves

- PVE 9.2 returned the JSON fields and types consumed by the current parser.
- OpenZFS 2.4 retained the exact `total_wait`, `disk_wait`, `syncq_wait`, and
  `asyncq_wait` ordering checked by the agent.
- Its scripted histogram had 37 power-of-two-minus-one buckets and 11 statistic
  columns after the boundary, with total-write at the expected position.
- Current Linux PSI and diskstats formats were accepted by the public parsers.

## What this does not prove

- The synthetic fixture cannot calibrate latency thresholds or demonstrate a
  production benefit.
- Compiled read compatibility does not prove the binary has least-privilege
  PVE host permissions. The Ubuntu rehearsal proves the portable live systemd
  mechanics, not the PVE-specific permission/runtime boundary.
- Portable integration tests prove SIGTERM/context cancellation reaps a child,
  and Ubuntu 24.04 `systemd-analyze` gives the proposed observer unit a 0.8
  exposure score. Neither result validates PVE ACLs, `/dev/zfs` access, output
  retention, or supervision on PVE.
- The external non-production validator is tested against success and failure
  fakes plus a compiled-binary negative path. The production compatibility
  result does not turn it into non-production permission or promotion evidence.
- One PVE/OpenZFS combination is not a general support claim.
- No SSH availability, guest task completion, notification, canary, production
  rollback, or actuator behavior was exercised.

The release-artifact compatibility gate is closed by `v0.1.0-rc.2`, and the
portable systemd lifecycle/rollback gate is closed by PR #46. The next
promotion gates remain non-root PVE permissions, live PVE systemd behavior,
sustained sampling, and controlled load. A production-host install remains an
explicit write checkpoint.
