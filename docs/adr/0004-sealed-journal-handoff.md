# ADR-0004: Verify sealed journals before external ingestion

## Status

Accepted — 2026-08-02

## Context

The shadow controller can append and sync a private JSONL decision journal, but
ITOps ingestion must not weaken controller isolation or read partially written
events. Live tailing couples an external reader to the active writer and storage
failure domain. A controller callback introduces network and credential handling
into the safety process. Blind file import cannot distinguish an unsafe target,
truncated record, forged event linkage, or a journal reused across storage
domains.

## Decision

- The active journal remains a local, single-writer file with no network export.
- External handoff uses a sealed file: stop or detach the writer, rotate the file
  under an operator-controlled procedure, then verify it before any importer can
  accept it.
- Verification is read-only, takes a non-blocking shared lock, and refuses an
  active exclusive writer, symlink, non-regular file, unsafe permissions, file
  over 256 MiB, event over 1 MiB, unknown field, invalid shadow safety state,
  forged event linkage, inconsistent observation age, or multiple storage
  domains.
- Successful output is a versioned, identity-free summary. It may expose counts
  and event time bounds but never domain, resource, observation, proposal, or
  event identifiers.
- Duplicate event IDs and timestamp regressions are counted for downstream
  review rather than silently removed. Verification performs no persistence,
  deduplication, sorting, incident creation, or actuation.
- Structural verification is not a cryptographic authenticity proof. File
  signing or hash chaining requires a separate decision.

## Consequences

- ITOps can build an explicit import step around a stable, bounded artifact
  without tailing the controller's active file.
- Handoff is not real-time. That is acceptable for decision audit; operational
  alerts continue to use the independent metrics path.
- Rotation and import remain operator approval points, and the private journal
  must remain outside the guarded storage domain.
- A valid summary does not authorize production ingestion or publication of the
  underlying private journal.

## Alternatives

- **Tail the active journal:** rejected because lock-free readers can observe
  partial writes and couple audit availability to the writer.
- **Send events directly from the controller:** rejected because network,
  credentials, retry queues, and backpressure do not belong in the safety
  controller.
- **Batch or asynchronously sync events:** rejected for now because it changes
  the existing persist-before-proposal durability contract.
