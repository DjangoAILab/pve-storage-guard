# Security policy

## Supported versions

PVE Storage Guard is pre-release and observer/shadow-only. Security fixes are
applied to the latest release line. No current version is approved for
unattended production actuation.

## Report a vulnerability

Use GitHub's private vulnerability reporting from this repository's **Security**
tab. Do not open a public issue for suspected vulnerabilities and do not include
production credentials, raw logs, internal addresses, host identities, guest
data, or exploit traffic from systems you do not own.

Include a minimal reproduction, affected version/commit, impact, and suggested
mitigation if known. Maintainers will acknowledge a valid channel report as soon
as practical and coordinate disclosure after a fix or mitigation is available.

## Security model

- Observer and shadow are the defaults.
- Policy code is unprivileged and cannot execute commands.
- A future actuator accepts only structured, allowlisted, expiring operations
  for exact enrolled resources.
- Telemetry freshness, policy version, lease ownership, hard bounds, prior
  effective state, apply/read-back, and rollback are mandatory gates.
- GitHub Actions never deploy directly to production PVE hosts.

The concrete PVE observer has a deliberately smaller boundary:

- it has no listener, remote client, credential input, actuator, or write API;
- its private config must be an owner-only regular file no larger than 64 KiB;
- validated bindings select only compiled-in `pvesh` and `zpool` read commands;
- commands use no shell, a fixed environment, deadlines, and bounded output;
- procfs reads are limited to `/proc/pressure/io` and `/proc/diskstats`;
- stdout uses opaque domain/resource keys and omits private node, storage, pool,
  device, address, guest, and task identities;
- unknown or mismatched evidence fails closed and diskstats cannot impersonate
  a latency percentile.

Running the binary on a production PVE host remains a separate approval gate.
See [the agent threat boundary and runbook](docs/PVE-AGENT.md) and ADR-0007.

See [architecture](docs/ARCHITECTURE.md) and
[policy safety invariants](docs/POLICY-DESIGN.md#safety-invariants).
