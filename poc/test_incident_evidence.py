import copy
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from incident_evidence import assess_incident_evidence, validate_incident_evidence


class IncidentEvidenceValidationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        fixture = Path(__file__).resolve().parent / "fixtures" / "reference-incident-evidence.json"
        with fixture.open(encoding="utf-8") as handle:
            cls.document = json.load(handle)

    def test_reference_document_is_valid(self):
        self.assertEqual(validate_incident_evidence(self.document), [])

    def test_rejects_timeline_that_starts_after_sampling(self):
        document = copy.deepcopy(self.document)
        document["timeline"]["events"][1]["observedAt"] = "2026-07-31T15:08:00Z"
        errors = validate_incident_evidence(document)
        self.assertTrue(any("event order" in error for error in errors), errors)

    def test_rejects_impossible_unsafe_count(self):
        document = copy.deepcopy(self.document)
        document["fieldValidations"][0]["unsafeSampleCount"] = 61
        errors = validate_incident_evidence(document)
        self.assertTrue(any("unsafeSampleCount" in error for error in errors), errors)

    def test_rejects_aggregate_evidence_claimed_as_replayable(self):
        document = copy.deepcopy(self.document)
        document["fieldValidations"][0]["replayable"] = True
        errors = validate_incident_evidence(document)
        self.assertTrue(any("replayable" in error for error in errors), errors)

    def test_rejects_same_episode_claimed_as_independent(self):
        document = copy.deepcopy(self.document)
        document["fieldValidations"][0]["independenceGroup"] = "another-incident"
        errors = validate_incident_evidence(document)
        self.assertTrue(any("independenceGroup" in error for error in errors), errors)


class IncidentEvidenceAssessmentTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        fixture_dir = Path(__file__).resolve().parent / "fixtures"
        with (fixture_dir / "reference-incident-evidence.json").open(encoding="utf-8") as handle:
            cls.evidence = json.load(handle)
        with (fixture_dir / "reference-write-wait-trace.json").open(encoding="utf-8") as handle:
            cls.wait_fixture = json.load(handle)

    def test_measures_detection_only_after_collection_begins(self):
        assessment = assess_incident_evidence(self.wait_fixture, self.evidence)
        detection = assessment["detection"]
        self.assertEqual(detection["pressureDetectionOffsetSeconds"], 1)
        self.assertEqual(detection["criticalDetectionOffsetSeconds"], 2)
        self.assertEqual(detection["samplingStartedAfterFailureSeconds"], 53)
        self.assertEqual(detection["pressureDetectionAfterFailureSeconds"], 54)
        self.assertEqual(
            detection["advanceWarningStatus"],
            "not_proven_telemetry_started_after_failure",
        )
        self.assertFalse(detection["advanceWarningProven"])

    def test_reports_missing_historical_corroboration(self):
        assessment = assess_incident_evidence(self.wait_fixture, self.evidence)
        self.assertEqual(
            assessment["corroboration"]["status"],
            "not_measurable_missing_series",
        )
        self.assertEqual(
            assessment["corroboration"]["missingSignals"],
            ["io_psi", "management_probe_series", "queue_depth"],
        )

    def test_fixed_cap_is_a_non_replayable_field_contradiction(self):
        assessment = assess_incident_evidence(self.wait_fixture, self.evidence)
        field = assessment["fieldValidation"]
        self.assertEqual(field["capMiBps"], 20)
        self.assertEqual(field["unsafeSamplePercent"], 36.67)
        self.assertEqual(field["p99WriteWaitMilliseconds"], 234.065464)
        self.assertEqual(field["productionFallbackStatus"], "rejected_by_observed_field_check")
        self.assertFalse(field["replayEligible"])
        self.assertFalse(field["independentTrace"])
        self.assertTrue(assessment["productionPromotionBlocked"])

    def test_rejects_wait_series_with_a_different_start(self):
        wait_fixture = copy.deepcopy(self.wait_fixture)
        wait_fixture["source"]["observedAt"] = "2026-07-31T15:07:43Z"
        with self.assertRaisesRegex(ValueError, "sampling start"):
            assess_incident_evidence(wait_fixture, self.evidence)

    def test_rejects_wait_series_with_altered_content(self):
        wait_fixture = copy.deepcopy(self.wait_fixture)
        wait_fixture["samples"][0] = wait_fixture["samples"][0] + 0.001
        with self.assertRaisesRegex(ValueError, "series digest"):
            assess_incident_evidence(wait_fixture, self.evidence)


if __name__ == "__main__":
    unittest.main()
