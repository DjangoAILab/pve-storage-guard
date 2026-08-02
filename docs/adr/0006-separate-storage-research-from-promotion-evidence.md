# ADR-0006: Separate storage-only research from promotion evidence

## Status

Accepted — 2026-08-02

## Context

Most public storage traces contain workload or block-I/O data without a
synchronized management-plane probe. ReplayTrace v1alpha1 represented
management health as a required boolean and had no `unknown` state. It also
computed sample completeness without excluding `waitValid=false` placeholders.
An importer could therefore fabricate healthy management samples or make an
incomplete wait series appear complete.

## Decision

- Keep v1alpha1 readable for diagnostics but ineligible for machine promotion,
  and introduce v1alpha2 for new exports.
- Replace the v1alpha1 management boolean with `managementPlaneStatus` values
  `healthy`, `unhealthy`, or `unknown` in v1alpha2.
- Require v1alpha2 to declare the write-wait measurement layer; only a
  storage-domain p95 matches the current controller observation contract.
- Report structural, valid-wait, and known-management completeness separately.
- Require all three completeness measures to reach 95% for the machine
  independence gate.
- Permit authorized storage-only traces as research inputs while requiring them
  to preserve management status as `unknown`.
- Provide no dataset downloader and redistribute no third-party trace data.

## Consequences

- External per-I/O traces can test p95 aggregation, workload shape, and
  controller behavior without being misrepresented as host-availability or
  storage-domain proof.
- A valid research trace can remain ineligible for production promotion; this
  is an intentional result rather than a validation failure.
- New ITOps exporters should emit v1alpha2 and join only time-aligned, genuinely
  observed management samples. Existing v1alpha1 exports remain assessable but
  must be regenerated as v1alpha2 before promotion.
- Human license, provenance, redaction, and independence review remains outside
  the machine validator.

## Alternatives

- **Assume missing management data is healthy:** rejected because absence of a
  measurement is not evidence of availability.
- **Reject every storage-only trace structurally:** rejected because the data is
  still useful for a clearly separated research lane.
- **Download and bundle a public candidate:** rejected because licenses and
  source terms vary, datasets are large, and no candidate closes the management
  evidence gap.
