# Content-addressed sealed journal batches implementation plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a content digest and bounded private batch command that lets an approved offline consumer read exact sealed journal content without a path-only TOCTOU gap.

**Architecture:** One lock-owning scanner validates and hashes the complete file and optionally retains a requested event range. Public verification emits only identity-free counts plus the digest; private batch output is produced only after the complete scan and expected-digest comparison succeed.

**Tech Stack:** Go 1.26 standard library, syscall flock, JSON Schema Draft 2020-12, existing CLI and GitHub Actions.

---

### Task 1: Version the digest and batch contracts

**Files:**
- Modify: `api/v1/types.go`
- Modify: `api/v1/schema/journal-verification.schema.json`
- Create: `api/v1/schema/decision-journal-batch.schema.json`
- Modify: `api/v1/types_test.go`

1. Write failing serialization/schema tests for canonical `sha256:<64 hex>`, batch offset/count/completion fields, and at most 64 strict decision events.
2. Add typed contracts and Draft 2020-12 schemas.
3. Run focused API and schema tests.

### Task 2: Refactor the sealed scanner and add batch reads

**Files:**
- Modify: `internal/telemetry/verify.go`
- Modify: `internal/telemetry/verify_test.go`

1. Write failing tests for exact raw-byte digest, valid pages, empty page, wrong digest, offset/limit bounds, active writer, and no private return on failure.
2. Refactor file ownership/locking and full validation into one internal scanner.
3. Hash the exact scanner byte stream and retain only the requested range.
4. Run telemetry tests with the race detector.

### Task 3: Expose the explicit private batch CLI

**Files:**
- Modify: `cmd/pve-storage-guard/main.go`
- Modify: `cmd/pve-storage-guard/main_test.go`

1. Write failing CLI tests for `journal batch`, invalid arguments, digest mismatch with empty stdout, pagination, and private-output warnings.
2. Add `--journal`, `--expected-digest`, `--offset`, and `--limit` flags.
3. Keep `journal verify` identity-free and update usage text.
4. Run command tests and benchmarks.

### Task 4: Document and verify

**Files:**
- Modify: `docs/ITOPS-INTEGRATION.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/CHECKLIST.md`
- Modify: `docs/adr/README.md`
- Modify: `website/src/content/docs/operations/itops.md`

1. Document content equality versus authenticity, private batch handling, O(n)
   rescans, and the no-runtime/no-network boundary.
2. Run Go race/vet/staticcheck/govulncheck, Python/golden/schema tests, Astro
   build, Gitleaks, and sensitive-pattern checks.
3. Commit, open a protected PR, and merge only after CI, CodeQL, and Pages pass.
