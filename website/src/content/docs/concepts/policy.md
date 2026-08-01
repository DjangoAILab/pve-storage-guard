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

The online observation field is a true p95 contract. Diskstats average service
time and ZFS total-wait cannot be substituted without a separately calibrated
policy. The historical incident therefore informs shadow behavior, not direct
production threshold values.

## Invariants

1. No proposal escapes hard storage-domain or disk bounds.
2. Stale, invalid, or conflicting telemetry never increases the budget.
3. An authoritative lease and fencing generation are required before actuation.
4. An authoritative, unexpired approval must match the exact resource envelope.
5. Unknown effective state blocks actuation.
6. Drift, actuator error, or read-back mismatch freezes the affected resource.
7. Any future restoration is pre-authorized; the current unwired gate only
   freezes and requires operator reconciliation.
8. The controller cannot stop guests, alter topology, or execute commands.
