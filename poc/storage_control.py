"""Pure storage-control domain primitives used by the offline POC.

This module intentionally contains no PVE, SSH, database, or filesystem
actuation code. It turns feedback into desired budgets and allocates a pool
budget across explicitly enrolled disks.
"""

from dataclasses import dataclass
from typing import Dict, Iterable, Optional


@dataclass(frozen=True)
class PoolPolicy:
    minimum_budget_mibps: float = 5
    initial_budget_mibps: float = 20
    maximum_budget_mibps: float = 40
    healthy_wait_milliseconds: float = 15
    target_wait_milliseconds: float = 25
    emergency_wait_milliseconds: float = 100
    additive_increase_mibps: float = 2
    multiplicative_decrease: float = 0.7
    healthy_windows: int = 3
    breach_windows: int = 2
    cooldown_seconds: int = 60

    def __post_init__(self) -> None:
        if not 0 < self.minimum_budget_mibps <= self.initial_budget_mibps <= self.maximum_budget_mibps:
            raise ValueError("budget bounds must satisfy 0 < minimum <= initial <= maximum")
        if not 0 <= self.healthy_wait_milliseconds < self.target_wait_milliseconds < self.emergency_wait_milliseconds:
            raise ValueError("wait thresholds must be strictly increasing")
        if self.additive_increase_mibps <= 0:
            raise ValueError("additive increase must be positive")
        if not 0 < self.multiplicative_decrease < 1:
            raise ValueError("multiplicative decrease must be between zero and one")
        if self.healthy_windows < 1 or self.breach_windows < 1 or self.cooldown_seconds < 0:
            raise ValueError("window counts must be positive and cooldown non-negative")


@dataclass(frozen=True)
class DiskPolicy:
    disk_key: str
    minimum_mibps: float
    maximum_mibps: float
    weight: float = 1

    def __post_init__(self) -> None:
        if not self.disk_key:
            raise ValueError("disk key is required")
        if not 0 <= self.minimum_mibps <= self.maximum_mibps:
            raise ValueError("disk bounds must satisfy 0 <= minimum <= maximum")
        if self.weight <= 0:
            raise ValueError("disk weight must be positive")


@dataclass(frozen=True)
class Feedback:
    at_seconds: float
    wait_milliseconds: Optional[float]
    stale: bool = False
    emergency: bool = False


@dataclass(frozen=True)
class Decision:
    at_seconds: float
    previous_budget_mibps: float
    budget_mibps: float
    changed: bool
    reason: str


@dataclass
class ControllerState:
    budget_mibps: float
    consecutive_healthy: int = 0
    consecutive_breaches: int = 0
    last_change_at_seconds: float = float("-inf")


@dataclass(frozen=True)
class AllocationResult:
    allocations: Dict[str, float]
    feasible: bool
    required_minimum_mibps: float
    unallocated_mibps: float


class AimdController:
    def __init__(self, policy: PoolPolicy):
        self.policy = policy
        self.state = ControllerState(policy.initial_budget_mibps)

    def _decision(self, feedback: Feedback, desired: float, reason: str, bypass_cooldown: bool = False) -> Decision:
        previous = self.state.budget_mibps
        desired = round(
            min(self.policy.maximum_budget_mibps, max(self.policy.minimum_budget_mibps, desired)),
            6,
        )
        cooldown_elapsed = feedback.at_seconds - self.state.last_change_at_seconds >= self.policy.cooldown_seconds
        changed = abs(desired - previous) > 1e-9 and (bypass_cooldown or cooldown_elapsed)
        if changed:
            self.state.budget_mibps = desired
            self.state.last_change_at_seconds = feedback.at_seconds
        elif abs(desired - previous) > 1e-9 and not cooldown_elapsed:
            reason = "cooldown_hold"
        return Decision(feedback.at_seconds, previous, self.state.budget_mibps, changed, reason)

    def observe(self, feedback: Feedback) -> Decision:
        wait = feedback.wait_milliseconds
        if feedback.stale or wait is None:
            self.state.consecutive_healthy = 0
            self.state.consecutive_breaches = 0
            return self._decision(feedback, self.state.budget_mibps, "stale_hold")

        if feedback.emergency or wait >= self.policy.emergency_wait_milliseconds:
            self.state.consecutive_healthy = 0
            self.state.consecutive_breaches = 0
            return self._decision(
                feedback,
                self.policy.minimum_budget_mibps,
                "emergency",
                bypass_cooldown=True,
            )

        if wait > self.policy.target_wait_milliseconds:
            self.state.consecutive_breaches += 1
            self.state.consecutive_healthy = 0
            if self.state.consecutive_breaches >= self.policy.breach_windows:
                self.state.consecutive_breaches = 0
                return self._decision(
                    feedback,
                    self.state.budget_mibps * self.policy.multiplicative_decrease,
                    "multiplicative_decrease",
                )
            return self._decision(feedback, self.state.budget_mibps, "breach_pending")

        if wait < self.policy.healthy_wait_milliseconds:
            self.state.consecutive_healthy += 1
            self.state.consecutive_breaches = 0
            if self.state.consecutive_healthy >= self.policy.healthy_windows:
                self.state.consecutive_healthy = 0
                return self._decision(
                    feedback,
                    self.state.budget_mibps + self.policy.additive_increase_mibps,
                    "additive_increase",
                )
            return self._decision(feedback, self.state.budget_mibps, "healthy_pending")

        self.state.consecutive_healthy = 0
        self.state.consecutive_breaches = 0
        return self._decision(feedback, self.state.budget_mibps, "hysteresis_hold")


