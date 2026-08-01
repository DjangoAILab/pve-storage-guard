# Shadow Stream Performance Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add reproducible local CLI benchmarks for the default shadow stream and its opt-in private decision journal, without claiming production or PVE adapter performance.

**Architecture:** Exercise the existing `run` command entrypoint with a fixed batch of schema-valid JSONL observations, real policy and enrollment files, discarded proposal output, and either no journal or a private temporary journal. Keep the benchmark harness in the command test package so production behavior does not change; include the command package in the existing short CI benchmark smoke test.

**Tech Stack:** Go `testing.B`, existing CLI/config/controller/telemetry packages, GitHub Actions, Markdown/Astro documentation.

---

### Task 1: Add the CLI benchmark harness

**Files:**
- Create: `cmd/pve-storage-guard/main_benchmark_test.go`

**Step 1: Add a fixed-batch input builder**

Build newline-delimited, schema-valid observations with deterministic public-only identifiers and one current timestamp. Keep setup outside the timed loop.

**Step 2: Add the default shadow benchmark**

Call the real `run` entrypoint repeatedly with the example policy and enrollment, the prebuilt input, and `io.Discard` outputs. Fail the benchmark on a non-zero exit and report bytes plus observations per operation.

**Step 3: Add the private-journal benchmark**

Use a benchmark temporary directory and the real `--journal` option. This path must include validation, append, and `fsync`; it must not bypass or weaken journal safety.

**Step 4: Run focused benchmarks**

Run:

```sh
go test -run '^$' -bench 'BenchmarkShadowCommand' -benchtime=100ms -benchmem ./cmd/pve-storage-guard
```

Expected: both benchmark paths pass, the journal path is materially slower because each event is synced, and no fixed latency threshold is enforced.

### Task 2: Add CI smoke coverage

**Files:**
- Modify: `.github/workflows/ci.yml`

**Step 1: Add the command package to the existing benchmark job**

Keep the existing `100ms` smoke duration and add `./cmd/pve-storage-guard`. Do not add an absolute performance gate on shared runners.

**Step 2: Validate the workflow and smoke command locally**

Run the exact expanded benchmark command and check YAML syntax through the repository's normal CI path.

### Task 3: Record repeated local evidence and limits

**Files:**
- Modify: `docs/PERFORMANCE.md`
- Modify: `website/src/content/docs/evidence/performance.md`
- Modify: `docs/CHECKLIST.md`

**Step 1: Run five repeated 500 ms samples**

Run:

```sh
go test -run '^$' -bench 'BenchmarkShadowCommand' -benchtime=500ms -benchmem -count=5 ./cmd/pve-storage-guard
```

Record the observed ranges, machine/runtime identity, batch sizes, and allocations. Treat journal numbers as local filesystem durability cost, not a universal expectation.

**Step 2: Document scope boundaries**

State explicitly that the benchmark includes local config reads, JSONL decode/encode, policy/allocation, and optional local append+sync, but excludes metric collection, PVE API/network latency, durable ITOps stores, QEMU changes, and host read-back.

**Step 3: Run full verification**

Run formatting, `go vet`, race tests, staticcheck, govulncheck, Python replay/golden verification, JSON validation, docs build, and Gitleaks. Push through a protected-branch PR, wait for all required checks, then verify the deployed Pages content.
