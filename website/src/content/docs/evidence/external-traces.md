---
title: External trace research
description: What public storage datasets can prove, what they cannot prove, and how to convert authorized data safely.
---

Public storage traces are valuable for workload-shape and policy stress tests,
but the sources reviewed so far do not contain synchronized evidence of SSH,
PVE API, or an equivalent host-management plane. Storage-only data therefore
cannot close the active-control promotion gate.

## Sources reviewed

| Source | Latency | Management evidence | Use here |
| --- | --- | --- | --- |
| [UMass/SPC traces](https://traces.cs.umass.edu/docs/traces/storage/) | no portable standard field | none | workload shape |
| [Tencent, Alibaba, CloudPhysics catalog](https://github.com/cacheMon/cache_dataset#block-cache-traces) | not in listed block formats | none | workload shape |
| [MSR Cambridge catalog](https://github.com/cacheMon/cache_dataset#msr-cambridge-traces) | per-I/O response time | none | held for license review |
| [Google Thesios](https://github.com/cacheMon/cache_dataset#google-synthetic-io-traces) | yes | none | synthetic tests only |

The CacheMon collection labels its work CC BY 4.0, while a read-only inspection
of the linked MSR archive found an older Microsoft disclaimer with materially
more restrictive redistribution terms. The project bundles neither that
archive nor derived MSR samples. Permission must be established independently.

## Safe research lane

The repository provides a standard-library-only converter for caller-supplied,
authorized CSV data. It accepts timestamp, operation, size, and response time;
drops every unspecified source column; and emits relative offsets, nearest-rank
write p95, IOPS, and throughput. Timestamp-ordered input is aggregated one
interval at a time rather than retained as a full per-I/O copy.
The caller must explicitly declare whether latency was measured at the storage
domain, block device, virtual disk, application, or an unknown layer.

```sh
python3 poc/io_csv_to_replay_trace.py authorized.csv \
  --name licensed-study-a \
  --source-kind observed \
  --independence-group external-study-a \
  --storage-class rotational-hdd \
  --workload-class mixed \
  --write-wait-measurement-layer block-device \
  --confirm-authorized-and-sanitized > candidate.json
```

Because block-I/O rows do not prove host availability, every converted sample
has `managementPlaneStatus=unknown`. The trace can exercise storage algorithms,
but the machine assessor must reject it as production promotion evidence. A
block-device p95 also cannot be relabeled as the controller's storage-domain
p95. The confirmation flag is an operator assertion, not an automated license
check.

[Read the complete review and input contract on GitHub](https://github.com/DjangoAILab/pve-storage-guard/blob/main/docs/EXTERNAL-TRACE-RESEARCH.md).
