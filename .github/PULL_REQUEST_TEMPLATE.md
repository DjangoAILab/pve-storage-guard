## Summary

<!-- What changes and why? -->

## Evidence

<!-- Tests, replay output, benchmarks, screenshots, or issue/ADR links. -->

## Safety and privacy

- [ ] Observer/shadow remains the default, or an ADR explains the change.
- [ ] Hard bounds, stale input, effective state, and rollback behavior are tested
      where applicable.
- [ ] Fixtures, logs, screenshots, and examples are sanitized.
- [ ] No modeled result is described as measured production performance.
- [ ] No new arbitrary-command or unbounded actuation surface is introduced.

## Verification

- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] `go test -race ./...`
- [ ] Python replay tests
- [ ] Documentation and checklist updated
- [ ] DCO sign-off present
