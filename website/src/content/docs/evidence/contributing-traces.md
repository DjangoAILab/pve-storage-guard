---
title: Contributing traces safely
description: Propose independent storage evidence without exposing production data or weakening promotion gates.
---

PVE Storage Guard welcomes independent evidence, but GitHub Issues and pull
requests are not raw-data upload channels. Begin with the
[metadata-only qualification form](https://github.com/DjangoAILab/pve-storage-guard/issues/new?template=trace.yml).

Never publish raw traces or logs, private or signed URLs, credentials, internal
network coordinates or paths, host/guest identities, device serials, absolute
production timestamps, or customer/workload data. Keep the source under your
control. Opening an Issue grants no permission to download, retain, transform,
or redistribute it. Submitted URLs are never fetched automatically.

If sensitive material is posted accidentally, do not quote it in a follow-up.
Remove it where possible, use the repository Security tab to report the
incident privately, rotate exposed credentials, and treat it as disclosed even
after removal.

## Two evidence lanes

**Storage research** accepts correctly labeled workload shape, block I/O, or
latency evidence without synchronized management observations. Missing
management data remains `unknown`. This lane can test aggregation and policy
behavior, but cannot prove PVE, SSH, or host-management availability.

**Promotion candidates** must declare observed independent provenance, known
storage/workload classes, compatible storage-domain write p95, synchronized
management status, a window of at least 600 seconds, and at least 95%
structural, valid-wait, and known-management coverage.

Diskstats average wait, block-device or application latency, and ZFS total wait
must retain their actual measurement labels. They cannot be renamed to match a
promotion contract.

## Review flow

1. Submit non-sensitive qualification metadata only.
2. Run conversion and the v1alpha2 assessor locally.
3. Let a maintainer review source authority, terms, provenance, independence,
   transformations, and residual disclosure risk.
4. Publish an approved sanitized artifact only through a separate focused pull
   request.

Machine qualification is necessary but cannot establish legal permission or
truth of self-declared evidence. It also never authorizes production control;
controlled-load, soak, canary, and explicit production gates remain.

[Read the complete contribution contract](https://github.com/DjangoAILab/pve-storage-guard/blob/main/docs/TRACE-CONTRIBUTION.md).
