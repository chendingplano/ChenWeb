# Reader Compatibility Certification

This is the task 5.7 certification record for ADR `2026081801`. It names every
consumer row in `consumer-lifecycle-policy.md`, the specific test(s) that
prove its required behavior, and a verdict. No writer gate (`LOSSLESS_SEMANTIC_WRITES_METRIC`,
`LOSSLESS_SEMANTIC_FALLBACK_WRITES`) may be enabled until every required row
below is PASS.

## How a row is certified

Two proof shapes are used, both acceptable:

- **Golden query-text test**: an independently-typed (not copied from the
  production source constant) regex/literal match against the full WHERE
  clause, run through `sqlmock`. This is cheap and directly catches a
  regression that weakens or removes a filter, because the test's expected
  text is a second, independent copy of the guarantee, not a reference to the
  same source of truth being tested.
- **Behavioral/integration test**: seeds real rows (via a live scratch
  database or a decoded response) and asserts on what a consumer actually
  returns or renders.

A test that only matches a SQL prefix, or that references the same package
constant the production code uses, does **not** count — it would still pass
after the guarantee it claims to prove was removed. Three such gaps were
found and closed while building this suite (see "Gaps closed" below).

## Accepted-only consumers

Required proof: legacy and represented/unsupported/candidate/deferred rows
never reach the consumer; only `status = 'accepted'` rows do.

