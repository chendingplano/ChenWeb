## Context

ADR `2026081701` defines the class and claim-identity foundations absent from the current ontology. `kb.ontology_terms` is presently both identity and mutable definition storage, metric assertions use occurrence-derived logical keys, and no persisted observed-class profile or redirect registry exists. ADR `2026081801` has added lossless state fields and reader compatibility work, but its metric writer and observed-profile slice remain blocked on these foundations.

The implementation must preserve source artifacts and historical assertion/evidence rows, use Goose migrations, keep writer gates off, and not change accepted-only governance reads until their compatibility suites pass.

## Goals / Non-Goals

**Goals:**

- Establish stable class identity independently of mutable, append-only contract revisions.
- Persist inclusive observed profiles and capability-validation evidence without promoting observations into governing contracts.
- Establish a versioned, concurrency-safe canonical claim registry and acyclic redirect histories.
- Add shadow-mode readers and verification so the lossless metric writer can later depend on these foundations safely.

**Non-Goals:**

- Enabling the lossless metric writer or replacing the legacy metric processor.
- Automatic contract activation, term merge/split execution, or LLM class adjudication.
- Backfilling/converging all legacy metrics in the same deployment.
- Changing accepted-only profile-rule or governance semantics.

## Decisions

### Stable headers, append-only revisions, and a compatibility view

`kb.ontology_terms` becomes a stable identity header keyed by `term_id`; definition and lifecycle history move to append-only revisions. `kb.ontology_terms_current` is the sole current-state read surface. This avoids changing foreign keys when a contract evolves and makes historical resolution reproducible. A direct base-table read is prohibited only after reader migration proof, not during the additive rollout.

### Observed profiles are evidence-bearing, not contracts

Observed attributes, values, units, cardinalities, examples, frequencies, contradictions, and outliers are stored in separate profile/observation tables keyed by stable class and source evidence. State-bearing observations—including represented, malformed, unparsed, missing, and nonconforming ones—remain visible. Contract synthesis consumes a reviewed/validated subset and creates a new contract revision; no profile write updates a contract.

### Claims use a versioned canonical-key registry

`kb.semantic_claim_identities` owns concurrency-safe find-or-create via a unique canonical key and has a stable `claim_id`. `semantic_assertions.logical_identity_key` stores that claim ID, rather than an occurrence-derived value. A canonical-key-version registry supports shadow computation and explicit atomic cutover. Evidence-only changes remain evidence changes; identity-bearing changes resolve to another claim.

### Redirects are append-only and validated

Term and assertion redirects use separate history tables with one active target per source, cycle rejection, and supersession. Reads resolve redirects through bounded traversal. Redirects are created in shadow mode only in this change; activation is separately governed.

### Metric supporting evidence is scoped

Before adding the metric-only current-support unique index, a report/backfill retires duplicates as history. The partial index covers only active `supports` links for metric occurrences, preserving generic non-metric and non-supporting evidence fan-out.

## Risks / Trade-offs

- [Base-term consumers missed during migration] → inventory all queries, migrate to the compatibility view, and gate reshaping on a read audit.
- [Large corpus profile growth] → aggregate attribute observations, cap retained examples, and measure storage/latency before activation.
- [Canonical-key collision or semantic merge] → run shadow keys beside legacy keys, require unique constraints, and make cutover explicit and reversible by supersession.
- [Redirect cycle or deep chain] → enforce cycle checks transactionally, single active target, and bounded read traversal.
- [Duplicate evidence cleanup changes provenance] → retain superseded rows and write an auditable cleanup report before the partial unique index.

## Migration Plan

1. Measure corpus state, direct base-term reads, duplicate metric support links, and projected profile/claim cardinality.
2. Add all new tables, views, indexes, and governed vocabulary additively; verify up/down migration behavior on a scratch database.
3. Dual-read current-term, profile, claim, and redirect surfaces in shadow mode without changing writers or governance defaults.
4. Backfill only audited, bounded batches; retain historical rows and publish exceptions.
5. Certify reader, collision, redirect, evidence-cardinality, and capacity suites.
6. Hand the verified foundations to `lossless-semantic-processing`; that change alone controls its writer-gate activation.

Rollback disables readers and writer gates while retaining additive historical tables. It never deletes source artifacts, evidence history, assertions, profiles, claims, or redirects.

## Open Questions

- Which current-term consumers can migrate in the first rollout versus requiring a compatibility adapter?
- What policy version and approval actor governs later autonomous provisional-class and contract activation?
- What initial caps apply to retained profile examples, redirect depth, same-class Review Document retrieval, and canonical-key shadow reports?
