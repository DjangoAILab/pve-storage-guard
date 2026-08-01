# Incident Evidence Gate Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a reproducible evidence gate that measures retained incident-signature detection without claiming unavailable lead time and records the observed failure of a fixed 20 MiB/s cap without fabricating samples.

**Architecture:** A versioned sanitized JSON fixture holds non-replayable timeline and aggregate field evidence. Dependency-free Python validation and assessment feed the existing deterministic PoC JSON/Markdown report while keeping observed replay and counterfactual modeling unchanged.

**Tech Stack:** Python standard library, JSON fixtures/schemas, `unittest`, generated Markdown/JSON golden reports, Astro documentation.

---

### Task 1: Define and reject invalid evidence

**Files:**
- Create: `poc/fixtures/reference-incident-evidence.json`
- Create: `poc/schema/incident-evidence.schema.json`
- Create: `poc/incident_evidence.py`
- Create: `poc/test_incident_evidence.py`

1. Write tests for a valid sanitized document and rejection of invalid event
   order, impossible sample counts, replayable aggregate evidence, and a falsely
   independent field check.
2. Run `python3 -m unittest poc.test_incident_evidence -v` and confirm failure
   because the validator does not exist.
3. Implement strict dependency-free validation with no filesystem, network, or
   production adapters, binding metric semantics, sample interval, count, and a
   canonical sample-array SHA-256.
4. Run the focused test and confirm it passes.
5. Commit the fixture, schema, validator, and tests.

### Task 2: Assess detection coverage and field contradiction

**Files:**
- Modify: `poc/incident_evidence.py`
- Modify: `poc/test_incident_evidence.py`

1. Add failing tests requiring a two-sample pressure detection at offset 1, a
   critical sample at offset 2, explicit post-failure lag, missing-signal output,
   and a blocked fixed-cap production claim.
2. Run the focused tests and confirm failure.
3. Implement deterministic assessment over the exact retained wait values and
   validated evidence metadata.
4. Run focused tests and confirm all pass.
5. Commit the assessor.

### Task 3: Integrate generated PoC reports

**Files:**
- Modify: `poc/simulate.py`
- Modify: `poc/test_simulate.py`
- Modify: `poc/results/report.json`
- Modify: `poc/results/report.md`

1. Add failing report tests for `observedIncidentAssessment` and
   `fieldValidation`, including the no-early-warning conclusion.
2. Run `python3 -m unittest poc.test_simulate -v` and confirm failure.
3. Load the incident evidence fixture, add the assessment to `run_poc`, and
   render a concise Markdown section. Do not change controller selection or
   counterfactual values.
4. Regenerate both golden reports and run all PoC tests.
5. Commit report integration and goldens.

### Task 4: Align public claims and checklist evidence

**Files:**
- Modify: `docs/POC.md`
- Modify: `docs/CASE-STUDY.md`
- Modify: `docs/POLICY-DESIGN.md`
- Modify: `docs/CHECKLIST.md`
- Modify: `website/src/content/docs/evidence/poc.md`
- Modify: `website/src/content/docs/evidence/case-study.md`

1. State that retained telemetry begins after the failure marker, so advance
   warning is not measurable from this incident.
2. State that the later fixed-20 natural-load check failed and was rolled back;
   fixed-20 remains a model comparator, not a production fallback.
3. Mark historical signature detection in progress with exact generated
   evidence, keeping production promotion blocked.
4. Commit documentation changes.

### Task 5: Verify and publish through protected review

1. Run `python3 -m unittest discover -s poc -p 'test_*.py' -v`.
2. Regenerate Markdown/JSON and confirm `git diff --exit-code`.
3. Run `go test ./...`, `go vet ./...`, `go test -race ./...`, JSON parsing,
   `npm run build --prefix website`, and sensitive-identifier scanning.
4. Push the feature branch, open a public PR, wait for required checks, and
   merge only when the head is clean and all checks pass.
5. Verify main CI/CodeQL/Pages and the deployed limitation language.
