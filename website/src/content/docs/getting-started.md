---
title: Getting started
description: Reproduce the observer-only offline policy evaluation.
---

The current release surface is deliberately offline and read-only. It has no
SSH, PVE API, database, or actuator integration.

## Run the reference replay

From a repository clone:

```sh
python3 -m unittest discover -s poc -p 'test_*.py' -v
python3 poc/simulate.py --format markdown
python3 poc/simulate.py --format json
```

The Markdown and JSON snapshots under `poc/results/` must be byte-reproducible.
Observed shadow replay preserves the captured wait samples. Counterfactual
sections are labeled estimates.

## Test the Go safety kernel

```sh
go test ./...
go vet ./...
go test -race ./...
go run ./cmd/pve-storage-guard version
```

## Container

The local image runs as a non-root user:

```sh
docker build -t pve-storage-guard:local .
docker run --rm pve-storage-guard:local version
```

Public images will appear at `ghcr.io/djangoailab/pve-storage-guard` only after
a validated tag. Do not install the pre-release observer or enable actuation on
a production host from these instructions.

## Next

- Understand the [storage-domain boundary](/pve-storage-guard/concepts/architecture/).
- Review [policy safety](/pve-storage-guard/concepts/policy/).
- Read the [PoC evidence limits](/pve-storage-guard/evidence/poc/).
- Plan [ITOps integration](/pve-storage-guard/operations/itops/) before any canary.
