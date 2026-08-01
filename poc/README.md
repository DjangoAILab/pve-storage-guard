# Offline replay reference

This directory is a standard-library-only reference implementation for policy
evaluation. It cannot connect to SSH, PVE, ITOps, a database, or an actuator.

Run:

```sh
python3 -m unittest discover -s poc -p 'test_*.py' -v
python3 poc/simulate.py --format markdown
python3 poc/simulate.py --format json
```

`io_csv_to_replay_trace.py` converts caller-provided, authorized per-I/O CSV
data into v1alpha2 research traces. It performs no download, drops unspecified
input columns, and marks unavailable management evidence `unknown`; those traces
cannot pass the active-control promotion gate by themselves. See
`docs/EXTERNAL-TRACE-RESEARCH.md`.

`fixtures/` contains anonymized derived observations. `results/` contains
reviewed snapshots. Observed shadow replay retains captured wait values exactly;
counterfactual values are estimates from the named monotonic pool models.

The bounded search selects an adaptive candidate only when its unsafe-second
count is no worse than fixed 20 MiB/s in conservative, nominal, and optimistic
models. Fixed 20 is a model comparator, not a validated production fallback: a
separate aggregate field check rejected it, but cannot be replayed. Passing this
offline gate authorizes shadow validation only.
