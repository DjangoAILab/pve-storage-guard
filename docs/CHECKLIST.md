# Execution checklist

Status values: `[x]` complete, `[~]` in progress, `[ ]` pending, `[!]` blocked
by an explicit safety checkpoint. Evidence links are repository-relative unless
marked as local-only.

Last updated: 2026-08-02 (Asia/Shanghai)

## 1. Current state and data safety

- [x] Create an isolated local repository under the established GitHub project
  workspace convention.
- [x] Confirm target GitHub repository does not already exist.
- [x] Inspect the ITOps main worktree; it is clean at the start of this project.
- [x] Inspect the isolated storage-controller PoC worktree; preserve its
  uncommitted experimental files without rewriting or publishing them.
- [x] Inventory the currently available incident evidence and its sampling
  granularity. Evidence: [PoC](POC.md#available-historical-evidence).
- [x] Define public sanitization rules. Evidence:
  [incident case study](CASE-STUDY.md#sanitization-contract).
- [x] Create anonymized fixtures and scan them for host, VM, network, pool, and
  workload identifiers. Evidence: `poc/fixtures/` and the 2026-08-01 local
  sensitive-identifier scan.

## 2. PoC and policy evaluation

- [x] Define no-control, fixed-20, threshold-step, and AIMD baselines.
- [x] Separate observed shadow replay from counterfactual modeling.
- [x] Define conservative, nominal, and optimistic storage models.
- [x] Define a safety-first selection gate and bounded parameter search.
- [x] Migrate the replay code and anonymized fixtures into this repository.
- [x] Run all 19 offline Python tests and reproduce Markdown/JSON reports locally.
  Evidence: `poc/results/report.md` and `poc/results/report.json`.
- [x] Add estimated job completion time and hourly decision churn to the report.
- [x] Record unavailable historical IOPS, PSI, queue, and management-plane
  samples as evidence gaps; do not synthesize them as observations.
- [ ] Add at least one independent trace before considering active control. A
  strict replay-trace schema and dependency-free assessor now reject synthetic,
  same-group, incomplete, unknown-class, and non-p95 evidence; the local audit
  found only same-episode aggregates without replayable samples.
- [x] Run one-at-a-time parameter-neighborhood sensitivity analysis. Evidence:
  `poc/results/report.md`; 16/18 neighbors pass, while faster additive increase
  and shorter healthy confirmation fail the safety gate.
- [ ] Add independent storage-class and workload-shape traces; do not relabel
  the three counterfactual capacity models as observed storage classes.
  ADR-0003 now makes metric statistic, provenance, and independence group
  mandatory trace fields.
- [!] Promote an adaptive policy beyond shadow mode only after PoC and live
  evidence gates pass.

## 3. Controller architecture

- [x] Select PVE product layer plus generic `storage-slo-guard` kernel.
  Evidence: [ADR-0001](adr/0001-pve-product-generic-policy-kernel.md).
- [x] Select one controller actor per storage domain.
- [x] Define PVE Adapter, Metrics Collector, Policy Engine, Actuator, Safety
  Controller, and Event/Telemetry boundaries.
- [x] Define exact disk > workload class > pool default policy precedence.
- [x] Exclude root and critical disks by default.
- [x] Define desired/effective state, lease, cooldown, and read-back contracts.
- [x] Implement versioned JSON schemas, Go API contracts, and in-code policy
  validation. Evidence: `api/v1/schema/`, `api/v1/`, and `internal/policy`.
- [x] Implement the deterministic controller and bounded allocator with unit
  tests under `internal/policy` and `internal/allocator`.
- [x] Implement read-only adapter and constrained actuator contracts without
  production mutation under `internal/adapter/pve` and `internal/actuator/pve`.
- [x] Implement the versioned JSONL shadow stream, strict configuration
  decoding, exact enrollment, telemetry-age checks, and non-actuating proposal
  output. An opt-in private append/sync decision journal now records the
  versioned event before its proposal reaches stdout and fails closed on journal
  errors. Evidence: `internal/controller`, `internal/config`,
  `internal/telemetry`, and `cmd/pve-storage-guard`.

## 4. Safety validation

- [x] Make read-only/dry-run the default architecture.
- [x] Specify hard minimum/maximum limits, hysteresis, timeout, cooldown, and
  emergency behavior.
- [x] Specify stale-input, restart, drift, and read-back mismatch behavior.
- [x] Add property/fuzz tests for controller bounds, homogeneous healthy/
  pressure monotonicity, allocator envelopes, feasibility, and input-order
  invariance. Evidence: race tests pass; dedicated 5-second fuzz runs exercised
  about 3.06 million policy inputs and 3.00 million allocator inputs on
  2026-08-02 without a failure.
- [x] Add restart, lease conflict, stale telemetry, and actuator fault tests.
  Restored cooldown/emergency behavior, impossible-state rejection,
  stale-input, and wrong-domain shadow tests pass. The generic safety-gate
  suite also validates authoritative lease and approval stores plus fencing
  generation, rejects conflicting/expired/unavailable authority before the
  actuator, and freezes a resource on effective-state drift, actuator failure,
  or read-back mismatch. The gate is not wired to a real actuator; durable
  authority storage, authorized restoration, and production enablement remain
  v0.2 gates.
- [ ] Complete a non-critical controlled load test.
- [!] Install or change a service on a production PVE host only after explicit
  approval at the production-write checkpoint.
- [!] Enable canary actuation only after explicit approval and a reviewed
  rollback command/state snapshot.

## 5. Open-source engineering

- [x] Persist required project documents and ADRs locally.
- [x] Add Apache-2.0 license, DCO, contributing guide, security policy, code of
  conduct, support/governance policy, Dependabot, and issue/PR templates.
- [~] Add unit, shadow CLI, replay, trace-qualification, and performance smoke
  tests. Decision-journal tests cover deterministic event mapping, append after
  reopen, private permissions, unsafe-target rejection, exclusive-writer
  locking, the 256 MiB hard bound, invalid-event rejection, CLI compatibility,
  and fail-before-output behavior. Repeated local benchmarks cover policy
  evaluation, 100-disk allocation, and the verified safety gate; end-to-end
  adapter/storage performance and controlled-load coverage remain.
- [x] Add pinned GitHub Actions for lint, test/race, replay golden, CodeQL,
  govulncheck, secret scan, OCI build/Trivy, docs build, and Pages.
- [x] Add multi-architecture release workflow with pinned actions, binary/image
  SBOM, provenance, checksums, and keyless container signing.
- [x] Build the GitHub Pages landing/documentation site; local Astro build has
  zero diagnostics and responsive browser review has no horizontal overflow.
- [~] Add verified architecture and incident/control-loop graphics; modeled
  policy effect and real ITOps dashboard graphics remain pending.
- [x] Complete the initial pre-publication review: identifier/IP/private-key
  pattern scan clean, Gitleaks v8.30.1 reports no leaks, Markdown links resolve,
  screenshots contain anonymized content and standard JFIF/sRGB metadata only.
- [x] Create and push the public GitHub repository only after a clean sensitive
  information scan and review of publishable artifacts. Evidence:
  [DjangoAILab/pve-storage-guard](https://github.com/DjangoAILab/pve-storage-guard).
- [x] Validate the repaired main CI across test/race, lint, static analysis,
  govulncheck, replay golden, secrets, OCI build, and Trivy. Evidence:
  [latest GitHub Actions run](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30708897639).
- [x] Enable and deploy GitHub Pages, set the repository homepage, verify HTTPS
  200, title/disclaimer content, and desktop width in a browser. Evidence:
  [public documentation site](https://djangoailab.github.io/pve-storage-guard/)
  and [Pages deployment](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30707972440).
- [x] Protect `main` with up-to-date PR branches, required test/lint/secret/
  container/CodeQL checks, resolved conversations, linear history, and disabled
  force-push/deletion. Administrator bypass remains available for documented
  emergency recovery in this single-maintainer bootstrap phase.
- [!] Make the tagged GHCR image publicly readable. The validated
  `v0.1.0-rc.1` release successfully built, signed, attested, and uploaded the
  `linux/amd64` and `linux/arm64` image, but GitHub still reports the package as
  private. Changing package visibility requires an authenticated owner browser
  session and is irreversible; an isolated no-credential manifest check still
  returned `unauthorized` on 2026-08-02.

## 6. Local practice and ITOps

- [x] Define the monitoring, alert, event, and runbook contract.
- [x] Run the observer locally in shadow mode and capture a structurally valid,
  non-actuating proposal (`actuationAllowed=false`). Evidence: documented
  quick-start plus the 2026-08-01 local test run.
- [ ] Verify that it detects the historical incident signature early enough to
  be operationally useful.
- [~] Add ITOps ingestion for storage latency, PSI, queue, management-plane
  health, and controller state. Internal draft PR #37 now has a
  read-only `pve.storage-pressure` probe and mappings for PSI, in-flight I/O,
  disk counters, derived IOPS/throughput/average wait/queue/utilization, and
  management-probe health. A pure sanitized replay-trace builder now preserves
  fixed `average`/`derived` diskstats semantics, relative offsets, declared
  gaps, and excludes internal selection identifiers; it has no route, writer,
  or deployment path. Merge, deployment, true p95 telemetry, trace review, and
  journal transport/persistence, true p95 telemetry, trace review, and incident
  linking remain. The public controller provides a tested opt-in private JSONL
  decision journal, and the ITOps draft now has a pure strict mapper returning
  an inert audit handoff plus low-cardinality derived metrics; neither side has
  network delivery or a repository write. Evidence: ITOps commits `90b04b0`,
  `44df889`, `242a7ff`, `c236d06`, and `4fe5b97`; all 1,240 backend tests across
  146 files, 16 focused PVE/runtime tests, 11 restricted-probe tests, four
  replay-export tests, four decision-mapper tests, build/lint, and dependency
  checks passed locally on 2026-08-02. The latest internal CI quality gate
  passed in 4m31s; the dependent linux/amd64 image job remained blocked by
  required conditions while the PR stayed Draft, so it is not recorded as a
  successful image build.
- [~] Add multi-signal warning/critical alerts and anti-noise behavior. The
  detector now requires write-wait plus PSI, queue, or management-plane
  corroboration and seeds disabled warning/critical rules. A persisted SQLite
  test now verifies two-sample debounce across evaluator restart, one firing,
  and one recovery with detector evidence attached; shadow-baseline tuning,
  real notification delivery, and explicit enablement remain.
- [~] Add dashboard, decision journal, and incident-review links. Internal
  Draft PR #37 now includes a tested PVE-only storage-pressure dashboard for
  PSI, management health, per-disk pressure evidence, and alert gates. The
  controller now has a private, append/sync shadow decision journal; a real
  shadow-baseline screenshot, reviewed journal reader/persistence adapter, and
  incident/runbook links remain. The ITOps pure mapper alone performs none of
  those side effects.
- [ ] Exercise cold restart and rollback in a non-critical environment.
- [!] Expand production control only after canary evidence is reviewed.

## Current risks and evidence gaps

- The historical store contains one-minute VM write counters but did not retain
  one-second storage latency, PSI, or queue history.
- The twelve one-second wait samples are an incident window, not a long-running
  baseline.
- Counterfactual outcomes depend on explicit monotonic pool models and are not
  causal proof.
- One incident cannot justify a universal default across SSD, HDD, ZFS special
  vdev, Ceph, or network-backed storage.
- GitHub organization policy visibility is limited by the current token scope;
  repository-local branch/ruleset and workflow protections must be verified
  independently before release.
