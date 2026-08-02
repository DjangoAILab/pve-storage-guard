# Roadmap

The roadmap is evidence-gated, not date-gated. A version may remain in a phase
until its safety criteria pass.

## v0.1 — Observer and offline replay

- Versioned observation, policy, decision, and event schemas.
- Deterministic `storage-slo-guard` AIMD and bounded allocator.
- PVE read-only discovery/metrics adapter (initial explicit OpenZFS binding
  implemented; sanitized PVE 9.2 / OpenZFS 2.4 source-format compatibility
  passed; serial watch, cancellation tests, and a statically verified hardened
  observer unit are implemented; an identity-free, digest-bound non-production
  validator is ready, while its compiled-PVE permission/runtime result remains
  gated).
- Offline replay with anonymized fixtures and golden reports.
- Prometheus metrics, structured decisions, ITOps integration examples.
- Local dry-run and shadow mode only.
- Apache-2.0 open-source foundation, CI, GHCR build, Pages documentation.

Exit gate: reproducible reports, independent trace, sensitivity analysis,
observer stability, useful multi-signal alerting, and no sensitive data in the
public distribution.

## v0.2 — Approved canary actuation

- Minimal structured PVE actuator with least privilege.
- Durable desired/effective state, lease, apply/read-back, expiry, and rollback.
- Non-critical disk canary workflow and fault injection.
- Policy approval envelope integrated with ITOps task handoff.
- Signed multi-architecture releases, SBOM, provenance, and operational package.

Exit gate: controlled load, cold restart, drift, stale input, lease conflict,
read-back mismatch, and rollback tests pass; canary evidence is reviewed.

## v0.3 — Controlled multi-disk operation

- Weighted allocation across multiple enrolled disks in one storage domain.
- Admission coordination for bulk tasks.
- Storage-class profiles based on retained evidence.
- Safer configuration rollout and policy-diff tooling.
- Expanded dashboard and incident comparison.

Exit gate: no cross-workload starvation regression and stable control under
bursty/mixed workloads.

## v1.0 — Production-ready PVE distribution

- Stable APIs and migration policy.
- Reviewed PVE version compatibility matrix.
- Debian packages, container, hardened systemd deployment, upgrade/rollback.
- Threat model, security audit, performance bounds, disaster recovery runbook.
- Published evidence limits and support expectations.

## Future platform adapters

After the PVE boundary is proven:

- ordinary Linux using cgroup v2 `io.max`, `io.latency`, or `io.cost` adapters;
- Kubernetes via cgroup/runtime/node integrations and explicit workload
  enrollment;
- other hypervisors through inventory and constrained actuator adapters;
- additional storage backends with backend-specific SLO observations.

The generic `storage-slo-guard` kernel may become a separately versioned module
only when two real adapters demonstrate that the boundary is stable. Predictive
or online-learning control remains deferred until it can meet the same
explainability, bounds, replay, and rollback requirements.
