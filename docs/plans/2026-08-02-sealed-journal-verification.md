# Sealed Journal Verification Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a bounded, read-only verifier for sealed private decision journals and expose an identity-free, versioned CLI summary suitable for a future explicit ITOps import gate.

**Architecture:** Keep file safety and JSONL validation in `internal/telemetry`; the command only parses flags and encodes a public summary. Verification acquires a non-blocking shared lock, rejects active writers and unsafe files, validates every event and cross-record domain invariants, and performs no rotation, network, database, alert, or actuator action.

**Tech Stack:** Go standard library, `syscall.Flock`, JSON Schema Draft 2020-12, existing versioned API and GitHub Actions.

---

### Task 1: Define the versioned identity-free result

**Files:**
- Modify: `api/v1/types.go`
- Create: `api/v1/schema/journal-verification.schema.json`

**Steps:**

1. Add a `DecisionJournalVerification` wire type with schema version, fixed kind,
   event/changed/policy-version/duplicate/regression counts, and optional first
   and last recorded times.
2. Add a strict JSON schema with no identifier-bearing fields.
3. Validate the schema as Draft 2020-12 and add a serialization test through the
   CLI in Task 3.

### Task 2: Implement strict sealed-file verification with TDD

**Files:**
- Create: `internal/telemetry/verify.go`
- Create: `internal/telemetry/verify_test.go`
- Modify: `internal/telemetry/journal.go`
- Modify: `internal/telemetry/journal_test.go`

**Steps:**

1. Write failing tests for a valid journal summary, empty journal, duplicate IDs,
   timestamp regressions, multiple domains, active writer lock, symlink, unsafe
   mode, over-limit line, unknown field, truncated JSON, forged event ID, and
   inconsistent observation age.
2. Strengthen the common event validator so append and verify both reject a
   forged event ID or an age mismatch with a 2 ms numeric tolerance.
3. Implement read-only open/Lstat/Fstat/SameFile checks, the existing 256 MiB
   file bound, non-blocking shared lock, 1 MiB scanner bound, strict one-object
   JSON decoding, and unchanged-size verification after the scan.
4. Aggregate only bounded counts and time ranges; retain raw identifiers only in
   verifier-local maps and never return them.
5. Run focused telemetry tests with the race detector.

### Task 3: Add the read-only CLI

**Files:**
- Modify: `cmd/pve-storage-guard/main.go`
- Modify: `cmd/pve-storage-guard/main_test.go`

**Steps:**

1. Write failing command tests for `journal verify --journal FILE`, active-writer
   rejection, unexpected flags, and absence of private identifiers in stdout.
2. Implement the subcommand as a one-shot call to the verifier and encode exactly
   one JSON summary. Do not add a rotate, delete, import, or network option.
3. Update usage and confirm `shadow` behavior and output remain unchanged.

### Task 4: Document the handoff and verify the release surface

**Files:**
- Modify: `README.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/ITOPS-INTEGRATION.md`
- Modify: `docs/CHECKLIST.md`
- Modify: `website/src/content/docs/operations/itops.md`
- Modify: `website/src/content/docs/getting-started.md`

**Steps:**

1. Document stop/detach, operator rotation, verification, and separately approved
   import as distinct phases. State that verification is structural, not a
   signature or publication approval.
2. Record tests and boundaries in the checklist without claiming that ITOps
   journal persistence exists.
3. Run gofmt, vet, race tests, staticcheck, govulncheck, replay/golden tests, JSON
   schema validation, docs build, Gitleaks, and linux/amd64 container smoke.
4. Submit through a protected-branch PR, wait for all required checks, merge, and
   verify the Pages content.
