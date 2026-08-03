# ADR-0011: Separate canary eligibility from actuation

## Status

Accepted — 2026-08-03

## Context

Historical monitoring can show storage pressure, but it cannot prove that a
particular guest disk is non-critical, non-boot, writable, and safe to load.
Names are not an authorization boundary. The production inventory currently
contains no guest with an explicit `non-critical`, `canary`, or
`pve-storage-guard` classification, so automatically choosing a candidate would
silently convert an inference into a production write decision.

The generic safety gate already requires enrollment, lease, approval, bounds,
effective-state continuity, and read-back. The PVE product layer still needs a
smaller read-only gate before controlled load or actuator work begins.

## Decision

- Add an owner-only `PVECanaryPreflightConfig` for one exact QEMU disk.
- Require the live guest to carry both `non-critical` and
  `pve-storage-guard` tags. A display name is never sufficient.
- Use fixed `pvesh` reads only. Verify management health, active ZFS storage,
  unlocked workload state, exact disk existence and storage, data-disk role,
  exclusion from the explicit boot order, writable state, and a static rollback
  limit inside the envelope.
- Emit only identity-free booleans and stable gap codes. Never echo node,
  storage, VM ID, disk key, tags beyond the reviewed names, or raw PVE output.
- Report `requestedMutations=0` and `activeControlEligible=false` even when the
  controlled-load eligibility checks pass.
- Keep load generation, alert arming, actuator implementation, approval/lease
  persistence, apply/read-back, and rollback as later independent gates.

## Consequences

- A missing classification stops the workflow instead of inviting name-based
  guessing.
- The preflight can be tested and deployed without adding a write capability.
- An operator must deliberately classify one workload and data disk before a
  real controlled-load rehearsal can be proposed.
- Passing preflight is necessary but not sufficient for canary actuation.
