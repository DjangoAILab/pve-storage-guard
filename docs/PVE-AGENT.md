# Read-only PVE agent

## Scope

The v0.1 agent is a same-host observer for explicitly bound Proxmox VE
`zfspool` storage. It has one-shot inventory/observation commands and a serial
continuous observation mode. It does not install itself, open a network
listener, discover arbitrary guests, change PVE configuration, or invoke an
actuator. Run it on fixtures first. Installing it on any production node is a
separate operator checkpoint.

## Why ZFS is the first adapter

`/proc/diskstats` contains cumulative counters and timing totals. Those values
can support pressure detection, but they cannot reconstruct an I/O latency
percentile. The agent sets `waitValid=true` only from the OpenZFS
`total_wait` write-latency histogram produced for an interval. It reports the
nearest-rank p95 bucket upper bound and preserves that statistic and source in
`waitEvidence`. PSI and diskstats stay explicitly typed as corroboration.
Diskstats reads/writes/sectors/timing values are emitted as cumulative totals;
an external collector may difference consecutive samples for IOPS, throughput,
and weighted queue-time rates, but a single total is never mislabeled as a
rate.

## Private configuration

Copy `configs/examples/reference-pve-agent.json` outside the repository, set
mode `0600`, and replace every private placeholder. `domainKey` and each
`resourceKey` are opaque values safe for downstream output. `node`, `storage`,
`zpool`, and `kernelDevice` are private bindings and never appear on stdout.

The ZFS pool is the top-level pool accepted by `zpool iostat`; a PVE storage
may point at that pool or one of its datasets. Resources are explicit: the
agent never auto-enrolls every disk or workload it can see.

```sh
install -m 0600 configs/examples/reference-pve-agent.json /private/path/agent.json
$EDITOR /private/path/agent.json
```

Treat the completed file as internal configuration. Do not commit it, attach
it to a public issue, or copy its stderr/stdout together without reviewing the
destination.

## Commands

On a PVE host with `/usr/bin/pvesh`, `/usr/sbin/zpool`, and host procfs:

```sh
pve-storage-guard agent inventory --config /private/path/agent.json
pve-storage-guard agent observe --config /private/path/agent.json
pve-storage-guard agent watch --config /private/path/agent.json --period 10s
```

`inventory` verifies that the configured PVE node is online/quorate, the
storage binding is an active `zfspool`, and every enrolled kernel device exists
in `/proc/diskstats`. Its JSON contains only opaque keys.

`observe` samples the OpenZFS wait histogram for the configured interval and
emits one normalized observation. A supervisor can schedule repeated calls or
pipe observations into `shadow`. `watch` performs the same operation serially,
emits JSONL, and waits the configured 1-second-to-1-hour period after each
completed sample. It never overlaps collectors. SIGINT/SIGTERM cancels an
in-flight child command or wait and exits cleanly. A collection or stdout error
fails the process so its supervisor can back off; it never emits a fabricated
sample. Zero writes produce `waitValid=false` rather than a fabricated
zero-latency p95.

## Fixed read-only surface

The executable mapping is compiled into the binary:

```text
/usr/bin/pvesh get /cluster/status --output-format json
/usr/bin/pvesh get /storage/<validated-storage> --output-format json
/usr/bin/pvesh get /nodes/<validated-node>/storage/<validated-storage>/status --output-format json
/usr/sbin/zpool iostat -w <validated-pool>
/usr/sbin/zpool iostat -wpH -y <validated-pool> <1..60> 1
read /proc/pressure/io
read /proc/diskstats
```

There is no shell, user-selected executable, free-form argument, pmxcfs tree
walk, secret lookup, network client, or write operation. Commands use a fixed
environment, bounded output, and a deadline. Unknown JSON or histogram shapes
fail closed.

## Deployment caveat

The existing distroless controller image does not contain PVE/OpenZFS host
tools and should not be described as a drop-in host collector. The
`v0.1.0-rc.2` host binary has passed the bounded production read-only
compatibility check, but it remains evidence-only: do not install it as a
production service until the explicit write checkpoint. A dedicated package or
minimal host-integrated image, live least-privilege permission validation, and
a broader version compatibility matrix remain promotion gates.

## Non-production evidence gate

After an operator has staged a reviewed release binary, its owner-only config,
and the validator from the same reviewed repository commit on a
**non-production** PVE host, run it as the intended non-root observer account:

```sh
python3 scripts/validate_nonprod_observer.py \
  --binary /absolute/path/pve-storage-guard \
  --config /absolute/private/path/agent.json \
  --expected-sha256 sha256:APPROVED_RELEASE_DIGEST
```

Obtain `APPROVED_RELEASE_DIGEST` from the previously reviewed release checksum
or provenance, not by trusting a fresh digest calculated from the staged file.
The validator rejects root, non-Linux hosts, missing PVE/OpenZFS executables,
relative paths, symlinks, unsafe owners/modes, and a digest mismatch. It
re-hashes the binary before every fixed child invocation.

The gate runs `version`, `agent inventory`, `agent observe`, and two `agent
watch` samples with bounded output and time. It parses raw output only in
memory, rejects duplicate JSON keys and any exact private node/storage/pool/
device value, sends SIGTERM, and requires a zero exit. Failure writes no stdout
and only a categorical stderr message. Success emits one identity-free
`PVEHostObserverValidation` document matching
`api/v1/schema/pve-host-observer-validation.schema.json`; it contains the
approved binary digest/version and boolean/count checks, but no host, user,
path, opaque domain/resource key, timestamp, metric, capacity, or child output.

