import unittest

from trace_contract import API_VERSION, assess_trace


def trace_document(source_kind="observed", statistic="p95", aggregation="none", group="independent-window"):
    return {
        "apiVersion": API_VERSION,
        "kind": "ReplayTrace",
        "metadata": {
            "name": "sanitized-validation-trace",
            "sourceKind": source_kind,
            "independenceGroup": group,
            "storageClass": "rotational-hdd",
            "workloadClass": "backup",
            "sampleIntervalSeconds": 1,
            "windowDurationSeconds": 600,
            "sanitized": True,
        },
        "metricSemantics": {
            "writeWaitStatistic": statistic,
            "controlWindowAggregation": aggregation,
            "provenance": "observed",
        },
        "samples": [
            {
                "offsetSeconds": second,
                "writeWaitMilliseconds": 10.0,
                "waitValid": True,
                "managementPlaneHealthy": True,
            }
            for second in range(600)
        ],
    }


class TraceContractTests(unittest.TestCase):
    def test_observed_complete_distinct_p95_trace_qualifies(self):
        assessment = assess_trace(trace_document(), "reference-incident")
        self.assertEqual(assessment.errors, [])
        self.assertEqual(assessment.completeness, 1.0)
        self.assertTrue(assessment.policy_signal_compatible)
        self.assertTrue(assessment.meets_machine_independence_gate)

    def test_synthetic_trace_never_qualifies_as_observed_evidence(self):
        assessment = assess_trace(trace_document(source_kind="synthetic"), "reference-incident")
        self.assertEqual(assessment.errors, [])
        self.assertFalse(assessment.meets_machine_independence_gate)

    def test_total_wait_samples_are_not_relabelled_as_p95(self):
        assessment = assess_trace(
            trace_document(statistic="total-wait", aggregation="p95"),
            "reference-incident",
        )
        self.assertEqual(assessment.errors, [])
        self.assertFalse(assessment.policy_signal_compatible)
        self.assertFalse(assessment.meets_machine_independence_gate)

    def test_same_independence_group_does_not_qualify(self):
        assessment = assess_trace(trace_document(group="reference-incident"), "reference-incident")
        self.assertFalse(assessment.meets_machine_independence_gate)

    def test_out_of_order_series_reports_error(self):
        document = trace_document()
        document["samples"][1]["offsetSeconds"] = 700
        assessment = assess_trace(document, "reference-incident")
        self.assertTrue(assessment.errors)
        self.assertFalse(assessment.meets_machine_independence_gate)

    def test_leading_and_trailing_gaps_count_against_declared_window(self):
        document = trace_document()
        document["samples"] = document["samples"][20:-20]
        assessment = assess_trace(document, "reference-incident")
        self.assertEqual(assessment.errors, [])
        self.assertLess(assessment.completeness, 0.95)
        self.assertFalse(assessment.meets_machine_independence_gate)

    def test_unknown_storage_class_does_not_meet_machine_gate(self):
        document = trace_document()
        document["metadata"]["storageClass"] = "unknown"
        assessment = assess_trace(document, "reference-incident")
        self.assertEqual(assessment.errors, [])
        self.assertFalse(assessment.meets_machine_independence_gate)


if __name__ == "__main__":
    unittest.main()
