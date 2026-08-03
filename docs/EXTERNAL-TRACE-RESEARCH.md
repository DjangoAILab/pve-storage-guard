# External trace research

## Decision summary

Observed public storage traces are useful for workload-shape and policy stress
tests, but none reviewed so far carries synchronized evidence of PVE, SSH, API,
or an equivalent host-management plane. They therefore cannot close the active
control promotion gate by themselves.

PVE Storage Guard supports a separate research lane for user-supplied,
authorized per-I/O CSV data. The converter produces a sanitized v1alpha2 trace
with management status explicitly set to `unknown`. Such a trace may validate
storage-signal handling, but the assessor must reject it as production
promotion evidence.

Community proposals begin with the metadata-only
[trace contribution process](TRACE-CONTRIBUTION.md). GitHub Issues and pull
requests are not raw-data upload channels.

## Candidate review

| Source | Observed | Latency | Management evidence | Current disposition |
| --- | --- | --- | --- | --- |
| [UMass/SPC OLTP and search traces](https://traces.cs.umass.edu/docs/traces/storage/) | yes | no standard latency field | none | one attributed search prefix accepted as workload-shape research evidence |
| [Alibaba Block Traces](https://github.com/alibaba/block-traces) | yes | no response latency field | none | one attributed Ultra Disk prefix accepted for workload-shape plus logical storage-class research |
| [Tencent and CloudPhysics block traces catalogued by CacheMon](https://github.com/cacheMon/cache_dataset#block-cache-traces) | yes | catalog formats expose arrival, size, and direction but not response latency | none | candidate workload-shape research only |
| [MSR Cambridge catalog entry](https://github.com/cacheMon/cache_dataset#msr-cambridge-traces) | yes | per-I/O response time | none | license review required; no project redistribution |
| [Google Thesios trace](https://github.com/cacheMon/cache_dataset#google-synthetic-io-traces) | synthetic | yes | none | synthetic testing only |
| [SNIA real-world workload captures](https://www.snia.org/blog/2019/your-questions-answered-now-you-can-be-part-real-world-workload-revolution) | yes | response-time and queue statistics may be captured | no synchronized host-management contract identified | promising research format, not a promotion trace yet |

The SPC standard record has ASU, LBA, size, opcode, and timestamp fields; its
optional fields are intentionally undefined. It therefore cannot supply a
portable latency or management-health contract. See the
[SPC trace format](https://skuld.cs.umass.edu/traces/storage/SPC-Traces.pdf).

### Accepted UMass/SPC workload shape

The UMass Trace Repository states that its data is CC BY 4.0 unless otherwise
specified, and the storage page gives no different terms for `WebSearch1`.
The repository therefore includes a 600-second, 10-second-bucket derived
[workload-shape artifact](../poc/fixtures/umass-spc-websearch1-workload-shape.json),
not the raw trace. The transformation drops ASU, LBA, optional columns, and the
absolute origin; it retains only read/write IOPS and throughput. The source and
artifact hashes, attribution, license links, and exact modifications are in
[third-party data notices](../THIRD-PARTY-DATA.md).

The independent validator reports 60/60 samples, 100% structural completeness,
60 read-active buckets (600 bucket-seconds), 19 write-active buckets (190
bucket-seconds), and
`meets_research_gate=true`. It also reports
`active_control_eligible=false`: the source provides neither latency nor a
synchronized management probe, and it does not identify a storage class. This
closes one independent workload-shape research sub-item only; it does not close
the independent promotion trace or storage-class gates.

Reproduce the identity-removing conversion from an independently downloaded,
hash-matched source file with:

```sh
python3 poc/spc_to_workload_shape.py WebSearch1.spc.bz2 \
  --name umass-spc-websearch1-prefix \
  --independence-group umass-spc-websearch-2004 \
  --workload-class search \
  --sample-interval-seconds 10 \
  --window-duration-seconds 600 \
  --confirm-authorized-and-sanitized \
  --output candidate.json

python3 poc/workload_shape_contract.py candidate.json \
  --reference-group reference-incident
```

### Accepted Alibaba Ultra Disk workload shape and storage class

The Alibaba source repository explicitly licenses the trace data and document
under CC BY 4.0 and describes the records as observed production Elastic Block
Storage requests for virtual Ultra Disk products. The accepted derivative uses
`network-block` as that logical product class. It does not infer HDD, SSD, or
NVMe media behind the service.

The committed
[workload-shape artifact](../poc/fixtures/alibaba-block-ultra-2020-prefix-workload-shape.json)
covers 600 seconds in 60 ten-second buckets. The bounded converter stops at the
first record outside the requested window and drops device IDs, offsets, and
absolute timestamps. The processed compressed byte range, its SHA-256, the
mirror object identity, artifact hash, license, attribution, and exact changes
are recorded in [third-party data notices](../THIRD-PARTY-DATA.md); neither raw
nor decompressed source data is committed.

An independent pre-boundary scan counted 5,180,237 records with strictly
nondecreasing arrival timestamps: 1,641,300 reads and 3,538,937 writes. Artifact
counts matched exactly, and byte sums matched within the mathematically bounded
six-decimal per-bucket rounding error. The assessor reports 60/60 samples,
100% structural completeness, `meets_research_gate=true`,
`storage_class_known=true`, and
`meets_storage_class_research_gate=true`.

It still reports `active_control_eligible=false`. The trace contains neither
response latency nor a synchronized management probe, so it cannot calibrate a
controller threshold, prove management-plane protection, or close the
promotion trace gate.

Reproduce the identity-removing conversion from an independently obtained,
license-reviewed, decompressed prefix with:

```sh
python3 poc/alibaba_block_to_workload_shape.py alibabaBlock2020-prefix.csv \
  --name alibaba-block-ultra-2020-prefix \
  --independence-group alibaba-block-traces-2020 \
  --workload-class mixed \
  --sample-interval-seconds 10 \
  --window-duration-seconds 600 \
  --confirm-authorized-and-sanitized \
  --output candidate.json

python3 poc/workload_shape_contract.py candidate.json \
  --reference-group reference-incident
```

### MSR licensing hold

The CacheMon collection labels its work CC BY 4.0. A read-only header inspection
of the linked MSR archive found an older Microsoft disclaimer with materially
more restrictive redistribution terms. Those statements have not been legally
reconciled. This repository therefore contains neither the archive nor raw or
derived MSR samples, and the converter performs no download. Anyone using that
source must independently establish permission before processing or publishing
results.

## Local-only conversion contract

`poc/io_csv_to_replay_trace.py` accepts a caller-provided CSV with these columns:

| Column | Unit / values |
| --- | --- |
| `timestamp_seconds` | finite non-negative seconds |
| `operation` | `read`, `r`, `write`, or `w` |
| `size_bytes` | non-negative integer bytes |
| `response_time_milliseconds` | finite non-negative milliseconds |

The caller must also declare the measurement layer as `storage-domain`,
`block-device`, `virtual-disk`, `application`, or `unknown`. This is a semantic
claim that requires source documentation; the converter does not infer it.

Additional input columns are ignored and never copied. Timestamp-ordered rows
are aggregated one interval at a time, so per-I/O memory is bounded by the
busiest interval rather than the full source. Output contains relative offsets,
nearest-rank write p95, write IOPS, write throughput, coarse class labels, and no
source identity. Safe slug validation reduces accidental metadata leakage, but
a human review is still mandatory.

Example:

```sh
python3 poc/io_csv_to_replay_trace.py authorized.csv \
  --name licensed-study-a \
  --source-kind observed \
  --independence-group external-study-a \
  --storage-class rotational-hdd \
  --workload-class mixed \
  --write-wait-measurement-layer block-device \
  --sample-interval-seconds 1 \
  --confirm-authorized-and-sanitized > candidate.json

python3 poc/trace_contract.py candidate.json \
  --reference-group reference-incident
```

The confirmation flag is an operator assertion, not a license detector. The
converter marks every management sample `unknown`, so this output cannot pass
the machine independence gate without a separately reviewed join to genuine
management-plane observations.

SPC inputs without a standardized latency field must use the separate
`WorkloadShapeTrace` converter above. They must not receive a placeholder
`writeWaitStatistic` merely to fit the replay schema.

## Evidence still required

A qualifying promotion trace needs both a policy-compatible write p95 and
synchronized management evidence for at least 95% of a declared window. It also
needs a matching storage-domain measurement layer, observed provenance,
independent origin, known storage/workload classes, at least ten minutes,
sanitization review, and the existing controlled-load and soak gates. External
storage-only evidence does not lower those requirements.
