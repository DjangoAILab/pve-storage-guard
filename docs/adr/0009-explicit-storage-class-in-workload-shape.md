# ADR-0009: Preserve explicit storage class in workload-shape evidence

## Status

Accepted — 2026-08-03. This supersedes ADR-0008 only where it said that every
workload-shape artifact is unable to close a storage-class research gate.

## Context

ADR-0008 introduced latency-free `WorkloadShapeTrace` evidence using an SPC
source whose storage class is unknown. Some licensed sources explicitly
identify a logical storage product class even though they still lack latency
and management-plane evidence. Treating that class as unknown discards useful
provenance; treating it as active-control evidence overstates what was
measured.

Alibaba Block Traces explicitly describes its observed source as production
Elastic Block Storage Ultra Disk requests received by a virtual block service.
It does not identify the physical media serving each request.

## Decision

- Upgrade the draft workload-shape contract to v1alpha2.
- Require `storageClass`, allowing the same conservative vocabulary used by the
  replay contract; use `network-block` for the documented virtual EBS class and
  `unknown` when the source does not make a supported claim.
- Preserve timestamp and I/O-layer semantics independently: issue time at a
  host-to-logical-unit boundary and arrival time at a virtual block service are
  not interchangeable.
- Report a distinct `meets_storage_class_research_gate` result only when the
  general research gate passes and `storageClass` is known.
- Continue to report `active_control_eligible=false` for every
  `WorkloadShapeTrace`; storage class does not create latency or synchronized
  management evidence.
- Do not infer HDD, SSD, or NVMe media behind a distributed or network block
  product.

## Consequences

- Licensed workload shape can now test a documented logical storage class
  without being forced into `ReplayTrace`.
- Storage-class research coverage and production promotion remain separate
  gates.
- v1alpha1 was still draft-only and is replaced before merge; no published
  release depends on it.
- Raw third-party data remains outside Git history and release assets.
