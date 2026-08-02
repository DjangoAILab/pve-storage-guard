# Production read-only compatibility evidence design

## Decision

Add a separate production-host compatibility validator instead of weakening or
renaming the existing non-production promotion gate. The new validator reuses
the same fixed-argument product commands and strict output validators, but it
produces a different document kind and schema:
`PVEHostObserverCompatibility`. It is explicitly ineligible for policy
promotion and records whether execution was non-root rather than treating root
execution as a successful hardening result.
Root execution is refused unless the operator supplies the explicit
`--allow-root` acknowledgement; even then it remains a recorded limitation.

The selected design has three layers. First, an operator supplies a reviewed,
digest-pinned compiled binary and an owner-only private PVE binding. Second, the
validator runs only `version`, `agent inventory`, `agent observe`, and a bounded
two-record `agent watch`; it re-hashes the binary before each launch, validates
all JSON in memory, rejects configured private identifiers in output, sends
SIGTERM, and requires a clean exit. Third, it emits only an identity-free
summary with zero requested mutations, an explicit production compatibility
scope, and fixed limitations covering service isolation, controlled load, and
actuation. Raw agent output is never persisted by the validator.

For the reference production dry-run, the release binary, validator, and
private config may exist only in a random owner-only directory on `/dev/shm`
and must be removed by an EXIT trap. The private config conservatively marks
every observed leaf device root and critical. No service, account, ACL, package,
PVE configuration, cgroup, or I/O limit may be created or changed. This proves
compiled read compatibility and cancellation only; it does not prove non-root
permissions, systemd hardening, workload behavior under controlled pressure,
or safe actuation.

## Alternatives rejected

- Reusing the non-production validator would mislabel production evidence.
- A source-format Python probe cannot prove the compiled product path.
- Installing a service or persistent binary crosses the separate production
  deployment checkpoint.

## Safety and acceptance

The compatibility result is accepted only when the binary digest matches,
inventory/observation/watch schemas validate, exactly two watch samples are
seen, SIGTERM exits zero, no private identity leaks, and requested mutations
remain zero. It must always report `promotionEligible: false`.
