# Contributing

Thank you for helping make storage control safer. PVE Storage Guard accepts bug
reports, operational evidence, documentation, tests, adapters, and carefully
bounded features.

## Before opening a change

- Search existing issues and ADRs.
- For behavioral or architectural changes, open a design issue first.
- Never attach raw production logs, hostnames, addresses, VM IDs, guest data,
  credentials, storage serials, or customer/workload names.
- Do not present modeled replay output as measured production performance.
- Actuation changes require a threat-model and rollback discussion.

Storage-trace proposals must begin with the
[metadata-only qualification process](docs/TRACE-CONTRIBUTION.md). Never upload
or link raw production data in an Issue or pull request. A valid machine
assessment does not replace permission, provenance, independence, and privacy
review.

## Development

Requirements: Go 1.24+ and Python 3.10+.

```sh
gofmt -w api cmd internal
go test ./...
go vet ./...
go test -race ./...
python3 -m unittest discover -s poc -p 'test_*.py' -v
```

Regenerate the reference reports and review any diff:

```sh
python3 poc/simulate.py --format markdown
python3 poc/simulate.py --format json
```

Generated results are reviewed evidence. Do not update golden files merely to
make CI green; explain the policy or model change in the pull request.

## Pull requests

- Keep changes focused and add tests.
- Update `docs/CHECKLIST.md` and relevant ADR/docs when project behavior or
  evidence changes.
- Use opaque/synthetic identities in examples and fixtures.
- Preserve observer/shadow defaults.
- Document new permissions, network endpoints, labels, and cardinality.
- Confirm rollback and effective-state behavior for any actuator-related work.
- Sign off commits using `git commit -s` to certify the Developer Certificate
  of Origin in [DCO.md](DCO.md).

## Decision process

Maintainers use evidence-first review. Safety regressions block throughput
improvements. Changes to the controller boundary, default modes, privilege,
policy semantics, or public claims require an ADR.

By participating, you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
