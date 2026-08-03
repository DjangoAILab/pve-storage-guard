import copy
import unittest

from workload_shape_contract import API_VERSION, KIND, assess_workload_shape


def valid_document():
    return {
        "apiVersion": API_VERSION,
        "kind": KIND,
        "metadata": {
            "name": "licensed-shape",
            "sourceKind": "observed",
            "independenceGroup": "external-study-a",
            "workloadClass": "search",
            "sampleIntervalSeconds": 60,
            "windowDurationSeconds": 600,
            "sanitized": True,
        },
        "metricSemantics": {
            "timestamp": "issue-offset-seconds",
            "ioLayer": "host-to-logical-unit",
            "latency": "unavailable",
            "managementPlane": "unavailable",
            "provenance": "derived",
        },
        "samples": [
            {
                "offsetSeconds": offset,
                "readIops": 1,
                "writeIops": 1 if offset == 0 else 0,
                "readThroughputMiBps": 0.1,
                "writeThroughputMiBps": 0.01 if offset == 0 else 0,
            }
            for offset in range(0, 600, 60)
        ],
    }


class WorkloadShapeContractTests(unittest.TestCase):
    def test_independent_observed_shape_meets_research_gate_only(self):
        assessment = assess_workload_shape(valid_document(), "reference-incident")
        self.assertEqual([], assessment.errors)
        self.assertTrue(assessment.meets_research_gate)
        self.assertFalse(assessment.active_control_eligible)
        self.assertEqual(60, assessment.write_active_bucket_seconds)

    def test_same_group_or_unknown_workload_cannot_meet_research_gate(self):
        same_group = assess_workload_shape(valid_document(), "external-study-a")
        self.assertFalse(same_group.meets_research_gate)
        unknown = valid_document()
        unknown["metadata"]["workloadClass"] = "unknown"
        self.assertFalse(assess_workload_shape(unknown, "reference-incident").meets_research_gate)

    def test_missing_samples_and_extra_fields_fail_closed(self):
        incomplete = valid_document()
        incomplete["samples"] = incomplete["samples"][:8]
        assessment = assess_workload_shape(incomplete, "reference-incident")
        self.assertEqual([], assessment.errors)
        self.assertEqual(0.8, assessment.completeness)
        self.assertFalse(assessment.meets_research_gate)

        extra = valid_document()
        extra["samples"][0]["latencyMilliseconds"] = 4
        self.assertIn(
            "sample 0 fields do not match the strict contract",
            assess_workload_shape(extra, "reference-incident").errors,
        )

    def test_semantic_drift_and_private_metadata_are_rejected(self):
        semantic_drift = valid_document()
        semantic_drift["metricSemantics"]["latency"] = "p95"
        self.assertIn(
            "metricSemantics must preserve the SPC research boundary",
            assess_workload_shape(semantic_drift, "reference-incident").errors,
        )

        private = copy.deepcopy(valid_document())
        private["metadata"]["name"] = "node 192.0.2.1"
        self.assertTrue(assess_workload_shape(private, "reference-incident").errors)


if __name__ == "__main__":
    unittest.main()
