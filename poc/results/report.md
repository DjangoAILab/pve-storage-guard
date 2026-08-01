# Storage controller offline POC report

This report is generated from read-only historical evidence. Counterfactual values are model-assisted estimates, not production measurements.

## Selection

- Gate: adaptive unsafe_seconds must be <= fixed_20 in every model
- Recommended shadow candidate: `aimd_poc_tuned`
- Eligible adaptive strategies: aimd_poc_tuned
- Rejected adaptive strategies: aimd_balanced, aimd_conservative, aimd_responsive

## Counterfactual comparison

### conservative

| Rank | Strategy | Unsafe s | Severe s | Recovery s | Admission | Est. completion | Changes/h |
|---:|---|---:|---:|---:|---:|---:|---:|
| 1 | `aimd_conservative` | 1 | 0 | 9 | 63.96% | 2345.11s | 19.20 |
| 2 | `aimd_poc_tuned` | 1 | 0 | 9 | 60.11% | 2495.51s | 7.20 |
| 3 | `fixed_20` | 1 | 0 | 9 | 59.26% | 2531.02s | 0.00 |
| 4 | `aimd_responsive` | 1 | 0 | 9 | 55.35% | 2710.27s | 33.60 |
| 5 | `aimd_balanced` | 20 | 0 | 9 | 59.51% | 2520.76s | 24.00 |
| 6 | `step_5_10_40` | 158 | 0 | 11 | 63.08% | 2378.10s | 40.80 |
| 7 | `no_limit` | 311 | 64 | 11 | 100.00% | 1500.00s | 0.00 |

### nominal

| Rank | Strategy | Unsafe s | Severe s | Recovery s | Admission | Est. completion | Changes/h |
|---:|---|---:|---:|---:|---:|---:|---:|
| 1 | `aimd_poc_tuned` | 0 | 0 | 0 | 63.73% | 2353.85s | 24.00 |
| 2 | `fixed_20` | 0 | 0 | 0 | 59.26% | 2531.02s | 0.00 |
| 3 | `aimd_conservative` | 2 | 0 | 10 | 69.90% | 2145.80s | 24.00 |
| 4 | `aimd_responsive` | 2 | 0 | 10 | 61.14% | 2453.28s | 55.20 |
| 5 | `aimd_balanced` | 20 | 0 | 9 | 70.85% | 2117.23s | 40.80 |
| 6 | `step_5_10_40` | 122 | 0 | 2 | 64.79% | 2315.04s | 26.40 |
| 7 | `no_limit` | 251 | 64 | 11 | 100.00% | 1500.00s | 0.00 |

### optimistic

| Rank | Strategy | Unsafe s | Severe s | Recovery s | Admission | Est. completion | Changes/h |
|---:|---|---:|---:|---:|---:|---:|---:|
| 1 | `aimd_responsive` | 0 | 0 | 0 | 72.94% | 2056.53s | 48.00 |
| 2 | `aimd_conservative` | 0 | 0 | 0 | 69.90% | 2145.80s | 24.00 |
| 3 | `aimd_poc_tuned` | 0 | 0 | 0 | 63.88% | 2348.22s | 24.00 |
| 4 | `fixed_20` | 0 | 0 | 0 | 59.26% | 2531.02s | 0.00 |
| 5 | `aimd_balanced` | 2 | 0 | 10 | 77.29% | 1940.62s | 24.00 |
| 6 | `step_5_10_40` | 2 | 0 | 10 | 75.97% | 1974.51s | 7.20 |
| 7 | `no_limit` | 71 | 4 | 11 | 100.00% | 1500.00s | 0.00 |

## Parameter sensitivity

One-at-a-time neighbors around the selected candidate; values remain model-assisted.

| Parameter | Value | Selected | Safety gate | Aggregate admission | Total changes |
|---|---:|:---:|:---:|---:|---:|
| `maximum_budget_mibps` | 20 | no | pass | 59.26% | 0 |
| `maximum_budget_mibps` | 25 | yes | pass | 62.57% | 23 |
| `maximum_budget_mibps` | 30 | no | pass | 62.57% | 27 |
| `additive_increase_mibps` | 0.25 | no | pass | 60.92% | 27 |
| `additive_increase_mibps` | 0.5 | yes | pass | 62.57% | 23 |
| `additive_increase_mibps` | 1 | no | fail | 64.07% | 13 |
| `multiplicative_decrease` | 0.3 | no | pass | 62.57% | 23 |
| `multiplicative_decrease` | 0.5 | yes | pass | 62.57% | 23 |
| `multiplicative_decrease` | 0.7 | no | pass | 62.57% | 23 |
| `healthy_windows` | 6 | no | fail | 64.29% | 28 |
| `healthy_windows` | 12 | yes | pass | 62.57% | 23 |
| `healthy_windows` | 18 | no | pass | 61.41% | 18 |
| `breach_windows` | 1 | no | pass | 59.32% | 28 |
| `breach_windows` | 2 | yes | pass | 62.57% | 23 |
| `breach_windows` | 3 | no | pass | 62.57% | 23 |
| `cooldown_seconds` | 30 | no | pass | 62.57% | 23 |
| `cooldown_seconds` | 60 | yes | pass | 62.57% | 23 |
| `cooldown_seconds` | 90 | no | pass | 62.57% | 23 |

## Limitations

- Observed replay says what the controller would decide; it does not alter captured wait values.
- Counterfactual wait values come from explicit monotonic models, not causal production measurements.
- One incident cannot establish a globally optimal policy; the recommendation is shadow-only.
