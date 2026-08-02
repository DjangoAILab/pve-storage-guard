# Host observer hardening implementation plan

**Stage objective:** Make the read-only PVE host observer continuously
supervisable and package its intended least-privilege systemd boundary without
installing or running it on a PVE host.

**Safety boundary:** This stage may build and test local binaries and Linux
containers. It must not upload a binary, create a PVE user or ACL, install a
unit, register a runtime, enable an alert, or change any PVE/ITOps state. The
compiled-on-PVE checklist item remains open until a separately approved
non-production PVE execution supplies that evidence.

## Acceptance criteria

1. `agent watch` emits one strict JSON observation per line, runs only one
   collection at a time, accepts a bounded cadence, and stops cleanly when its
   context is cancelled.
2. A cancelled or timed-out child command is reaped and returns an opaque,
   operation-scoped error without command output or private bindings.
3. The host observer unit runs as a dedicated non-root account, has no network
   access or writable filesystem path, retains only the device/procfs/PVE reads
   required by the current adapter, and cannot start until `agent inventory`
   succeeds.
4. The unit is checked both by repository tests and Linux
   `systemd-analyze`; deployment documentation states which directives remain
   environment-dependent and how to validate permissions in non-production.
5. Existing race, static-analysis, vulnerability, secret, replay-golden,
   container, and documentation gates remain green.

## Implementation batches

### Batch 1: cancellation and continuous read-only observation

- Add a context-aware watch loop behind the existing agent reader.
- Unit-test immediate sampling, serial cadence, encoding failure, observation
  failure, and cancellation while collecting and while waiting.
- Exercise the real bounded child runner with a test helper process to prove
  cancellation, timeout, output caps, and opaque errors.

### Batch 2: host service boundary

- Add a reviewed observer-only systemd service under `deploy/systemd/`.
- Add a dependency-free unit contract check and a Linux verification script.
- Run the script in CI and in a disposable local Linux container.
- Document the static account, owner-only config, PVE audit ACL, OpenZFS device
  access, journald output classification, stop behavior, and uninstall path.

### Batch 3: evidence and publication

- Run the complete local gate set and update `docs/CHECKLIST.md`, architecture,
  compatibility, roadmap, and Pages content with evidence boundaries.
- Scan the diff and history for secrets and private identifiers.
- Open a public PR, require all protected checks, merge only when green, then
  verify main CI/CodeQL/Pages and the live documentation.

## Explicitly out of scope

- PVE host installation or ACL changes.
- Containerizing the host collector by mounting host executables or `/dev`.
- ITOps runtime registration, dashboard capture, or alert enablement.
- Any actuator, I/O limit, canary, or rollback mutation.
