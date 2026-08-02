# Production Read-only Compatibility Validator Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Validate a digest-pinned compiled observer on a production PVE host without misrepresenting that evidence as non-production promotion proof.

**Architecture:** Refactor the existing dependency-free validator so its bounded product execution and output validation can be reused. Add a separate production compatibility entry point and JSON schema that always fails closed for promotion while preserving root/non-root as an observed fact.

**Tech Stack:** Python 3 standard library, JSON Schema 2020-12, Go cross-build, SSH, `/dev/shm` transient staging.

---

### Task 1: Extract the shared observer evidence collector

**Files:**
- Modify: `scripts/validate_nonprod_observer.py`
- Test: `scripts/test_validate_nonprod_observer.py`

1. Add a failing test proving the non-production API still rejects root and
   retains its exact v1alpha1 output.
2. Extract a private collector that accepts a `require_non_root` flag and
   returns the validated binary/config facts without selecting an evidence
   scope.
3. Run `python3 scripts/test_validate_nonprod_observer.py` and require all
   existing tests to pass.

### Task 2: Add the production compatibility validator

**Files:**
- Create: `scripts/validate_prod_observer_compatibility.py`
- Create: `scripts/test_validate_prod_observer_compatibility.py`
- Create: `api/v1/schema/pve-host-observer-compatibility.schema.json`

1. Write tests for root and non-root summaries, private-identity rejection,
   digest drift, malformed output, bounded watch, and zero-exit SIGTERM.
2. Implement the wrapper using the shared collector with
   `require_non_root=False`.
3. Refuse root unless the CLI includes the explicit `--allow-root`
   acknowledgement; never convert that acknowledgement into a passing
   hardening result.
4. Emit `PVEHostObserverCompatibility`, scope
   `production-read-only-compatibility`, the observed `nonRoot` boolean,
   `promotionEligible:false`, fixed limitations, and zero requested mutations.
5. Validate successful test output against the new strict JSON schema.

### Task 3: Document the non-promotion boundary

**Files:**
- Modify: `docs/PVE-AGENT.md`
- Modify: `docs/CHECKLIST.md`
- Modify: `site/src/content/docs/operations/pve-agent.md`
- Modify: `README.md`

1. Document transient `/dev/shm` staging as an optional operator procedure,
   never an installer.
2. State that production compatibility cannot close non-root, systemd,
   controlled-load, or actuation gates.
3. Link the new schema and keep the original non-production validator guidance
   unchanged.

### Task 4: Verify locally and on the approved production dry-run target

**Files:**
- Modify: `docs/CHECKLIST.md`
- Modify: `docs/COMPATIBILITY.md`

1. Run Python tests, Go tests/race/vet, staticcheck, govulncheck, Gitleaks,
   JSON/schema checks, and docs build.
2. Cross-build the exact `linux/amd64` binary and record its SHA-256 locally.
3. On the approved PVE host, generate a conservative private config in a random
   owner-only `/dev/shm` directory, run the compatibility validator, retain only
   its anonymous summary, and remove the directory through an EXIT trap.
4. Confirm no service, ACL, PVE configuration, persistent file, alert, or I/O
   control changed.
5. Commit, push a PR, and merge only after GitHub CI, CodeQL, and Pages pass.
