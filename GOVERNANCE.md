# Governance

The DjangoAILab maintainers steward PVE Storage Guard using transparent,
evidence-first review.

## Maintainer responsibilities

- protect observer/shadow defaults and the least-privilege boundary;
- require reproducible evidence for performance or prevention claims;
- review dependencies, releases, SBOMs, and security reports;
- keep project decisions in ADRs and execution evidence in the checklist;
- avoid conflicts of interest and enforce the Code of Conduct.

## Decisions

Routine changes use pull-request consensus. The following require an ADR and at
least one approving maintainer who did not author the change: controller or
privilege boundaries, default operating mode, actuation semantics, safety
invariants, public claims, license, governance, and stable API compatibility.

When evidence is inconclusive, the safer state wins: retain shadow/fixed
fallback, gather more data, and revisit the decision.
