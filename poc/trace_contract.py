#!/usr/bin/env python3
"""Validate and qualify sanitized replay traces without external packages."""

import argparse
import json
import math
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Dict, List, Optional


LEGACY_API_VERSION = "guard.storage-slo.io/replay-trace/v1alpha1"
API_VERSION = "guard.storage-slo.io/replay-trace/v1alpha2"
SUPPORTED_API_VERSIONS = {LEGACY_API_VERSION, API_VERSION}
MINIMUM_DURATION_SECONDS = 600
MINIMUM_COMPLETENESS = 0.95
STORAGE_CLASSES = {"rotational-hdd", "sata-ssd", "nvme", "network-block", "unknown"}
WORKLOAD_CLASSES = {"bulk-import", "backup", "migration", "build", "database", "mixed", "unknown"}
MEASUREMENT_LAYERS = {"storage-domain", "block-device", "virtual-disk", "application", "unknown"}


@dataclass(frozen=True)
class TraceAssessment:
    errors: List[str]
    sample_count: int
    duration_seconds: int
    completeness: float
    wait_completeness: float
    management_plane_completeness: float
    policy_signal_compatible: bool
    meets_machine_independence_gate: bool


def assess_trace(document: Dict[str, object], reference_group: Optional[str] = None) -> TraceAssessment:
    """Validate shape and assess evidence qualification.

    Structural validity, policy-signal compatibility, and independence are
    separate claims. A valid synthetic or total-wait trace remains useful for
    tests, but it cannot become independent production evidence by naming it so.
    """
    if not isinstance(document, dict):
        return TraceAssessment(["trace must be an object"], 0, 0, 0.0, 0.0, 0.0, False, False)

    errors: List[str] = []
    allowed_top_level = {"apiVersion", "kind", "metadata", "metricSemantics", "samples"}
    if set(document) != allowed_top_level:
        errors.append("top-level fields do not match the strict contract")
    api_version = document.get("apiVersion")
    if api_version not in SUPPORTED_API_VERSIONS or document.get("kind") != "ReplayTrace":
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
        "name", "sourceKind", "independenceGroup", "storageClass",
        "workloadClass", "sampleIntervalSeconds", "windowDurationSeconds", "sanitized",
    }
    if metadata and set(metadata) != allowed_metadata:
        errors.append("metadata fields do not match the strict contract")
    allowed_semantics = {"writeWaitStatistic", "controlWindowAggregation", "provenance"}
    if api_version == API_VERSION:
        allowed_semantics.add("writeWaitMeasurementLayer")
    if semantics and set(semantics) != allowed_semantics:
        errors.append("metricSemantics fields do not match the strict contract")

    interval = metadata.get("sampleIntervalSeconds")
    if type(interval) is not int or interval < 1 or interval > 60:
        errors.append("sampleIntervalSeconds must be an integer from 1 to 60")
        interval = 1
    window_duration = metadata.get("windowDurationSeconds")
    if type(window_duration) is not int or window_duration < interval or window_duration > 2_678_400:
        errors.append("windowDurationSeconds must cover 1 interval to 31 days")
        window_duration = 0
    elif window_duration % interval:
        errors.append("windowDurationSeconds must be divisible by sampleIntervalSeconds")
    if metadata.get("sourceKind") not in {"observed", "synthetic", "modeled"}:
        errors.append("sourceKind is invalid")
    if metadata.get("sanitized") is not True:
        errors.append("sanitized must be true")
    for field in ("name", "independenceGroup", "storageClass", "workloadClass"):
        if not isinstance(metadata.get(field), str) or not metadata[field]:
            errors.append(f"metadata.{field} is required")
        elif len(metadata[field]) > 128:
            errors.append(f"metadata.{field} exceeds 128 characters")
    if metadata.get("storageClass") not in STORAGE_CLASSES:
        errors.append("storageClass is invalid")
    if metadata.get("workloadClass") not in WORKLOAD_CLASSES:
        errors.append("workloadClass is invalid")

    wait_statistic = semantics.get("writeWaitStatistic")
    aggregation = semantics.get("controlWindowAggregation")
    if wait_statistic not in {"p95", "average", "total-wait"}:
        errors.append("writeWaitStatistic is invalid")
    measurement_layer = semantics.get("writeWaitMeasurementLayer")
    if api_version == API_VERSION and measurement_layer not in MEASUREMENT_LAYERS:
        errors.append("writeWaitMeasurementLayer is invalid")
    if aggregation not in {"none", "p95"}:
        errors.append("controlWindowAggregation is invalid")
    if semantics.get("provenance") not in {"observed", "derived", "modeled"}:
        errors.append("metric provenance is invalid")

    offsets: List[int] = []
    management_field = (
        "managementPlaneHealthy"
        if api_version == LEGACY_API_VERSION
        else "managementPlaneStatus"
    )
    required_sample_fields = {"offsetSeconds", "writeWaitMilliseconds", "waitValid", management_field}
    optional_numeric_fields = {
        "writeIops", "writeThroughputMiBps", "queueDepth", "psiSomePercent", "psiFullPercent",
    }
    allowed_sample_fields = required_sample_fields | optional_numeric_fields | {"taskProgressBytes"}
    for index, sample in enumerate(samples):
        if not isinstance(sample, dict):
            errors.append(f"sample {index} must be an object")
            continue
        if not required_sample_fields.issubset(sample) or not set(sample).issubset(allowed_sample_fields):
            errors.append(f"sample {index} fields do not match the strict contract")
        offset = sample.get("offsetSeconds")
        if type(offset) is not int or offset < 0:
            errors.append(f"sample {index} offsetSeconds is invalid")
        else:
            offsets.append(offset)
        wait = sample.get("writeWaitMilliseconds")
        if type(wait) not in (int, float) or not math.isfinite(wait) or wait < 0:
            errors.append(f"sample {index} writeWaitMilliseconds is invalid")
        if type(sample.get("waitValid")) is not bool:
            errors.append(f"sample {index} waitValid must be boolean")
        if api_version == LEGACY_API_VERSION:
            if type(sample.get("managementPlaneHealthy")) is not bool:
                errors.append(f"sample {index} managementPlaneHealthy must be boolean")
        elif sample.get("managementPlaneStatus") not in {"healthy", "unhealthy", "unknown"}:
            errors.append(f"sample {index} managementPlaneStatus is invalid")
        for field in optional_numeric_fields:
            if field not in sample:
                continue
            value = sample[field]
            if type(value) not in (int, float) or not math.isfinite(value) or value < 0:
                errors.append(f"sample {index} {field} is invalid")
            if field in {"psiSomePercent", "psiFullPercent"} and type(value) in (int, float) and value > 100:
                errors.append(f"sample {index} {field} exceeds 100 percent")
        if "taskProgressBytes" in sample and (type(sample["taskProgressBytes"]) is not int or sample["taskProgressBytes"] < 0):
            errors.append(f"sample {index} taskProgressBytes is invalid")

    if len(offsets) != len(set(offsets)) or offsets != sorted(offsets):
        errors.append("sample offsets must be unique and strictly increasing")
    if offsets and any(offset % interval for offset in offsets):
        errors.append("sample offsets must align to the declared window start")
    if offsets and any(offset >= window_duration for offset in offsets):
        errors.append("sample offset is outside windowDurationSeconds")
    if len(samples) < 2:
        errors.append("at least two samples are required")

    duration = window_duration
    expected = duration // interval if duration and duration % interval == 0 else 0
    completeness = len(offsets) / expected if expected else 0.0
    if completeness > 1:
        errors.append("sample offsets are not aligned to sampleIntervalSeconds")
        completeness = 0.0

    valid_waits = sum(
        1 for sample in samples
        if isinstance(sample, dict) and sample.get("waitValid") is True
    )
    known_management = sum(
        1 for sample in samples
        if isinstance(sample, dict)
        and (
            (api_version == LEGACY_API_VERSION and type(sample.get("managementPlaneHealthy")) is bool)
            or (
                api_version == API_VERSION
                and sample.get("managementPlaneStatus") in {"healthy", "unhealthy"}
            )
        )
    )
    wait_completeness = valid_waits / expected if expected else 0.0
    management_completeness = known_management / expected if expected else 0.0
    if completeness > 1:
        wait_completeness = 0.0
        management_completeness = 0.0

    compatible = (
        api_version == API_VERSION
        and wait_statistic == "p95"
        and measurement_layer == "storage-domain"
        and aggregation == "none"
        and semantics.get("provenance") in {"observed", "derived"}
    )
    independent = (
        not errors
        and api_version == API_VERSION
        and metadata.get("sourceKind") == "observed"
        and bool(reference_group)
        and metadata.get("independenceGroup") != reference_group
        and duration >= MINIMUM_DURATION_SECONDS
        and completeness >= MINIMUM_COMPLETENESS
        and wait_completeness >= MINIMUM_COMPLETENESS
        and management_completeness >= MINIMUM_COMPLETENESS
        and compatible
        and metadata.get("storageClass") != "unknown"
        and metadata.get("workloadClass") != "unknown"
    )
    return TraceAssessment(
        errors=errors,
        sample_count=len(samples),
        duration_seconds=duration,
        completeness=round(completeness, 6),
        wait_completeness=round(wait_completeness, 6),
        management_plane_completeness=round(management_completeness, 6),
        policy_signal_compatible=compatible,
        meets_machine_independence_gate=independent,
    )


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("trace", type=Path)
    parser.add_argument("--reference-group", required=True)
    args = parser.parse_args()
    with args.trace.open(encoding="utf-8") as handle:
        document = json.load(handle)
    assessment = assess_trace(document, args.reference_group)
    print(json.dumps(asdict(assessment), indent=2, sort_keys=True))
    raise SystemExit(1 if assessment.errors else 0)


if __name__ == "__main__":
    main()
