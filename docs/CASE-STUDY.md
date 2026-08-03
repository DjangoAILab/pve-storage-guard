# Case study: management-plane starvation on a production PVE node

## Summary

A bulk data operation on a virtual disk saturated shared ZFS write service time.
The pool remained structurally healthy, yet synchronous storage operations and
the host management plane became severely delayed. Ending the initiating user
session did not immediately restore SSH because the storage workload and queued
I/O outlived the interactive session.

## Impact

- SSH and management operations became unreliable or unavailable.
- Durability-sensitive services recorded multi-second sync operations.
- Operators initially had weak attribution because host health and pool health
  did not express the latency SLO failure.
- Recovery depended on reducing the source workload and allowing the queue to
  drain, not merely terminating a client session.

## Observed evidence

- One attributed virtual disk wrote 2,177,396,736 bytes over approximately
  18 seconds: about 115.36 MiB/s.
- Twelve one-second ZFS write-wait samples were retained. Eleven exceeded 50 ms;
  two exceeded 100 ms and the peak was approximately 348.87 ms. The final sample
  fell below the 25 ms gate.
- Independent durability-sensitive events took approximately 4.4 and 9.3
  seconds around the incident window.
- Retained write-wait collection began after the retained management-service
  failure marker. The pressure signature is visible one second after collection
  starts, but that is 54 seconds after the failure marker; advance warning is
  therefore not proven by this history.
- A later 60-sample natural-load check with a 20 MiB/s cap still had 22 samples
  above 25 ms and a 234.065464 ms p99. Controlled load was not started; the cap
  was rejected and rolled back. Only the summary survived, so it is not a
  replayable or independent trace.
- The historical metrics store retained one-minute virtual-disk write counters,
  but not the one-second pool latency, PSI, queue, or management probes needed
  for early detection and causal comparison.

## Root cause

The root cause was not pool failure or lack of free space. It was ungoverned bulk
write demand sharing the same storage failure domain as the host management and
durability-sensitive paths. Monitoring emphasized capacity and structural health
but lacked a storage latency SLO and workload attribution. There was no bounded,
disk-scoped admission/control mechanism to protect management-plane headroom.

## Why ending a session was insufficient

An interactive session is not necessarily the lifetime owner of the I/O it
started. The process may be detached, supervised, running in a guest, or may
have already filled kernel, QEMU, and storage queues. Even after demand stops,
queued synchronous work must drain. Reliable recovery therefore requires
attribution, effective-state verification, and storage-domain signals—not only
session termination.

## What would have detected it earlier

A multi-signal detector should combine:

- pool write-wait p95/p99 or equivalent device latency;
- I/O PSI and queue pressure;
- dominant per-disk write rate/share;
- SSH and PVE API probe latency/success;
- controller/collector freshness.

The alert should distinguish “bulk workload is dominant but SLO healthy” from
“storage pressure plus management degradation,” avoiding a noisy single-rate
threshold.

## What PVE Storage Guard adds

Existing primitives can enforce static QEMU limits, and Linux control groups can
model latency/cost. PVE Storage Guard adds a PVE-oriented, storage-domain feedback
loop with exact disk enrollment, bounded allocation, dry-run/shadow operation,
decision explanations, ITOps integration, and safe apply/read-back/rollback.

The current replay suggests a slow bounded AIMD candidate can admit slightly
more modeled work than fixed 20 MiB/s without worse modeled unsafe time across
three sensitivity scenarios. This is a hypothesis for shadow validation, not a
production effect claim. The observed fixed-cap check also means fixed 20 may
only be used as a model comparator, not advertised as a validated fallback.

## What the first observer rollout proved

The initial production rollout was deliberately split into independent probe
and application checkpoints. The restricted read-only probe update completed,
including an exact rollback-and-retry rehearsal and validation of every allowed
operation. The application checkpoint then stopped at a protected host-state
invariant before containers, the database, credentials, alerts, or registry
state changed. The source transaction and image selector returned to their
previous values.

The rejected candidate had an incomplete allowlist for an existing protected
certificate pair. A separate read-only audit also found one pre-existing
directory whose root mode was broader than the private-tree contract, while all
descendants were already root-owned and private. These are different failure
classes: the candidate defect requires reviewed code and regression tests; the
host drift requires its own exact, non-recursive production approval.

This does not prove the observer or controller is production-ready. It does
prove that staged checkpoints, exact backups, fail-closed validation, and
transactional source recovery prevented a monitoring rollout from becoming a
second incident. The application, alerts, notifications, journal import, and
actuation remain disabled.

## Sanitization contract

The public project may include:

- derived numeric samples and units;
- relative/UTC incident timing needed for replay;
- generic platform/storage descriptions;
- generated charts and model parameters;
- conclusions whose evidence is present in the repository.

It must not include:

- the original host name or identity label;
- internal IP/MAC addresses, domains, UUIDs, serials, pool/device identifiers;
- original VM/CT IDs or disk keys;
- workload, customer, user, session, or application names;
- credentials, tokens, command histories, or unrestricted raw logs;
- guest data or content;
- screenshots containing any of the above.

Aliases use `reference-node`, `bulk-pool`, `bulk-import-workload`, and opaque
resource keys. A pre-publish scan and human review are required before public
repository creation or Pages deployment.

## Corrective-action path

1. Retain one-second storage latency, PSI, queue, per-disk rate/share, and
   management probe metrics.
2. Run PVE Storage Guard in observer and shadow mode.
3. Validate alerts and decisions across independent bursts and quiet periods.
4. Derive and review a storage-domain-specific static rollback limit, then
   load-test one non-critical enrolled disk.
5. Fault-inject stale telemetry, restart, drift, apply/read-back mismatch, and
   rollback.
6. Only then request approval for a time-bounded canary.
