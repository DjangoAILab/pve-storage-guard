#!/usr/bin/env python3
"""Convert an authorized per-I/O CSV into a sanitized v1alpha2 replay trace.

The converter has no downloader and never copies source-only columns. Management
plane evidence is deliberately marked unknown because block-I/O rows cannot
establish host-management availability.
"""

import argparse
import csv
import json
import math
import re
import sys
from dataclasses import dataclass
from typing import Dict, List, TextIO

from trace_contract import API_VERSION, MEASUREMENT_LAYERS, STORAGE_CLASSES, WORKLOAD_CLASSES


REQUIRED_COLUMNS = {
    "timestamp_seconds", "operation", "size_bytes", "response_time_milliseconds",
}
SAFE_SLUG = re.compile(r"^[a-z0-9][a-z0-9.-]{0,127}$")


@dataclass(frozen=True)
class ConversionOptions:
    name: str
    source_kind: str
    independence_group: str
    storage_class: str
    workload_class: str
    write_wait_measurement_layer: str
    sample_interval_seconds: int
    window_duration_seconds: int = 0
    authorized_and_sanitized: bool = False


def _validate_options(options: ConversionOptions) -> None:
    if options.authorized_and_sanitized is not True:
        raise ValueError("authorized-and-sanitized confirmation is required")
    for label, value in (("name", options.name), ("independence group", options.independence_group)):
        if not SAFE_SLUG.fullmatch(value):
            raise ValueError(f"{label} must be a non-identifying safe slug")
    if options.source_kind not in {"observed", "synthetic", "modeled"}:
        raise ValueError("source kind is invalid")
    if options.storage_class not in STORAGE_CLASSES:
        raise ValueError("storage class is invalid")
    if options.workload_class not in WORKLOAD_CLASSES:
        raise ValueError("workload class is invalid")
    if options.write_wait_measurement_layer not in MEASUREMENT_LAYERS:
        raise ValueError("write-wait measurement layer is invalid")
    if not 1 <= options.sample_interval_seconds <= 60:
        raise ValueError("sample interval must be between 1 and 60 seconds")
    if options.window_duration_seconds < 0:
        raise ValueError("window duration must be non-negative")
    if options.window_duration_seconds % options.sample_interval_seconds:
        raise ValueError("window duration must be divisible by sample interval")


def _finite_number(value: str, label: str, line: int) -> float:
    try:
        parsed = float(value)
    except (TypeError, ValueError) as error:
        raise ValueError(f"line {line} {label} is not numeric") from error
    if not math.isfinite(parsed) or parsed < 0:
        raise ValueError(f"line {line} {label} must be finite and non-negative")
    return parsed


def _nearest_rank_p95(values: List[float]) -> float:
    ordered = sorted(values)
    return ordered[max(0, math.ceil(0.95 * len(ordered)) - 1)]


