# Publication privacy gate design

## Problem

Secret scanners do not treat an internal hostname or private network coordinate
as a credential. That leaves a gap between a clean secret scan and the project's
public sanitization contract. The current public tree has been corrected, but a
future documentation edit must fail before an unreviewed URL host or private IP
can merge.

## Decision

Add a dependency-free scanner for author-maintained publication surfaces. It
examines tracked root documentation, `docs`, Pages content and static SVGs,
public JSON schemas/examples, and GitHub templates/workflows. Dependency
lockfiles and generated build output are outside this gate.

Absolute HTTP(S) URL hosts must match a small exact allowlist of reviewed public
documentation, standards, community, and project hosts. Suffix matching is not
allowed. The scanner also rejects URL user information, localhost-style names,
RFC1918 IPv4 addresses, and loopback/link-local/private URL literals.

Failure output must not repeat the rejected hostname or address. It reports only
the tracked public path, line number, stable category, and a short SHA-256
fingerprint where correlation is useful. A passing run emits only aggregate
file and URL counts.

## CI boundary

The scanner and its unit tests run inside the existing required `secrets` job
before Gitleaks. This keeps credential detection and publication-identity
validation separate while making both mandatory for pull requests and `main`.
The allowlist is repository-reviewed code; adding a new public source requires
an explicit diff rather than a broad wildcard.

## Tests

Tests cover an accepted public URL, exact-host enforcement, private IPv4 prose,
private and loopback URL literals, URL user information, redacted failure
output, tracked-surface selection, and an all-valid aggregate result. The real
repository scan and the existing secret scan both must pass before publication.

## Non-goals

- Rewriting public Git history or release tags.
- Claiming that an allowlisted host makes linked content trustworthy.
- Scanning third-party dependency metadata as authored project content.
- Replacing Gitleaks, human redaction review, or release provenance.
