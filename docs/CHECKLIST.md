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
- [x] Re-run all 11 unit tests and reproduce Markdown/JSON reports locally.
  Evidence: `poc/results/report.md` and `poc/results/report.json`.
- [x] Add estimated job completion time and hourly decision churn to the report.
- [x] Record unavailable historical IOPS, PSI, queue, and management-plane
  samples as evidence gaps; do not synthesize them as observations.
- [ ] Add at least one independent trace before considering active control.
- [x] Run one-at-a-time parameter-neighborhood sensitivity analysis. Evidence:
  `poc/results/report.md`; 16/18 neighbors pass, while faster additive increase
  and shorter healthy confirmation fail the safety gate.
- [ ] Add independent storage-class and workload-shape traces; do not relabel
  the three counterfactual capacity models as observed storage classes.
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
  output. Evidence: `internal/controller`, `internal/config`, and
  `cmd/pve-storage-guard`.

## 4. Safety validation

- [x] Make read-only/dry-run the default architecture.
- [x] Specify hard minimum/maximum limits, hysteresis, timeout, cooldown, and
  emergency behavior.
- [x] Specify stale-input, restart, drift, and read-back mismatch behavior.
- [~] Add property/fuzz tests for controller bounds and allocator envelopes;
  monotonic sequence and feasibility properties need broader coverage.
- [~] Add restart, lease conflict, stale telemetry, and actuator fault tests;
  stale-input and wrong-domain shadow tests are complete.
- [ ] Complete a non-critical controlled load test.
- [!] Install or change a service on a production PVE host only after explicit
  approval at the production-write checkpoint.
- [!] Enable canary actuation only after explicit approval and a reviewed
  rollback command/state snapshot.

## 5. Open-source engineering

- [x] Persist required project documents and ADRs locally.
- [x] Add Apache-2.0 license, DCO, contributing guide, security policy, code of
  conduct, support/governance policy, Dependabot, and issue/PR templates.
- [~] Add unit, shadow CLI, and replay tests; integration and performance
  coverage remain.
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
  session and is irreversible; anonymous pull verification remains pending.

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
  management-probe health; merge, deployment, p95 telemetry, and controller
  events remain. Evidence: ITOps commit `90b04b0`; all 1,231 backend tests,
  16 focused PVE/runtime tests, 11 restricted-probe tests, build/lint, and
  dependency checks passed on 2026-08-02.
- [~] Add multi-signal warning/critical alerts and anti-noise behavior. The
  detector now requires write-wait plus PSI, queue, or management-plane
  corroboration and seeds disabled warning/critical rules; persisted integration
  tests, shadow baseline tuning, and explicit enablement remain.
- [ ] Add dashboard, decision journal, and incident-review links.
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
