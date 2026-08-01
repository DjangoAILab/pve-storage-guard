# Project goal

## Mission

Deliver a high-quality, publicly distributable PVE Storage Guard project and a
rigorous local production practice that closes the loop between detection,
explainable control, audit, rollback, and ITOps alerting.

The first product serves Proxmox VE. Its internal `storage-slo-guard` policy
kernel must remain independent of PVE inventory and mutation semantics so that
future adapters can support ordinary Linux, Kubernetes, and other virtualized
environments.

## Success criteria

### Evidence and policy

- Reproducible replay compares no control, fixed limiting, threshold limiting,
  and bounded adaptive control.
- Results cover latency, IOPS, throughput, queue pressure, management-plane
  availability, job completion time, and limit-change churn when data exists.
- Missing historical signals are explicitly identified; they are not invented.
- A recommended default passes a safety-first gate across conservative,
  nominal, and optimistic models and remains shadow-only until live gates pass.

### Architecture and safety

- Components have explicit ownership and privilege boundaries.
- Policy is scoped primarily to a storage domain, with per-disk envelopes and
  precedence rules.
- Default operation is read-only/dry-run; all proposals are explainable and
  auditable.
- Stale data, controller failure, policy drift, apply/read-back mismatch, and
  restart have safe behaviors.
- Actuation is allowlisted, recoverable, bounded, and incapable of arbitrary
  command execution.

### Open-source delivery

- Public repository: `DjangoAILab/pve-storage-guard`.
- Public image: `ghcr.io/djangoailab/pve-storage-guard`.
- CI covers lint, tests, replay, build, security scanning, SBOM, and docs.
- GitHub Pages hosts the project landing and documentation site.
- README and site use the SEO title **PVE Storage Guard — Prevent Proxmox VE
  Disk I/O Starvation** and include an independence disclaimer.
- No unverified effect is advertised as a production result.

### Local practice and ITOps

- Observer/dry-run operation detects the historical failure signature and
  records decisions without mutation.
- ITOps receives disk latency, PSI, queue, management-plane health, controller
  health, decision, and actuation-verification signals.
- Alerts combine multiple signals and link to a runbook and decision journal.
- Canary and rollback are rehearsed before scope is expanded.

## Non-goals for the first release

- A universal storage performance optimizer.
- Automatic enrollment of every disk or control of root/critical disks.
- PID, predictive ML, or online self-tuning.
- Workload scheduling, VM lifecycle operations, or storage repair.
- Direct GitHub Actions deployment to production PVE hosts.
- Claims of causal prevention based on a single historical incident.

## Phase boundaries

1. **Foundation:** local fact documents, sanitization rules, repeatable PoC.
2. **Observer:** PVE metrics and policy proposals with no mutation.
3. **Shadow:** durable policy state, decisions, alerts, and effective-state reads.
4. **Canary:** one explicitly enrolled non-critical disk, approved actuation,
   fault injection, and rollback rehearsal.
5. **Controlled release:** public repository, packages/images/docs, then gradual
   local scope expansion after evidence gates pass.

Production writes, disclosure of uncertain incident material, and irreversible
operations are explicit human checkpoints rather than implicit phase steps.

## Source-of-truth rule

This repository is the project fact source. `docs/CHECKLIST.md` records status
and evidence; ADRs record decisions. Session messages, scratch worktrees, and
generated reports do not supersede reviewed repository documents.
