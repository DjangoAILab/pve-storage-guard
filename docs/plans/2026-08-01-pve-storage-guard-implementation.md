# PVE Storage Guard implementation plan

## Task 1: Persist and validate the project facts

- Output: README, GOAL, CHECKLIST, architecture, policy, PoC, anonymized case,
  ITOps integration, roadmap, and ADRs.
- Test: all required files exist; Markdown links resolve; terminology and product
  identifiers are consistent; sensitive identifier scan is clean.

## Task 2: Migrate the offline PoC

- Output: standard-library Python reference engine, replay simulator, tests,
  anonymized fixtures, and generated JSON/Markdown reports under `poc/`.
- Test: unit tests pass; rerunning the simulator reproduces reviewed snapshots;
  public fixtures contain no original host/pool/VM/workload identifiers.

## Task 3: Establish the Go product skeleton and policy kernel

- Output: Go module, CLI modes, versioned schemas, deterministic AIMD and
  allocator, observer-only PVE adapter interfaces, telemetry contracts.
- Test: fmt/vet/test/race pass; property/fuzz tests enforce safety invariants;
  Go replay agrees with reference golden outputs.

## Task 4: Add open-source and release engineering

- Output: license/community/security files, templates, pinned CI, CodeQL,
  govulncheck/Trivy/secret/license scans, OCI build, SBOM/provenance/signing, and
  release configuration.
- Test: all workflows validate locally where possible; OCI image runs as
  non-root for replay/controller; no push occurs in pull-request workflows.

## Task 5: Build the landing and documentation site

- Output: Astro Starlight Pages site with project story, architecture, verified
  charts, dashboard example, quick start, operations, security, and disclaimer.
- Test: production build and link check pass; visual review at desktop/mobile;
  no unsupported claim or sensitive incident detail is visible.

## Task 6: Publish the repository and pre-release artifacts

- Output: public `DjangoAILab/pve-storage-guard`, protected main branch/ruleset,
  Pages deployment, and public GHCR image from a validated tag.
- Test: pre-publish secret/PII scan; public clone builds/tests; image digest,
  SBOM, provenance, and docs URLs verify.
- Checkpoint: pause if any incident material cannot be confidently classified as
  publishable.

## Task 7: Integrate ITOps observer/shadow

- Output: ingestion schema, metrics, dashboard, multi-signal alerts, decision
  journal links, and runbook; local observer/shadow service configuration.
- Test: historical replay and synthetic fault cases create the expected events
  and alert levels without mutation.
- Checkpoint: installing services or changing production PVE/ITOps configuration
  is a production write and requires explicit approval.

## Task 8: Canary and rollback

- Output: reviewed policy version, exact non-critical enrollment, prior-state
  snapshot, expiring approval, apply/read-back verification, rollback exercise,
  and evidence report.
- Test: all pre-canary gates pass; automatic stop conditions work; management
  probes and storage SLO remain healthy during controlled load.
- Checkpoint: explicit approval is required immediately before actuation.
