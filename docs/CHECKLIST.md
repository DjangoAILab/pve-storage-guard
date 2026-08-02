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
- [x] Run all 42 offline Python tests and reproduce Markdown/JSON reports locally.
  Evidence: `poc/results/report.md` and `poc/results/report.json`.
- [x] Add estimated job completion time and hourly decision churn to the report.
- [x] Record unavailable historical IOPS, PSI, queue, and management-plane
  samples as evidence gaps; do not synthesize them as observations.
- [ ] Add at least one independent trace before considering active control. A
  strict replay-trace schema and dependency-free assessor now reject synthetic,
  same-group, incomplete, unknown-class, and non-p95 evidence; the local audit
  found only same-episode aggregates without replayable samples. External trace
  research found useful observed storage datasets but no synchronized management
  evidence. v1alpha2 now represents management `unknown` and the wait
  measurement layer explicitly, and requires at least 95% valid wait and
  known-management coverage, preventing a
  storage-only trace from falsely closing this gate. Evidence:
  [external trace research](EXTERNAL-TRACE-RESEARCH.md) and [ADR-0006](adr/0006-separate-storage-research-from-promotion-evidence.md).
- [x] Capture and sanitize a synchronized read-only live window without a host
  install. The 30-second result records ZFS interval-mean `total_wait`, pool
  IOPS/throughput, member-disk average wait/queue/utilization, PSI, and four
  successful management probes. It is explicitly not replay-qualified because
  it is short, uncontrolled, lacks exact workload binding, and contains no true
  I/O p95. Evidence: [PoC live baseline](POC.md#read-only-live-baseline-not-replay-qualified).
- [x] Run one-at-a-time parameter-neighborhood sensitivity analysis. Evidence:
  `poc/results/report.md`; 16/18 neighbors pass, while faster additive increase
  and shorter healthy confirmation fail the safety gate.
- [ ] Add independent storage-class and workload-shape traces; do not relabel
  the three counterfactual capacity models as observed storage classes.
  ADR-0003 now makes metric statistic, provenance, and independence group
  mandatory trace fields. A tested local converter can aggregate authorized
  per-I/O CSV into identity-free, layer-typed v1alpha2 research traces without
  downloading or bundling third-party data; license clearance and an eligible trace remain
  pending.
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
- [x] Implement the concrete public PVE ZFS inventory/metrics adapter and
  read-only `agent inventory` / `agent observe` modes behind those contracts.
  Evidence: `internal/adapter/pve`, `cmd/pve-storage-guard`,
  `api/v1/schema/pve-agent-config.schema.json`,
  `api/v1/schema/pve-inventory.schema.json`, and ADR-0007. The adapter uses
  OpenZFS write total-wait histograms for typed p95 upper-bound evidence;
  diskstats and PSI cannot substitute for it.
- [x] Validate the concrete adapter parsers against a reviewed, sanitized
  real-PVE ZFS source-format fixture and compatibility matrix before any host
  installation. A remote in-memory, read-only probe validated PVE 9.2 / OpenZFS
  2.4 management, storage, histogram, PSI, and diskstats shapes, then emitted
  only fixed aliases and synthetic values. Evidence:
  `internal/adapter/pve/testdata/pve-9.2-openzfs-2.4/`, its privacy/parser test,
  [compatibility evidence](COMPATIBILITY.md), and public
  [PR #32](https://github.com/DjangoAILab/pve-storage-guard/pull/32). Final PR
  validation passed [CI](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30727736537),
  [CodeQL](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30727736533),
  and the [Pages build](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30727736532).
- [x] Add serial continuous observation, portable in-flight child cancellation,
  and a proposed observer-only systemd boundary without expanding the command,
  network, device, or write surface. Repository contract tests pin the unit's
  non-root identity, fixed inventory/watch commands, empty capabilities,
  read-only filesystem, private network, closed device policy, restart/stop
  bounds, and lack of an actuator. Disposable Ubuntu 24.04
  `systemd-analyze verify` passed and the security exposure was 0.8 (SAFE),
  below the enforced 1.0 ceiling. Evidence: `cmd/pve-storage-guard`,
  `internal/adapter/pve/runner_test.go`, `deploy/systemd/`,
  `scripts/verify-systemd-unit.sh`, and the
  [host-hardening plan](plans/2026-08-02-host-observer-hardening.md). Public
  review: [PR #33](https://github.com/DjangoAILab/pve-storage-guard/pull/33).
  This is portable/static evidence only and caused no PVE or ITOps write.
- [x] Add a digest-bound, non-root, non-production host evidence gate without
  adding a product self-attestation mode. The dependency-free validator uses
  fixed argv, re-hashes the binary before every launch, keeps bounded child
  output in memory, rejects duplicate JSON and configured private identities,
  requires two valid watch records plus a zero-exit SIGTERM, and emits only the
  versioned identity-free summary schema. Twelve local success/fault cases pass,
  including an actual compiled product-binary categorical failure; none is
  labeled as PVE runtime evidence. Evidence:
  `scripts/validate_nonprod_observer.py`,
  `scripts/test_validate_nonprod_observer.py`,
  `api/v1/schema/pve-host-observer-validation.schema.json`, and the
  [validator design](plans/2026-08-02-nonprod-host-validator-design.md).
- [ ] Execute the compiled agent in a non-production PVE environment and
  validate Unix permissions, repeated sampling, process cancellation, and
  live systemd behavior. Run the digest-bound validator first and retain only
  its reviewed identity-free summary. No binary was uploaded to or run on the
  reference production host; fake validation, static unit analysis, and
  portable cancellation tests do not close this host-runtime gate.
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
  and fail-before-output behavior. Sealed-journal verification tests cover
  active-writer locks, symlink/permission/file/event bounds, strict JSONL,
  forged linkage, age consistency, single-domain ownership, anomaly counts,
  exact raw-byte SHA-256, and identity-free CLI output. Content-addressed batch
  tests cover approved-digest matching, active-writer rejection, empty files,
  offset pagination, a 64-event hard bound, and zero stdout on digest failure.
  Repeated local benchmarks cover policy evaluation, 100-disk allocation, the
  verified safety gate, and 32-observation
  local shadow-command batches with and without real per-event journal sync.
  Default shadow measured 4.048–4.155 µs/observation and the opt-in private
  journal measured 3.770–3.827 ms/observation on the documented local machine;
  concrete PVE adapter tests now cover fixed shell-free argv, strict owner-only
  config, PVE/ZFS binding, histogram semantics, empty/malformed samples,
  PSI/diskstats parsing, opaque output, and journal evidence preservation. The
  40-bucket histogram parser measured 3.867–4.005 µs/op on the same local
  machine. On 2026-08-02, full tests and race tests, vet, staticcheck,
  govulncheck, Gitleaks, JSON/schema checks, and the 15-page Astro build passed;
  public [PR #31](https://github.com/DjangoAILab/pve-storage-guard/pull/31)
  then passed the final [CI run](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30726851178),
  [CodeQL run](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30726851185),
  and [Pages build](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30726851172);
  the internal pure replay exporter now also has a no-global-rescan regression
  gate and deterministic 1/7/14-day benchmark. Five 14-day runs exported 20,160
  intervals from 141,120 metrics in 80.472–82.093 ms with identical output
  checksums. Restricted-probe/SSH, PVE REST adapter, persistent ITOps, real
  storage, and controlled-load performance remain.
- [x] Add pinned GitHub Actions for lint, test/race, replay golden, CodeQL,
  govulncheck, secret scan, OCI build/Trivy, docs build, and Pages.
- [x] Add multi-architecture release workflow with pinned actions, binary/image
  SBOM, provenance, checksums, and keyless container signing.
- [x] Build the GitHub Pages landing/documentation site; local Astro build has
  zero diagnostics and responsive browser review has no horizontal overflow.
- [~] Add verified architecture and incident/control-loop graphics. The modeled
  policy-effect SVG is now deterministically generated from the reviewed PoC
  golden JSON, visibly labeled counterfactual/not-production, and checked in
  CI. A real ITOps shadow-baseline dashboard graphic remains pending.
- [x] Complete the initial pre-publication review: identifier/IP/private-key
  pattern scan clean, Gitleaks v8.30.1 reports no leaks, Markdown links resolve,
  screenshots contain anonymized content and standard JFIF/sRGB metadata only.
- [x] Create and push the public GitHub repository only after a clean sensitive
  information scan and review of publishable artifacts. Evidence:
  [DjangoAILab/pve-storage-guard](https://github.com/DjangoAILab/pve-storage-guard).
- [x] Validate the repaired main CI across test/race, lint, static analysis,
  govulncheck, replay golden, secrets, OCI build, and Trivy. Evidence:
  [main CI for `b651099`](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30718700567).
- [x] Enable and deploy GitHub Pages, set the repository homepage, verify HTTPS
  200, title/disclaimer content, and desktop width in a browser. Evidence:
  [public documentation site](https://djangoailab.github.io/pve-storage-guard/)
  and [Pages deployment for `b651099`](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30718700564).
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
- [~] Verify that it detects the historical incident signature early enough to
  be operationally useful. The generated evidence gate finds two consecutive
  samples above 25 ms one second after retained collection starts and a 100 ms
  sample at offset two seconds. However collection began 53 seconds after the
  management-failure marker, so advance warning is not proven; PSI, queue, and
  management-probe series are also absent. A separate same-episode aggregate
  check rejected and rolled back fixed 20 after 22/60 unsafe samples and a
  234.065464 ms p99, but cannot be replayed or count as an independent trace.
- [~] Add ITOps ingestion for storage latency, PSI, queue, management-plane
  health, and controller state. Internal draft PR #37 now has a
  read-only `pve.storage-pressure` probe and mappings for PSI, in-flight I/O,
  disk counters, derived IOPS/throughput/average wait/queue/utilization, and
  management-probe health. The draft now also parses one complete fixed-argv
  `zpool iostat -lpH 1 2` interval into separately typed ZFS pool
  `total_wait`/`disk_wait`, IOPS, and throughput gauges. ZFS values are labeled
  as one-second interval means, remain outside detector v1, and cannot be
  relabeled as disk average wait or I/O p95. A live stdin-only probe check
  succeeded without installation or remote writes. A pure sanitized replay-trace builder now preserves
  fixed `average`/`derived` block-device diskstats semantics, relative offsets, declared
  gaps, and excludes internal selection identifiers. Its v1alpha2 form keeps a
  valid wait sample while marking absent management evidence `unknown`; it has
  no route, writer, or deployment path. Merge, deployment, true p95 telemetry, trace review,
  approved runtime invocation and incident linking remain.
  The public controller provides a tested opt-in private JSONL decision journal
  plus a read-only sealed-file verifier and bounded content-addressed private
  batch reader. The reader rescans the complete sealed file under lock before
  emitting and has no credentials, network, or persistence. Public PR #21
  merged this boundary as `b651099`; main CI, CodeQL, and Pages passed, and the
  deployed ITOps page returned HTTPS 200 with the final run-155 evidence. The ITOps draft has
  a pure strict mapper plus a runtime-uninvoked persistence service. It revalidates the expected domain and
  policy revision, resolves storage/disk resources only through reviewed
  target-scoped bindings, and atomically stores an idempotent private audit row
  with low-cardinality derived metrics. A canonical metric-projection digest
  rejects altered retries. An approval-bound internal capability and exact-argv
  subprocess adapter are implemented and tested but deliberately absent from
  the production registry. The repository rechecks running task, proposal,
  envelope hash, approval, and expiry inside each audit/metric transaction;
  an optional review group must already exist, and historical import does not
  create alert state. Task evidence contains only digest/count reconciliation.
  A real cross-repository local test built the public binary and passed one
  synthetic private event through the compiled ITOps reader without printing
  event content. Neither side
  has network delivery, runtime registration, alert processing, or production
  deployment. Evidence: public ADR 0005 and public content-addressed batch tests;
  ITOps commits `90b04b0`, `44df889`, `242a7ff`, `c236d06`, `4fe5b97`,
  `40e7d0a`, `51cc834`, `e7e7997`, and `ccbbabd`; all 1,268 backend tests across 151 files passed
  locally on 2026-08-02, including the SQLite approval-to-import handoff,
  exact-argv reader, and registry-absence tests. Backend build/lint/dependency checks and
  all 101 frontend tests plus build/lint passed. Internal CI run 153 confirmed
  both the quality gate (4m35s) and dependent linux/amd64 image build (4m57s)
  successful for the earlier importer commit while the PR remained Draft.
  Internal run 154 then validated `51cc834`: quality gates passed in 4m31s and
  the image build in 4m47s. Run 155 validated follow-up `e7e7997`: quality
  gates passed in 4m34s and the image build in 4m45s. Final internal
  [run 156](https://gitea.wj2015.com/PEM/itops-agent-platform/actions/runs/156)
  validated typed ZFS shadow telemetry `ccbbabd`: quality gates passed in 4m33s
  and the linux/amd64 image build in 5m10s. Internal commits `c2ed208` and
  `db5493b` now
  persist a staged observer rollout packet with separate merge, probe-write,
  application-deploy, alert-arm, journal-registration, and actuation gates. It
  also makes live acceptance verify PSI/diskstats/management provenance,
  detector-v1 labels, the exact 28-rule set, and typed ZFS interval semantics.
  Run 157 correctly failed when an inserted test line invalidated an existing
  synthetic-secret fingerprint; the fix preserved the fixture line numbers and
  did not expand the ignore list. Final internal
  [run 158](https://gitea.wj2015.com/PEM/itops-agent-platform/actions/runs/158)
  passed Node quality gates in 4m33s and the linux/amd64 image build in 10m59s;
  the latter recovered through its bounded retry after a transient registry
  mirror DNS timeout. Internal replay-export commits `60b8359` and `8104bb2`
  now emit v1alpha2, preserve absent management evidence as `unknown`, and type
  diskstats wait as `block-device`. A compiled-exporter-to-public-assessor
  synthetic check reported 100% structural/wait/management coverage when both
  management samples were present and 50% management coverage when one was
  absent; both correctly remained policy-incompatible and ineligible because
  average block-device wait is not storage-domain p95. Final internal
  [run 160](https://gitea.wj2015.com/PEM/itops-agent-platform/actions/runs/160)
  passed the Node quality gate in 4m38s and linux/amd64 image build in 4m49s.
  Internal commit `324422f` then replaced repeated full-array scans in the pure
  replay exporter with request-scoped indexes and added a deterministic
  one-day CI smoke plus 1/7/14-day local diagnostics. Final
  [run 161](https://gitea.wj2015.com/PEM/itops-agent-platform/actions/runs/161)
  passed the Node quality gate in 4m34s and linux/amd64 image build in 4m45s.
  PR #37 remains Draft and undeployed.
- [~] Add multi-signal warning/critical alerts and anti-noise behavior. The
  detector now requires write-wait plus PSI, queue, or management-plane
  corroboration and seeds disabled warning/critical rules. A persisted SQLite
  test now verifies two-sample debounce across evaluator restart, one firing,
  and one recovery with detector evidence attached. The general recommended-rule
  command now excludes both storage-pressure rules unless a second exact opt-in
  is present, and an idempotent rollback helper disables only those two IDs.
  A 24-hour window is deployment acceptance only; 7–14 days of representative
  shadow data, real notification testing, and explicit enablement remain.
- [~] Add dashboard, decision journal, and incident-review links. Internal
  Draft PR #37 now includes a tested PVE-only storage-pressure dashboard for
  PSI, management health, separately typed ZFS-pool and per-disk pressure
  evidence, and alert gates. The pool table states that its one-second means do
  not participate in detector v1. The
  controller now has a private append/sync journal, an identity-free sealed
  verifier, and a bounded digest-matched reader; the internal draft has a tested
  but uninvoked persistence adapter.
  A real shadow-baseline screenshot, approved transport/runtime registration,
  and incident/runbook links remain. The verifier and importer expose no route,
  scheduler, alert evaluation, or production side effect.
- [ ] Exercise cold restart and rollback in a non-critical environment.
- [!] Expand production control only after canary evidence is reviewed.

## Current risks and evidence gaps

- The historical store contains one-minute VM write counters but did not retain
  one-second storage latency, PSI, or queue history.
- The twelve one-second wait samples are an incident window, not a long-running
  baseline.
- The 2026-08-02 live window is only 30 seconds of natural load. Its ZFS
  `total_wait` percentile is across interval means, while diskstats wait is a
  derived device average; neither is a true I/O latency percentile.
- Counterfactual outcomes depend on explicit monotonic pool models and are not
  causal proof.
- One incident cannot justify a universal default across SSD, HDD, ZFS special
  vdev, Ceph, or network-backed storage.
- GitHub organization policy visibility is limited by the current token scope;
  repository-local branch/ruleset and workflow protections must be verified
  independently before release.