`requestedMutations: 0` means the validator requested only the compiled
read-only CLI operations. It is not a syscall audit, release signature check,
24-hour stability result, systemd start/restart test, or proof that the host is
safe for actuation. Review the summary before retaining or sharing it. A local
fake pass tests the validator only; the checklist gate closes only with a real
non-production PVE result plus live service/rollback evidence.

## Production read-only compatibility

If a non-production PVE host is unavailable, an operator may run the separate
compatibility validator after the production dry-run checkpoint is approved:

```sh
python3 scripts/validate_prod_observer_compatibility.py \
  --binary /absolute/path/pve-storage-guard \
  --config /absolute/private/path/agent.json \
  --expected-sha256 sha256:APPROVED_RELEASE_DIGEST
```

Root execution fails closed unless the operator also supplies `--allow-root`.
Use that explicit acknowledgement only when no least-privilege observer account
exists and the exact reviewed binary digest is approved for this narrow dry-run;
the resulting evidence records `nonRoot: false` and remains ineligible.

The binary, config, and both validator modules may be staged in a random
owner-only directory on `/dev/shm` and removed by an EXIT trap, so this flow
does not require a persistent installation. The private config should mark all
resources root and critical unless an independent enrollment review proves
otherwise. Do not create an account, ACL, service, package, journal, cgroup, or
I/O limit for this check.

The command runs the same digest-bound `version`, `inventory`, `observe`, and
two-record `watch` sequence and applies the same identity-leak and cancellation
checks as the non-production validator. Its output is a separate
`PVEHostObserverCompatibility` document matching
`api/v1/schema/pve-host-observer-compatibility.schema.json`. It records root
execution as a limitation, always sets `promotionEligible: false`, and cannot
close non-root permissions, systemd isolation, controlled-load, sustained
sampling, rollback, alert, or actuation gates. Production deployment and every
control write remain separate approval points.

## Proposed systemd boundary

`deploy/systemd/pve-storage-guard-observer.service` is a reviewable
observer-only unit, not an installer. It runs `inventory` before `watch` under
the static non-root `pve-storage-guard` account. Its contract includes:

- an empty capability and ambient-capability set with `NoNewPrivileges`;
- a strict read-only host filesystem, hidden home directories, restricted
  procfs process visibility, and no writable path declaration;
- a private network namespace, all IP addresses denied, and only local Unix
  sockets permitted;
- a closed device policy exposing only `/dev/zfs`; libzfs requires opening that
  control device read/write even for the compiled read-only `zpool iostat`
  operations, while block devices remain unavailable;
- bounded tasks, memory, file descriptors, restart rate, start/stop time, and
  a SIGTERM-to-SIGKILL shutdown sequence; and
- owner-only configuration plus journald output. Opaque keys avoid infrastructure
  identity disclosure, but observations are still internal operational data
  and must not be forwarded to a public log sink.

Repository tests pin the execution surface and required hardening directives.
`scripts/verify-systemd-unit.sh` checks unit syntax and enforces a maximum 1.0
`systemd-analyze security` exposure score. The current disposable Ubuntu 24.04
check scored 0.8 (SAFE). A separate ephemeral Ubuntu PID-1 rehearsal runs the
real compiled CLI through non-root initial start, SIGKILL supervision, candidate
cold start, and exact baseline binary/config rollback. It uses only public
synthetic fixtures, never enables the unit, restores the runner's pre-existing
directory metadata, and requires complete cleanup. This proves portable
systemd lifecycle mechanics, not PVE ACLs, `/dev/zfs`, sustained load, or PVE
runtime behavior.

Before any installation, use the evidence gate on a non-production PVE node,
then prove the remaining live service behavior under the dedicated account:
`pvesh` read authorization at the smallest reviewed scope, `/dev/zfs` access,
both procfs reads, at least 24 hours of repeated samples, restart backoff,
journal classification/retention, and full stop/disable/removal. The repository
does not create the account, PVE ACL, config, binary, unit, or log policy.
Failure of any check leaves the service disabled. A production host remains an
explicit write checkpoint.

## Compatibility status

The first sanitized real-host source-format fixture covers PVE 9.2 with
OpenZFS 2.4. It validates the JSON and histogram shapes consumed by the parser,
but all public operational values are synthetic and the fixture is ineligible
for policy or performance claims. Public topology/cardinality is synthetic as
well. The `v0.1.0-rc.2` binary has executed only through the approved transient,
read-only compatibility check; it was not installed on that production host.
See the
[compatibility matrix](COMPATIBILITY.md) for the exact evidence boundary.

## Triage

| Symptom | Meaning | Safe response |
| --- | --- | --- |
| management plane unavailable | `pvesh` failed, node is offline, or cluster lost quorum | investigate PVE health; do not increase budgets |
| storage binding invalid | configured storage is not the expected ZFS pool | stop and review private binding |
| histogram invalid | unsupported OpenZFS shape or malformed sample | stop the watch process; collect a sanitized fixture |
| no write samples | interval had no writes | expected invalid wait sample; retry later |
| device inventory unavailable | configured device absent or procfs unavailable | stop enrollment; review mapping |

Never “fix” an invalid histogram by relabeling diskstats average timing as p95.

## Source contracts

- [OpenZFS `zpool iostat`](https://openzfs.github.io/openzfs-docs/man/master/8/zpool-iostat.8.html)
  documents latency histograms, exact-value output, scripted output, and
  interval sampling.
- [Linux I/O statistics](https://docs.kernel.org/admin-guide/iostats.html)
  defines the cumulative `/proc/diskstats` fields.
- [Proxmox VE `pvesh`](https://pve.proxmox.com/pve-docs/pvesh.1.html) is the
  same-host read-only API shell used for the fixed management probes.