def build_trace(source: TextIO, options: ConversionOptions) -> Dict[str, object]:
    """Aggregate per-I/O rows while dropping every unspecified input column."""
    _validate_options(options)
    reader = csv.DictReader(source)
    if reader.fieldnames is None or not REQUIRED_COLUMNS.issubset(reader.fieldnames):
        missing = sorted(REQUIRED_COLUMNS - set(reader.fieldnames or []))
        raise ValueError(f"CSV is missing required columns: {', '.join(missing)}")

    origin = None
    previous_timestamp = None
    current_bucket_index = 0
    current_latencies: List[float] = []
    current_bytes = 0
    last_bucket_index = 0
    write_summaries = {}
    for line, row in enumerate(reader, start=2):
        operation = (row.get("operation") or "").strip().lower()
        if operation not in {"read", "r", "write", "w"}:
            raise ValueError(f"line {line} operation must be read/r/write/w")
        timestamp = _finite_number(row.get("timestamp_seconds", ""), "timestamp", line)
        response = _finite_number(
            row.get("response_time_milliseconds", ""), "response time", line,
        )
        size_value = _finite_number(row.get("size_bytes", ""), "size", line)
        if not size_value.is_integer():
            raise ValueError(f"line {line} size must be an integer number of bytes")
        if previous_timestamp is not None and timestamp < previous_timestamp:
            raise ValueError(f"line {line} timestamp is earlier than the preceding row")
        if origin is None:
            origin = timestamp
        previous_timestamp = timestamp
        bucket_index = int((timestamp - origin) // options.sample_interval_seconds)
        if bucket_index != current_bucket_index:
            if current_latencies:
                write_summaries[current_bucket_index] = (
                    _nearest_rank_p95(current_latencies), len(current_latencies), current_bytes,
                )
            current_bucket_index = bucket_index
            current_latencies = []
            current_bytes = 0
        last_bucket_index = bucket_index
        if operation in {"write", "w"}:
            current_latencies.append(response)
            current_bytes += int(size_value)

    if origin is None:
        raise ValueError("CSV must contain at least one I/O row")
    if current_latencies:
        write_summaries[current_bucket_index] = (
            _nearest_rank_p95(current_latencies), len(current_latencies), current_bytes,
        )
    interval = options.sample_interval_seconds
    bucket_count = last_bucket_index + 1
    required_duration = bucket_count * interval
    duration = options.window_duration_seconds or required_duration
    if duration < required_duration:
        raise ValueError("window duration does not contain every source row")
    if duration > 2_678_400:
        raise ValueError("window duration exceeds 31 days")
    if duration < 2 * interval:
        raise ValueError("window duration must contain at least two samples")

    samples = []
    for index in range(duration // interval):
        summary = write_summaries.get(index)
        write_p95, write_count, write_bytes = summary or (0, 0, 0)
        samples.append({
            "offsetSeconds": index * interval,
            "writeWaitMilliseconds": round(write_p95, 6),
            "waitValid": write_count > 0,
            "managementPlaneStatus": "unknown",
            "writeIops": round(write_count / interval, 6),
            "writeThroughputMiBps": round(write_bytes / interval / 1_048_576, 6),
        })

    return {
        "apiVersion": API_VERSION,
        "kind": "ReplayTrace",
        "metadata": {
            "name": options.name,
            "sourceKind": options.source_kind,
            "independenceGroup": options.independence_group,
            "storageClass": options.storage_class,
            "workloadClass": options.workload_class,
            "sampleIntervalSeconds": interval,
            "windowDurationSeconds": duration,
            "sanitized": True,
        },
        "metricSemantics": {
            "writeWaitStatistic": "p95",
            "writeWaitMeasurementLayer": options.write_wait_measurement_layer,
            "controlWindowAggregation": "none",
            "provenance": "derived",
        },
        "samples": samples,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("csv", type=argparse.FileType("r", encoding="utf-8"))
    parser.add_argument("--name", required=True)
    parser.add_argument(
        "--source-kind", required=True, choices=("observed", "synthetic", "modeled"),
    )
    parser.add_argument("--independence-group", required=True)
    parser.add_argument("--storage-class", required=True, choices=sorted(STORAGE_CLASSES))
    parser.add_argument("--workload-class", required=True, choices=sorted(WORKLOAD_CLASSES))
    parser.add_argument(
        "--write-wait-measurement-layer", required=True, choices=sorted(MEASUREMENT_LAYERS),
    )
    parser.add_argument("--sample-interval-seconds", type=int, default=1)
    parser.add_argument("--window-duration-seconds", type=int, default=0)
    parser.add_argument("--confirm-authorized-and-sanitized", action="store_true")
    args = parser.parse_args()
    if not args.confirm_authorized_and_sanitized:
        parser.error("--confirm-authorized-and-sanitized is required")
    options = ConversionOptions(
        name=args.name,
        source_kind=args.source_kind,
        independence_group=args.independence_group,
        storage_class=args.storage_class,
        workload_class=args.workload_class,
        write_wait_measurement_layer=args.write_wait_measurement_layer,
        sample_interval_seconds=args.sample_interval_seconds,
        window_duration_seconds=args.window_duration_seconds,
        authorized_and_sanitized=True,
    )
    try:
        document = build_trace(args.csv, options)
    except ValueError as error:
        parser.error(str(error))
    json.dump(document, sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
