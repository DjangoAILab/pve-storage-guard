# Community trace intake design

## Outcome

Create a public, low-risk path for contributors to propose independent storage
evidence without turning GitHub Issues into a production-data upload channel.
The path must preserve the distinction between a structurally valid trace, a
storage-research trace, and evidence eligible for active-control promotion.

## Options considered

1. Accept raw trace attachments in Issues or pull requests. This is convenient
   but irreversibly publishes identifiers, workload shapes, timestamps, source
   terms, or guest data when sanitization fails. Rejected.
2. Operate a private upload service. This could support access control, but it
   creates credential, retention, deletion, breach-response, and availability
   obligations that are disproportionate for v0.1. Rejected.
3. Accept qualification metadata only, then let the contributor perform local
   conversion and assessment. This adds no service, credential, runtime, or
   data-retention boundary and reuses the existing v1alpha2 contract. Selected.

## Boundary and flow

The GitHub Issue form accepts only non-sensitive metadata: intended lane,
source/permission category, declared metric semantics, coarse storage and
workload classes, sampling shape, management-plane coverage, and sanitization
attestations. It repeatedly prohibits raw logs, trace rows, attachments,
private URLs, host/resource identities, storage serials, absolute timestamps,
and customer data.

The contributor keeps source data under their control. After a maintainer
classifies the request, the contributor runs the existing converter and
assessor locally. Storage-only inputs remain in the research lane with
`managementPlaneStatus=unknown`. A promotion candidate must independently meet
the v1alpha2 machine gate, including compatible storage-domain write p95,
synchronized management observations, at least 600 seconds, and 95% structural,
valid-wait, and known-management coverage.

Machine success is necessary but never sufficient. A maintainer separately
reviews authority, source terms, provenance, independence, transformations,
and residual disclosure risk. Publishing a sanitized replay trace or derived
result requires a focused pull request and explicit review. Non-redistributable
inputs may inform private research, but cannot be described as publicly
reproducible evidence.

## Failure behavior and verification

Unclear permission, an attachment, sensitive coordinates, self-declared
semantics without source support, missing management observations, or a failed
machine assessment stops qualification. No maintainer should ask a contributor
to paste data publicly as a workaround. Submitted URLs are never fetched
automatically and do not grant retrieval permission. An accidental sensitive
submission is treated as disclosure: maintainers avoid quoting it, remove it
where possible, direct the reporter to the private security channel, and never
promise deletion from platform history or caches.

Verification covers GitHub Issue-form YAML parsing, the repository publication
privacy gate, Markdown links, the existing trace-contract unit suite, and the
rendered documentation site. This change does not add a downloader, uploader,
network endpoint, dependency, production adapter, or promotion-eligible trace,
and it does not close either independent-evidence checklist item.
