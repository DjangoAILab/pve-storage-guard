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

- size: 10,442 bytes
- SHA-256: `13e2825963dd9d07a48bec2314d1fa89bc5f865368835c5d4df1e9475246f860`
