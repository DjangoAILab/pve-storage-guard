# ADR-0008: Separate licensed workload-shape artifacts from replay evidence

## Status

Accepted — 2026-08-03. This partially supersedes ADR-0006's blanket
no-redistribution rule only for explicitly licensed, identity-reducing,
aggregated workload-shape artifacts.

## Context

The UMass/SPC traces are observed and licensed for reuse, but their standardized
records contain issue time, operation, and size without portable latency,
storage-class, or management-plane fields. Forcing those records into
`ReplayTrace` would require a fictional wait statistic and would weaken the
promotion contract. Refusing every such source would leave workload-shape
generalization untested.

## Decision

- Introduce a separate `WorkloadShapeTrace` research contract.
- Preserve only relative bucket offsets, read/write IOPS, and read/write
  throughput.
- Drop ASU, LBA, optional fields, raw timestamps, and the raw source archive.
- Require an observed source, a distinct independence group, a known workload
  class, at least 600 seconds, 95% structural completeness, explicit
  authorization/sanitization confirmation, and human license review.
- Make every workload-shape assessment report
  `active_control_eligible=false`; latency and management evidence cannot be
  joined or inferred by this contract.
- Permit a small derived artifact only when attribution, source and artifact
  hashes, license, and exact transformations are committed beside it.

## Consequences

- Observed workload burstiness can be tested without inventing latency.
- A workload-shape artifact cannot close storage-class or active-control gates.
- The privacy scanner covers committed PoC fixtures as publication surfaces.
- Raw third-party traces remain outside Git history and release assets.

## Alternatives

- **Emit `waitValid=false` ReplayTrace samples:** rejected because the required
  wait-statistic label would still suggest a latency semantic the source lacks.
- **Bundle the original trace:** rejected because it is unnecessary, large, and
  retains logical-address coordinates.
- **Treat workload shape as promotion evidence:** rejected because no storage
  response or management availability was observed.
