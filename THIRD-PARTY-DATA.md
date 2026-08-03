# Third-party data notices

## UMass/SPC WebSearch1 derived workload shape

The file
[`poc/fixtures/umass-spc-websearch1-workload-shape.json`](poc/fixtures/umass-spc-websearch1-workload-shape.json)
is an adapted, identity-reducing aggregate of the `WebSearch1.spc.bz2` trace
published by the [UMass Trace Repository](https://traces.cs.umass.edu/docs/traces/storage/).
The repository credits Ken Bates of HP, Bruce McNutt of IBM, and the Storage
Performance Council for the source trace.

The UMass repository states that its data is provided under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/) unless otherwise
specified; the storage page identifies no different terms for this trace. The
adaptation is provided under those terms. This project and its use are not
endorsed by UMass, HP, IBM, or the Storage Performance Council.

Source reviewed on 2026-08-03:

- source URL: `https://skulddata.cs.umass.edu/traces/storage/WebSearch1.spc.bz2`
- source size: 7,963,620 bytes
- source SHA-256: `af0fdbd26834387c8b8ad3d5052c8a1310136a1fd3514f00f663726bd679b15a`
- SPC format: [revision 1.0.1](https://skuld.cs.umass.edu/traces/storage/SPC-Traces.pdf)

Changes made:

- selected the first 600 seconds relative to the first source record;
- aggregated records into 60 ten-second buckets;
- retained only read/write IOPS and read/write throughput;
- removed ASU, LBA, optional fields, source timestamps, and the raw archive;
- labeled latency and management-plane evidence unavailable;
- labeled the storage class unavailable rather than inferring hardware.

Derived artifact:

- size: 10,473 bytes
- SHA-256: `7b3d6211107b58ba8707ae8f076c970ff07ee3b777188dbb6d7cc50e02449719`

The artifact contract was upgraded from draft v1alpha1 to v1alpha2 before
merge. That change adds an explicit `storageClass: unknown` field; it does not
add or infer storage-class evidence.

## Alibaba Block Traces Ultra Disk derived workload shape

The file
[`poc/fixtures/alibaba-block-ultra-2020-prefix-workload-shape.json`](poc/fixtures/alibaba-block-ultra-2020-prefix-workload-shape.json)
is an adapted, identity-reducing aggregate of the
[Alibaba Block Traces](https://github.com/alibaba/block-traces). Alibaba states
that the source records were observed in a production Elastic Block Storage
cluster, cover virtual Ultra Disk products, and that both the trace data and
documentation are licensed under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). The adaptation is
provided under those terms. This project and its use are not endorsed by
Alibaba or the CacheMon maintainers.

The project used the CC BY 4.0 converted plain-text mirror documented by the
[CacheMon dataset catalog](https://github.com/cacheMon/cache_dataset#alibaba-cloud-ebs-traces).
The raw mirror object and fixed compressed prefix remain outside Git history.
Source reviewed on 2026-08-03:

- mirror object: `https://cache-datasets.s3.amazonaws.com/cache_dataset_txt/2020_alibabaBlock/alibabaBlock2020.csv.zst`
- full object size: 150,421,221,737 bytes
- full object ETag: `f3e7bc83ebc8577e8deb21fb998cfb80-8966` (multipart identity, not a content hash)
- processed byte range: 0–301,989,887 inclusive
- processed compressed-prefix SHA-256: `25c03ea788d3d94478532a231e0a3f7c5373fd2f63b069690cd880237909e040`

Changes made:

- selected records arriving in the first 600 seconds relative to the first
  source record, then stopped at the first record outside the window;
- aggregated 5,180,237 records into 60 ten-second buckets;
- retained only read/write IOPS and read/write throughput;
- removed anonymized device IDs, byte offsets, absolute source timestamps, and
  the raw compressed/decompressed prefixes;
- preserved `network-block` only as the documented virtual EBS product class;
  no physical disk technology is inferred;
- labeled latency and management-plane evidence unavailable.

Independent count and byte-sum checks matched the artifact within the declared
six-decimal per-bucket rounding bound. Derived artifact:

- size: 11,176 bytes
- SHA-256: `1af0eecd16a2d9df3cf460eb5ef0a1c68727002892647f0db4a079eb703b09f4`
