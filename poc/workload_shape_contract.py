#!/usr/bin/env python3
"""Validate identity-free workload-shape traces for the research lane."""

from __future__ import annotations

import argparse
import json
import math
import re
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Dict, List, Optional


API_VERSION = "guard.storage-slo.io/workload-shape-trace/v1alpha2"
KIND = "WorkloadShapeTrace"
MAXIMUM_DURATION_SECONDS = 86_400
MINIMUM_RESEARCH_DURATION_SECONDS = 600
MINIMUM_COMPLETENESS = 0.95
SAFE_SLUG = re.compile(r"^[a-z0-9][a-z0-9.-]{0,127}$")
WORKLOAD_CLASSES = {"backup", "build", "database", "migration", "search", "mixed", "unknown"}
STORAGE_CLASSES = {"rotational-hdd", "sata-ssd", "nvme", "network-block", "unknown"}
TIMESTAMP_SEMANTICS = {"issue-offset-seconds", "arrival-offset-seconds"}
IO_LAYERS = {"host-to-logical-unit", "virtual-block-service"}
SEMANTIC_PAIRS = {
    ("issue-offset-seconds", "host-to-logical-unit"),
    ("arrival-offset-seconds", "virtual-block-service"),
}


@dataclass(frozen=True)
class WorkloadShapeAssessment:
    errors: List[str]
    sample_count: int
    duration_seconds: int
    completeness: float
    read_active_bucket_seconds: int
    write_active_bucket_seconds: int
    storage_class_known: bool
    meets_research_gate: bool
    meets_storage_class_research_gate: bool
    active_control_eligible: bool


