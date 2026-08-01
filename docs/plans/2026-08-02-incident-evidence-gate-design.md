# Incident Evidence Gate Design

## Problem

The retained one-second write-wait samples begin after the first retained
durability degradation and after a management service restart. They prove that
the storage-pressure signature is detectable once collection starts, but they
cannot prove advance warning. A later 60-sample natural-load check also rejected
a 20 MiB/s per-disk cap, but only aggregate statistics survived. Treating that
summary as a replayable trace would manufacture observations.

## Decision

Keep three evidence classes separate:

1. the existing observed one-second wait samples, which may be shadow-replayed;
2. sanitized timeline markers, which may establish coverage and relative
   ordering but may not be fed into a controller;
3. aggregate field-validation results, which may block promotion or invalidate
   a fallback claim but may not be expanded into samples.

A dependency-free validator and assessor will consume a new versioned incident
evidence document. Its metric semantics, sample interval, count, and canonical
sample-array SHA-256 bind conclusions to the exact public observed series. The
generated report will state the first two-sample pressure
detection offset, the first critical-sample offset, whether telemetry existed
before the failure marker, and which corroborating signals are absent. It will
also state that fixed 20 is a model comparator, not a validated production
fallback, when the field-validation outcome is rejected and rolled back.

## Alternatives

- Documentation only was rejected because generated reports could silently
  regress to stronger claims.
- Synthesizing the missing 60 samples was rejected because aggregate p99 and
  threshold counts do not determine a unique time series.
- Mixing timeline markers into the replay trace was rejected because event
  ordering and latency samples have different semantics.

## Safety and privacy

The fixture uses semantic event kinds and the existing public aliases. It
contains no host, VM, application, user, network, device, pool, or credential
identity. Raw logs and command output remain private. The assessor fails closed
on invalid ordering, count/range errors, a field check falsely marked replayable,
or a claimed independent validation group.

## Promotion meaning

The output can prove rapid recognition after telemetry begins. It cannot prove
pre-failure warning, causal prevention, or a safe active default. Promotion
remains blocked pending a qualifying independent trace, continuous 1 Hz live
history, and a controlled non-critical load test.
