"""Validate and assess sanitized, non-replayable incident evidence."""

from datetime import datetime, timedelta, timezone
from typing import Dict, List, Mapping, Optional, Sequence


EVENT_KINDS = (
    "durability_degradation_first",
    "management_service_restart",
    "write_wait_sampling_started",
)
MISSING_SIGNALS = {"io_psi", "queue_depth", "management_probe_series"}


def _exact_keys(value: object, expected: set, path: str, errors: List[str]) -> bool:
    if not isinstance(value, dict):
        errors.append(f"{path} must be an object")
        return False
    actual = set(value)
    missing = sorted(expected - actual)
    extra = sorted(actual - expected)
    if missing:
        errors.append(f"{path} missing keys: {', '.join(missing)}")
    if extra:
        errors.append(f"{path} unknown keys: {', '.join(extra)}")
    return not missing and not extra


def _utc_timestamp(value: object, path: str, errors: List[str]) -> Optional[datetime]:
    if not isinstance(value, str):
        errors.append(f"{path} must be an RFC3339 timestamp")
        return None
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        errors.append(f"{path} must be an RFC3339 timestamp")
        return None
    if parsed.tzinfo is None:
        errors.append(f"{path} must include a timezone")
        return None
    return parsed.astimezone(timezone.utc)


def _positive_number(value: object) -> bool:
    return type(value) in (int, float) and value > 0


def _non_negative_number(value: object) -> bool:
    return type(value) in (int, float) and value >= 0


