# ADR-0007: Use a local, read-only PVE agent with typed evidence

## Status

Accepted — 2026-08-02

## Context

The public project has a policy kernel and PVE reader contract, but no concrete
PVE inventory or metrics implementation. Linux diskstats exposes cumulative
counters and average timing; it cannot recover an I/O latency percentile. A
collector that labels that average as p95 would violate ADR-0003 and could tune
the controller against the wrong statistic.

The first collector also has to run safely on a PVE node. It must not require a
network listener or API token, traverse `/etc/pve`, accept arbitrary commands,
or publish node, pool, storage, VM, container, or device identities.

## Decision

- The v0.1 PVE agent runs on the PVE host and exposes one-shot `inventory` and
  `observe` CLI operations. A supervisor may schedule or stream those calls;
  the agent itself opens no network socket.
- Configuration binds private PVE node, storage, ZFS pool, and kernel-device
  names to public-safe opaque domain and resource keys. Only those opaque keys
  may appear on stdout.
- The process runner exposes typed operations. Each operation maps to fixed
  `pvesh` or `zpool` argv; no shell, arbitrary executable, free-form argv, or
  executable path is accepted from configuration.
- PVE inventory and management health use read-only `pvesh` endpoints. The
  storage binding must be enabled, active, and of type `zfspool` before it is
  trusted.
- Storage-domain write p95 is estimated conservatively from the OpenZFS
  `total_wait` write-latency histogram. The result records that it is a
  histogram-bucket upper bound, its source, interval, and sample weight.
- `waitValid` is true only when the histogram header/shape is recognized, the
  sample contains writes, and the storage binding is verified. Diskstats and
  PSI retain their own statistic and layer labels and never substitute for p95.
- Every child command has a deadline, bounded output, a minimal fixed
  environment, and fail-closed parsing. Management-probe failure emits an
  unhealthy observation; required storage/evidence failures return an error
  without a misleading observation.
- The implementation reads only `/proc/pressure/io` and `/proc/diskstats` by
  exact path. It does not recursively read `/etc/pve` or access `/etc/pve/priv`.
- The v0.1 agent has no actuator, mutation endpoint, remote delivery, automatic
  enrollment, or credential handling.

## Consequences

- OpenZFS-backed PVE storage is the first supported high-fidelity adapter.
  LVM-thin, Ceph, directory storage, generic Linux, and remote API collection
  require separate typed adapters and evidence contracts.
- Deployment must mount the host procfs read-only and make `pvesh` and `zpool`
  available. The default distroless image is not by itself a host collector;
  packaging must document a host binary or purpose-built privileged-minimal
  image instead of implying otherwise.
- Explicit private-to-opaque bindings add configuration work but prevent
  accidental identity disclosure and implicit control scope expansion.
- OpenZFS histogram output is an evolving interface. Supported shapes are
  fixture-tested, and unknown shapes fail closed.

## Alternatives

- **Direct HTTPS API client:** deferred because it adds token, TLS, retry, and
  secret-lifecycle complexity to the initial same-host use case.
- **Parse the whole pmxcfs tree:** rejected because it broadens sensitive file
  access and couples the agent to text formats unnecessarily.
- **User-provided collector scripts:** rejected because arbitrary execution and
  untyped metric output undermine the security and semantics boundaries.
- **Use diskstats average latency as p95:** rejected as statistically false.

