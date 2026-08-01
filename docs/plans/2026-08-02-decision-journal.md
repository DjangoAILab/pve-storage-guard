# Decision Journal Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an opt-in, append-only JSONL decision journal for shadow evaluations without changing proposal stdout or enabling actuation.

**Architecture:** `api/v1` owns the versioned event contract, while a small
`internal/telemetry` package constructs and appends events. The `shadow` command
opens a journal only when `--journal` is supplied, writes and syncs each event
before exposing the corresponding proposal on stdout, and fails closed on
journal errors. There is no network exporter, ITOps callback, rotation manager,
or default filesystem write.

**Tech Stack:** Go standard library, JSON Schema draft 2020-12, existing Go and
CLI test harnesses.

---

## Design decision

Three approaches were considered:

1. Reuse proposal JSONL as the journal. This preserves compatibility but omits
   observation evidence and a stable event envelope.
2. Add an explicit local JSONL journal while preserving proposal stdout. This
   supplies an auditable handoff without coupling controller timing to ITOps.
3. Push events directly to ITOps or OpenTelemetry. This adds credentials,
   retries, backpressure, and a new failure dependency before a baseline exists.

Use approach 2. Journaling remains opt-in; new files are mode `0600`; symlinks,
non-regular files, and existing files accessible by group/other are rejected.
The event contains opaque domain/resource keys but never raw device details,
guest data, network addresses, credentials, command output, or actuator state
that shadow mode did not evaluate.

### Task 1: Versioned decision-event contract

**Files:**

- Modify: `api/v1/types.go`
- Create: `api/v1/schema/decision-event.schema.json`
- Create: `internal/telemetry/decision_test.go`
- Create: `internal/telemetry/decision.go`

**Steps:**

1. Write failing tests proving stable event IDs, observation evidence mapping,
   proposal linkage, and `actuationAllowed=false`.
2. Run `go test ./internal/telemetry` and confirm the package is missing.
3. Add typed nested event fields and a pure `NewShadowDecisionEvent` builder.
4. Add the strict JSON Schema with `additionalProperties: false` at every level.
5. Run `gofmt` and `go test ./internal/telemetry`; expect PASS.

### Task 2: Fail-closed append-only journal

**Files:**

- Create: `internal/telemetry/journal_test.go`
- Create: `internal/telemetry/journal.go`

**Steps:**

1. Write failing tests for creation at `0600`, appending two complete JSONL
   records, rejection of symlinks/non-regular files/insecure existing modes,
   and append after reopen.
2. Run `go test ./internal/telemetry`; expect FAIL.
3. Implement `OpenJournal`, `Append`, and `Close` with regular-file checks,
   JSON marshal plus newline, append-only opening, and `Sync` after each event.
4. Run `go test ./internal/telemetry`; expect PASS, including race mode.

### Task 3: Opt-in shadow CLI integration

**Files:**

- Modify: `cmd/pve-storage-guard/main.go`
- Modify: `cmd/pve-storage-guard/main_test.go`

**Steps:**

1. Write failing tests proving no journal is created by default, an explicit
   journal receives one event while stdout remains a proposal, append across
   runs, and an invalid journal target fails before any proposal is emitted.
2. Run `go test ./cmd/pve-storage-guard`; expect FAIL.
3. Add `--journal FILE`, open it after all read-only configuration validation,
   append/sync the decision event before encoding the proposal, and close with
   error propagation.
4. Update CLI usage without implying production durability or ITOps delivery.
5. Run the focused package tests and `go test -race ./...`; expect PASS.

### Task 4: Documentation and evidence

**Files:**

- Modify: `README.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/ITOPS-INTEGRATION.md`
- Modify: `docs/CHECKLIST.md`
- Modify: `website/src/content/docs/getting-started.md`
- Modify: `website/src/content/docs/operations/itops.md`

**Steps:**

1. Document opt-in behavior, ownership, permissions, append/sync semantics,
   failure policy, opaque-key privacy boundary, and lack of network delivery.
2. Record that ITOps still needs an approved ingestion/linking adapter.
3. Run `git diff --check`, JSON Schema validation, full Go race tests, all 19
   Python replay tests, website build, and Gitleaks.
4. Review the diff for internal identifiers and unsupported durability claims.
5. Commit, push a protected-branch PR, wait for all required checks, merge, and
   verify the deployed Pages content.
