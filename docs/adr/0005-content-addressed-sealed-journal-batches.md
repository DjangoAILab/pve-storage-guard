# ADR-0005: Use content-addressed batches for sealed journal import

## Status

Accepted for observer/shadow transport. This does not approve an ITOps runtime
integration or production deployment.

## Context

A journal path is not an immutable authorization resource. Verifying a path,
releasing the lock, and later reading it from another process creates a
time-of-check/time-of-use window. Streaming a whole 256 MiB private journal is
also a poor fit for a bounded importer, while copying it creates another
sensitive artifact.

The public project must remain platform-neutral at the handoff boundary. It
cannot depend on an ITOps API, credential, database, or approval model.

## Decision

- Compute `sha256:<hex>` over the exact sealed journal bytes while holding the
  existing shared lock and include it in the identity-free verification summary.
- Add a private `DecisionJournalBatch` contract and `journal batch` command.
- Require the caller to provide the already-reviewed content digest. The command
  scans and validates the complete journal, compares the digest, checks the
  target before and after the scan, and returns at most 64 events only after all
  checks pass.
- Bind pagination to `(contentDigest, offset, limit)`. Every call revalidates the
  complete artifact; retries are safe and batches cannot silently switch files.
- Keep batch output local and explicit. It contains private identifiers and must
  be piped directly into an authorized consumer, never published or logged.

## Consequences

### Positive

- Approval can name immutable content instead of a mutable path.
- The verifier and batch reader share file safety, schema validation, and size
  bounds.
- Memory stays bounded by one batch rather than the complete journal.
- No network, credential, database, or ITOps dependency enters the public core.

### Negative

- Every batch rescans the complete journal, so large journals have increasing
  offline import cost. This is acceptable for the first operator-driven PoC and
  must be benchmarked before production use.
- SHA-256 proves content equality, not creator authenticity. A principal able to
  replace the file and obtain a new approval can still provide forged content.
- Batch output is private even though the verification summary remains
  identity-free.

## Alternatives considered

- **Approve a path:** rejected because the file can change after approval.
- **Let ITOps read after `journal verify`:** rejected because Node cannot retain
  the verifier's lock and file identity across the second read.
- **Copy to an export file:** rejected because it creates another long-lived
  sensitive artifact.
- **Network exporter:** deferred because it introduces credentials, retry queues,
  and controller/ITOps availability coupling.
- **Cryptographic signing/hash chain:** valuable future provenance work, but it
  requires key lifecycle and trust-root decisions beyond content equality.

## References

- [ADR-0004: Verify sealed journals before external ingestion](0004-sealed-journal-handoff.md)
- [ITOps integration](../ITOPS-INTEGRATION.md)
