import io
import unittest

from io_csv_to_replay_trace import ConversionOptions, build_trace
from trace_contract import API_VERSION, assess_trace


class IOCSVToReplayTraceTests(unittest.TestCase):
    def test_aggregates_authorized_rows_without_copying_identity_columns(self):
        source = io.StringIO(
            "timestamp_seconds,operation,size_bytes,response_time_milliseconds,hostname\n"
            "100.1,write,1048576,10,private-node\n"
            "100.8,W,1048576,30,private-node\n"
            "101.2,read,4096,4,private-node\n"
            "102.1,write,524288,90,private-node\n"
        )
        trace = build_trace(source, ConversionOptions(
            name="licensed-sample",
            source_kind="observed",
            independence_group="external-study-a",
            storage_class="rotational-hdd",
            workload_class="mixed",
            write_wait_measurement_layer="storage-domain",
            sample_interval_seconds=1,
            authorized_and_sanitized=True,
        ))

        self.assertEqual(trace["apiVersion"], API_VERSION)
        self.assertNotIn("private-node", str(trace))
        self.assertEqual(trace["metadata"]["windowDurationSeconds"], 3)
        self.assertEqual(trace["samples"][0]["writeWaitMilliseconds"], 30)
        self.assertEqual(trace["samples"][0]["writeIops"], 2)
        self.assertEqual(trace["samples"][0]["writeThroughputMiBps"], 2)
        self.assertFalse(trace["samples"][1]["waitValid"])
        self.assertEqual(trace["samples"][1]["managementPlaneStatus"], "unknown")

        assessment = assess_trace(trace, "reference-incident")
        self.assertEqual(assessment.errors, [])
        self.assertEqual(assessment.management_plane_completeness, 0)
        self.assertFalse(assessment.meets_machine_independence_gate)

    def test_rejects_unsafe_metadata_instead_of_claiming_sanitization(self):
        source = io.StringIO(
            "timestamp_seconds,operation,size_bytes,response_time_milliseconds\n"
            "0,write,4096,1\n"
        )
        with self.assertRaisesRegex(ValueError, "safe slug"):
            build_trace(source, ConversionOptions(
                name="node 192.0.2.1",
                source_kind="observed",
                independence_group="external-study-a",
                storage_class="rotational-hdd",
                workload_class="mixed",
                write_wait_measurement_layer="storage-domain",
                sample_interval_seconds=1,
                authorized_and_sanitized=True,
            ))

    def test_rejects_unknown_operations_and_non_finite_values(self):
        source = io.StringIO(
            "timestamp_seconds,operation,size_bytes,response_time_milliseconds\n"
            "0,discard,4096,nan\n"
        )
        with self.assertRaisesRegex(ValueError, "operation"):
            build_trace(source, ConversionOptions(
                name="licensed-sample",
                source_kind="observed",
                independence_group="external-study-a",
                storage_class="rotational-hdd",
                workload_class="mixed",
                write_wait_measurement_layer="storage-domain",
                sample_interval_seconds=1,
                authorized_and_sanitized=True,
            ))

    def test_rejects_unconfirmed_or_out_of_order_input(self):
        unconfirmed = io.StringIO(
            "timestamp_seconds,operation,size_bytes,response_time_milliseconds\n"
            "0,write,4096,1\n1,write,4096,1\n"
        )
        with self.assertRaisesRegex(ValueError, "confirmation"):
            build_trace(unconfirmed, ConversionOptions(
                name="licensed-sample",
                source_kind="observed",
                independence_group="external-study-a",
                storage_class="rotational-hdd",
                workload_class="mixed",
                write_wait_measurement_layer="storage-domain",
                sample_interval_seconds=1,
            ))

        out_of_order = io.StringIO(
            "timestamp_seconds,operation,size_bytes,response_time_milliseconds\n"
            "2,write,4096,1\n1,write,4096,1\n"
        )
        with self.assertRaisesRegex(ValueError, "earlier"):
            build_trace(out_of_order, ConversionOptions(
                name="licensed-sample",
                source_kind="observed",
                independence_group="external-study-a",
                storage_class="rotational-hdd",
                workload_class="mixed",
                write_wait_measurement_layer="storage-domain",
                sample_interval_seconds=1,
                authorized_and_sanitized=True,
            ))

    def test_rejects_ambiguous_headers_and_non_integer_byte_counts(self):
        duplicate_header = io.StringIO(
            "timestamp_seconds,operation,size_bytes,size_bytes,response_time_milliseconds\n"
            "0,write,4096,4096,1\n1,write,4096,4096,1\n"
        )
        options = ConversionOptions(
            name="licensed-sample",
            source_kind="observed",
            independence_group="external-study-a",
            storage_class="rotational-hdd",
            workload_class="mixed",
            write_wait_measurement_layer="storage-domain",
            sample_interval_seconds=1,
            authorized_and_sanitized=True,
        )
        with self.assertRaisesRegex(ValueError, "duplicate"):
            build_trace(duplicate_header, options)

        decimal_size = io.StringIO(
            "timestamp_seconds,operation,size_bytes,response_time_milliseconds\n"
            "0,write,4096.0,1\n1,write,4096,1\n"
        )
        with self.assertRaisesRegex(ValueError, "decimal integer"):
            build_trace(decimal_size, options)


if __name__ == "__main__":
    unittest.main()
