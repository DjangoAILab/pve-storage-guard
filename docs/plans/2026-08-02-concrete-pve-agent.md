# Concrete PVE agent implementation plan

## Goal

Ship a testable, public, read-only PVE inventory and metrics agent for one
explicitly bound OpenZFS storage domain. The agent must preserve metric
semantics and emit no private infrastructure identity on stdout.

## Boundaries

- No production connection, installation, service registration, actuation, or
  network delivery in this change.
- No automatic workload enrollment and no read below `/etc/pve/priv`.
- No claim of support for non-ZFS storage until an adapter and fixtures exist.
- Existing observation input remains compatible; new evidence fields are
  additive and optional for existing private replay data.

## Work packages

1. **Contracts and strict configuration**
   - Add versioned agent configuration and schemas.
   - Validate opaque public keys separately from private PVE identifiers.
   - Define typed evidence and corroborating signals.
2. **Pure parsers, test first**
   - Parse PVE cluster status and storage/status JSON fixtures.
   - Parse supported OpenZFS `total_wait` histogram fixtures and compute the
     nearest-rank p95 bucket upper bound.
   - Parse PSI and diskstats without changing their semantics.
   - Reject malformed, oversized, ambiguous, identity-mismatched, and empty
     samples.
3. **Constrained local runner**
   - Map typed operations to fixed `pvesh`/`zpool` argv.
   - Use no shell, a fixed environment, deadlines, and bounded stdout/stderr.
   - Read only allowlisted procfs files.
4. **PVE reader and CLI**
   - Implement inventory verification and one-shot observation.
   - Add `agent inventory` and `agent observe` commands with JSON/JSONL output.
   - Keep private identifiers out of output and sanitized errors.
5. **Audit trail and integration**
   - Preserve wait evidence in shadow decision events.
   - Add schemas, examples, deployment notes, threat model, and runbook.
6. **Verification**
   - Run unit, integration, race, lint/vet, security, fixture, and bounded
     performance tests.
   - Inspect output for private fixture identities and record evidence in
     `docs/CHECKLIST.md`.

## Acceptance criteria

- A fixture-backed PVE ZFS domain emits a valid observation whose p95 comes
  only from a labeled OpenZFS write `total_wait` histogram.
- Diskstats or PSI alone can never set `waitValid=true`.
- Unknown histogram shape, zero write weight, storage mismatch, timeout, or
  truncated output fails closed.
- Public output contains opaque keys only; tests prove private fixture names do
  not occur.
- CLI subprocesses are drawn from an immutable operation mapping and never use
  a shell.
- All repository validation passes and public docs state deployment and support
  limits without overstating production readiness.

