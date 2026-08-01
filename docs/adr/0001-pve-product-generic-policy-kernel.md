# ADR-0001: PVE product layer with a generic policy kernel

## Status

Accepted — 2026-08-01

## Context

The immediate operational problem is on Proxmox VE, whose inventory, storage
relationships, and disk throttling semantics are platform-specific. The control
problem itself is broader: protect a shared storage SLO by assigning a bounded
budget to enrolled consumers.

A generic-first product would delay a useful PVE integration and obscure its
operator experience. A PVE-coupled algorithm would prevent independent replay,
testing, and future adapters.

## Decision

Build **PVE Storage Guard** as the product and distribution. Keep PVE inventory,
identity mapping, metrics details, and actuation behind a PVE Adapter. Implement
the deterministic platform-neutral policy and allocation core as the internal
`storage-slo-guard` package.

The public CLI/service is `pve-storage-guard`; the first container is
`ghcr.io/djangoailab/pve-storage-guard`. The generic engine is not initially a
separate deployable product or repository.

## Consequences

### Positive

- The first release has a clear user, terminology, quick start, and test target.
- Policy replay and safety tests do not require PVE.
- Linux, Kubernetes, and other virtualization adapters can be added without
  changing the policy state machine.
- Privileged platform code stays small and reviewable.

### Negative

- Adapter contracts require deliberate versioning.
- Some metrics and actuation capabilities will not map perfectly across
  platforms.
- The internal generic package must resist importing PVE-specific types.

## Alternatives

- **Generic storage controller as the initial product:** rejected because it
  weakens the initial operator experience and expands scope before validation.
- **PVE-only monolith:** rejected because it couples privileged operations to
  policy logic and makes offline testing harder.
- **Two repositories immediately:** rejected because it adds release and API
  coordination before the boundary has field evidence.
