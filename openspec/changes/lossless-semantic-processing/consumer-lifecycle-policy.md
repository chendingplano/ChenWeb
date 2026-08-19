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
| Authoritative classification projection ("semantic projection") | `api/ontology/assertions/classification_projection.go`, `api/ontology/assertions/project_semantics.go` | **accepted-only**. `ProjectSemantics.Run` explicitly rebuilds derived projections "from accepted assertions" (DR8 Phase D); this is the "semantic projection" consumer named in task 5.2. | Already covered by this audit's accepted-only classification; no widening required. Note: `web/src/lib/components/home3/semantic-projections-view.svelte` is an unrelated feature over `kb.semantic_projections` (document topic/keyword summaries) and does not read `kb.semantic_assertions` at all — a naming collision, not a consumer of this system. |
| Keyword-concept alignment | `api/ontology/keywords/alignment.go` | **accepted-only**. An `aligns_to_term` relation changes governed term identity and remains a curator/automatic-governance decision. | Retain filter; represented metric assertions are out of this family and must not be treated as an alignment. |
| Assertion-store accepted watermark | `api/ontology/assertions/assertions_store.go` (`LastAcceptedForSubject`) | **accepted-only**. The API's name and its callers make it an explicit endorsed-truth query. | Retain filter; do not reuse it for discovery or diagnostics. |
| Semantic assertion diagnostic API | `api/kbhandler/semantic_assertions_handler.go`, `api/ontology/assertions/assertions_store.go` (`GET /kb/semantic-assertions?input_record_id=`) | **dual-read diagnostic discovery**. The document-scoped query returns every lifecycle status having a current evidence link for that input record; an explicit status filter remains available. | Implemented as the first Review Document/discovery reader slice. It exposes raw and independent-state fields already present in the assertion response without widening any governance query. |
| Assertion lifecycle on evidence loss/restoration | `api/ontology/assertions/evidence_store.go` | **all evidence-supported lifecycle states**: `represented`, `candidate`, `in_review`, `deferred`, and `accepted` transition to `unsupported` on loss of final qualifying evidence; restoration returns the recorded prior status. | Implemented as part of task 5.2; remaining reader work is tracked separately. |
| Directional comparison | `api/ontology/comparison/compare.go`, `evaluate_cell.go` | **capability-aware**. Every selected comparison pair produces a cell. Missing normalization or incompatible quantity/component capabilities returns `no_verdict` with a persisted rationale; comparable pairs retain their existing verdicts. | Implemented in task 5.4; later assertion readers must pass raw-preserved states into this path rather than dropping them. |
| Metric search registry | `api/doc-processing/search_indexing.go` (`buildMetricRegistryRows`) | **dual-read discovery**. Every legacy metric remains indexed; when a current supporting assertion exists, both its raw representation and its normalized representation are appended to the metric search document and recorded in semantic payload. | Implemented as the search slice of task 5.6. The optional lateral assertion read preserves legacy-only rows and does not change any writer gate. |
| Semantic completeness projection | `api/ontology/semantic/completeness.go` | **diagnostic / capability-aware**. It reports supporting-link coverage rather than endorsing an assertion. | Retain its state-neutral coverage behavior; add represented/raw-preserved cases to the reader compatibility suite. |
| Review Document — Semantic Diagnostics tab | `web/src/lib/components/home3/doc-review-semantic-view.svelte`, calling the semantic assertion diagnostic API above | **dual-read discovery**. A read-only tab on the doc-review results view; displays raw value, normalized value, all four independent states, processing errors, class confidence, and active evidence for every lifecycle status, including represented/unsupported/raw-preserved. Does not route through the accepted-only profile-rule loader and never labels a non-empty finding list as a processing failure. | Implemented as task 5.5. |
| Semantic processing association | `api/ontology/assertions/associate_semantics.go` | **legacy writer, not a consumer**. It still promotes successful ingestion to `accepted`. | Keep unchanged until Phase 3 task 6.6; it is deliberately outside the Phase 2 reader cutover. |
| Semantic run reports ("reports") | `api/ontology/semantic/report.go` (DR11 per-run outcome/finding report), `cmd/semantic-baseline` (Phase 0 corpus report) | **diagnostic**. Both report by outcome/finding disposition and severity, not by assertion status; neither filters to `accepted`. This is the "reports" consumer named in task 5.2 — no separate assertion-status-filtered report exists elsewhere in the codebase. | Already dual-read by construction (Phase 0/1); no widening required. |
| Semantic retry queue reader ("retry tooling") | `api/ontology/semantic/retry.go` (`RetryQueue.List`), `api/kbhandler/semantic_retry_queue_handler.go` (`GET /kb/semantic-retry-queue`), `web/src/lib/components/home3/semantic-retry-queue-view.svelte` | **diagnostic**. `kb.semantic_retry_queue` has no accepted/represented distinction of its own — every job is diagnostic by nature. Before task 5.2, this queue (implemented in Phase 1 task 3.7) had no reader at all: `RetryQueue` exposed only `Enqueue`/`Claim`/`MarkDone`/`MarkStale`/`MarkFailed`/`ScheduleForDependencyChange`, none of which list rows. | Implemented as part of task 5.2: added `RetryQueue.List` (filters by state/outcome_id, joins outcome artifact identity), its handler/route, and a read-only admin page under Sysadmin → Doc Process → Semantic Retry Queue. |
| Observed class profile aggregation | `api/ontology/classfoundation/observed_profiles.go` (`ObservedProfileStore`), `observed_profile_reader.go` (`ObservedProfileReader`) | **diagnostic / evidence-only, never authoritative**. `ObservedProfileObservation.ObservationState` is required and never dropped — malformed, missing, unparsed, conflicting, and conforming observations are all retained, and exceptions carry an explicit `outlier`/`contradiction` kind in a table (`kb.ontology_observed_class_profile_exceptions`) `contracts_store.go` never reads from. The reader hardcodes `Authoritative = false` and an explicit `Authority = "observed evidence; non-authoritative"` label on every response. This infrastructure was built for ADR `2026081701` (canonical-metric-class-foundations, archived in shadow mode with writers off) and already satisfies task 5.6's "outliers included, never promoted" requirement at the schema/store level. | No code change needed for task 5.6. Note for future sessions: this store currently has **zero production callers** (`grep -rn "ObservedProfileStore{" server/` outside tests returns nothing) — wiring it into the live metric pipeline is writer-shaped work and belongs to Phase 3/4 family migration (tasks 6.4+/7.x), not Phase 2, since Phase 2's design explicitly keeps writers on the legacy path. |
| Completeness: absent artifact vs. present-with-missing-value | `api/ontology/semantic/completeness.go` (`CompletenessReport`) | **diagnostic**. Already distinguishes `ArtifactsMissingAnyStage`/`MissingStageOutcomes` (no processing occurred — absent) from `ArtifactsWithMissingValue` (processed, evidence-supported, but the governed `value_state_term_id` is explicitly `missing` — present artifact, absent value), per the field's own doc comment: "These are present artifacts, not absent-artifact gaps." Implemented in Phase 1 task 3.10. | No code change needed for task 5.6; already satisfies the requirement. |
| Search: raw and normalized text indexing | `api/doc-processing/search_indexing.go` (`buildMetricRegistryRows`) | **dual-read discovery**. The registry query already joins `raw_text`/`raw_payload` and normalized (`numeric_value`/`lower_value`/`upper_value`) fields from the current supporting assertion into the search document (see row above, "Metric search registry"). | Already satisfies task 5.6's search clause; same implementation as the row above. |

