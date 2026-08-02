import unittest

from trace_contract import API_VERSION, LEGACY_API_VERSION, assess_trace


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
            "writeWaitMeasurementLayer": "storage-domain",
            "controlWindowAggregation": aggregation,
            "provenance": "observed",
        },
        "samples": [
            {
                "offsetSeconds": second,
                "writeWaitMilliseconds": 10.0,
                "waitValid": True,
                "managementPlaneStatus": "healthy",
            }
            for second in range(600)
        ],
    }


class TraceContractTests(unittest.TestCase):
    def test_observed_complete_distinct_p95_trace_qualifies(self):
        assessment = assess_trace(trace_document(), "reference-incident")
        self.assertEqual(assessment.errors, [])
        self.assertEqual(assessment.completeness, 1.0)
        self.assertEqual(assessment.wait_completeness, 1.0)
        self.assertEqual(assessment.management_plane_completeness, 1.0)
        self.assertTrue(assessment.policy_signal_compatible)
        self.assertTrue(assessment.meets_machine_independence_gate)

    def test_unknown_management_evidence_is_valid_but_never_promotes(self):
        document = trace_document()
        for sample in document["samples"]:
            sample["managementPlaneStatus"] = "unknown"
        assessment = assess_trace(document, "reference-incident")
        self.assertEqual(assessment.errors, [])
        self.assertEqual(assessment.wait_completeness, 1.0)
        self.assertEqual(assessment.management_plane_completeness, 0.0)
        self.assertTrue(assessment.policy_signal_compatible)
        self.assertFalse(assessment.meets_machine_independence_gate)

    def test_invalid_wait_placeholders_do_not_count_as_evidence(self):
        document = trace_document()
        for sample in document["samples"][:31]:
            sample["waitValid"] = False
            sample["writeWaitMilliseconds"] = 0
        assessment = assess_trace(document, "reference-incident")
        self.assertEqual(assessment.errors, [])
        self.assertLess(assessment.wait_completeness, 0.95)
        self.assertFalse(assessment.meets_machine_independence_gate)

    def test_v1alpha1_boolean_management_samples_remain_readable_but_do_not_promote(self):
        document = trace_document()
        document["apiVersion"] = LEGACY_API_VERSION
        document["metricSemantics"].pop("writeWaitMeasurementLayer")
        for sample in document["samples"]:
            sample["managementPlaneHealthy"] = sample.pop("managementPlaneStatus") == "healthy"
        assessment = assess_trace(document, "reference-incident")
        self.assertEqual(assessment.errors, [])
        self.assertEqual(assessment.management_plane_completeness, 1.0)
        self.assertFalse(assessment.meets_machine_independence_gate)

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

    def test_block_device_p95_is_not_storage_domain_policy_evidence(self):
        document = trace_document()
        document["metricSemantics"]["writeWaitMeasurementLayer"] = "block-device"
        assessment = assess_trace(document, "reference-incident")
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

    def test_invalid_offsets_and_waits_do_not_inflate_reported_coverage(self):
        document = trace_document()
        document["samples"].append({
            "offsetSeconds": 600,
            "writeWaitMilliseconds": 10.0,
            "waitValid": True,
            "managementPlaneStatus": "healthy",
        })
        document["samples"][0]["writeWaitMilliseconds"] = float("nan")
        assessment = assess_trace(document, "reference-incident")
        self.assertTrue(assessment.errors)
        self.assertEqual(assessment.completeness, 1.0)
        self.assertLess(assessment.wait_completeness, 1.0)
        self.assertEqual(assessment.management_plane_completeness, 1.0)
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
