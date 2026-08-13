# Metric Semantic Convergence Design

## Goal

Allow normally supported metrics to progress from extraction to accepted
semantic assertions automatically, and make every remaining deferral
actionable and observable.

## Scope

The repair covers the metric semantic-association path in the 2026081302 bug
report:

- canonicalize the extractor's accepted `value_range_type` variants before
  structured metric normalization;
- accept a metric assertion kind when its governed `mea:*` term is released,
  rather than maintaining a drifting Go allowlist;
- retry deferred candidates according to the dependency that changed;
- retry/rebuild subject links for older metric records before re-normalizing;
- retain `metric_definition_term_id` through the candidate into the accepted
  assertion's qualifiers;
- report deferred candidates by dependency reason in normal Phase-D telemetry.

This does not introduce a new asynchronous release-event service. The
existing bounded backlog drain remains the recovery mechanism, which is safe
to call repeatedly and does not require new deployment infrastructure.

## Design

### Canonical metric values

`MetricNormalizer` will map extraction synonyms to its four canonical
structured forms before resolving values: `min`/`minimum`/`min_threshold` to
`lower_bound`, `max`/`maximum` to `upper_bound`, the exact structured forms to
`exact`, and supported range synonyms to `range`. Unknown nonempty forms stay
honestly unparsed; they will not fall through to free-text parsing.

### Term-governed acceptance

`AssociateSemantics.processMetric` will reject only empty or explicitly
unparsed assertion kinds. For every other kind it will ask whether
`mea:measured_by` and `mea:<assertion_kind>` are released. Thus a newly
released `mea:exact_value` takes effect without a source change.

### Deferred recovery

The backlog drain will group deferred records by dependency class. For an
unresolved referent, it will first ensure metric subject objects are
reconciled, then re-normalize; a payload change creates the proper new
candidate revision. For governed-term and assertion-kind deferrals, it will
reopen the existing candidate only after a computed term-availability
fingerprint differs, then run association. Candidates that remain unsupported
or unresolved retain a precise current reason.

### Identity and telemetry

The normalizer will select `metric_definition_term_id` from `kb.metrics` and
put it in the candidate. Association will include it in assertion qualifiers,
making the resolved measurement definition queryable without a schema
migration. The phase-D report will additionally bucket deferred candidates by
their dependency fingerprint/reason and log that map per run.

## Testing

Tests will first demonstrate real extractor variants (`min`, `max`,
`minimum`, `maximum`, exact count/duration/ratio/specification and range)
failing normalization, then lock in their canonical outcomes. SQL-mock tests
will cover deferred term retry selection/fingerprints and associate runs that
include reopened candidates. Candidate payload/qualifier tests will cover
metric-definition identity propagation. Existing package tests plus the
affected document-processing package tests will be run after each change.

## Documentation impact

The bug report is the change driver. The metric semantic-processing manual
remains correct about the intended automatic behavior; it needs no semantic
rewrite. This design and its implementation plan document the new recovery
mechanics. No public API or database schema changes are planned.
