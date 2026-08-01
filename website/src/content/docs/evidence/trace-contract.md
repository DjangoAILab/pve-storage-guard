---
title: Replay trace contract
description: How PVE Storage Guard separates valid test data from independent observed evidence.
---

PVE Storage Guard records metric semantics before it evaluates evidence. A
synthetic trace, a modeled scenario, or another window derived from the same
incident can be useful without being independent production evidence.

Every candidate trace declares its source kind, independence group, storage and
workload classes, sampling interval, sanitization status, write-wait statistic,
full window duration, window aggregation, and provenance. Relative offsets
replace host timestamps and identities. Completeness uses the declared window,
so missing samples at its beginning or end cannot disappear through cropping.

The current controller API consumes a real p95 observation. Diskstats average
service time and ZFS total-wait must retain those labels; neither can be renamed
as I/O p95. The reference incident is therefore a pressure-proxy replay, not
production p95 threshold calibration.

An independent observed trace currently requires a distinct independence
group, at least 600 seconds, at least 95% completeness, known storage/workload
classes, compatible p95 semantics, and a clean structural validation result.

```sh
python3 poc/trace_contract.py candidate.json \
  --reference-group reference-incident
```

The schema and validator live in the public repository. No qualifying second
trace is claimed yet. Passing the machine gate is necessary but cannot prove a
self-declared provenance; source hashing, redaction review, and human evidence
approval remain required.
