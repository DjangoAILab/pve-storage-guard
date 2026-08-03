# ADR-0008: Calibrate alerts with evidence coverage, not elapsed time

## Status

Accepted — 2026-08-03

## Context

A fixed observation period is simple to communicate, but elapsed time does not
prove that the sample contains quiet, busy, severe, firing, and recovery
behavior. A long all-quiet window may contain less calibration evidence than a
shorter representative trace. Deployment health and alert readiness are also
different claims: accepting a read-only observer must not silently authorize an
alert or an actuator.

## Decision

- Accept deployment health through bounded, independent checks of exact release
  identity, collector freshness, metric semantics, privacy, rollback readiness,
  and disabled alert state. Record the real observation length and do not claim
  a soak that did not occur.
- Calibrate storage-pressure alerts with a deterministic, read-only replay over
  completed collection cycles rather than a fixed number of hours or days.
- Require structural completeness plus quiet, busy, and severe regimes; exact
  detector recomputation; and firing/recovery lifecycles for warning and
  critical rules.
- Require the exact reviewed rule set and configuration, including metric,
  operator, value, debounce, cooldown, labels, and disabled state.
- Emit aggregate gate results only. Target IDs, resource IDs, run IDs,
  timestamps, endpoints, and raw labels remain private.
- If natural traffic lacks a required regime, use a separately approved,
  non-critical controlled load. Do not lower the gate merely to obtain a pass.
- Keep alert arming, notification delivery, journal registration, and actuation
  behind separate approvals.

## Consequences

- Short, healthy deployment acceptance can complete without inventing a
  statistical coverage claim.
- Alert readiness depends on representative evidence and lifecycle behavior,
  not calendar duration.
- A calibration failure is actionable: it reports missing evidence categories
  while leaving every production control disabled.
