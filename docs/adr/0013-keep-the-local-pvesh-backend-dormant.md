# ADR-0013: Keep the local pvesh backend dormant behind a fixed command boundary

## Status

Proposed — OS-backed implementation remains unreachable, 2026-08-04

## Context

ADR-0012 defines an offline actuator core for one existing QEMU `bps_wr`
limit. Its injected backend made the mutation state machine testable without
creating an operating-system capability. The next design question is how to
express the local Proxmox operation without turning the actuator into a shell,
generic PVE client, or prematurely reachable production path.

The QEMU configuration API returns a SHA-1 version digest and accepts that
digest on updates to reject concurrent configuration changes. The digest is a
concurrency fence, not proof of authenticity. A local `pvesh` invocation also
inherits the caller's host authority, so successful synthetic execution does
not prove an acceptable production Unix identity or PVE permission set.

## Decision

- Add an OS-backed implementation only inside `internal/actuator/pve`. Keep its
  constructor unexported. No CLI, agent, listener, service unit, container
  entrypoint, configuration switch, or dependency-injection path can construct
  it.
- Bind one backend instance to the same validated, owner-only
  `PVECanaryPreflightConfig` as the actuator. Recheck the exact node, workload,
  and disk arguments at every method boundary.
- Generate only these two command shapes with the absolute
  `/usr/bin/pvesh` path:

  ```text
  get /nodes/<node>/qemu/<vmid>/config --output-format json
  set /nodes/<node>/qemu/<vmid>/config --digest <digest> --<disk> <full-value>
  ```

- Do not invoke a shell, read command text from configuration, use inherited
  environment variables, accept additional flags, or expose the command
  executor outside the package.
- Before the set command, independently require the exact enrolled storage,
  one existing positive `bps_wr`, no conflicting rate option, a valid lowercase
  40-hex digest, and a rate inside the enrollment envelope. The actuator remains
  responsible for comparing unmanaged state before and after the update.
- Apply the owner-configured timeout in the backend as well as the actuator.
  Use a fixed locale, timezone, `PATH`, and working directory; bound stdout to
  1 MiB and stderr to 64 KiB; never include process output in returned errors.
- Never retry a set command. Timeout, cancellation, output overflow, and process
  failure are ambiguous apply failures and flow to the existing resource-freeze
  behavior.
- Test the argv and process boundary with an injected recorder and disposable
  test subprocesses. Tests must not invoke host `pvesh` or mutate a PVE config.
- Fail CI if the shipped CLI dependency graph includes the actuator package or
  if its built binary contains the dormant backend's symbols.
- Treat exporting the constructor, wiring it into a runtime, granting host/PVE
  permissions, or using it against a real disk as separate checkpoints. They
  require a reviewed non-critical candidate, controlled-load evidence,
  rollback proof, and explicit production approval.

## Consequences

- The repository can review and fuzz the real serialization and process
  boundary while keeping the shipped binary incapable of actuation.
- Arbitrary command execution, VM lifecycle operations, automatic unlimited
  bootstrap, and update retries remain outside the capability boundary.
- This change does not establish least privilege, production compatibility, or
  canary safety. Those claims remain blocked on live non-root PVE evidence and
  a separately approved runtime design.
- A future API-backed implementation can satisfy the same two-method backend
  contract without changing the platform-neutral policy or Safety Gate.

## References

- [Proxmox VE `qm(1)` disk options and digest](https://pve.proxmox.com/pve-docs/qm.1.html)
- [ADR-0012: Bound the PVE actuator to one existing QEMU write limit](0012-bound-the-pve-actuator-to-one-existing-qemu-write-limit.md)