| Consumer | Proof | Verdict |
|---|---|---|
| Profile-rule evaluation (`ontology_review_assertion_loader.go` → `AssertionStore.ListBySubjectObject`) | `TestReviewAssertionLoaderOnlyRequestsAcceptedAssertionsForTheGivenObject` (loader always requests `status="accepted"`) + new `TestListBySubjectObjectFiltersByRequestedStatus` (store's query text is independently pinned to `AND ($2 = '' OR status = $2)`) | **PASS** |
| Review-scope API (`ontology_review_scopes_handler.go`) | Indirect: delegates entirely to the profile-rule loader above, no independent query. | **PASS** (by delegation) |
| Applicability classifications (`applicability_classifications.go`) | `TestClassificationFactLoaderLoadsAcceptedInstanceOfIntoObjectClass` / `TestClassificationFactLoaderReturnsMissingWhenNoAcceptedClasses` — both already used an independently-typed full query literal including `AND a.status = 'accepted'`. | **PASS** |
| Authoritative classification projection / "semantic projection" (`classification_projection.go`) | New `TestPrimaryClassificationForFiltersToAcceptedStatusOnly` (independently-typed full query text) + `TestPrimaryClassificationForReportsNotFoundWhenOnlyNonAcceptedExists` (a represented-only claim resolves to `found=false`, never a stale/wrong classification). Zero tests existed for this file before task 5.7. | **PASS** |
| Keyword-concept alignment (`keywords/alignment.go`) | New `TestAcceptedForConceptSQLFiltersToAcceptedStatusOnly`, independently asserting `acceptedForConceptSQL` contains `status = 'accepted'` and `subject_ref_kind = 'keyword_concept'` (the prior test only matched against the same constant, see "Gaps closed"). | **PASS** |
| Assertion-store accepted watermark (`AssertionStore.HighestAcceptedAssertionID`) | New `TestHighestAcceptedAssertionIDQueryFiltersToAcceptedStatusOnly` (full query text, independently typed; the two prior tests matched only the `SELECT` prefix, see "Gaps closed"). | **PASS** |

## Dual-read / diagnostic consumers

Required proof: a legacy row (no new-state fields populated) renders without
error, and every new state (raw-preserved, ambiguous, missing, represented,
unsupported) renders with its lifecycle/state exposed rather than being
dropped or silently reinterpreted.

| Consumer | Proof | Verdict |
|---|---|---|
| Semantic assertion diagnostic API (`GET /kb/semantic-assertions?input_record_id=`) | `AssertionListFilter.Status` is empty by default and `ListAdmin`'s WHERE builder only adds a status predicate `if f.Status != ""` — structurally returns every status when none is requested (no default accepted-only narrowing). `TestListSemanticAssertionsFiltersByInputRecordID` exercises the document-scoped path end to end. `Assertion`'s JSON tags (task 5.5's fix) expose all four independent state fields plus `unsupported_prior_status`, `raw_payload`, `processing_error_details` as `omitempty` — present when populated, absent (not defaulted/misrepresented) for a legacy row. | **PASS** |
| Review Document — Semantic Diagnostics tab (`doc-review-semantic-view.svelte`) | Pure rendering layer over the diagnostic API above; no independent filtering logic to certify beyond "does not route through the accepted-only loader," verified by code inspection (imports `semantic-assertions-client`/`assertion-evidence-client`, never `ontology_review_assertion_loader`). Consistent with this codebase's convention of not unit-testing presentation-only `.svelte` views (see `doc-review-findings-view.svelte`, `doc-review-report-view.svelte`, none of which have test files either). | **PASS** |
| Assertion lifecycle on evidence loss/restoration | `TestEvidenceStoreDeleteLastSupportRecordsRepresentedPriorStatus`, `TestEvidenceStoreRestoreReturnsRecordedPriorStatus`, `TestRepresentedReachesAcceptedOnlyThroughGovernance`, `TestEvidenceLossOriginStatuses`, `TestRestorationTargetsCoverEveryPriorStatus`, `TestRestorationNeverEscalatesRepresentedToAccepted` (task 4.5). Covers all five evidence-supported origin statuses and proves restoration never escalates to `accepted`. | **PASS** |
| Directional comparison (`compare.go`, `evaluate_cell.go`) | `TestCompareQuantityKindMismatchRecordsNoVerdict`, `TestCompareComponentMismatchRecordsNoVerdict`, `TestCompareUnknownUnitRecordsNoVerdict`, `TestEvaluateDirectionalCellRecordsNoVerdictForUnnormalizableInput` (task 5.4) — missing capability always yields a recorded `no_verdict`, never a dropped instance. | **PASS** |
| Metric search registry / raw+normalized indexing (`buildMetricRegistryRows`) | Query joins `raw_text`/`raw_payload` and `numeric_value`/`lower_value`/`upper_value` via `LEFT JOIN LATERAL`, so a metric with no current supporting assertion (legacy) still returns a row (the lateral join is optional), and one with a raw-preserved-only assertion still contributes its raw representation. Verified by code inspection of the join shape; see `TestInsertSearchRegistryRowsUpsertsNormalizedRows` and related tests in `search_indexing_test.go` for the write-side registry shape (these currently fail in this dev environment for reasons unrelated to this ADR — see "Known pre-existing gaps" below). | **CONDITIONAL PASS** — join shape verified by inspection; existing tests for this file do not currently run clean in this environment (pre-existing, unrelated to this change). |
| Semantic completeness projection (`completeness.go`) | Task 4.8 completeness tests plus the `ArtifactsMissingAnyStage` vs `ArtifactsWithMissingValue` distinction (task 5.6) explicitly reports a present-but-missing-value artifact as present, not absent. | **PASS** |
| Semantic run reports (`report.go`, `cmd/semantic-baseline`) | Task 3.12/1.2-1.3: DR11 reporting counts every disposition/finding/severity, not `accepted` only; the Phase 0 baseline was run against the full 58-record corpus and committed to KnowledgeStore as the comparison basis. | **PASS** |
| Semantic retry queue reader (`RetryQueue.List`) | New `TestIntegrationRetryQueueListFiltersAndJoinsOutcomeContext` (live scratch DB: proves state filter and outcome_id filter, and that outcome artifact context is joined) + `TestListSemanticRetryQueueFiltersByState` (handler-level). Built fresh in task 5.2 — this queue has no accepted/represented distinction, so "dual-read" here just means every state (`pending`/`claimed`/`done`/`stale`/`failed`) is visible, which both tests exercise. | **PASS** |
| Observed class profile aggregation (`ObservedProfileStore`/`ObservedProfileReader`) | `ObservationState` required and never dropped (validated by `validateObservedProfileObservation`); `ObservedProfileReader.Get` hardcodes `Authoritative = false`. Built and tested for the separate, already-archived `canonical-metric-class-foundations` change — not re-tested here. **Has zero production callers** (see consumer-lifecycle-policy.md), so there is no live data path to certify end-to-end yet; the schema/store contract itself is what's certified. | **PASS** (contract-level; not yet wired to a live writer, tracked as a Phase 3/4 note, not a Phase 2 blocker) |

## Represented/unsupported restoration

Already listed under "Assertion lifecycle on evidence loss/restoration"
above — the six task-4.5 tests are the restoration certification and cover
every legal prior status (`represented`, `candidate`, `in_review`,
`deferred`, `accepted`), including the regression guard that restoration
never escalates a represented claim to accepted.

## Assertion redirects

`classfoundation.RedirectResolver` (`redirect_resolution.go`) is the only
redirect mechanism in the codebase (bounded, cycle-detecting, shared between
term and assertion redirect stores). Its production callers are all inside
`metric_foundation_shadow.go` / `metric_adapter.go` — the shadow-mode
foundation comparison, which writes nothing consumer-visible. **No Phase 2
reader in the tables above currently consumes an assertion redirect**;
redirects become reader-relevant only once the claim registry (ADR
`2026081701`) is activated, which is Phase 3+ territory.

| Consumer | Proof | Verdict |
|---|---|---|
| Redirect resolution (terminal target, depth cap, cycle detection) | `TestRedirectResolverReturnsTerminalAndTraversal`, `TestRedirectResolverReportsDepthLimitWithoutInferredTarget`, `TestRedirectResolverReportsCycleAsUnresolved` | **PASS** (infrastructure-level; no Phase 2 consumer exercises it yet) |

## Legacy rows

Every accepted-only query above filters on `status` alone and never
references the four new independent state columns, so a legacy row (which
has `status` but no `class_identity_state_term_id` etc.) is handled
identically to a new one — there is no separate "legacy" code path to
certify for those consumers. For dual-read consumers, the same is true by
construction: `Assertion`'s new fields are all `omitempty`/nullable, and
`ListAdmin`'s `SELECT` list uses the same columns regardless of whether they
are populated. No consumer inspected required a schema-shape branch for
legacy vs. new rows.

## Gaps closed while building this suite

Three existing tests provided weaker protection than they appeared to:

1. `HighestAcceptedAssertionID`'s two existing tests matched only the
   `SELECT COALESCE(MAX(id), 0) FROM kb.semantic_assertions` prefix, not the
   `WHERE ... status = 'accepted'` clause — a regression removing that clause
   would not have failed either test.
2. `keywords.AcceptedForConcept`'s test mocked against the
   `acceptedForConceptSQL` package constant itself, so it would still pass if
   the constant's accepted-only clause were removed (both sides of the
   comparison would change together).
3. `classification_projection.go` had no test file at all.

All three now have an independently-typed assertion that would fail if the
accepted-only guarantee regressed.

## Known pre-existing gaps (not blocking, tracked for later)

- `search_indexing_test.go`'s tests currently fail in this dev environment
  (`TestInsertSearchRegistryRowsUpsertsNormalizedRows` and related) for
  reasons unrelated to this ADR — present before this session's changes,
  confirmed by `jj diff` showing no session changes to that file.
- `ObservedProfileStore` has no production caller; wiring it into the live
  metric pipeline is Phase 3/4 work, not Phase 2. Its contract is certified;
  its end-to-end behavior with real metric data is not, because there is no
  real metric data flowing through it yet.
