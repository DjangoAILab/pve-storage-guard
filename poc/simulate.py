#!/usr/bin/env python3
"""Offline replay and counterfactual simulator for storage-control policies.

The simulator is deliberately dependency-free and has no production adapters.
Observed replay preserves the captured wait values exactly. Counterfactual results
are kept separate and are labelled with the pool-model assumptions that produced
them.
"""

import argparse
import json
from dataclasses import asdict, dataclass, replace
from datetime import datetime, timezone
from pathlib import Path
from statistics import quantiles
from typing import Dict, Iterable, List, Optional, Sequence, Tuple

from storage_control import AimdController, Feedback, FixedController, PoolPolicy, StepController


@dataclass(frozen=True)
class DemandSample:
    at_seconds: int
    timestamp: str
    controlled_demand_mibps: float
    background_mibps: float
    incident_override: bool = False


@dataclass(frozen=True)
class PoolModel:
    name: str
    base_wait_ms: float
    safe_capacity_mibps: float
    slope_ms_per_mibps: float

    def wait_milliseconds(self, total_write_mibps: float, disturbance_factor: float = 1.0) -> float:
        overload = max(0.0, total_write_mibps - self.safe_capacity_mibps)
        return self.base_wait_ms + overload * self.slope_ms_per_mibps * max(0.0, disturbance_factor)


@dataclass(frozen=True)
class StrategyResult:
    strategy: str
    unsafe_seconds: int
    severe_seconds: int
    recovery_seconds: int
    admitted_mib: float
    demanded_mib: float
    estimated_completion_seconds: float
    limit_changes: int
    limit_changes_per_hour: float
    decisions: List[Dict[str, object]]

    def to_dict(self) -> Dict[str, object]:
        value = asdict(self)
        value["admitted_mib"] = round(self.admitted_mib, 3)
        value["demanded_mib"] = round(self.demanded_mib, 3)
        value["estimated_completion_seconds"] = round(self.estimated_completion_seconds, 2)
        value["limit_changes_per_hour"] = round(self.limit_changes_per_hour, 2)
        value["admission_percent"] = round(100 * self.admitted_mib / self.demanded_mib, 2) if self.demanded_mib else 100.0
        return value


def _parse_timestamp(value: str) -> datetime:
    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)