def validate_incident_evidence(document: object) -> List[str]:
    """Return every validation error; an empty list means the document is safe."""

    errors: List[str] = []
    if not _exact_keys(document, {"schemaVersion", "source", "timeline", "fieldValidations"}, "document", errors):
        return errors
    assert isinstance(document, dict)

    if document["schemaVersion"] != 1:
        errors.append("schemaVersion must equal 1")

    source = document["source"]
    source_keys = {"kind", "recoveredFrom", "independenceGroup"}
    if _exact_keys(source, source_keys, "source", errors):
        assert isinstance(source, dict)
        if source["kind"] != "observed-summary":
            errors.append("source.kind must equal observed-summary")
        for key in ("recoveredFrom", "independenceGroup"):
            if not isinstance(source[key], str) or not source[key]:
                errors.append(f"source.{key} must be a non-empty string")

    timeline = document["timeline"]
    if _exact_keys(timeline, {"events", "missingSignals"}, "timeline", errors):
        assert isinstance(timeline, dict)
        events = timeline["events"]
        timestamps: Dict[str, datetime] = {}
        if not isinstance(events, list) or len(events) != len(EVENT_KINDS):
            errors.append("timeline.events must contain exactly three events")
        else:
            for index, event in enumerate(events):
                path = f"timeline.events[{index}]"
                if not _exact_keys(event, {"kind", "observedAt"}, path, errors):
                    continue
                assert isinstance(event, dict)
                kind = event["kind"]
                if kind not in EVENT_KINDS:
                    errors.append(f"{path}.kind is not supported")
                    continue
                if kind in timestamps:
                    errors.append(f"{path}.kind must be unique")
                    continue
                timestamp = _utc_timestamp(event["observedAt"], f"{path}.observedAt", errors)
                if timestamp is not None:
                    timestamps[kind] = timestamp
            if set(timestamps) == set(EVENT_KINDS):
                ordered = [timestamps[kind] for kind in EVENT_KINDS]
                if ordered != sorted(ordered):
                    errors.append("timeline event order must be durability, management failure, then sampling")

        missing = timeline["missingSignals"]
        if not isinstance(missing, list) or not missing:
            errors.append("timeline.missingSignals must be a non-empty array")
        elif any(not isinstance(value, str) or value not in MISSING_SIGNALS for value in missing):
            errors.append("timeline.missingSignals contains an unsupported signal")
        elif len(missing) != len(set(missing)):
            errors.append("timeline.missingSignals must be unique")

    validations = document["fieldValidations"]
    if not isinstance(validations, list) or len(validations) != 1:
        errors.append("fieldValidations must contain exactly one validation")
    else:
        validation = validations[0]
        keys = {
            "kind",
            "independenceGroup",
            "capMiBps",
            "controlledLoadStarted",
            "sampleCount",
            "unsafeThresholdMilliseconds",
            "unsafeSampleCount",
            "p99WriteWaitMilliseconds",
            "replayable",
            "outcome",
        }
        if _exact_keys(validation, keys, "fieldValidations[0]", errors):
            assert isinstance(validation, dict)
            if validation["kind"] != "fixed_cap_natural_load":
                errors.append("fieldValidations[0].kind must equal fixed_cap_natural_load")
            source_group = source.get("independenceGroup") if isinstance(source, dict) else None
            if validation["independenceGroup"] != source_group:
                errors.append("fieldValidations[0].independenceGroup must remain in the reference incident group")
            if not _positive_number(validation["capMiBps"]):
                errors.append("fieldValidations[0].capMiBps must be positive")
            if validation["controlledLoadStarted"] is not False:
                errors.append("fieldValidations[0].controlledLoadStarted must be false")
            sample_count = validation["sampleCount"]
            unsafe_count = validation["unsafeSampleCount"]
            if type(sample_count) is not int or sample_count < 1:
                errors.append("fieldValidations[0].sampleCount must be a positive integer")
            if type(unsafe_count) is not int or unsafe_count < 0:
                errors.append("fieldValidations[0].unsafeSampleCount must be a non-negative integer")
            elif type(sample_count) is int and unsafe_count > sample_count:
                errors.append("fieldValidations[0].unsafeSampleCount cannot exceed sampleCount")
            if not _positive_number(validation["unsafeThresholdMilliseconds"]):
                errors.append("fieldValidations[0].unsafeThresholdMilliseconds must be positive")
            if not _non_negative_number(validation["p99WriteWaitMilliseconds"]):
                errors.append("fieldValidations[0].p99WriteWaitMilliseconds must be non-negative")
            elif (
                type(unsafe_count) is int
                and unsafe_count > 0
                and _positive_number(validation["unsafeThresholdMilliseconds"])
                and validation["p99WriteWaitMilliseconds"] <= validation["unsafeThresholdMilliseconds"]
            ):
                errors.append("fieldValidations[0].p99WriteWaitMilliseconds contradicts unsafeSampleCount")
            if validation["replayable"] is not False:
                errors.append("fieldValidations[0].replayable must be false for aggregate evidence")
            if validation["outcome"] != "rejected_and_rolled_back":
                errors.append("fieldValidations[0].outcome must equal rejected_and_rolled_back")

    return errors


def _first_consecutive_above(values: Sequence[float], threshold: float, count: int) -> Optional[int]:
    run = 0
    for index, value in enumerate(values):
        run = run + 1 if value > threshold else 0
        if run >= count:
            return index
    return None


def _first_at_or_above(values: Sequence[float], threshold: float) -> Optional[int]:
    return next((index for index, value in enumerate(values) if value >= threshold), None)


