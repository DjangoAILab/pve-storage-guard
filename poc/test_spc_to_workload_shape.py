import bz2
import io
from pathlib import Path
import tempfile
import unittest

from spc_to_workload_shape import ConversionOptions, _open_source, build_workload_shape
from workload_shape_contract import assess_workload_shape


class SPCToWorkloadShapeTests(unittest.TestCase):
    def options(self, **overrides):
        values = {
            "name": "licensed-shape",
            "independence_group": "external-study-a",
            "workload_class": "search",
            "sample_interval_seconds": 10,
            "window_duration_seconds": 20,
            "authorized_and_sanitized": True,
        }
        values.update(overrides)
        return ConversionOptions(**values)

    def test_aggregates_counts_and_bytes_without_coordinates(self):
        source = io.StringIO(
            "0,657728,1048576,R,100.1,ignored-private-label\n"
            "1,31244784,1048576,W,100.2\n"
            "2,11813968,524288,W,109.9\n"
            "0,32234560,2097152,R,110.1\n"
        )
        trace = build_workload_shape(source, self.options())
        rendered = str(trace)
        self.assertNotIn("657728", rendered)
        self.assertNotIn("ignored-private-label", rendered)
        self.assertEqual(0.1, trace["samples"][0]["readIops"])
        self.assertEqual(0.2, trace["samples"][0]["writeIops"])
        self.assertEqual(0.1, trace["samples"][0]["readThroughputMiBps"])
        self.assertEqual(0.15, trace["samples"][0]["writeThroughputMiBps"])
        self.assertEqual(0.2, trace["samples"][1]["readThroughputMiBps"])
        self.assertEqual("unavailable", trace["metricSemantics"]["latency"])
        self.assertFalse(
            assess_workload_shape(trace, "reference-incident").active_control_eligible
        )

    def test_explicit_window_stops_before_out_of_scope_rows(self):
        source = io.StringIO(
            "0,1,4096,R,10.0\n"
            "0,2,4096,W,29.9\n"
            "0,3,4096,W,30.0\n"
        )
        trace = build_workload_shape(
            source,
            self.options(window_duration_seconds=20),
        )
        self.assertEqual(2, len(trace["samples"]))
        self.assertEqual(0.1, trace["samples"][1]["writeIops"])

    def test_rejects_unconfirmed_invalid_or_out_of_order_input(self):
        with self.assertRaisesRegex(ValueError, "confirmation"):
            build_workload_shape(
                io.StringIO("0,1,4096,R,0.0\n0,2,4096,W,10.0\n"),
                self.options(authorized_and_sanitized=False),
            )
        for source, message in (
            ("0,1,4096,X,0.0\n", "opcode"),
            ("0,1,4.5,R,0.0\n", "size"),
            ("0,1,4096,R,nan\n", "finite"),
            ("0,1,4096,R,2.0\n0,2,4096,R,1.0\n", "earlier"),
            ("0,1,4096\n", "fewer than five"),
        ):
            with self.subTest(message=message):
                with self.assertRaisesRegex(ValueError, message):
                    build_workload_shape(io.StringIO(source), self.options())

    def test_rejects_oversized_records_and_unsafe_metadata(self):
        oversized = "0,1,4096,R,0.0," + "x" * 65_536 + "\n"
        with self.assertRaisesRegex(ValueError, "bounded record size"):
            build_workload_shape(io.StringIO(oversized), self.options())
        with self.assertRaisesRegex(ValueError, "safe slug"):
            build_workload_shape(
                io.StringIO("0,1,4096,R,0.0\n0,2,4096,R,10.0\n"),
                self.options(name="private node"),
            )

    def test_reads_bzip2_input_and_bounds_numeric_fields(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "trace.spc.bz2"
            with bz2.open(path, mode="wt", encoding="ascii") as compressed:
                compressed.write("0,1,4096,R,0.0\n0,2,4096,W,10.0\n")
            with _open_source(path) as source:
                trace = build_workload_shape(source, self.options())
        self.assertEqual(2, len(trace["samples"]))

        too_wide = "0," + "9" * 21 + ",4096,R,0.0\n"
        with self.assertRaisesRegex(ValueError, "LBA"):
            build_workload_shape(io.StringIO(too_wide), self.options())


if __name__ == "__main__":
    unittest.main()