def _format_timestamp(value: datetime) -> str:
    return value.astimezone(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def build_demand_trace(fixture: Dict[str, object]) -> List[DemandSample]:
    """Expand sparse counter observations into one-second demand samples.

    Each sampled rate is derived from a counter delta and therefore describes
    the interval immediately preceding its timestamp.
    """
    rows = fixture.get("samples", [])
    if len(rows) < 2:
        return []
    override = fixture.get("incidentOverride") or {}
    override_start = _parse_timestamp(override["start"]) if override else None
    override_end = override_start.timestamp() + float(override.get("durationSeconds", 0)) if override_start else None
    parsed_rows = [(_parse_timestamp(row[0]), row) for row in rows]
    origin = parsed_rows[0][0]
    trace: List[DemandSample] = []
    complete_seconds = max(0, int((parsed_rows[-1][0] - origin).total_seconds()))
    observation_index = 1
    for second in range(complete_seconds):
        at = origin.timestamp() + second
        while observation_index < len(parsed_rows) - 1 and at >= parsed_rows[observation_index][0].timestamp():
            observation_index += 1
        row = parsed_rows[observation_index][1]
        controlled = float(row[1])
        background = float(row[2])
        is_override = override_start is not None and override_start.timestamp() <= at < override_end
        trace.append(
            DemandSample(
                at_seconds=second,
                timestamp=_format_timestamp(datetime.fromtimestamp(at, timezone.utc)),
                controlled_demand_mibps=float(override["controlledDemandMiBps"]) if is_override else controlled,
                background_mibps=background,
                incident_override=is_override,
            )
        )
    return trace


def _p95(values: Sequence[float]) -> float:
    if not values:
        raise ValueError("p95 requires at least one value")
    if len(values) < 2:
        return values[0]
    return quantiles(values, n=100, method="inclusive")[94]


def _decision_dict(decision) -> Dict[str, object]:
    return {
        "atSeconds": decision.at_seconds,
        "previousBudgetMiBps": decision.previous_budget_mibps,
        "budgetMiBps": decision.budget_mibps,
        "reason": decision.reason,
    }


def replay_observed_wait(controller, waits: Sequence[float], control_interval_seconds: int = 10) -> Dict[str, object]:
    """Shadow-replay exact observed waits; never predicts or rewrites them."""
    decisions: List[Dict[str, object]] = []
    window: List[float] = []
    emergency_threshold = getattr(getattr(controller, "policy", None), "emergency_wait_milliseconds", 100)
    for second, wait in enumerate(waits):
        window.append(float(wait))
        if wait >= emergency_threshold:
            decision = controller.observe(Feedback(second, float(wait), emergency=True))
            if decision.changed:
                decisions.append(_decision_dict(decision))
            window = []
        elif (second + 1) % control_interval_seconds == 0:
            decision = controller.observe(Feedback(second, _p95(window)))
            if decision.changed:
                decisions.append(_decision_dict(decision))
            window = []
    return {
        "kind": "observed-shadow-replay",
        "observedWaitMilliseconds": list(waits),
        "decisions": decisions,
        "finalBudgetMiBps": controller.state.budget_mibps,
    }


def rank_results(results: Iterable[StrategyResult]) -> List[StrategyResult]:
    """Safety-first, deterministic lexicographic ranking."""
    return sorted(
        results,
        key=lambda item: (
            item.unsafe_seconds,
            item.severe_seconds,
            item.recovery_seconds,
            -item.admitted_mib,
            item.limit_changes,
            item.strategy,
        ),
    )


def _disturbance_factors(
    model: PoolModel,
    waits: Sequence[float],
    incident_total_demand_mibps: float,
) -> List[float]:
    overload_cost = max(0.0, incident_total_demand_mibps - model.safe_capacity_mibps) * model.slope_ms_per_mibps
    if overload_cost <= 0:
        return [1.0 for _ in waits]
    return [max(0.0, (float(wait) - model.base_wait_ms) / overload_cost) for wait in waits]


def _recovery_seconds(waits: Sequence[float], incident_start_index: Optional[int], safe_wait_ms: float = 25, stable_seconds: int = 30) -> int:
    if incident_start_index is None:
        return 0
    for start in range(incident_start_index, len(waits)):
        end = start + stable_seconds
        if end <= len(waits) and all(value <= safe_wait_ms for value in waits[start:end]):
            return start - incident_start_index
    return len(waits) - incident_start_index


def simulate_strategy(
    strategy: str,
    controller,
    trace: Sequence[DemandSample],
    model: PoolModel,
    incident_waits: Sequence[float],
    control_interval_seconds: int = 10,
) -> StrategyResult:
    if not trace:
        return StrategyResult(strategy, 0, 0, 0, 0, 0, 0, 0, 0, [])
    incident_indexes = [index for index, sample in enumerate(trace) if sample.incident_override]
    incident_start = incident_indexes[0] if incident_indexes else None
    incident_total = max(
        (sample.controlled_demand_mibps + sample.background_mibps for sample in trace if sample.incident_override),
        default=0,
    )
    factors = _disturbance_factors(model, incident_waits, incident_total)
    waits: List[float] = []
    window: List[float] = []
    decisions: List[Dict[str, object]] = []
    admitted = 0.0
    demanded = 0.0
    incident_offset = 0
    emergency_threshold = getattr(getattr(controller, "policy", None), "emergency_wait_milliseconds", 100)

    for sample in trace:
        budget = controller.state.budget_mibps
        actual_controlled = min(sample.controlled_demand_mibps, budget)
        admitted += actual_controlled
        demanded += sample.controlled_demand_mibps
        disturbance = 1.0
        if sample.incident_override and factors:
            disturbance = factors[min(incident_offset, len(factors) - 1)]
            incident_offset += 1
        wait = model.wait_milliseconds(actual_controlled + sample.background_mibps, disturbance)
        waits.append(wait)
        window.append(wait)

        decision = None
        if wait >= emergency_threshold:
            decision = controller.observe(Feedback(sample.at_seconds, wait, emergency=True))
            window = []
        elif (sample.at_seconds + 1) % control_interval_seconds == 0:
            decision = controller.observe(Feedback(sample.at_seconds, _p95(window)))
            window = []
        if decision is not None and decision.changed:
            decisions.append(_decision_dict(decision))

    duration_seconds = len(trace)
    estimated_completion_seconds = (
        duration_seconds * demanded / admitted if admitted > 0 else float("inf")
    )
    limit_changes = len(decisions)
    return StrategyResult(
        strategy=strategy,
        unsafe_seconds=sum(value > 25 for value in waits),
        severe_seconds=sum(value >= 100 for value in waits),
        recovery_seconds=_recovery_seconds(waits, incident_start),
        admitted_mib=admitted,
        demanded_mib=demanded,
        estimated_completion_seconds=estimated_completion_seconds,
        limit_changes=limit_changes,
        limit_changes_per_hour=(3600 * limit_changes / duration_seconds if duration_seconds else 0),
        decisions=decisions,
    )


def _strategy_factories(tuned_policy: Optional[PoolPolicy] = None):
    base = dict(
        minimum_budget_mibps=5,
        initial_budget_mibps=20,
        healthy_wait_milliseconds=15,
        target_wait_milliseconds=25,
        emergency_wait_milliseconds=100,
        healthy_windows=6,
        breach_windows=2,
        cooldown_seconds=30,
    )
    factories = {
        "no_limit": lambda: FixedController(1000),
        "fixed_20": lambda: FixedController(20),
        "step_5_10_40": lambda: StepController(PoolPolicy(maximum_budget_mibps=40, **base), 10),
        "aimd_conservative": lambda: AimdController(
            PoolPolicy(maximum_budget_mibps=30, additive_increase_mibps=1, multiplicative_decrease=0.5, **base)
        ),
        "aimd_balanced": lambda: AimdController(
            PoolPolicy(maximum_budget_mibps=40, additive_increase_mibps=2, multiplicative_decrease=0.7, **base)
        ),
        "aimd_responsive": lambda: AimdController(
            PoolPolicy(
                maximum_budget_mibps=40,
                additive_increase_mibps=1,
                multiplicative_decrease=0.5,
                **{**base, "breach_windows": 1},
            )
        ),
    }
    if tuned_policy is not None:
        factories["aimd_poc_tuned"] = lambda: AimdController(tuned_policy)
    return factories


def search_aimd_policy(
    trace: Sequence[DemandSample],
    models: Sequence[PoolModel],
    incident_waits: Sequence[float],
    maximum_unsafe_seconds: Sequence[int],
) -> Tuple[Optional[PoolPolicy], Dict[str, object]]:
    """Run a bounded, reproducible safety-constrained parameter search.

    This tuning is deliberately an offline POC facility. The selected parameters
    must not be promoted directly to production from one incident trace.
    """
    evaluated = 0
    eligible: List[Tuple[List[StrategyResult], PoolPolicy]] = []
    for initial in (15, 17.5, 20):
        for maximum in (25, 30, 40):
            if maximum < initial:
                continue
            for additive in (0.5, 1, 2):
                for decrease in (0.3, 0.5, 0.7):
                    for healthy_windows in (3, 6, 12):
                        for breach_windows in (1, 2):
                            for cooldown in (30, 60):
                                policy = PoolPolicy(
                                    minimum_budget_mibps=5,
                                    initial_budget_mibps=initial,
                                    maximum_budget_mibps=maximum,
                                    healthy_wait_milliseconds=15,
                                    target_wait_milliseconds=25,
                                    emergency_wait_milliseconds=100,
                                    additive_increase_mibps=additive,
                                    multiplicative_decrease=decrease,
                                    healthy_windows=healthy_windows,
                                    breach_windows=breach_windows,
                                    cooldown_seconds=cooldown,
                                )
                                evaluated += 1
                                results = [
                                    simulate_strategy(
                                        "search_candidate",
                                        AimdController(policy),
                                        trace,
                                        model,
                                        incident_waits,
                                    )
                                    for model in models
                                ]
                                if all(
                                    result.unsafe_seconds <= maximum
                                    for result, maximum in zip(results, maximum_unsafe_seconds)
                                ):
                                    eligible.append((results, policy))
    eligible.sort(
        key=lambda pair: (
            -sum(result.admitted_mib for result in pair[0]),
            sum(result.severe_seconds for result in pair[0]),
            sum(result.recovery_seconds for result in pair[0]),
            sum(result.limit_changes for result in pair[0]),
            pair[1].maximum_budget_mibps,
            abs(pair[1].multiplicative_decrease - 0.5),
            -pair[1].cooldown_seconds,
        )
    )
    selected = eligible[0] if eligible else None
    evidence = {
        "kind": "bounded-grid-search",
        "objective": "maximize aggregate admitted MiB subject to unsafe seconds <= fixed_20 in every model",
        "evaluatedPolicies": evaluated,
        "eligiblePolicies": len(eligible),
        "selectedPolicy": asdict(selected[1]) if selected else None,
        "selectedResults": {
            model.name: result.to_dict()
            for model, result in zip(models, selected[0])
        } if selected else None,
        "warning": "POC fit on one incident; validate on more incidents and in shadow mode before actuation.",
    }
    return (selected[1] if selected else None), evidence


def analyze_parameter_sensitivity(
    policy: PoolPolicy,
    trace: Sequence[DemandSample],
    models: Sequence[PoolModel],
    incident_waits: Sequence[float],
    maximum_unsafe_seconds: Sequence[int],
) -> Dict[str, object]:
    """Evaluate one-at-a-time neighbors around the selected shadow policy."""
    neighborhoods = {
        "maximum_budget_mibps": (20, 25, 30),
        "additive_increase_mibps": (0.25, 0.5, 1),
        "multiplicative_decrease": (0.3, 0.5, 0.7),
        "healthy_windows": (6, 12, 18),
        "breach_windows": (1, 2, 3),
        "cooldown_seconds": (30, 60, 90),
    }
    rows: List[Dict[str, object]] = []
    for parameter, values in neighborhoods.items():
        for value in values:
            candidate = replace(policy, **{parameter: value})
            results = [
                simulate_strategy(
                    "sensitivity_candidate",
                    AimdController(candidate),
                    trace,
                    model,
                    incident_waits,
                )
                for model in models
            ]
            admitted = sum(result.admitted_mib for result in results)
            demanded = sum(result.demanded_mib for result in results)
            rows.append({
                "parameter": parameter,
                "value": value,
                "selectedValue": value == getattr(policy, parameter),
                "passesSafetyGate": all(
                    result.unsafe_seconds <= maximum
                    for result, maximum in zip(results, maximum_unsafe_seconds)
                ),
                "unsafeSeconds": {
                    model.name: result.unsafe_seconds
                    for model, result in zip(models, results)
                },
                "aggregateAdmissionPercent": round(100 * admitted / demanded, 2),
                "totalLimitChanges": sum(result.limit_changes for result in results),
            })
    return {
        "kind": "one-at-a-time-neighborhood",
        "warning": "Sensitivity is model-assisted and holds all non-varied parameters constant.",
        "rows": rows,
    }


def run_poc(demand_fixture: Dict[str, object], wait_fixture: Dict[str, object]) -> Dict[str, object]:
    trace = build_demand_trace(demand_fixture)
    observed_waits = [float(value) for value in wait_fixture["samples"]]
    models = [
        PoolModel("conservative", 12, 15, 1.2),
        PoolModel("nominal", 10, 20, 0.9),
        PoolModel("optimistic", 8, 30, 0.6),
    ]
    fixed_results = [
        simulate_strategy("fixed_20", FixedController(20), trace, model, observed_waits)
        for model in models
    ]
    tuned_policy, tuning_evidence = search_aimd_policy(
        trace,
        models,
        observed_waits,
        [result.unsafe_seconds for result in fixed_results],
    )
    parameter_sensitivity = (
        analyze_parameter_sensitivity(
            tuned_policy,
            trace,
            models,
            observed_waits,
            [result.unsafe_seconds for result in fixed_results],
        )
        if tuned_policy is not None
        else None
    )
    scenarios: Dict[str, object] = {}
    scenario_objects: Dict[str, List[StrategyResult]] = {}
    for model in models:
        results = [
            simulate_strategy(name, factory(), trace, model, observed_waits)
            for name, factory in _strategy_factories(tuned_policy).items()
        ]
        ranked = rank_results(results)
        scenario_objects[model.name] = results
        scenarios[model.name] = {
            "model": asdict(model),
            "ranking": [item.strategy for item in ranked],
            "results": [item.to_dict() for item in ranked],
        }

    by_scenario = {
        name: {item.strategy: item for item in results}
        for name, results in scenario_objects.items()
    }
    adaptive_names = sorted(
        item.strategy
        for item in scenario_objects["conservative"]
        if item.strategy.startswith("aimd_")
    )
    eligible_names = [
        name for name in adaptive_names
        if all(
            by_scenario[scenario][name].unsafe_seconds <= by_scenario[scenario]["fixed_20"].unsafe_seconds
            for scenario in by_scenario
        )
    ]
    eligible_names.sort(
        key=lambda name: (
            -sum(by_scenario[scenario][name].admitted_mib for scenario in by_scenario),
            sum(by_scenario[scenario][name].severe_seconds for scenario in by_scenario),
            sum(by_scenario[scenario][name].recovery_seconds for scenario in by_scenario),
            sum(by_scenario[scenario][name].limit_changes for scenario in by_scenario),
            name,
        )
    )
    recommended = eligible_names[0] if eligible_names else None
    rejected = sorted(set(adaptive_names) - set(eligible_names))

    shadow_controller = AimdController(
        PoolPolicy(
            minimum_budget_mibps=5,
            initial_budget_mibps=20,
            maximum_budget_mibps=40,
            healthy_windows=6,
            breach_windows=2,
            cooldown_seconds=30,
        )
    )
    return {
        "schemaVersion": 1,
        "mode": "offline-read-only-poc",
        "trace": {
            "seconds": len(trace),
            "firstTimestamp": trace[0].timestamp if trace else None,
            "lastTimestamp": trace[-1].timestamp if trace else None,
            "incidentSeconds": sum(sample.incident_override for sample in trace),
        },
        "observedShadowReplay": replay_observed_wait(shadow_controller, observed_waits),
        "counterfactual": scenarios,
        "parameterSearch": tuning_evidence,
        "parameterSensitivity": parameter_sensitivity,
        "selectionGate": {
            "rule": "adaptive unsafe_seconds must be <= fixed_20 in every model",
            "fixed20UnsafeSeconds": {
                scenario: by_scenario[scenario]["fixed_20"].unsafe_seconds
                for scenario in by_scenario
            },
            "eligibleAdaptiveStrategies": eligible_names,
            "rejectedAdaptiveStrategies": rejected,
            "recommendedShadowCandidate": recommended,
        },
        "limitations": [
            "Observed replay says what the controller would decide; it does not alter captured wait values.",
            "Counterfactual wait values come from explicit monotonic models, not causal production measurements.",
            "One incident cannot establish a globally optimal policy; the recommendation is shadow-only.",
        ],
    }


def render_markdown(report: Dict[str, object]) -> str:
    lines = [
        "# Storage controller offline POC report",
        "",
        "This report is generated from read-only historical evidence. Counterfactual values are model-assisted estimates, not production measurements.",
        "",
        "## Selection",
        "",
        "- Gate: " + report["selectionGate"]["rule"],
        "- Recommended shadow candidate: `" + str(report["selectionGate"]["recommendedShadowCandidate"]) + "`",
        "- Eligible adaptive strategies: " + ", ".join(report["selectionGate"]["eligibleAdaptiveStrategies"]),
        "- Rejected adaptive strategies: " + (", ".join(report["selectionGate"]["rejectedAdaptiveStrategies"]) or "none"),
        "",
        "## Counterfactual comparison",
        "",
    ]
    for name, scenario in report["counterfactual"].items():
        lines.extend([
            "### " + name,
            "",
            "| Rank | Strategy | Unsafe s | Severe s | Recovery s | Admission | Est. completion | Changes/h |",
            "|---:|---|---:|---:|---:|---:|---:|---:|",
        ])
        for rank, result in enumerate(scenario["results"], 1):
            lines.append(
                "| {rank} | `{strategy}` | {unsafe_seconds} | {severe_seconds} | {recovery_seconds} | {admission_percent:.2f}% | {estimated_completion_seconds:.2f}s | {limit_changes_per_hour:.2f} |".format(
                    rank=rank, **result
                )
            )
        lines.append("")
    sensitivity = report.get("parameterSensitivity")
    if sensitivity:
        lines.extend([
            "## Parameter sensitivity",
            "",
            "One-at-a-time neighbors around the selected candidate; values remain model-assisted.",
            "",
            "| Parameter | Value | Selected | Safety gate | Aggregate admission | Total changes |",
            "|---|---:|:---:|:---:|---:|---:|",
        ])
        for row in sensitivity["rows"]:
            lines.append(
                "| `{parameter}` | {value} | {selected} | {safe} | {admission:.2f}% | {changes} |".format(
                    parameter=row["parameter"],
                    value=row["value"],
                    selected="yes" if row["selectedValue"] else "no",
                    safe="pass" if row["passesSafetyGate"] else "fail",
                    admission=row["aggregateAdmissionPercent"],
                    changes=row["totalLimitChanges"],
                )
            )
        lines.append("")
    lines.extend(["## Limitations", ""] + ["- " + value for value in report["limitations"]])
    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser()
    fixture_dir = Path(__file__).resolve().parent / "fixtures"
    parser.add_argument("--demand", type=Path, default=fixture_dir / "reference-demand-trace.json")
    parser.add_argument("--wait", type=Path, default=fixture_dir / "reference-write-wait-trace.json")
    parser.add_argument("--format", choices=("json", "markdown"), default="json")
    args = parser.parse_args()
    with args.demand.open(encoding="utf-8") as handle:
        demand_fixture = json.load(handle)
    with args.wait.open(encoding="utf-8") as handle:
        wait_fixture = json.load(handle)
    report = run_poc(demand_fixture, wait_fixture)
    if args.format == "markdown":
        print(render_markdown(report))
    else:
        print(json.dumps(report, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