class FixedController:
    def __init__(self, budget_mibps: float):
        if budget_mibps <= 0:
            raise ValueError("fixed budget must be positive")
        self.state = ControllerState(budget_mibps)

    def observe(self, feedback: Feedback) -> Decision:
        return Decision(
            feedback.at_seconds,
            self.state.budget_mibps,
            self.state.budget_mibps,
            False,
            "fixed",
        )


class StepController:
    """A deliberately simple threshold-table baseline."""

    def __init__(self, policy: PoolPolicy, pressure_budget_mibps: float):
        if not policy.minimum_budget_mibps <= pressure_budget_mibps <= policy.maximum_budget_mibps:
            raise ValueError("pressure budget must be within pool bounds")
        self.policy = policy
        self.pressure_budget_mibps = pressure_budget_mibps
        self.state = ControllerState(policy.initial_budget_mibps)

    def observe(self, feedback: Feedback) -> Decision:
        previous = self.state.budget_mibps
        wait = feedback.wait_milliseconds
        if feedback.stale or wait is None:
            desired, reason = previous, "stale_hold"
        elif feedback.emergency or wait >= self.policy.emergency_wait_milliseconds:
            desired, reason = self.policy.minimum_budget_mibps, "emergency"
        elif wait > self.policy.target_wait_milliseconds:
            desired, reason = self.pressure_budget_mibps, "pressure_step"
        elif wait < self.policy.healthy_wait_milliseconds:
            desired, reason = self.policy.maximum_budget_mibps, "healthy_step"
        else:
            desired, reason = previous, "hysteresis_hold"
        bypass = reason == "emergency"
        cooldown_elapsed = feedback.at_seconds - self.state.last_change_at_seconds >= self.policy.cooldown_seconds
        changed = abs(desired - previous) > 1e-9 and (bypass or cooldown_elapsed)
        if changed:
            self.state.budget_mibps = desired
            self.state.last_change_at_seconds = feedback.at_seconds
        elif abs(desired - previous) > 1e-9 and not cooldown_elapsed:
            reason = "cooldown_hold"
        return Decision(feedback.at_seconds, previous, self.state.budget_mibps, changed, reason)


def allocate_budget(budget_mibps: float, disks: Iterable[DiskPolicy]) -> AllocationResult:
    ordered = sorted(disks, key=lambda disk: disk.disk_key)
    if budget_mibps < 0:
        raise ValueError("budget must be non-negative")
    if not ordered:
        return AllocationResult({}, True, 0, budget_mibps)

    required = sum(disk.minimum_mibps for disk in ordered)
    allocations = {disk.disk_key: disk.minimum_mibps for disk in ordered}
    if required > budget_mibps + 1e-9:
        return AllocationResult(allocations, False, required, 0)

    remaining = budget_mibps - required
    eligible = {disk.disk_key: disk for disk in ordered if disk.maximum_mibps > disk.minimum_mibps}
    while remaining > 1e-9 and eligible:
        total_weight = sum(disk.weight for disk in eligible.values())
        distributed = 0.0
        saturated = []
        for key, disk in eligible.items():
            share = remaining * disk.weight / total_weight
            room = disk.maximum_mibps - allocations[key]
            grant = min(share, room)
            allocations[key] += grant
            distributed += grant
            if room - grant <= 1e-9:
                saturated.append(key)
        remaining -= distributed
        for key in saturated:
            eligible.pop(key, None)
        if distributed <= 1e-12:
            break

    rounded = {key: round(value, 6) for key, value in allocations.items()}
    return AllocationResult(rounded, True, required, round(max(0, remaining), 6))
