import math
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from storage_control import (
    AimdController,
    DiskPolicy,
    Feedback,
    PoolPolicy,
    allocate_budget,
)


class AimdControllerTests(unittest.TestCase):
    def setUp(self):
        self.policy = PoolPolicy(
            minimum_budget_mibps=5,
            initial_budget_mibps=20,
            maximum_budget_mibps=40,
            additive_increase_mibps=2,
            multiplicative_decrease=0.7,
            healthy_windows=2,
            breach_windows=2,
            cooldown_seconds=30,
        )

    def test_emergency_bypasses_cooldown_and_clamps_to_minimum(self):
        controller = AimdController(self.policy)
        controller.observe(Feedback(at_seconds=0, wait_milliseconds=10))
        decision = controller.observe(Feedback(at_seconds=1, wait_milliseconds=120))
        self.assertTrue(decision.changed)
        self.assertEqual(decision.budget_mibps, 5)
        self.assertEqual(decision.reason, "emergency")

    def test_stale_feedback_never_increases(self):
        controller = AimdController(self.policy)
        decision = controller.observe(Feedback(at_seconds=100, wait_milliseconds=None, stale=True))
        self.assertFalse(decision.changed)
        self.assertEqual(decision.budget_mibps, 20)
        self.assertEqual(decision.reason, "stale_hold")

    def test_healthy_windows_add_and_breaches_multiply(self):
        controller = AimdController(self.policy)
        controller.observe(Feedback(at_seconds=0, wait_milliseconds=10))
        increased = controller.observe(Feedback(at_seconds=30, wait_milliseconds=10))
        self.assertEqual(increased.budget_mibps, 22)
        controller.observe(Feedback(at_seconds=60, wait_milliseconds=30))
        decreased = controller.observe(Feedback(at_seconds=90, wait_milliseconds=30))
        self.assertEqual(decreased.budget_mibps, 15.4)

    def test_budget_never_exceeds_policy_bounds(self):
        controller = AimdController(self.policy)
        for index in range(50):
            controller.observe(Feedback(at_seconds=index * 30, wait_milliseconds=1))
        self.assertLessEqual(controller.state.budget_mibps, 40)
        controller.observe(Feedback(at_seconds=2000, wait_milliseconds=1000))
        self.assertEqual(controller.state.budget_mibps, 5)


class AllocationTests(unittest.TestCase):
    def test_weighted_allocation_respects_minimums_and_maximums(self):
        disks = [
            DiskPolicy("a", minimum_mibps=5, maximum_mibps=20, weight=1),
            DiskPolicy("b", minimum_mibps=10, maximum_mibps=50, weight=3),
        ]
        result = allocate_budget(50, disks)
        self.assertTrue(result.feasible)
        self.assertTrue(math.isclose(sum(result.allocations.values()), 50))
        self.assertGreater(result.allocations["b"], result.allocations["a"])
        self.assertLessEqual(result.allocations["a"], 20)
        self.assertLessEqual(result.allocations["b"], 50)

    def test_infeasible_minimums_are_explicit(self):
        disks = [
            DiskPolicy("a", minimum_mibps=10, maximum_mibps=20, weight=1),
            DiskPolicy("b", minimum_mibps=10, maximum_mibps=20, weight=1),
        ]
        result = allocate_budget(15, disks)
        self.assertFalse(result.feasible)
        self.assertEqual(result.required_minimum_mibps, 20)
        self.assertEqual(result.allocations, {"a": 10, "b": 10})


if __name__ == "__main__":
    unittest.main()
