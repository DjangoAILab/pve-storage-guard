# Non-production host validator design

## Decision and scope

The next promotion gate needs evidence from a compiled binary running as the
intended non-root observer identity on a non-production PVE node. A prose or
shell checklist is too easy to execute inconsistently and too likely to retain
raw operational output. Adding a self-validation mode to the product binary
would enlarge the public CLI and let the component under test define its own
success. The selected design is therefore a dependency-free external Python
validator under `scripts/`.

The validator does not install a binary, create an account or PVE ACL, copy a
configuration, manage systemd, use SSH, open a network connection, write an
evidence file, or invoke an actuator. An operator must stage those inputs in an
approved non-production environment. The validator refuses root execution,
symlinks, an owner-mismatched or group/other-readable config, and a binary whose
content does not match the operator-supplied SHA-256. It executes only that
binary with fixed `version`, `agent inventory`, `agent observe`, and `agent
watch` argument shapes. This stage may test the validator with fakes locally,
but only a later real PVE run can close the host-runtime checklist gate.

## Data flow and privacy contract

The validator reads the private agent JSON only to verify its schema envelope,
file ownership/mode, and enumerate the exact private node, storage, ZFS pool,
and kernel-device strings that must not appear in output. It never echoes those
values. Each child receives a minimal fixed environment, bounded stdout/stderr,
and a deadline. One-shot output and at most two watch records are decoded from
memory with duplicate-key rejection and strict top-level structural checks.
Raw stdout/stderr is discarded whether the check passes or fails; errors name
only the failed phase and reason class.

For watch validation, the process must emit two newline-delimited observations
within a bounded window. The validator then sends SIGTERM to the direct child,
requires a zero exit within the stop deadline, and rejects any trailing stderr,
extra oversized line, invalid JSON, schema/kind/domain mismatch, private-value
leak, or unbounded output. It does not claim that no descendant process exists;
the product runner tests and systemd `KillMode=mixed` cover their narrower
boundaries separately.

Successful stdout is one versioned identity-free JSON document containing the
binary digest/version, platform class, executed check names, observation count,
SIGTERM result, privacy result, and `requestedMutations: 0`. It contains no
hostname, username, config path, resource/domain key, PVE identifier, metric,
timestamp, capacity, policy threshold, or raw child content. Failure produces
no stdout, preventing partial evidence from being mistaken for a pass.

## Error and trust model

The expected digest is authority for the executable content, not the path. The
validator opens and hashes a regular non-symlink file before execution, then
rechecks its device/inode/size/mtime identity immediately before every child
launch. This detects ordinary replacement but is not a signature verifier or a
defense against a malicious local root. Release signature/provenance validation
remains an operator prerequisite.

All child operations are fail-closed. Non-zero exit, timeout, signal mismatch,
stderr, output limit, parse failure, identity change, private-value presence, or
unexpected result shape aborts before the summary is emitted. Messages are
categorical and omit child text. The validator itself must run without root so
the evidence proves the observer account, rather than an accidentally
privileged manual execution. A `--allow-root-for-tests` escape hatch is
explicitly rejected; automated tests inject the effective UID internally
instead of weakening the public command.

## Verification matrix

Unit tests use a temporary fake executable and owner-only config. Positive
coverage includes the exact argv sequence, digest binding, two serial watch
records, SIGTERM zero exit, identity-free summary, and no persistent raw output.
Negative coverage includes root, symlink/config permissions, wrong digest,
binary replacement, unknown config fields, non-zero child, stderr, timeout,
oversized output, duplicate JSON keys, private identity leak, wrong schema,
wrong domain, missing watch record, ignored SIGTERM, and non-zero SIGTERM exit.

CI runs the validator tests, JSON Schema validation, Gitleaks, Go/replay gates,
container build, and documentation build. A project-binary negative integration
check invokes the real compiled CLI with an intentionally invalid private
config and requires a categorical failure with zero stdout; it does not pretend
that macOS or a fake executable is PVE evidence. After public review, the next
explicit action is an operator-approved run on a named non-production PVE host.
