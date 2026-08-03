#!/usr/bin/env python3
"""Convert an authorized Alibaba Block Trace CSV into a sanitized shape trace.

The converter performs no download or decompression. It accepts the documented
headerless ``device_id,opcode,offset,length,timestamp`` records, drops device
and address coordinates, and fixes the semantic boundary to observed Alibaba
Ultra Disk requests arriving at a virtual block service. It cannot emit latency
or management-plane claims.
"""

from __future__ import annotations

import argparse
import json
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, TextIO, Tuple

from workload_shape_contract import (
    API_VERSION,
    KIND,
    MAXIMUM_DURATION_SECONDS,
    SAFE_SLUG,
    WORKLOAD_CLASSES,
)


MAXIMUM_LINE_CHARACTERS = 65_536
MICROSECONDS_PER_SECOND = 1_000_000


@dataclass(frozen=True)
class ConversionOptions:
    name: str
    independence_group: str
    workload_class: str
    sample_interval_seconds: int
    window_duration_seconds: int = 600
    authorized_and_sanitized: bool = False


def _validate_options(options: ConversionOptions) -> None:
    if options.authorized_and_sanitized is not True:
        raise ValueError("authorized-and-sanitized confirmation is required")
    for label, value in (
        ("name", options.name),
        ("independence group", options.independence_group),
    ):
        if not SAFE_SLUG.fullmatch(value):
            raise ValueError(f"{label} must be a non-identifying safe slug")
    if options.workload_class not in WORKLOAD_CLASSES:
        raise ValueError("workload class is invalid")
    interval = options.sample_interval_seconds
    if type(interval) is not int or not 1 <= interval <= 60:
        raise ValueError("sample interval must be between 1 and 60 seconds")
    duration = options.window_duration_seconds
    if (
        type(duration) is not int
        or duration < 2 * interval
        or duration > MAXIMUM_DURATION_SECONDS
    ):
        raise ValueError("window duration must contain at least two samples and at most 86400 seconds")
    if duration % interval:
        raise ValueError("window duration must be divisible by sample interval")


def _nonnegative_integer(value: str, label: str, line: int) -> int:
    value = value.strip()
    if len(value) > 20 or not value.isascii() or not value.isdecimal():
        raise ValueError(f"line {line} {label} must be a non-negative decimal integer")
    return int(value)


def build_workload_shape(source: TextIO, options: ConversionOptions) -> Dict[str, object]:
    """Aggregate a bounded arrival-time window and discard source coordinates."""
    _validate_options(options)
    interval = options.sample_interval_seconds
    interval_microseconds = interval * MICROSECONDS_PER_SECOND
    buckets: Dict[int, Tuple[int, int, int, int]] = {}
    origin_microseconds = None
    previous_timestamp_microseconds = None
    last_bucket_index = 0

    for line_number, raw_line in enumerate(source, start=1):
        if len(raw_line) > MAXIMUM_LINE_CHARACTERS:
            raise ValueError(f"line {line_number} exceeds the bounded record size")
        if not raw_line.strip():
            raise ValueError(f"line {line_number} is empty")
        fields = raw_line.rstrip("\r\n").split(",")
        if len(fields) != 5:
            raise ValueError(f"line {line_number} must have exactly five fields")
        _nonnegative_integer(fields[0], "device ID", line_number)
        operation = fields[1].strip().lower()
        if operation not in {"r", "w"}:
            raise ValueError(f"line {line_number} opcode must be R or W")
        _nonnegative_integer(fields[2], "offset", line_number)
        size_bytes = _nonnegative_integer(fields[3], "length", line_number)
        timestamp_microseconds = _nonnegative_integer(fields[4], "timestamp", line_number)
        if (
            previous_timestamp_microseconds is not None
            and timestamp_microseconds < previous_timestamp_microseconds
        ):
            raise ValueError(f"line {line_number} timestamp is earlier than the preceding record")
        if origin_microseconds is None:
            origin_microseconds = timestamp_microseconds
        previous_timestamp_microseconds = timestamp_microseconds
        offset_microseconds = timestamp_microseconds - origin_microseconds
        if offset_microseconds >= options.window_duration_seconds * MICROSECONDS_PER_SECOND:
            break
        bucket_index = offset_microseconds // interval_microseconds
        last_bucket_index = bucket_index
        read_count, write_count, read_bytes, write_bytes = buckets.get(
            bucket_index, (0, 0, 0, 0)
        )
        if operation == "r":
            read_count += 1
            read_bytes += size_bytes
        else:
            write_count += 1
            write_bytes += size_bytes
        buckets[bucket_index] = (read_count, write_count, read_bytes, write_bytes)

    if origin_microseconds is None:
        raise ValueError("Alibaba Block Trace must contain at least one record")
    required_duration = (last_bucket_index + 1) * interval
    duration = options.window_duration_seconds
    if duration < required_duration:
        raise ValueError("window duration does not contain every selected record")
    if duration < 2 * interval:
        raise ValueError("window duration must contain at least two samples")

    samples = []
    for index in range(duration // interval):
        read_count, write_count, read_bytes, write_bytes = buckets.get(
            index, (0, 0, 0, 0)
        )
        samples.append({
            "offsetSeconds": index * interval,
            "readIops": round(read_count / interval, 6),
            "writeIops": round(write_count / interval, 6),
            "readThroughputMiBps": round(read_bytes / interval / 1_048_576, 6),
            "writeThroughputMiBps": round(write_bytes / interval / 1_048_576, 6),
        })

    return {
        "apiVersion": API_VERSION,
        "kind": KIND,
        "metadata": {
            "name": options.name,
            "sourceKind": "observed",
            "independenceGroup": options.independence_group,
            "storageClass": "network-block",
            "workloadClass": options.workload_class,
            "sampleIntervalSeconds": interval,
            "windowDurationSeconds": duration,
            "sanitized": True,
        },
        "metricSemantics": {
            "timestamp": "arrival-offset-seconds",
            "ioLayer": "virtual-block-service",
            "latency": "unavailable",
            "managementPlane": "unavailable",
            "provenance": "derived",
        },
        "samples": samples,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("csv", type=Path)
    parser.add_argument("--name", required=True)
    parser.add_argument("--independence-group", required=True)
    parser.add_argument("--workload-class", required=True, choices=sorted(WORKLOAD_CLASSES))
    parser.add_argument("--sample-interval-seconds", type=int, default=10)
    parser.add_argument("--window-duration-seconds", type=int, default=600)
    parser.add_argument("--confirm-authorized-and-sanitized", action="store_true")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    options = ConversionOptions(
        name=args.name,
        independence_group=args.independence_group,
        workload_class=args.workload_class,
        sample_interval_seconds=args.sample_interval_seconds,
        window_duration_seconds=args.window_duration_seconds,
        authorized_and_sanitized=args.confirm_authorized_and_sanitized,
    )
    try:
        with args.csv.open(encoding="ascii", errors="strict") as source:
            document = build_workload_shape(source, options)
    except (OSError, UnicodeError, ValueError) as error:
        parser.error(str(error))
    rendered = json.dumps(document, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.write_text(rendered, encoding="utf-8")
    else:
        print(rendered, end="")


if __name__ == "__main__":
    main()
