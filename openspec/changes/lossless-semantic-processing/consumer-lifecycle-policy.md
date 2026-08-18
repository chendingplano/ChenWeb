# Semantic Assertion Consumer Lifecycle Policy

This audit implements Phase 2 task 5.1 for ADR `2026081801` DR6/DR8.  It
enumerates every current production read of `kb.semantic_assertions` or an
assertion lifecycle status, assigns its policy, and identifies the work needed
before a lossless metric writer can be enabled.  Test-only SQL is excluded.

## Policies

| Consumer | Current location | Policy | Phase 2 disposition |
|---|---|---|---|
| Profile-rule evaluation | `api/kbhandler/ontology_review_assertion_loader.go`, `api/ontology/profiles/review_service.go`, `rule_required_assertion_pattern.go` | **accepted-only**. Profile rules evaluate endorsed truth and must not treat represented source claims as conforming. | Retain filter; make the policy explicit in API/type documentation and certify legacy plus non-accepted inputs remain excluded. |
| Review-scope API | `api/kbhandler/ontology_review_scopes_handler.go` | **accepted-only** indirectly, through the profile-rule assertion loader. | Retain; expose no represented assertion as a rule fact. |
| Applicability classifications | `api/doc-processing/applicability_classifications.go` | **accepted-only**. Classifications select governance/review applicability and therefore require an endorsed `core:instance_of` assertion. | Retain filter; certify represented classifications do not alter applicability. |
| Authoritative classification projection | `api/ontology/assertions/classification_projection.go` | **accepted-only**. This is the authoritative object classification projection, not an observed-profile projection. | Retain filter; a future observed profile must be a distinct, inclusive projection. |
| Keyword-concept alignment | `api/ontology/keywords/alignment.go` | **accepted-only**. An `aligns_to_term` relation changes governed term identity and remains a curator/automatic-governance decision. | Retain filter; represented metric assertions are out of this family and must not be treated as an alignment. |
| Assertion-store accepted watermark | `api/ontology/assertions/assertions_store.go` (`LastAcceptedForSubject`) | **accepted-only**. The API's name and its callers make it an explicit endorsed-truth query. | Retain filter; do not reuse it for discovery or diagnostics. |
| Assertion lifecycle on evidence loss/restoration | `api/ontology/assertions/evidence_store.go` | **all evidence-supported lifecycle states**: `represented`, `candidate`, `in_review`, `deferred`, and `accepted` transition to `unsupported` on loss of final qualifying evidence; restoration returns the recorded prior status. | Implemented as part of task 5.2; remaining reader work is tracked separately. |
| Directional comparison | `api/ontology/comparison/compare.go`, `evaluate_cell.go` | **capability-aware**. Every selected comparison pair produces a cell. Missing normalization or incompatible quantity/component capabilities returns `no_verdict` with a persisted rationale; comparable pairs retain their existing verdicts. | Implemented in task 5.4; later assertion readers must pass raw-preserved states into this path rather than dropping them. |
| Semantic completeness projection | `api/ontology/semantic/completeness.go` | **diagnostic / capability-aware**. It reports supporting-link coverage rather than endorsing an assertion. | Retain its state-neutral coverage behavior; add represented/raw-preserved cases to the reader compatibility suite. |
| Semantic processing association | `api/ontology/assertions/associate_semantics.go` | **legacy writer, not a consumer**. It still promotes successful ingestion to `accepted`. | Keep unchanged until Phase 3 task 6.6; it is deliberately outside the Phase 2 reader cutover. |

## Consumers not currently implemented

The Phase 2 scope also requires search, Review Document rendering,
generic semantic discovery, diagnostic projections, reports, and retry tooling
to be dual-read.  The production tree has no implementation that reads
`kb.semantic_assertions` for search or comparison; the only comparison matches
are handler tests.  Those consumers therefore require new reader paths and
compatibility certification rather than a filter change.  Review Document
currently receives assertion-derived facts through the accepted-only profile
path; its flagged-instance display model has not yet been implemented.

## Certification rule

The reader compatibility suite in task 5.7 must name each row above.  It must
prove that accepted-only consumers ignore represented/unsupported assertions,
while diagnostic/discovery consumers render them with lifecycle and independent
semantic-state warnings.  No writer gate may be enabled until every required
consumer has a passing certification record.
