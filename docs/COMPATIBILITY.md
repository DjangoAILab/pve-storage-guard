# Compatibility evidence

Compatibility is evidence-scoped. A parser accepting one product combination
does not imply that host packaging, permissions, service supervision,
performance, or actuation have been validated.

## Current matrix

| Proxmox VE | OpenZFS | Evidence level | Validated surfaces | Not validated |
| --- | --- | --- | --- | --- |
| 9.2 | 2.4 | Source-format compatible | cluster status JSON; ZFS storage config/status JSON; `total_wait` header semantics; 37-bucket/12-column scripted histogram; PSI; diskstats | compiled binary on host; Unix permissions; systemd/container packaging; sustained sampling; alerts; policy calibration; actuation |

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
- Source-format compatibility does not prove the compiled binary has the
  required host permissions or behaves correctly under a systemd/container
  sandbox.
- One PVE/OpenZFS combination is not a general support claim.
- No SSH availability, guest task completion, notification, canary, rollback,
  or actuator behavior was exercised.

The next compatibility gate is a compiled-binary dry-run in a non-production
PVE environment, followed by reviewed packaging and permission tests. A
production-host install remains an explicit write checkpoint.