def _iso(value: datetime) -> str:
    return value.astimezone(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def assess_incident_evidence(
    wait_fixture: Mapping[str, object],
    evidence: Mapping[str, object],
    pressure_threshold_milliseconds: float = 25,
    pressure_consecutive_samples: int = 2,
    critical_threshold_milliseconds: float = 100,
) -> Dict[str, object]:
    """Assess only what the retained timeline and samples can actually prove."""

    errors = validate_incident_evidence(evidence)
    if errors:
        raise ValueError("invalid incident evidence: " + "; ".join(errors))
    if pressure_consecutive_samples < 1:
        raise ValueError("pressure_consecutive_samples must be positive")

    wait_source = wait_fixture.get("source")
    samples = wait_fixture.get("samples")
    if not isinstance(wait_source, dict) or not isinstance(samples, list) or not samples:
        raise ValueError("wait fixture must contain source and samples")
    if any(type(value) not in (int, float) or value < 0 for value in samples):
        raise ValueError("wait fixture samples must be non-negative numbers")
    waits = [float(value) for value in samples]

    timeline = evidence["timeline"]
    assert isinstance(timeline, dict)
    events = timeline["events"]
    assert isinstance(events, list)
    event_times = {
        event["kind"]: datetime.fromisoformat(event["observedAt"].replace("Z", "+00:00")).astimezone(timezone.utc)
        for event in events
    }
    sampling_start = event_times["write_wait_sampling_started"]
    wait_start_errors: List[str] = []
    wait_start = _utc_timestamp(wait_source.get("observedAt"), "wait source observedAt", wait_start_errors)
    if wait_start is None or wait_start != sampling_start:
        raise ValueError("wait fixture sampling start does not match incident evidence")

    pressure_offset = _first_consecutive_above(
        waits,
        pressure_threshold_milliseconds,
        pressure_consecutive_samples,
    )
    critical_offset = _first_at_or_above(waits, critical_threshold_milliseconds)
    pressure_at = sampling_start + timedelta(seconds=pressure_offset) if pressure_offset is not None else None
    critical_at = sampling_start + timedelta(seconds=critical_offset) if critical_offset is not None else None
    failure_at = event_times["management_service_restart"]
    sampling_lag = int((sampling_start - failure_at).total_seconds())
    detection_lag = int((pressure_at - failure_at).total_seconds()) if pressure_at is not None else None

    if sampling_lag > 0:
        advance_status = "not_proven_telemetry_started_after_failure"
        advance_proven = False
        advance_lead = None
    elif detection_lag is not None and detection_lag < 0:
        advance_status = "proven_from_retained_timeline"
        advance_proven = True
        advance_lead = -detection_lag
    else:
        advance_status = "not_proven_detection_not_before_failure"
        advance_proven = False
        advance_lead = None

    validations = evidence["fieldValidations"]
    assert isinstance(validations, list)
    field = validations[0]
    assert isinstance(field, dict)
    unsafe_percent = round(100 * field["unsafeSampleCount"] / field["sampleCount"], 2)

    return {
        "kind": "observed-incident-evidence-assessment",
        "detection": {
            "pressureThresholdMilliseconds": pressure_threshold_milliseconds,
            "pressureConsecutiveSamples": pressure_consecutive_samples,
            "pressureDetectionOffsetSeconds": pressure_offset,
            "pressureDetectedAt": _iso(pressure_at) if pressure_at is not None else None,
            "criticalThresholdMilliseconds": critical_threshold_milliseconds,
            "criticalDetectionOffsetSeconds": critical_offset,
            "criticalDetectedAt": _iso(critical_at) if critical_at is not None else None,
            "samplingStartedAfterFailureSeconds": sampling_lag,
            "pressureDetectionAfterFailureSeconds": detection_lag,
            "advanceWarningStatus": advance_status,
            "advanceWarningProven": advance_proven,
            "advanceWarningLeadSeconds": advance_lead,
        },
        "corroboration": {
            "status": "not_measurable_missing_series",
            "missingSignals": sorted(timeline["missingSignals"]),
        },
        "fieldValidation": {
            "kind": field["kind"],
            "capMiBps": field["capMiBps"],
            "controlledLoadStarted": field["controlledLoadStarted"],
            "sampleCount": field["sampleCount"],
            "unsafeThresholdMilliseconds": field["unsafeThresholdMilliseconds"],
            "unsafeSampleCount": field["unsafeSampleCount"],
            "unsafeSamplePercent": unsafe_percent,
            "p99WriteWaitMilliseconds": field["p99WriteWaitMilliseconds"],
            "outcome": field["outcome"],
            "productionFallbackStatus": "rejected_by_observed_field_check",
            "replayEligible": False,
            "independentTrace": False,
        },
        "productionPromotionBlocked": True,
    }
