#!/usr/bin/env python3
"""Convert an authorized SPC trace into a sanitized workload-shape artifact.

The converter performs no download. It drops ASU, LBA, and all optional fields,
retains only per-window operation counts and transferred bytes, and fixes the
storage class to unknown because SPC records do not carry that semantic claim.
"""

from __future__ import annotations

import argparse
import bz2
import json
import math
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


@dataclass(frozen=True)
class ConversionOptions:
    name: str
    independence_group: str
    workload_class: str
    sample_interval_seconds: int
    window_duration_seconds: int = 0
    authorized_and_sanitized: bool = False


def _validate_options(options: ConversionOptions) -> None:
    if options.authorized_and_sanitized is not True:
        raise ValueError("authorized-and-sanitized confirmation is required")
    for label, value in (("name", options.name), ("independence group", options.independence_group)):
        if not SAFE_SLUG.fullmatch(value):
            raise ValueError(f"{label} must be a non-identifying safe slug")
    if options.workload_class not in WORKLOAD_CLASSES:
        raise ValueError("workload class is invalid")
    if type(options.sample_interval_seconds) is not int or not 1 <= options.sample_interval_seconds <= 60:
        raise ValueError("sample interval must be between 1 and 60 seconds")
    duration = options.window_duration_seconds
    if type(duration) is not int or duration < 0 or duration > MAXIMUM_DURATION_SECONDS:
        raise ValueError("window duration must be between 0 and 86400 seconds")
    if duration and duration % options.sample_interval_seconds:
        raise ValueError("window duration must be divisible by sample interval")


def _nonnegative_integer(value: str, label: str, line: int) -> int:
    value = value.strip()
    if len(value) > 20 or not value.isascii() or not value.isdecimal():
        raise ValueError(f"line {line} {label} must be a non-negative decimal integer")
    return int(value)


def _timestamp(value: str, line: int) -> float:
    try:
        parsed = float(value.strip())
    except ValueError as error:
        raise ValueError(f"line {line} timestamp is not numeric") from error
    if not math.isfinite(parsed) or parsed < 0:
        raise ValueError(f"line {line} timestamp must be finite and non-negative")
    return parsed


def build_workload_shape(source: TextIO, options: ConversionOptions) -> Dict[str, object]:
    """Aggregate an SPC stream while discarding all source identity coordinates."""
    _validate_options(options)
    interval = options.sample_interval_seconds
    buckets: Dict[int, Tuple[int, int, int, int]] = {}
    origin = None
    previous_timestamp = None
    last_bucket_index = 0

    for line_number, raw_line in enumerate(source, start=1):
        if len(raw_line) > MAXIMUM_LINE_CHARACTERS:
            raise ValueError(f"line {line_number} exceeds the bounded record size")
        if not raw_line.strip():
            raise ValueError(f"line {line_number} is empty")
        fields = raw_line.rstrip("\r\n").split(",", 5)
        if len(fields) < 5:
            raise ValueError(f"line {line_number} has fewer than five SPC fields")
        _nonnegative_integer(fields[0], "ASU", line_number)
        _nonnegative_integer(fields[1], "LBA", line_number)
        size_bytes = _nonnegative_integer(fields[2], "size", line_number)
        operation = fields[3].strip().lower()
        if operation not in {"r", "w"}:
            raise ValueError(f"line {line_number} opcode must be R or W")
        timestamp = _timestamp(fields[4], line_number)
        if previous_timestamp is not None and timestamp < previous_timestamp:
            raise ValueError(f"line {line_number} timestamp is earlier than the preceding record")
        if origin is None:
            origin = timestamp
        previous_timestamp = timestamp
        offset = timestamp - origin
        if options.window_duration_seconds and offset >= options.window_duration_seconds:
            break
        bucket_index = int(offset // interval)
        last_bucket_index = bucket_index
        read_count, write_count, read_bytes, write_bytes = buckets.get(bucket_index, (0, 0, 0, 0))
        if operation == "r":
            read_count += 1
            read_bytes += size_bytes
        else:
            write_count += 1
            write_bytes += size_bytes
        buckets[bucket_index] = (read_count, write_count, read_bytes, write_bytes)

    if origin is None:
        raise ValueError("SPC trace must contain at least one record")
    required_duration = (last_bucket_index + 1) * interval
    duration = options.window_duration_seconds or required_duration
    if duration < required_duration:
        raise ValueError("window duration does not contain every selected record")
    if duration < 2 * interval:
        raise ValueError("window duration must contain at least two samples")

    samples = []
    for index in range(duration // interval):
        read_count, write_count, read_bytes, write_bytes = buckets.get(index, (0, 0, 0, 0))
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
            "storageClass": "unknown",
            "workloadClass": options.workload_class,
            "sampleIntervalSeconds": interval,
            "windowDurationSeconds": duration,
            "sanitized": True,
        },
        "metricSemantics": {
            "timestamp": "issue-offset-seconds",
            "ioLayer": "host-to-logical-unit",
            "latency": "unavailable",
            "managementPlane": "unavailable",
            "provenance": "derived",
        },
        "samples": samples,
    }


def _open_source(path: Path) -> TextIO:
    if path.suffix == ".bz2":
        return bz2.open(path, mode="rt", encoding="ascii", errors="strict")
    return path.open(encoding="ascii", errors="strict")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("spc", type=Path)
    parser.add_argument("--name", required=True)
    parser.add_argument("--independence-group", required=True)
    parser.add_argument("--workload-class", required=True, choices=sorted(WORKLOAD_CLASSES))
    parser.add_argument("--sample-interval-seconds", type=int, default=10)
    parser.add_argument("--window-duration-seconds", type=int, default=0)
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
        with _open_source(args.spc) as source:
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
