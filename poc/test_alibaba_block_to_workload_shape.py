import io
import unittest

from alibaba_block_to_workload_shape import ConversionOptions, build_workload_shape
from workload_shape_contract import assess_workload_shape


class AlibabaBlockToWorkloadShapeTests(unittest.TestCase):
    def options(self, **overrides):
        values = {
            "name": "alibaba-ultra-prefix",
            "independence_group": "alibaba-block-2020",
            "workload_class": "mixed",
            "sample_interval_seconds": 10,
            "window_duration_seconds": 20,
            "authorized_and_sanitized": True,
        }
        values.update(overrides)
        return ConversionOptions(**values)

    def test_aggregates_arrivals_without_device_or_offset_coordinates(self):
        source = io.StringIO(
            "731,R,126703661056,1048576,1577808000000046\n"
            "932,W,31244784,1048576,1577808000200046\n"
            "731,W,11813968,524288,1577808009900046\n"
            "932,R,32234560,2097152,1577808010100046\n"
        )
        trace = build_workload_shape(source, self.options())
        rendered = str(trace)
        self.assertNotIn("731", rendered)
        self.assertNotIn("126703661056", rendered)
        self.assertEqual("network-block", trace["metadata"]["storageClass"])
        self.assertEqual("arrival-offset-seconds", trace["metricSemantics"]["timestamp"])
        self.assertEqual("virtual-block-service", trace["metricSemantics"]["ioLayer"])
        self.assertEqual(0.1, trace["samples"][0]["readIops"])
        self.assertEqual(0.2, trace["samples"][0]["writeIops"])
        assessment = assess_workload_shape(trace, "reference-incident")
        self.assertTrue(assessment.storage_class_known)
        self.assertFalse(assessment.active_control_eligible)

    def test_explicit_window_stops_before_truncated_suffix(self):
        source = io.StringIO(
            "0,R,1,4096,100000000\n"
            "0,W,2,4096,119999999\n"
            "0,R,3,4096,120000000\n"
            "truncated suffix is never parsed"
        )
        trace = build_workload_shape(source, self.options())
        self.assertEqual(2, len(trace["samples"]))
        self.assertEqual(0.1, trace["samples"][1]["writeIops"])

    def test_rejects_unconfirmed_malformed_and_out_of_order_input(self):
        with self.assertRaisesRegex(ValueError, "confirmation"):
            build_workload_shape(
                io.StringIO("0,R,1,4096,0\n0,W,2,4096,10000000\n"),
                self.options(authorized_and_sanitized=False),
            )
        for source, message in (
            ("device_id,opcode,offset,length,timestamp\n", "device ID"),
            ("0,X,1,4096,0\n", "opcode"),
            ("0,R,1,4.5,0\n", "length"),
            ("0,R,1,4096,-1\n", "timestamp"),
            ("0,R,1,4096,2\n0,R,2,4096,1\n", "earlier"),
            ("0,R,1,4096\n", "five fields"),
        ):
            with self.subTest(message=message):
                with self.assertRaisesRegex(ValueError, message):
                    build_workload_shape(io.StringIO(source), self.options())

    def test_rejects_oversized_records_and_unsafe_metadata(self):
        oversized = "0,R,1,4096,0," + "x" * 65_536 + "\n"
        with self.assertRaisesRegex(ValueError, "bounded record size"):
            build_workload_shape(io.StringIO(oversized), self.options())
        with self.assertRaisesRegex(ValueError, "safe slug"):
            build_workload_shape(
                io.StringIO("0,R,1,4096,0\n0,W,2,4096,10000000\n"),
                self.options(name="private node"),
            )
        with self.assertRaisesRegex(ValueError, "at least two samples"):
            build_workload_shape(
                io.StringIO("0,R,1,4096,0\n0,W,2,4096,10000000\n"),
                self.options(window_duration_seconds=0),
            )


if __name__ == "__main__":
    unittest.main()
