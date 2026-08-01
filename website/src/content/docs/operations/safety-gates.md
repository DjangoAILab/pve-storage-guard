---
title: Safety gates
description: The evidence path from offline replay to an expiring one-disk canary.
---

1. **Offline replay:** deterministic tests, golden reports, bounded sensitivity.
2. **Observer:** collect 1 Hz storage and management signals without proposals.
3. **Shadow:** journal bounded proposals and tune alerts without mutation.
4. **Controlled load:** use one non-critical disk with a reviewed,
   storage-domain-specific static rollback limit.
5. **Fault injection:** stale telemetry, restart, lease conflict, drift,
   apply/read-back mismatch, and rollback.
6. **Canary approval:** bind exact resource, immutable policy, envelope, expiry,
   prior effective state, and rollback.
7. **Review:** expand only after SLO and management-plane evidence passes.

Production service installation, configuration changes, or actuation are human
checkpoints. GitHub Actions never deploy directly to a PVE host.
