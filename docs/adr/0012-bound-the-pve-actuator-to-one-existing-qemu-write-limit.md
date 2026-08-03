# ADR-0012: Bound the PVE actuator to one existing QEMU write limit

## Status

Proposed — offline core only, 2026-08-04

## Context

The platform-neutral Safety Gate already verifies an authoritative lease and
approval, immutable policy version, enrollment envelope, expected effective
state, and read-back. It freezes a resource after drift, an apply error, or a
read-back mismatch. The PVE product layer still needs a concrete implementation
of the narrow platform operation beneath that gate.

Proxmox QEMU disks support write bandwidth limits and the QEMU configuration
write API accepts a SHA-1 configuration version digest. It is a concurrency
token, not an authenticity primitive. Sending it causes a concurrent
configuration change to reject the write. A broad command runner,
guest lifecycle permission, or automatic conversion from an unlimited disk
would enlarge the failure boundary unnecessarily.

## Decision

- Bind one actuator instance to one owner-only `PVECanaryPreflightConfig`. Add
  an opaque `resourceKey` so the generic enrollment and the private PVE disk
  cannot be confused.
- Support only QEMU `ide`, `sata`, `scsi`, and `virtio` data disks already
  carrying a positive `bps_wr` value inside the approved envelope. Express
  desired MiB/s as an exact integer byte rate; do not use the ambiguous
  decimal-megabyte `mbps_wr` field.
- Before every effective-state read and apply, revalidate the exact node,
  workload, disk, storage, required tags, unlocked state, non-boot role,
  writable data-disk role, and existing bounded rate.
- Reject every other `bps*`, `mbps*`, or `iops*` option. The actuator must not
  silently combine, remove, or reinterpret an existing limiter.
- Read the QEMU configuration and its digest, submit the complete disk value
  with only `bps_wr` changed, and pass that digest as the concurrency fence.
  Read the configuration again and require the volume plus every unmanaged disk
  option to remain identical.
- Return the actual read-back rate to the generic Safety Gate. A different
  rate becomes `readback_mismatch`; an ambiguous update, read error, or binding
  drift becomes an apply failure. Both paths freeze the resource and prohibit
  automatic retry.
- Keep rollback explicit. The configured rollback value is another approved,
  fenced apply after an operator-owned checkpoint; the actuator does not
  schedule or guess a rollback after an ambiguous write.
- Define only a two-method injected backend: read one QEMU config and update one
  exact disk with a digest. Do not add a shell runner, PVE API client, CLI,
  listener, service unit, container entrypoint, credentials, or production
  configuration in this change.

## Consequences

- Unit and Safety Gate tests can exercise the complete mutation state machine
  without creating a production capability.
- An unlimited disk cannot be the first canary. An operator must deliberately
  establish and verify a bounded recovery baseline through a separately
  reviewed process.
- Hot-plug, lock, tag, boot-order, storage, rate-option, or unmanaged-option
  drift fails closed.
- A future live backend requires its own ADR/security review, least-privilege
  PVE permission proof, non-critical candidate, controlled-load evidence,
  rollback rehearsal, and explicit production approval.

## References

- [Proxmox VE `qm(1)` disk options and digest](https://pve.proxmox.com/pve-docs/qm.1.html)
- [ADR-0011: Separate canary eligibility from actuation](0011-separate-canary-eligibility-from-actuation.md)
