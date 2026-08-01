---
title: Policy and safety
description: Disk envelopes, bounded AIMD, precedence, and safe degradation.
---

## Resolution and enrollment

Resolution order is exact disk override, workload class, then storage-domain
default. Exact adapter identity is required for enrollment. Root, boot, quorum,
database, and other critical disks are excluded by default.

## Current shadow candidate

| Parameter | Value |
| --- | ---: |
| Minimum / initial / maximum | 5 / 20 / 25 MiB/s |
| Healthy / target / emergency wait | 15 / 25 / 100 ms |
| Additive increase | 0.5 MiB/s |
| Multiplicative decrease | 0.5 |
| Healthy / breach windows | 12 / 2 |
| Control interval / cooldown | 10 / 60 s |

In sensitivity replay, 16 of 18 one-at-a-time neighbors passed the fixed-20
safety gate. Increasing by 1 MiB/s or increasing after only six healthy windows
failed. The conservative rise is therefore retained for shadow validation.

## Invariants

1. No proposal escapes hard storage-domain or disk bounds.
2. Stale, invalid, or conflicting telemetry never increases the budget.
3. Only one lease owner may write a storage domain.
4. Unknown effective state blocks actuation.
5. Drift freezes the affected resource.
6. Apply/read-back mismatch restores only pre-authorized prior state, then stops.
7. The controller cannot stop guests, alter topology, or execute commands.
