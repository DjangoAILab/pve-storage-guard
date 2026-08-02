# Real PVE/OpenZFS compatibility evidence plan

## Goal

Prove that the public read-only PVE adapter accepts the output shape of the
reference production host without exporting host identity, storage identity,
device identity, addresses, guest data, raw logs, or raw operational metrics.
This is compatibility evidence, not production-policy calibration.

## Safety boundary

- Remote actions are limited to fixed read-only `pvesh`, `zpool`, and exact
  procfs reads. No upload, package install, file write, service change, API
  mutation, guest query, actuator, or limit change is allowed.
- A remote in-memory sanitizer validates private values and output semantics
  before emitting anything. Raw command stdout/stderr never leaves the host and
  is never written locally.
- Public fixtures use fixed aliases and synthetic metric values while retaining
  the observed field types, column labels, column count, and bucket boundaries.
- Only product major/minor compatibility may be retained. Patch/build strings,
  node/storage/pool/device names, addresses, capacities, and timestamps are
  discarded.
- Failure output contains only a stage name and error class. Private argv and
  subprocess stderr are suppressed.
- Any remote or repository write is outside this plan and requires the
  production-write checkpoint.

## Work packages

1. Verify read-only connectivity and required command availability without
   returning host details.
2. Run one bounded remote in-memory compatibility probe that:
   - selects configured `zfspool` entries through the PVE API;
   - validates local node/cluster health and active storage bindings;
   - validates the OpenZFS human header and scripted histogram shape;
   - validates PSI and diskstats grammar;
   - emits aliases, normalized values, and product major/minor only.
3. Independently scan the emitted bundle for forbidden identifiers and ensure
   it contains no raw state that could support capacity or workload inference.
4. Add the sanitized fixture and provenance manifest to adapter `testdata`, then
   drive the real parser tests from that fixture rather than duplicating it in
   test source.
5. Run full race, lint, static analysis, vulnerability, secret, schema, replay,
   container, and Pages checks; update the compatibility matrix and checklist.

## Acceptance criteria

- The probe performs zero remote writes and returns no private identity or raw
  metric value.
- At least one active PVE `zfspool` binding passes the exact public parser
  contracts, including `total_wait` write-column semantics.
- The public fixture is explicitly labeled `observed-shape / synthetic-values`
  and cannot be used as performance or policy evidence.
- Tests fail if the fixture layout, pool binding, JSON types, PSI grammar, or
  diskstats field count changes incompatibly.
- Repository and public CI security gates pass before merge.

