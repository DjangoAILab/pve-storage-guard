# Ephemeral systemd lifecycle rehearsal design

## Status

Approved by the existing project plan for offline/simulated verification. This
batch must not run on a PVE host or change any production system.

Implemented in public PR #46. The first two CI attempts failed closed before
mutation because the GitHub runner's `/usr/local/bin` was not a root-trusted
parent; the final design snapshots its inode/metadata, normalizes it during the
run, and restores it exactly. A later attempt proved initial start and
supervised restart before rejecting a non-essential `reset-failed` helper; that
helper was removed so the production unit's native start-rate limits are tested.
Final
[run 30791350808](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30791350808)
passed the one-minute lifecycle job. Its evidence artifact SHA-256 is
`1c23af970801c23d00a0e7d50c02bacc128276c6c6550288dbd4df9366c10bc3`.

## Evidence gap

The repository already proves the observer's process-level cancellation,
strict systemd unit syntax, and an offline systemd security score. Those checks
do not prove that the production-shaped unit can:

1. pass `ExecStartPre` and start as the dedicated non-root account;
2. emit valid observations while systemd is PID 1;
3. restart after an injected process failure;
4. stop completely and cold-start with a different PID;
5. upgrade to a byte-distinct candidate and restore the exact previous binary
   and owner-only configuration.

No second PVE instance is available, so the missing lifecycle evidence will be
generated on an ephemeral GitHub-hosted Ubuntu VM. Public, synthetic PVE 9.2 /
OpenZFS 2.4 compatibility fixtures stand in for the fixed read-only commands.
This is supervision and rollback evidence only; it is not PVE permission,
performance, alert-calibration, or policy-effect evidence.

## Safety boundary

The rehearsal is fail-closed before its first mutation unless all of the
following are true:

- Linux is running with systemd as PID 1;
- the effective user is root and `CI=true`;
- an exact, non-secret opt-in guard is present;
- `/etc/pve` and every production-path rehearsal target are absent;
- the baseline/candidate binaries, production unit, and fixture directory are
  regular trusted inputs with the expected structure;
- every fixed parent is root-owned and not writable by any non-root identity
  (a root-group write bit is accepted only if the root group has no non-root
  primary or supplemental member).

GitHub's ephemeral runner may initially assign `/usr/local/bin` to the runner
identity. That one fixed parent is accepted only when it is owned by root or
the exact `SUDO_UID`/`SUDO_GID`, its inode is recorded, and the binary target is
absent. The rehearsal changes it to root-owned `0755` before installing the
observer, then restores the original owner/group/mode on the same inode during
cleanup. A restoration mismatch fails the job.

It refuses real PVE hosts rather than attempting to distinguish
"production" from "non-production" PVE. It creates only exact allowlisted
paths, records which parent directories it created, and removes only those
paths in a finally block. A failure to clean up fails the CI job.

The unit is loaded transiently from `/run/systemd/system`; it is never enabled.
Synthetic command shims accept only the five exact read-only command shapes
used by the adapter. They cannot execute arbitrary arguments or mutate host
state. Raw journal output remains on the disposable CI VM; the script emits
only categorical results, counts, artifact digests, and lifecycle PIDs.

## Rehearsal sequence

1. Create the fixed service account, owner-only config, synthetic read-only
   command shims, fixture bundle, and exact production observer unit.
2. Install a baseline binary, run `daemon-reload`, start the unit, and validate
   two strict JSON observations plus non-root `MainPID` ownership.
3. Kill the main process and require systemd `Restart=on-failure` to produce a
   different active PID and another valid observation.
4. Stop the unit, require no remaining cgroup process, install the byte-distinct
   candidate binary/config, and cold-start it successfully.
5. Stop again, restore the exact baseline binary/config, cold-start, and prove
   both SHA-256 values match their initial values.
6. Stop, remove all rehearsal artifacts, reload systemd, remove the service
   account, and prove every target is absent.

## Acceptance criteria

- The integration job passes on Ubuntu 24.04 with the real compiled CLI.
- Every observed process UID is the dedicated non-root account.
- Initial start, supervised restart, candidate cold start, and rollback cold
  start each emit schema-valid, identity-safe synthetic observations.
- The candidate differs from the baseline by binary and config digest.
- Rollback restores the exact baseline binary and config digests.
- The unit is never enabled and every created target is absent after cleanup.
- The pre-existing `/usr/local/bin` inode and metadata are restored exactly.
- Existing race, static-analysis, privacy, secret, container, and release gates
  remain green.

## Explicit non-claims

- Synthetic command shims do not validate real PVE ACLs or `/dev/zfs` access.
- The short VM run is not sustained-sampling or controlled-load evidence.
- No alert, notification, journal import, actuator, or storage limit is enabled.
- Passing this rehearsal does not authorize a production observer install.
