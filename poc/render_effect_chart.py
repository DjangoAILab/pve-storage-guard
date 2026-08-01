#!/usr/bin/env python3
"""Render the reviewed counterfactual PoC result as a deterministic SVG."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


SCENARIOS = ("conservative", "nominal", "optimistic")
SCENARIO_LABELS = {
    "conservative": "Conservative",
    "nominal": "Nominal",
    "optimistic": "Optimistic",
}
STRATEGIES = (
    ("aimd_poc_tuned", "Selected AIMD", "#22d3ee"),
    ("fixed_20", "Fixed 20", "#94a3b8"),
    ("step_5_10_40", "Threshold step", "#f59e0b"),
    ("no_limit", "No limit", "#ef4444"),
)


def load_results(path: Path) -> dict[str, dict[str, dict[str, Any]]]:
    document = json.loads(path.read_text(encoding="utf-8"))
    counterfactual = document.get("counterfactual")
    if not isinstance(counterfactual, dict):
        raise ValueError("report has no counterfactual results")

    result: dict[str, dict[str, dict[str, Any]]] = {}
    for scenario in SCENARIOS:
        scenario_document = counterfactual.get(scenario)
        rows = scenario_document.get("results") if isinstance(scenario_document, dict) else None
        if not isinstance(rows, list):
            raise ValueError(f"scenario {scenario} has no result rows")
        by_strategy = {
            row.get("strategy"): row
            for row in rows
            if isinstance(row, dict) and isinstance(row.get("strategy"), str)
        }
        missing = [strategy for strategy, _, _ in STRATEGIES if strategy not in by_strategy]
        if missing:
            raise ValueError(f"scenario {scenario} is missing strategies: {', '.join(missing)}")
        result[scenario] = by_strategy
    return result


def svg_text(x: float, y: float, value: str, *, css_class: str = "label", anchor: str = "start") -> str:
    return f'<text x="{x:g}" y="{y:g}" class="{css_class}" text-anchor="{anchor}">{value}</text>'


def build_svg(results: dict[str, dict[str, dict[str, Any]]]) -> str:
    width, height = 1200, 760
    plot_left, plot_width = 96, 1000
    scenario_centers = (250, 600, 950)
    lines = [
        '<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="760" viewBox="0 0 1200 760" role="img" aria-labelledby="title desc">',
        '<title id="title">Counterfactual policy comparison across three storage models</title>',
        '<desc id="desc">Modeled admission compares fixed 20 MiB per second with selected AIMD. Unsafe seconds compare selected AIMD, fixed 20, threshold step, and no limit. These are model outputs, not production measurements.</desc>',
        '<style>',
        '  .title{font:700 32px system-ui,sans-serif;fill:#f8fafc}.subtitle{font:600 15px ui-monospace,monospace;fill:#22d3ee;letter-spacing:1.5px}',
        '  .section{font:700 19px system-ui,sans-serif;fill:#e2e8f0}.label{font:500 14px system-ui,sans-serif;fill:#cbd5e1}.small{font:500 12px system-ui,sans-serif;fill:#94a3b8}',
        '  .value{font:700 14px ui-monospace,monospace;fill:#f8fafc}.axis{stroke:#334155;stroke-width:1}.grid{stroke:#1e293b;stroke-width:1}.panel{fill:#0f172a;stroke:#334155;stroke-width:1}',
        '</style>',
        '<rect width="1200" height="760" rx="22" fill="#020617"/>',
        '<rect x="32" y="28" width="1136" height="704" rx="16" class="panel"/>',
        svg_text(72, 82, "PVE Storage Guard — modeled policy comparison", css_class="title"),
        svg_text(72, 112, "COUNTERFACTUAL MODEL • NOT A PRODUCTION MEASUREMENT", css_class="subtitle"),
        svg_text(72, 158, "Admitted demand (%)", css_class="section"),
        svg_text(1096, 158, "higher is better after the safety gate", css_class="small", anchor="end"),
    ]

    top_y, top_height = 190, 190
    for tick in (0, 25, 50, 75, 100):
        y = top_y + top_height - tick / 100 * top_height
        lines.append(f'<line x1="{plot_left}" y1="{y:g}" x2="{plot_left + plot_width}" y2="{y:g}" class="grid"/>')
        lines.append(svg_text(plot_left - 14, y + 5, str(tick), css_class="small", anchor="end"))

    admission_strategies = (
        ("fixed_20", "Fixed 20", "#94a3b8", -58),
        ("aimd_poc_tuned", "Selected AIMD", "#22d3ee", 18),
    )
    for center, scenario in zip(scenario_centers, SCENARIOS):
        for strategy, _, color, offset in admission_strategies:
            value = float(results[scenario][strategy]["admission_percent"])
            bar_height = value / 100 * top_height
            x, y = center + offset, top_y + top_height - bar_height
            lines.append(f'<rect x="{x}" y="{y:g}" width="58" height="{bar_height:g}" rx="5" fill="{color}"/>')
            lines.append(svg_text(x + 29, y - 9, f"{value:.2f}%", css_class="value", anchor="middle"))
        lines.append(svg_text(center, top_y + top_height + 28, SCENARIO_LABELS[scenario], anchor="middle"))

    legend_y = 420
    for index, (_, label, color, _) in enumerate(admission_strategies):
        x = 392 + index * 230
        lines.append(f'<rect x="{x}" y="{legend_y - 13}" width="18" height="18" rx="4" fill="{color}"/>')
        lines.append(svg_text(x + 28, legend_y + 1, label))

    lines.extend([
        svg_text(72, 472, "Unsafe seconds above 25 ms", css_class="section"),
        svg_text(1096, 472, "lower is better • common scale: 0–311 s", css_class="small", anchor="end"),
    ])
    bottom_y, bottom_height, unsafe_max = 500, 145, 311
    for tick in (0, 100, 200, 311):
        y = bottom_y + bottom_height - tick / unsafe_max * bottom_height
        lines.append(f'<line x1="{plot_left}" y1="{y:g}" x2="{plot_left + plot_width}" y2="{y:g}" class="grid"/>')
        lines.append(svg_text(plot_left - 14, y + 5, str(tick), css_class="small", anchor="end"))

    offsets = (-76, -36, 4, 44)
    for center, scenario in zip(scenario_centers, SCENARIOS):
        for (strategy, _, color), offset in zip(STRATEGIES, offsets):
            value = int(results[scenario][strategy]["unsafe_seconds"])
            bar_height = value / unsafe_max * bottom_height
            visible_height = max(bar_height, 2 if value > 0 else 0)
            x, y = center + offset, bottom_y + bottom_height - visible_height
            lines.append(f'<rect x="{x}" y="{y:g}" width="30" height="{visible_height:g}" rx="3" fill="{color}"/>')
            lines.append(svg_text(x + 15, y - 7, str(value), css_class="value", anchor="middle"))
        lines.append(svg_text(center, bottom_y + bottom_height + 27, SCENARIO_LABELS[scenario], anchor="middle"))

    for index, (_, label, color) in enumerate(STRATEGIES):
        x = 192 + index * 230
        lines.append(f'<rect x="{x}" y="{690}" width="16" height="16" rx="3" fill="{color}"/>')
        lines.append(svg_text(x + 25, 703, label, css_class="small"))
    lines.append(svg_text(1128, 716, "Source: poc/results/report.json", css_class="small", anchor="end"))
    lines.append('</svg>')
    return "\n".join(lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--report", type=Path, default=Path("poc/results/report.json"))
    parser.add_argument("--output", type=Path, default=Path("docs/assets/policy-effect.svg"))
    parser.add_argument("--website-output", type=Path, default=Path("website/public/policy-effect.svg"))
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()

    rendered = build_svg(load_results(args.report))
    if args.check:
        for output in (args.output, args.website_output):
            if not output.is_file() or output.read_text(encoding="utf-8") != rendered:
                raise SystemExit(f"generated chart is stale: {output}")
        return 0
    for output in (args.output, args.website_output):
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(rendered, encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
