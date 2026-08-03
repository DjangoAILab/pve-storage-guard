# Contributing storage traces safely

PVE Storage Guard welcomes independent evidence, but GitHub is not a raw-data
drop. Start with the
[trace qualification Issue form](https://github.com/DjangoAILab/pve-storage-guard/issues/new?template=trace.yml).
The form collects qualification metadata only.

## Never publish these inputs

Do not attach, paste, commit, or link to:

- raw production traces or logs;
- credentials, internal or signed URLs, network coordinates, or file paths;
- hostnames, node names, VM/CT IDs, pool names, device serials, or guest data;
- customer, organization, user, workload, or project identities;
- absolute production timestamps or an incident narrative that can re-identify
  an operator or system.

Keep the source data under your control. Opening an Issue does not grant the
project permission to download, retain, transform, or redistribute it. If a
maintainer needs information that cannot safely be public, the request remains
on hold; do not work around the boundary by posting it in another public place.
Maintainers must not automatically fetch a submitted URL or treat a URL as
authorization to retrieve its target; only an ordinary public documentation
page may be reviewed manually.

If sensitive material is posted accidentally, do not add more detail or quote
it in a follow-up. Remove the content and attachment where possible, report the
incident privately through the repository Security tab, and rotate any exposed
credential. Treat public submission as disclosure even after removal because
the project cannot guarantee deletion from platform history or caches.

## Choose the evidence lane

### Storage research

Use this lane when the source can describe workload shape, I/O latency, IOPS,
or throughput but has no synchronized management-plane observation. The
repository's CSV converter deliberately emits
`managementPlaneStatus=unknown`. Such a trace can test aggregation and policy
behavior, but cannot prove SSH, PVE API, or host-management availability and
cannot pass the active-control promotion gate.

### Promotion candidate

Use this lane only when the source can support every claim below:

- observed provenance and an independence group distinct from the reference
  incident;
- known storage and workload classes;
- a storage-domain write-latency p95 compatible with the controller input;
- synchronized, genuinely observed management-plane status;
- a declared window of at least 600 seconds;
- at least 95% structural, valid-wait, and known-management coverage;
- documented permission, transformations, and sanitization review.

Diskstats average wait, block-device latency, application latency, ZFS
`total_wait`, and a p95 computed over the wrong measurement layer must retain
their real labels. They are not interchangeable with storage-domain write p95.

## Local preparation

For authorized per-I/O CSV data, use the bounded-column converter described in
[external trace research](EXTERNAL-TRACE-RESEARCH.md#local-only-conversion-contract).
It drops unspecified columns and replaces timestamps with relative offsets.
The authorization flag is an operator assertion, not a license detector.

```sh
python3 poc/io_csv_to_replay_trace.py authorized.csv \
  --name licensed-study-a \
  --source-kind observed \
  --independence-group external-study-a \
  --storage-class rotational-hdd \
  --workload-class mixed \
  --write-wait-measurement-layer block-device \
  --confirm-authorized-and-sanitized > candidate.json

python3 poc/trace_contract.py candidate.json \
  --reference-group reference-incident
```

Review the JSON assessment, not just the process exit code. A structurally
valid trace can correctly report `policy_signal_compatible: false` or
`meets_machine_independence_gate: false`.

Before proposing any generated artifact, inspect it manually for residual
identity and confirm that its source terms permit the exact proposed use and
redistribution. Do not submit the candidate trace until a maintainer has
classified the metadata Issue and requested a focused pull request.

## Review states

1. **Metadata review:** the Issue contains no data and declares the intended
   lane, source authority, semantics, sampling shape, and privacy posture.
2. **Local assessment:** the contributor runs conversion and validation without
   giving the project raw data.
3. **Human evidence review:** a maintainer reviews permission, provenance,
   independence, semantic support, transformations, and disclosure risk.
4. **Publication review:** a separate pull request may contain only artifacts
   explicitly approved for redistribution. Privacy CI and normal review apply.
5. **Accepted for research or promotion candidate:** the final disposition is
   recorded precisely. Machine qualification does not authorize production
   control; controlled-load, soak, canary, and explicit production gates remain.

Unclear permission, incomplete semantics, or sensitive data keeps the request
on hold. Rejection from the promotion lane does not make a trace useless: it may
still be valuable research evidence when labeled accurately.
