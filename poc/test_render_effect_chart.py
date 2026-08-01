import tempfile
import unittest
from pathlib import Path

from render_effect_chart import build_svg, load_results


ROOT = Path(__file__).resolve().parents[1]


class EffectChartTests(unittest.TestCase):
    def test_chart_is_deterministic_and_matches_committed_asset(self):
        results = load_results(ROOT / "poc" / "results" / "report.json")
        rendered = build_svg(results)

        self.assertEqual(rendered, build_svg(results))
        self.assertEqual(rendered, (ROOT / "docs" / "assets" / "policy-effect.svg").read_text())
        self.assertEqual(rendered, (ROOT / "website" / "public" / "policy-effect.svg").read_text())
        self.assertIn("NOT A PRODUCTION MEASUREMENT", rendered)
        self.assertIn("60.11%", rendered)
        self.assertIn("158", rendered)
        self.assertNotIn("t7610", rendered.lower())

    def test_missing_required_strategy_fails_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            incomplete = Path(directory) / "report.json"
            incomplete.write_text('{"counterfactual": {}}')
            with self.assertRaisesRegex(ValueError, "has no result rows"):
                load_results(incomplete)


if __name__ == "__main__":
    unittest.main()
