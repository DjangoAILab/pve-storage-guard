import sys
import json
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from simulate import (
    DemandSample,
    PoolModel,
    StrategyResult,
    build_demand_trace,
    rank_results,
    replay_observed_wait,
    run_poc,
)
from storage_control import AimdController, PoolPolicy


class DemandTraceTests(unittest.TestCase):
    def test_rates_describe_the_preceding_interval_and_override_exact_incident(self):
        fixture = {
            "samples": [
                ["2026-01-01T00:00:00Z", 1, 0.5],
                ["2026-01-01T00:00:03Z", 3, 0.25],
                ["2026-01-01T00:00:05Z", 5, 0.75],
            ],
            "incidentOverride": {
                "start": "2026-01-01T00:00:04Z",
                "durationSeconds": 1,
                "controlledDemandMiBps": 10,
            },
        }
        trace = build_demand_trace(fixture)
        self.assertEqual([sample.controlled_demand_mibps for sample in trace], [3, 3, 3, 5, 10])
        self.assertEqual(trace[0].background_mibps, 0.25)
        self.assertTrue(trace[-1].incident_override)


class PoolModelTests(unittest.TestCase):
    def test_wait_is_monotonic_and_safe_capacity_has_baseline_wait(self):
        model = PoolModel("test", base_wait_ms=10, safe_capacity_mibps=20, slope_ms_per_mibps=1)
        self.assertEqual(model.wait_milliseconds(10), 10)
        self.assertEqual(model.wait_milliseconds(20), 10)
        self.assertGreater(model.wait_milliseconds(40), model.wait_milliseconds(30))


class ShadowReplayTests(unittest.TestCase):
    def test_observed_wait_is_not_rewritten_and_emergency_clamps(self):
        controller = AimdController(PoolPolicy(cooldown_seconds=60))
        waits = [50, 80, 120, 30]
        replay = replay_observed_wait(controller, waits, control_interval_seconds=10)
        self.assertEqual(replay["observedWaitMilliseconds"], waits)
        self.assertEqual(replay["decisions"][0]["reason"], "emergency")
        self.assertEqual(replay["decisions"][0]["budgetMiBps"], 5)


class RankingTests(unittest.TestCase):
    def test_safety_is_ranked_before_throughput(self):
        unsafe_fast = StrategyResult("unsafe", 3, 1, 5, 1000, 1000, 1000, 1, 1, [])
        safe_slow = StrategyResult("safe", 0, 0, 0, 100, 1000, 10000, 5, 5, [])
        ranked = rank_results([unsafe_fast, safe_slow])
        self.assertEqual(ranked[0].strategy, "safe")


class ReportGateTests(unittest.TestCase):
    def test_recommended_candidate_is_no_less_safe_than_fixed_in_every_model(self):
        fixture_dir = Path(__file__).resolve().parent / "fixtures"
        with (fixture_dir / "reference-demand-trace.json").open(encoding="utf-8") as handle:
            demand = json.load(handle)
        with (fixture_dir / "reference-write-wait-trace.json").open(encoding="utf-8") as handle:
            waits = json.load(handle)
        report = run_poc(demand, waits)
        candidate = report["selectionGate"]["recommendedShadowCandidate"]
        self.assertEqual(candidate, "aimd_poc_tuned")
        for scenario in report["counterfactual"].values():
            by_strategy = {result["strategy"]: result for result in scenario["results"]}
            self.assertLessEqual(
                by_strategy[candidate]["unsafe_seconds"],
                by_strategy["fixed_20"]["unsafe_seconds"],
            )

    def test_selected_policy_has_explicit_parameter_neighborhood(self):
        fixture_dir = Path(__file__).resolve().parent / "fixtures"
        with (fixture_dir / "reference-demand-trace.json").open(encoding="utf-8") as handle:
            demand = json.load(handle)
        with (fixture_dir / "reference-write-wait-trace.json").open(encoding="utf-8") as handle:
            waits = json.load(handle)
        report = run_poc(demand, waits)
        rows = report["parameterSensitivity"]["rows"]
        self.assertEqual(len(rows), 18)
        selected_rows = [row for row in rows if row["selectedValue"]]
        self.assertEqual(len(selected_rows), 6)
        self.assertTrue(all(row["passesSafetyGate"] for row in selected_rows))


if __name__ == "__main__":
    unittest.main()
