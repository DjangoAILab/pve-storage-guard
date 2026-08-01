import copy
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from incident_evidence import validate_incident_evidence


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


if __name__ == "__main__":
    unittest.main()
