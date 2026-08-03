# Execution checklist

Status values: `[x]` complete, `[~]` in progress, `[ ]` pending, `[!]` blocked
by an explicit safety checkpoint. Evidence links are repository-relative unless
marked as local-only.

Last updated: 2026-08-04 (Asia/Shanghai)

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
- [x] Add independent storage-class and workload-shape traces; do not relabel
  the three counterfactual capacity models as observed storage classes.
  ADR-0003 now makes metric statistic, provenance, and independence group
  mandatory trace fields. A tested local converter can aggregate authorized
  per-I/O CSV into identity-free, layer-typed v1alpha2 research traces without
  downloading or bundling third-party data. A metadata-only community intake
  now separates storage research from promotion candidates, prohibits public
  raw-data submission, and requires machine plus human permission/provenance/
  privacy review before a publication PR. Public
  [Issue #48](https://github.com/DjangoAILab/pve-storage-guard/issues/48) now
  solicits independent observed storage plus synchronized management evidence
  under the same no-upload boundary. Creating an intake and solicitation is not
  evidence receipt: a CC BY 4.0 UMass/SPC search prefix is now accepted as a
  separate `WorkloadShapeTrace`. Its 600-second/60-sample artifact passes the
  research gate with 100% structural completeness while remaining explicitly
  ineligible for active control because latency, storage class, and management
  evidence are unavailable. A second CC BY 4.0 Alibaba Block Trace derivative
  covers 600 seconds of observed production Ultra Disk arrivals, explicitly
  preserves the logical `network-block` class, and passes both the general and
  storage-class research gates. Its device IDs, offsets, absolute times, raw
  source, and physical-media assumptions are absent. It remains categorically
  ineligible for active control because response latency and synchronized
  management evidence are unavailable. Source/prefix/artifact hashes,
  attribution, transformations, and rounding checks are recorded. The
  promotion-compatible independent trace remains pending.
  Evidence: [trace contribution guide](TRACE-CONTRIBUTION.md),
  [external trace research](EXTERNAL-TRACE-RESEARCH.md), and
  [third-party data notices](../THIRD-PARTY-DATA.md). Public
  [PR #52](https://github.com/DjangoAILab/pve-storage-guard/pull/52) passed
  [CI](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30813831906),
  [CodeQL](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30813831197),
  and the [Pages build](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30813830918).
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
  Public review: [PR #34](https://github.com/DjangoAILab/pve-storage-guard/pull/34).
- [x] Execute the compiled agent through an explicitly approved production
  read-only compatibility dry-run because no non-production PVE environment is
  available. A clean, source-bound linux/amd64 build of public main `bfab0fb`
  (`v0.1.0-dev.bfab0fb`, SHA-256
  `b12b3be070c70ed87685c93c6e768f04ad23576e430b809e465a56936ac7e96e`)
  ran from an owner-only random `/dev/shm` directory. Digest-bound inventory,
  one observation, two serial watch samples, and SIGTERM zero exit passed;
  private-identity leakage and raw-output persistence were false and requested
  mutations were zero. The identity-free result explicitly records root
  execution, `promotionEligible: false`, and unvalidated non-root, service,
  controlled-load, and actuation boundaries. Post-run checks found zero RAM
  artifacts, observer processes, service accounts, or units. The first attempt
  with the verified `v0.1.0-rc.1` asset failed closed at inventory because that
  older release predates the agent CLI. The successor `v0.1.0-rc.2` release is
  bound to main `412ddcf`; all four archives, SPDX SBOMs, checksums, GitHub
  provenance, Linux agent smoke gate, multi-architecture image build, and
  keyless signature passed
  [release run 30732828939](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30732828939).
  Its exact linux/amd64 binary (`0.1.0-rc.2`, SHA-256
  `5917ab568f94451ba2d125adb5633ab77288885237d41fc14c74971e6012c84a`)
  repeated inventory, observation, two-record watch, and SIGTERM successfully
  through the same RAM-only production compatibility validator. The result
  remained `promotionEligible: false`, requested zero mutations, leaked no
  private identity, persisted no raw output, and left zero RAM artifacts,
  processes, accounts, or units.
  Evidence: [compatibility evidence](COMPATIBILITY.md#production-read-only-compiled-compatibility)
  and public [PR #35](https://github.com/DjangoAILab/pve-storage-guard/pull/35).
  Post-merge public main passed
  [CI](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30731900106),
  [CodeQL](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30731900131),
  and [Pages](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30731900122);
  the deployed agent guide and compatibility schema both returned HTTPS 200.
- [~] Validate actual non-root PVE ACL/device permissions, live PVE systemd
  behavior, and sustained supervision. An ephemeral Ubuntu PID-1 rehearsal now
  closes only the portable non-root systemd start/restart/cold-start and exact
  artifact rollback mechanics; the production compatibility result and the
  synthetic rehearsal cannot close real PVE ACL or `/dev/zfs` gates.
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
- [~] Complete a non-critical controlled load test. The read-only canary
  preflight now validates live explicit tags, workload lock state, management
  and storage health, exact disk existence/storage, writable data-disk role,
  exclusion from boot order, and a bounded static rollback value without
  exposing PVE identity. A 2026-08-03 read-only production inventory audit found
  no explicitly classified guest, so candidate selection and all load remain
  stopped until one exact non-critical data disk is deliberately enrolled.
  Public [PR #54](https://github.com/DjangoAILab/pve-storage-guard/pull/54)
  merged the mutation-free preflight as `06e8168`; protected-main
  [CI](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30830219723),
  [CodeQL](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30830219451),
  and [Pages](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30830218936)
  passed. This closes only the enrollment-readiness sub-gate, not the controlled
  load or actuation gate.
- [!] Install or change a service on a production PVE host only after explicit
  approval at the production-write checkpoint.
- [!] Enable canary actuation only after explicit approval and a reviewed
  rollback command/state snapshot. A passing read-only preflight still emits
  `activeControlEligible=false`; the v0.2 actuator does not yet exist.

## 5. Open-source engineering

- [x] Persist required project documents and ADRs locally.
- [x] Add Apache-2.0 license, DCO, contributing guide, security policy, code of
  conduct, support/governance policy, Dependabot, and issue/PR templates. The
  repository now contains every label referenced by the bug, design, and trace
  forms; Issue #48 verified exact `help wanted` plus `triage` assignment after
  the missing `triage` and `design` labels were added on 2026-08-03.
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
  SBOM, provenance, checksums, and keyless container signing. Public
  [PR #37](https://github.com/DjangoAILab/pve-storage-guard/pull/37) further
  pinned Syft, Buildx, Cosign, and the QEMU image digest; applied per-job least
  privilege; added GitHub binary provenance; and made container publication
  wait for downloaded checksum, SPDX SBOM, version, and agent-command checks.
  PR and post-merge main passed
  [CI](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30732681360),
  [CodeQL](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30732681361),
  [main CI](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30732751753),
  and [main CodeQL](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30732751758).
- [x] Build the GitHub Pages landing/documentation site; local Astro build has
  zero diagnostics and responsive browser review has no horizontal overflow.
- [x] Add verified architecture and incident/control-loop graphics. The modeled
  policy-effect SVG is now deterministically generated from the reviewed PoC
  golden JSON, visibly labeled counterfactual/not-production, and checked in
  CI. On 2026-08-03, an authenticated real ITOps production shadow view was
  captured and deterministically cropped to the detector title and four
  aggregate status cells. Manual visual review confirms the public PNG excludes
  the target header, host/account identity, and per-pool/per-disk tables; it
  shows the alert gate still disabled. Public
  [PR #53](https://github.com/DjangoAILab/pve-storage-guard/pull/53) merged the
  reviewed documentation asset, and
  [PR #54](https://github.com/DjangoAILab/pve-storage-guard/pull/54) published it
  on the project site. The live PNG returned HTTP 200 with SHA-256
  `e67b83b35e066046c1175504c437fce4fa5fd0bc01cd351d28effea365d8b9a0`,
  exactly matching the locally reviewed asset.
- [x] Complete the initial pre-publication review: identifier/IP/private-key
  pattern scan clean, Gitleaks v8.30.1 reports no leaks, Markdown links resolve,
  screenshots contain anonymized content and standard JFIF/sRGB metadata only.
- [x] Enforce the publication sanitization contract in required CI. The
  dependency-free gate scans only tracked author-maintained publication
  surfaces, uses an exact public-host allowlist, rejects URL user information,
  local/private hosts and RFC1918 coordinates, and emits only categorical
  findings with short hashes. Nine focused tests, a real-tree scan of 88 files
  and 94 URLs, and a redacted Gitleaks scan of 98 commits passed locally.
  Public [PR #43](https://github.com/DjangoAILab/pve-storage-guard/pull/43)
  merged as `79a1324`; protected-main
  [CI](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30785829508)
  and [CodeQL](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30785829524)
  passed. The main secret job reran the nine tests, scanned 88 files and 94
  URLs, and found no leak across all 75 commits reachable from the squashed
  public history. Existing release history was intentionally not rewritten.
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
- [x] Make the tagged GHCR image publicly readable. On 2026-08-02 an
  authenticated organization owner enabled the organization policy that
  permits public packages and changed only `pve-storage-guard` from private to
  public. A no-credential registry-token request and tag listing returned HTTP
  200; an anonymous manifest read of image tag `0.1.0-rc.2` returned HTTP 200,
  linux/amd64 and linux/arm64 manifests, and the exact reviewed OCI index digest
  `sha256:c59935c8c9e90815e9a0d324c66ef63b0e30d76b6a89b3a3fc5a8eb2db4b72c9`.
  GitHub Release tag `v0.1.0-rc.2` intentionally retains the leading `v`, while
  the container image tag does not. Evidence:
  [public GHCR package](https://github.com/DjangoAILab/pve-storage-guard/pkgs/container/pve-storage-guard).
  A 2026-08-03 delivery re-audit fetched the live Pages site over HTTPS and
  matched the required SEO title and description; the latest five main Pages
  workflows were successful. The published OCI index still resolved to the
  reviewed digest and both architectures, each with SPDX and SLSA in-toto
  layers. Cosign v3.0.6 verified the exact release-workflow OIDC identity,
  certificate chain, and transparency-log claim. A downloaded linux/amd64
  archive and SPDX SBOM matched `checksums.txt`, and their GitHub provenance
  attestations verified. No package-token scope was added for this audit.

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
  health, and controller state. Merged internal PR #37 contains a
  read-only `pve.storage-pressure` probe and mappings for PSI, in-flight I/O,
  disk counters, derived IOPS/throughput/average wait/queue/utilization, and
  management-probe health. The merged integration also parses one complete fixed-argv
  `zpool iostat -lpH 1 2` interval into separately typed ZFS pool
  `total_wait`/`disk_wait`, IOPS, and throughput gauges. ZFS values are labeled
  as one-second interval means, remain outside detector v1, and cannot be
  relabeled as disk average wait or I/O p95. A live stdin-only probe check
  succeeded without installation or remote writes. A pure sanitized replay-trace builder now preserves
  fixed `average`/`derived` block-device diskstats semantics, relative offsets, declared
  gaps, and excludes internal selection identifiers. Its v1alpha2 form keeps a
  valid wait sample while marking absent management evidence `unknown`; it has
  no route, writer, or deployment path. Deployment, true p95 telemetry, trace review,
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
  `40e7d0a`, `51cc834`, `e7e7997`, `ccbbabd`, `324422f`, `48cb1a6`,
  and `62cc3e1`; all 1,270 backend tests across 151 files passed
  locally on 2026-08-02, including the SQLite approval-to-import handoff,
  exact-argv reader, and registry-absence tests. Backend build/lint/dependency checks and
  all 101 frontend tests plus build/lint passed. Internal CI run 153 confirmed
  both the quality gate (4m35s) and dependent linux/amd64 image build (4m57s)
  successful for the earlier importer commit while the PR remained Draft.
  Internal run 154 then validated `51cc834`: quality gates passed in 4m31s and
  the image build in 4m47s. Run 155 validated follow-up `e7e7997`: quality
  gates passed in 4m34s and the image build in 4m45s. Final internal
  internal run 156
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
  internal run 158
  passed Node quality gates in 4m33s and the linux/amd64 image build in 10m59s;
  the latter recovered through its bounded retry after a transient registry
  mirror DNS timeout. Internal replay-export commits `60b8359` and `8104bb2`
  now emit v1alpha2, preserve absent management evidence as `unknown`, and type
  diskstats wait as `block-device`. A compiled-exporter-to-public-assessor
  synthetic check reported 100% structural/wait/management coverage when both
  management samples were present and 50% management coverage when one was
  absent; both correctly remained policy-incompatible and ineligible because
  average block-device wait is not storage-domain p95. Final internal
  internal run 160
  passed the Node quality gate in 4m38s and linux/amd64 image build in 4m49s.
  Internal commit `324422f` then replaced repeated full-array scans in the pure
  replay exporter with request-scoped indexes and added a deterministic
  one-day CI smoke plus 1/7/14-day local diagnostics. Final
  internal run 161
  passed the Node quality gate in 4m34s and linux/amd64 image build in 4m45s.
  Final pre-merge
  internal run 163
  validated exact head `62cc3e1`: the Node 22 quality gate and dependent
  linux/amd64 image build passed. PR #37 then merged to internal main as
  `1a94834` after explicit approval. Post-merge
  internal run 164
  then passed both the Node 22 quality gate and linux/amd64 image build. At
  that point it remained undeployed: no collector,
  runtime registration, journal import, dashboard, alert, notification, or
  control path was enabled.
  A documentation-only follow-up corrected four internal rollout fact sources
  so that checkpoint A is recorded as complete while deployment checkpoints
  B/C and all later alert, notification, journal-registration, and actuation
  gates remain explicitly unapproved. Internal
  internal PR #39
  merged as `055b092` after branch
  internal run 168
  and post-merge main
  internal run 169
  passed the Node 22 quality gate and linux/amd64 image build. The merge itself
  caused no runtime or production change.
  A later redacted preflight rejected that provisional candidate after repeated
  pre-handshake SSH connection-loss errors escaped a one-shot emitter listener
  and reached the backend process-level handler. Internal PR #42 kept client and
  channel error listeners installed for their emitter lifetime, added repeated
  post-settlement regression tests, classified pre-handshake loss as a stable
  network failure, passed branch run 175, and merged as `470d9a6` without a
  deployment. Final immutable ITOps observer candidate `9fab00b` then passed
  post-merge
  internal run 179:
  Node 22 quality gates, linux/amd64 publish/read-back, runtime smoke tests,
  exact OCI revision/source labels, provenance, and SBOM all passed. Its exact
  uncompressed source archive self-identified the same commit; its unpacked
  deployment fault tests and pinned Gitleaks scan also passed. Internal
  internal PR #45
  merged the access-controlled rollout evidence as `89958bc` after rebased
  branch
  internal run 183
  passed both jobs. Registry coordinates, credentials, raw logs, and target
  identity remain outside this public record. The candidate is selected but
  was not deployed at that point.
  Checkpoint B was later explicitly approved and completed. The first
  acceptance attempt failed because the diagnostic depended on an undeclared
  host runtime; the exact previous probe boundary was restored and revalidated
  before retry. A runtime-independent validator then confirmed all nine
  restricted read-only operations, including bounded PSI, disk, and ZFS
  envelope evidence. Checkpoint C was separately approved but failed closed
  during protected host-state validation. The source transaction and image
  selector were restored before any container, database, registry, credential,
  or alert change. Review found one candidate allowlist defect and one bounded
  pre-existing directory-mode drift. The successor source fix passed clean-tree
  deployment tests, internal full quality gates, and isolated linux/amd64 image
  smoke in internal run 201;
  the rebased four-file PR head passed the same gates in
  internal run 204.
  The protected-PKI fix then merged internally after run 204; post-merge main
  run 205 passed both jobs. A separately approved, non-recursive repair changed
  only the pre-existing `source-backups` root directory from `0755` to `0700`
  while holding both deployment locks. Its inode was preserved and a digest of
  all four descendants' type, mode, owner, size, mtime, and inode was unchanged.
  A successor read-only protected-state check then reported zero rollback-tree
  violations and the exact two-file root-only CA contract. An exact release ref
  remained bound to `ecfa810`; internal dispatch run 206 passed the Node quality
  gate and linux/amd64 image publication, runtime smoke, immutable read-back,
  revision/source-label, SBOM, and provenance verification without promoting a
  mutable main tag. Registry coordinates and digests remain outside this public
  record. A private candidate-specific Checkpoint C pack now binds that exact
  revision to its source archive, release-tree tool, and both immutable image
  indexes. Its offline verifier rejects digest/size drift, symlink or non-regular
  inputs, unsafe or duplicate archive members, and redacts failure details.
  Twelve binding/negative tests, Bash syntax, ShellCheck, the full deployment
  static and fault-injection suites, whitespace checks, and a pinned tracked-tree
  secret scan passed locally. The pack contains no registry coordinates.
  Checkpoint C was then separately re-authorized and completed with that exact
  successor. A fresh preflight first detected one additional predecessor restart
  and stopped before writes; identity-free correlation matched the already
  tested SSH lifecycle failure signature. After the reviewed fingerprint was
  updated, source replacement preserved protected runtime inodes and retained
  the predecessor rollback anchor. The first image invocation failed safely at
  missing root registry authentication before database backup, container stop,
  or cutover. An interactive operator login allowed the exact-digest retry to
  complete, and registry authentication was removed afterward. Live acceptance
  passed with all collectors fresh/up, the exact 28-rule set, the existing 22
  general rules enabled, and all GPU and storage-pressure rules disabled. The
  active pair was healthy with zero restarts; deployment journals were absent;
  and the stopped exact predecessor plus a private database archive remained
  available for rollback. No alert, notification, journal-registration, or
  actuation path was enabled. A second read-only checkpoint also passed with
  unchanged identity, health, rollback, rule, privacy, and notification state.
  The operator explicitly shortened the deployment-acceptance window after
  those two checks. Deployment acceptance is therefore complete with a recorded
  evidence limitation; no 24-hour coverage or alert-calibration claim is made.
  The offline archive path was safe but caused a long maintenance pause. A
  separate repository-only internal Draft now uses SQLite's online backup API,
  copies stable ordinary non-database files into a root-only staging tree, and
  keeps the exact active pair healthy until the archive, summary, hashes, and
  recovery records are durable. It checks capacity before staging and again
  before compression using actual staged bytes plus bounded headroom. It fails
  closed on source drift, unsafe file types or permissions, insufficient
  capacity, timeout, integrity failure, unsupported runtime constraints, or a
  stale labeled helper. Helpers are networkless, capability- and
  resource-bounded, including a best-effort low block-I/O weight; exact
  release labels let the failure trap remove only current-release helpers. Five repeated
  helper tests, concurrent-WAL coverage, an identity-free 512 MiB database plus
  64 MiB attachment rehearsal, deployment crash/fault tests, full application
  quality gates, and isolated linux/amd64 image smoke passed. The first clean
  runner correctly rejected two high-entropy test placeholders; they were
  replaced with explicit non-secret values without adding a scanner exemption,
  and the successor run passed. The local gate was then strengthened to scan
  both the staged index and every tracked or untracked, non-ignored working-tree
  file. An untracked high-entropy negative probe failed as required, the clean
  dual scan passed, and the earlier quality/image CI run passed. The final
  hardening head also passed the complete local quality matrix. Its first CI
  run exposed an npm registry `EIDLETIMEOUT` that the bounded retry classifier
  did not recognize; an exact regression added only that transient code without
  retrying deterministic failures. Successor internal Draft run 235 passed the
  Node quality gate in 6m17s and isolated linux/amd64 image smoke in 5m34s. A read-only host
  check proved that the installed timeout implementation accepts the exact
  bounded arguments and returns status 124, but cgroup-v2 block-I/O enforcement
  remains unproven. This follow-up remains Draft, unmerged, and undeployed. A
  separately bound candidate, synthetic production-host rehearsal, effective
  block-I/O verification, and explicit production approval remain required.
- [~] Add multi-signal warning/critical alerts and anti-noise behavior. The
  merged detector requires write-wait plus PSI, queue, or management-plane
  corroboration and seeds disabled warning/critical rules. A persisted SQLite
  test now verifies two-sample debounce across evaluator restart, one firing,
  and one recovery with detector evidence attached. The general recommended-rule
  command now excludes both storage-pressure rules unless a second exact opt-in
  is present, and an idempotent rollback helper disables only those two IDs.
  The operator-shortened window is deployment acceptance only. A deterministic,
  identity-free, read-only evidence gate now replaces fixed 24-hour / 7–14-day
  waits for alert calibration and verifies the exact disabled rule contract.
  A first sanitized production replay found 203 complete cycles and 812 disk
  detector samples. A later replay found 240 complete cycles and 960 disk
  detector samples with zero detector-v1 mismatches, four warning firing/recovery
  lifecycles, no critical lifecycle, and only one quiet cycle. It correctly
  rejected arming because the quiet-regime and critical-lifecycle gates remain
  open. The sanitized evidence follow-up merged internally after its branch
  quality/image run passed; the post-merge main quality and linux/amd64 image
  run also passed. It caused no deployment, rule, notification, database, or
  control change. Internal Draft #61 at exact head `1cffa52` now proposes v2
  per-severity review eligibility while keeping the combined result as warning
  AND critical and always proposing that both rules stay disabled. Shared gates
  cover structural/detector/exact-disabled-rule-set evidence; each severity has
  independent contract, exposure, lifecycle, and per-firing-resource baseline
  gates. Local focused and read-only SQLite tests, deployment state machines,
  15 storage-control tests, architecture/dependency checks, backend/frontend
  lint/build/coverage, the one-day replay smoke, and the staged-tree Gitleaks
  scan passed. The Draft remains unmerged and undeployed. The exact evaluator
  was then streamed to the active backend as stdin without installation and
  opened production SQLite read-only/query-only. Its 240 complete cycles and
  960 detector samples had zero mismatches; both warning-firing resources met
  their own baseline and all four warning firings recovered. Warning review
  evidence passed, while critical and combined eligibility remained false
  because no critical firing/recovery existed. Both rules stayed disabled and
  no notification or control action followed. Critical evidence must arise
  naturally rather than through generated production pressure. Representative
  evidence, real notification testing, and explicit enablement therefore
  remain.
- [~] Add dashboard, decision journal, and incident-review links. Internal
  Merged PR #37 includes a tested PVE-only storage-pressure dashboard for
  PSI, management health, separately typed ZFS-pool and per-disk pressure
  evidence, and alert gates. The pool table states that its one-second means do
  not participate in detector v1. The
  controller now has a private append/sync journal, an identity-free sealed
  verifier, and a bounded digest-matched reader; the internal draft has a tested
  but uninvoked persistence adapter.
  The real identity-free shadow-baseline screenshot is now published.
  An authenticated read-only production review subsequently found that the
  deployed page's generic 2,000-series latest query could omit the named
  PVE/ZFS evidence and that generic workload throughput could create a false
  unregistered-disk row. A separate internal read/display-only Draft adds a
  maximum-32-name query for the 13 panel metrics and restricts rows plus pressure
  aggregation to registered `disk` / `zfs_pool` resources. Focused tests passed
  31/31 backend and 8/8 frontend; full regression passed 1,413/1,413 backend and
  104/104 frontend, with lint/build/architecture/dependency checks also passing.
  It remains unmerged and undeployed and changes no collector, schema, rule,
  notification, journal, actuator, or control state.
  Approved transport/runtime registration and incident/runbook links remain. The
  verifier and importer expose no route, scheduler, alert evaluation, or
  production side effect.
- [x] Exercise cold restart and rollback in a non-critical environment. Public
  PR #46 [run 30791350808](https://github.com/DjangoAILab/pve-storage-guard/actions/runs/30791350808)
  used the production observer unit and real compiled
  CLI on an ephemeral Ubuntu 24.04 VM with public synthetic PVE/OpenZFS
  fixtures. It proved non-root initial start with two samples, a SIGKILL-driven
  supervised restart after the configured 30-second delay, byte-distinct
  candidate binary/config cold start, and exact baseline binary/config rollback
  cold start with four unique PIDs. The unit was never enabled; all fixed
  artifacts and the service identity were removed; the pre-existing
  `/usr/local/bin` inode/owner/group/mode was restored. The identity-free
  evidence artifact SHA-256 is
  `1c23af970801c23d00a0e7d50c02bacc128276c6c6550288dbd4df9366c10bc3`.
  This is portable lifecycle evidence, not real PVE permission, sustained-load,
  or policy-effect evidence.
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