## Consumers not currently implemented

Generic semantic discovery and diagnostic projections beyond what is listed
above remain to be built as Phase 2 continues (task 5.6 onward). Metric
search now reads supporting assertions when available while preserving
legacy-only rows. The semantic assertion diagnostic API can now scope all
lifecycle states to the document through active evidence links. Review
Document's governance rendering still goes through the accepted-only profile
path unchanged; its "Semantic Diagnostics" tab (task 5.5) is a separate,
additive, read-only projection over the same document-scoped diagnostic API.
Semantic projection, reports, and retry tooling — the three remaining items
literally named in task 5.2 — are now all accounted for (rows above).

## Task 5.8 — blocked on Phase 3 task 6.6, not actionable in Phase 2

Task 5.8 ("retrain dashboards and alerts to stop reading semantic findings as
failures") has no live signal to retrain against today. Every dashboard/alert
read of processing status (`web/src/lib/components/home3/doc-processor-dashboard-view.svelte`,
`DocMetadataSQLStore.ListRecordsWithFailedDocProcessors` in
`extract-doc-metadata-store.go`, backed by the `has_failed_proc` DB-trigger
rollup) reads only the legacy `proc_status` column, which is still written
exclusively by the unchanged legacy writer (`associate_semantics.go`). That
writer still marks a run `failed` on any mapping-miss (design.md's documented
current-state finding), and removing that is Phase 3 task 6.6 — deliberately
deferred past Phase 2 because doing it earlier "would expose represented rows
to consumers that have not been certified to read them" (handoff §5). Task
5.7, just completed, is that certification, so 6.6 is now unblocked in terms
of prerequisites, but 6.6 is explicitly Phase 3 scope, not 5.8's.

The reader-side infrastructure this retraining will eventually consume is
already built and ready: `semantic.ExecutionStatus`, `LegacyProcStatus`,
`FindingSummary.DisplayStatus`, and `SetsHasFailedProc` (Phase 1 task 3.12,
D6) already compute "failed" from `ExecutionStatus` alone, never from
`finding_count`. There is nothing for a Phase 2 dashboard to retrain against
until a writer populates outcome/finding data that a dashboard could read
instead of legacy `proc_status` — and no writer is enabled until Phase 3.
5.8 is left unchecked rather than marked done on documentation alone, since
unlike 5.2/5.6's already-satisfied items, this one has no code or doc gap to
close today — it is genuinely blocked, not merely undocumented.

**Re-verified 2026-08-19** (task 6.9 now genuinely closed, and — per ADR §1.2 —
the `LOSSLESS_SEMANTIC_WRITES_METRIC` gate now defaults ON in code, not just
locally): 6.6 is done, and the mapping-miss-fails-the-run behavior this
section describes no longer runs by default for the **metric** family — a
proposed/ambiguous mapping now completes with a finding instead of marking
`proc_status`/`has_failed_proc` failed. There is now live signal to retrain
against for metrics (doc 416's outcome/finding rows). But 5.8's own scope is
the dashboard/alert code itself
(`doc-processor-dashboard-view.svelte`, `ListRecordsWithFailedDocProcessors`)
switching to read `ExecutionStatus`/`FindingSummary.DisplayStatus` instead of
legacy `proc_status` — that code change has not been made. It also remains
blocked for every non-metric family (provisions, entities, ...): those still
run the pre-DR12 legacy path unconditionally (Phase 4, not started — see
`tasks.md` §7), so retraining today would only be correct for metrics and
would still misread other families' semantic findings as failures. 5.8 stays
unchecked; the blocker has narrowed from "no live signal at all" to "the
retraining code itself, plus Phase 4 family coverage."

## Certification rule

The reader compatibility suite in task 5.7 must name each row above.  It must
prove that accepted-only consumers ignore represented/unsupported assertions,
while diagnostic/discovery consumers render them with lifecycle and independent
semantic-state warnings.  No writer gate may be enabled until every required
consumer has a passing certification record.
