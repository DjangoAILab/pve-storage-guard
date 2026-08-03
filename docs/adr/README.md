# Architecture decision records

- [ADR-0001: PVE product layer with a generic policy kernel](0001-pve-product-generic-policy-kernel.md)
- [ADR-0002: One bounded controller per storage domain](0002-one-controller-per-storage-domain.md)
- [ADR-0003: Preserve metric semantics and qualify replay evidence](0003-explicit-metric-semantics-and-trace-evidence.md)
- [ADR-0004: Verify sealed journals before external ingestion](0004-sealed-journal-handoff.md)
- [ADR-0005: Use content-addressed batches for sealed journal import](0005-content-addressed-sealed-journal-batches.md)
- [ADR-0006: Separate storage-only research from promotion evidence](0006-separate-storage-research-from-promotion-evidence.md)
- [ADR-0007: Use a local, read-only PVE agent with typed evidence](0007-local-read-only-pve-agent.md)
- [ADR-0008: Separate licensed workload-shape artifacts from replay evidence](0008-licensed-workload-shape-research.md)
- [ADR-0009: Preserve explicit storage class in workload-shape evidence](0009-explicit-storage-class-in-workload-shape.md)
- [ADR-0010: Calibrate alerts with evidence coverage, not elapsed time](0010-evidence-based-alert-calibration.md)

ADRs are immutable decision history. A superseding ADR changes a decision; the
original record remains.
