## Why

Metric definition terms currently provide vocabulary identity but not durable ontology-class
semantics. This leaves no governed place to retain the full observed structure of instances,
distinguish evolving class contracts from stable class identity, or converge claims safely. ADR
`2026081701` defines those foundations and is the prerequisite for the remaining lossless semantic
reader and writer rollout.

## What Changes

- Add a stable ontology-class identity layer with a current-term compatibility view and append-only
  term and class-contract revisions.
- Add observed class profiles and per-capability validation records that retain represented,
  malformed, and outlier observations without granting them contract authority.
- Add typed instance-to-class references, canonical claim identities, canonical-key versioning, and
  append-only term/assertion redirects in shadow mode.
- Add explicit class-resolution decisions and governed identity/definition/capability vocabulary.
- Provide an auditable metric supporting-evidence cleanup and scoped cardinality constraint, while
  preserving non-metric evidence fan-out.
- Migrate reader paths to the current-term surface before any base-term reshaping or lossless writer
  activation.

## Capabilities

### New Capabilities

- `ontology-class-identity`: Stable ontology class identities, append-only term history, and the
  current-term compatibility surface.
- `ontology-class-contracts`: Append-only class contracts and independently validated capabilities.
- `observed-class-profiles`: Inclusive class observations, evidence distribution, contradictions,
  and outliers that never automatically alter an authoritative contract.
- `canonical-semantic-claims`: Versioned canonical claim keys, concurrency-safe claim identities,
  typed instance-to-class references, and assertion redirects.
- `semantic-identity-redirects`: Acyclic, single-active-target term and assertion redirect history.
- `metric-supporting-evidence-cardinality`: Auditable duplicate cleanup and a metric-only current
  supporting-evidence uniqueness rule.
- `class-resolution-decisions`: Append-only decision records for resolved, provisional, ambiguous,
  and rejected class identity choices.

### Modified Capabilities

- `semantic-assertion-lifecycle`: Assertions gain class identity references and must preserve the
  distinction between represented source admission and accepted governance.
- `lossless-semantic-processing`: The dependent metric pipeline consumes the class, claim, and
  redirect foundations once this change completes its shadow-mode certification.

## Impact

- Affects `kb.ontology_terms`, `kb.semantic_assertions`, `kb.assertion_evidence`, ontology
  migrations and stores, Phase D metric processing, semantic projections, search, comparison, and
  Review Document retrieval.
- Depends on ADR `2026081701` as the normative design and ADR `2026081801` for lossless lifecycle
  and state semantics.
- Does not enable `LOSSLESS_SEMANTIC_WRITES_METRIC`; writer cutover remains owned by the dependent
  lossless-semantic-processing change.
