---
title: Getting started
description: Reproduce the observer-only offline policy evaluation.
---

The current release surface is deliberately read-only. It has no PVE actuator
integration.

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

## Exercise the shadow stream

The command reads newline-delimited observations from stdin and writes
proposals to stdout. It does not load PVE credentials, and every proposal has
`actuationAllowed: false`.

```sh
OBSERVED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
printf '{"schemaVersion":"guard.storage-slo.io/v1alpha1","id":"quickstart-1","observedAt":"%s","domainKey":"reference-bulk-pool","writeWaitP95Milliseconds":42,"waitValid":true,"emergency":false,"managementPlaneHealthy":true}\n' "$OBSERVED_AT" \
  | go run ./cmd/pve-storage-guard shadow \
      --policy configs/examples/reference-shadow-policy.json \
      --enrollment configs/examples/reference-enrollment.json
```

Only explicitly enrolled, non-critical resources can appear in an allocation.
Unknown fields, unsupported schemas, and mismatched storage domains are
rejected; stale observations cannot increase a budget.

## Optional private decision journal

Add `--journal FILE` to the shadow command to append a versioned JSONL decision
event before each proposal is written to stdout. New files are `0600`; symlinks,
non-regular targets, and existing files accessible by group or other are
rejected. Every event is synced, and a journal error suppresses the matching
proposal. A non-blocking exclusive lock enforces one writer. The command stops
before the journal reaches 256 MiB and remains fail-closed until the operator
rotates the file.

The journal is disabled by default and has no built-in rotation, network export, ITOps
callback, or actuator. It contains absolute event times and operator-provided
opaque keys, so do not publish it as a replay trace without the separate
sanitization and export review.

Once the writer is stopped or detached and the file is rotated by an approved
operator procedure, verify the sealed artifact:

```sh
go run ./cmd/pve-storage-guard journal verify \
  --journal /path/to/sealed-decisions.jsonl
```

The verifier takes a shared non-blocking lock, rejects an active writer and
unsafe or malformed content, and outputs a versioned identity-free summary with
the exact raw-file `sha256:` digest. After that digest and summary have been
reviewed and approved, an authorized local consumer can request a bounded page:

```sh
go run ./cmd/pve-storage-guard journal batch \
  --journal /path/to/sealed-decisions.jsonl \
  --expected-digest sha256:APPROVED_DIGEST \
  --offset 0 --limit 64
```

The command revalidates the full file before emitting anything. Batch stdout is
private; pipe it only to the approved consumer and never to general logs. These
commands do not rotate, sanitize, sign, publish, import, or deliver the journal.

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