def assess_workload_shape(
    document: Dict[str, object], reference_group: Optional[str] = None,
) -> WorkloadShapeAssessment:
    """Validate shape without implying latency or management-plane evidence."""
    errors: List[str] = []
    if not isinstance(document, dict):
        return WorkloadShapeAssessment(
            errors=["trace must be an object"],
            sample_count=0,
            duration_seconds=0,
            completeness=0.0,
            read_active_bucket_seconds=0,
            write_active_bucket_seconds=0,
            storage_class_known=False,
            meets_research_gate=False,
            meets_storage_class_research_gate=False,
            active_control_eligible=False,
        )

    if set(document) != {"apiVersion", "kind", "metadata", "metricSemantics", "samples"}:
        errors.append("top-level fields do not match the strict contract")
    if document.get("apiVersion") != API_VERSION or document.get("kind") != KIND:
        errors.append("unsupported apiVersion or kind")

    metadata = document.get("metadata")
    semantics = document.get("metricSemantics")
    samples = document.get("samples")
    if not isinstance(metadata, dict):
        errors.append("metadata must be an object")
        metadata = {}
    if not isinstance(semantics, dict):
        errors.append("metricSemantics must be an object")
        semantics = {}
    if not isinstance(samples, list):
        errors.append("samples must be an array")
        samples = []

    allowed_metadata = {
        "name", "sourceKind", "independenceGroup", "storageClass", "workloadClass",
        "sampleIntervalSeconds", "windowDurationSeconds", "sanitized",
    }
    if set(metadata) != allowed_metadata:
        errors.append("metadata fields do not match the strict contract")
    for field in ("name", "independenceGroup"):
        if not isinstance(metadata.get(field), str) or not SAFE_SLUG.fullmatch(metadata[field]):
            errors.append(f"metadata.{field} must be a non-identifying safe slug")
    if metadata.get("sourceKind") != "observed":
        errors.append("sourceKind must be observed")
    if metadata.get("workloadClass") not in WORKLOAD_CLASSES:
        errors.append("workloadClass is invalid")
    if metadata.get("storageClass") not in STORAGE_CLASSES:
        errors.append("storageClass is invalid")
    if metadata.get("sanitized") is not True:
        errors.append("sanitized must be true")

    interval = metadata.get("sampleIntervalSeconds")
    if type(interval) is not int or not 1 <= interval <= 60:
        errors.append("sampleIntervalSeconds must be an integer from 1 to 60")
        interval = 1
    duration = metadata.get("windowDurationSeconds")
    if type(duration) is not int or not 2 <= duration <= MAXIMUM_DURATION_SECONDS:
        errors.append("windowDurationSeconds must cover 2 seconds to 24 hours")
        duration = 0
    elif duration % interval:
        errors.append("windowDurationSeconds must be divisible by sampleIntervalSeconds")

    if set(semantics) != {
        "timestamp", "ioLayer", "latency", "managementPlane", "provenance",
    }:
        errors.append("metricSemantics fields do not match the strict contract")
    if semantics.get("timestamp") not in TIMESTAMP_SEMANTICS:
        errors.append("metricSemantics.timestamp is invalid")
    if semantics.get("ioLayer") not in IO_LAYERS:
        errors.append("metricSemantics.ioLayer is invalid")
    if (semantics.get("timestamp"), semantics.get("ioLayer")) not in SEMANTIC_PAIRS:
        errors.append("timestamp and I/O layer semantics are incompatible")
    if semantics.get("latency") != "unavailable":
        errors.append("metricSemantics.latency must remain unavailable")
    if semantics.get("managementPlane") != "unavailable":
        errors.append("metricSemantics.managementPlane must remain unavailable")
    if semantics.get("provenance") != "derived":
        errors.append("metricSemantics.provenance must be derived")

    required_sample_fields = {
        "offsetSeconds", "readIops", "writeIops",
        "readThroughputMiBps", "writeThroughputMiBps",
    }
    offsets: List[int] = []
    read_active_bucket_seconds = 0
    write_active_bucket_seconds = 0
    for index, sample in enumerate(samples):
        if not isinstance(sample, dict):
            errors.append(f"sample {index} must be an object")
            continue
        if set(sample) != required_sample_fields:
            errors.append(f"sample {index} fields do not match the strict contract")
        offset = sample.get("offsetSeconds")
        if type(offset) is not int or offset < 0:
            errors.append(f"sample {index} offsetSeconds is invalid")
        else:
            offsets.append(offset)
        for field in required_sample_fields - {"offsetSeconds"}:
            value = sample.get(field)
            if type(value) not in (int, float) or not math.isfinite(value) or value < 0:
                errors.append(f"sample {index} {field} is invalid")
        if type(sample.get("readIops")) in (int, float) and sample["readIops"] > 0:
            read_active_bucket_seconds += interval
        if type(sample.get("writeIops")) in (int, float) and sample["writeIops"] > 0:
            write_active_bucket_seconds += interval

    if len(samples) < 2:
        errors.append("at least two samples are required")
    if offsets != sorted(set(offsets)):
        errors.append("sample offsets must be unique and strictly increasing")
    if offsets and any(offset % interval for offset in offsets):
        errors.append("sample offsets must align to the declared window start")
    if offsets and any(offset >= duration for offset in offsets):
        errors.append("sample offset is outside windowDurationSeconds")

    expected_count = duration // interval if duration and duration % interval == 0 else 0
    eligible_offsets = {offset for offset in offsets if 0 <= offset < duration and not offset % interval}
    completeness = len(eligible_offsets) / expected_count if expected_count else 0.0
    meets_research_gate = (
        not errors
        and metadata.get("sourceKind") == "observed"
        and bool(reference_group)
        and metadata.get("independenceGroup") != reference_group
        and metadata.get("workloadClass") != "unknown"
        and duration >= MINIMUM_RESEARCH_DURATION_SECONDS
        and completeness >= MINIMUM_COMPLETENESS
    )
    storage_class_known = metadata.get("storageClass") in STORAGE_CLASSES - {"unknown"}
    return WorkloadShapeAssessment(
        errors=errors,
        sample_count=len(samples),
        duration_seconds=duration,
        completeness=round(completeness, 6),
        read_active_bucket_seconds=read_active_bucket_seconds,
        write_active_bucket_seconds=write_active_bucket_seconds,
        storage_class_known=storage_class_known,
        meets_research_gate=meets_research_gate,
        meets_storage_class_research_gate=meets_research_gate and storage_class_known,
        # This artifact has neither latency nor management-plane evidence by design.
        active_control_eligible=False,
    )


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("trace", type=Path)
    parser.add_argument("--reference-group", required=True)
    args = parser.parse_args()
    with args.trace.open(encoding="utf-8") as handle:
        document = json.load(handle)
    assessment = assess_workload_shape(document, args.reference_group)
    print(json.dumps(asdict(assessment), indent=2, sort_keys=True))
    raise SystemExit(1 if assessment.errors else 0)


if __name__ == "__main__":
    main()
