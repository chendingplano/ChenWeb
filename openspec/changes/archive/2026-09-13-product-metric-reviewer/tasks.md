## 1. Schema and configuration

- [x] 1.1 Migration: `kb.product_profiles` (name, product description, version, status, tenant, truncation flags, timestamps) and `kb.product_profile_nodes` (node kind, parent, labels, aliases, depth, origin, status, confidence, rationale, `object_id`/`concept_id`/`term_id`, grounding state, reconcile state, aspect key, embedding, source edge refs), with the indexes the tree and grounding lookups need
- [x] 1.2 Migration: `kb.product_review_requests` and `kb.product_review_runs` (run number, status, timings, counts, truncated counts, `report_json`, `report_md`, error message), indexed by request and status
- [x] 1.3 Migration: `kb.product_review_run_documents` (run, input record, fused score, matching node ids, matching paths, doc kind) and `kb.product_review_results` (run, artifact type/id, source row, input record, matched node, tier, score, paths, inclusion reason, line spans), indexed for the node/tier/document/document filters in `product-metric-review-runs`
- [x] 1.4 Verify migrations applied cleanly against the dev DB (`SELECT * FROM project_db_migration ORDER BY id DESC LIMIT 5`) — air auto-applies on rebuild, so confirm before editing any of the three files further
- [x] 1.5 `ChenWeb/product-review.local.toml`: aspect vocabulary (`aspect_key`, `name_<locale>`, `desc_<locale>`, `relation_types` or `match_mode = "lexical"`), models, grounding thresholds, depth/node/document/run budgets, standards boost weight
- [x] 1.6 `ChenWeb/prompts/prompt-product-structure-v1.md` and `prompt-product-aspect-mapping-v1.md` — no prompt text in Go

## 2. Profile construction

- [x] 2.1 `server/api/product-reviews/` package skeleton: config loading with locale resolution (mirror `doc-reviews` aspect-label resolution), store types, `CWB_KB_PMR_0xx` error codes
- [x] 2.2 Profile + node store: create, read tree, mutate node, version bump on any node-set mutation, cycle rejection (`CWB_KB_PMR_010`), single-`product`-root invariant
- [x] 2.3 Propose pass: prompt render, one LLM call, tree parse, depth/node budget with breadth-first truncation and `truncated` recording, failure path leaving the profile untouched (`CWB_KB_PMR_011`)
- [x] 2.4 Ground pass: ordered resolution through `kb.object_nodes` (normalized names/aliases, then embedding above threshold), `kb.keyword_concepts` scope `metric_subject`, `kb.ontology_terms` labels; record grounding state, persist node embedding once, propagate `ambiguous`/`pending_review` reconcile state, degrade to lexical when `kbsearch.SemanticSearchEnabled` is false
- [x] 2.5 Expand pass: accepted `core:part_of`/`core:component_of` assertions plus `kb.products` `component_of`/`contains_product` rows; insert with `origin = graph_expanded` and source-edge reference, dedupe against already-grounded nodes, respect budgets
- [x] 2.6 Aspect attachment from configured vocabulary, carrying each aspect's `relation_types` mapping onto the node
- [x] 2.7 Unit tests for 2.2–2.6 against `go-sqlmock`, covering every scenario in `specs/product-scope-profile/spec.md`

## 3. Retrieval

- [x] 3.1 Artifact-type registry and adapter interface (list by document, match by subject concept, search partition, project to result row); register `metric`; reject unregistered types with `CWB_KB_PMR_020`
- [x] 3.2 Document scoping: Path A `kb.products` identity match with `relation_type` weighting, Path B RRF over the `product`/`summary`/`topic` partitions reusing the `kbsearch` fusion helpers, Path C standards boost as a boost and never a filter; persist per-document match reasons
- [x] 3.3 Path D (document-first) retrieval through the registered adapter
- [x] 3.4 Path E (direct) retrieval: `subject_concept_id` equality plus RRF over the type's partition, unrestricted by the document scope set
- [x] 3.5 Union, dedupe on `(artifact_type, artifact_id)` retaining all paths, tier assignment by matched node with the `direct` > `part` > `aspect` > `document_scope` precedence, score fusion, inclusion-reason text
- [x] 3.6 Caps: per-document and per-run truncation keeping highest scores, removing `document_scope` before attributed tiers, recording truncated counts on the run
- [x] 3.7 Determinism test: two runs over an unchanged fixture corpus and profile version produce identical id sets and tiers, with no LLM call in the retrieval path
- [x] 3.8 Unit tests covering every scenario in `specs/product-artifact-retrieval/spec.md`, including the part-metric-outside-scope and unattributed-metric cases

## 4. Runs, report, and API

- [x] 4.1 Run controller: request creation pinning `profile_version`, run state machine, refusal of `draft` profiles (`CWB_KB_PMR_030`), failure recording that leaves the request re-runnable
- [x] 4.2 Persist the scoped document set and the result rows with full provenance
- [x] 4.3 Report builder: per-node coverage table, gap list, attributed vs `document_scope` counts, truncation disclosure; store as `report_json` and rendered `report_md`
- [x] 4.4 Diff: compare a run against the previous run of its request — added, removed, re-tiered — and flag a `profile_version` difference
- [x] 4.5 Echo handlers and route block in `server/api/routes.go` after the existing `/kb/products` routes: profile create/read/expand/node-mutate, aspect vocabulary with `?lang=`, review create/list/read, results with node/tier/path/document filters and sort, scoped documents, diff, export
- [x] 4.6 Handler and store tests covering every scenario in `specs/product-metric-review-runs/spec.md`

## 5. Frontend

- [x] 5.1 `web/src/lib/services/productMetricReviewService.ts` — typed client for the endpoints in 4.5
- [x] 5.2 Route `web/src/routes/home3/product-metric-review/` and the page shell: scope-tree pane, results pane, evidence drawer, report tab; light and dark from the first commit
- [x] 5.3 Scope-tree component: hierarchy with kind/origin/grounding badges and per-node counts, node accept/reject/edit/add/delete, ambiguous-grounding flag, stale-version warning with re-run affordance
- [x] 5.4 Results component: grouping by node, filters (node, tier, path, document, text), score sort, `document_scope` collapsed by default with its count, empty-state linking to the gap list
- [x] 5.5 Evidence drawer: artifact fields, document title and number, line spans, inclusion reason, paths, and navigation to the source location through the existing document/PDF-locator surfaces
- [x] 5.6 Report tab: coverage table, gap list, truncation disclosure, diff against previous run
- [x] 5.7 i18n chrome messages plus server-resolved aspect labels; component tests for the scenarios in `specs/product-metric-reviewer-page/spec.md`

## 6. Verification and documentation

- [x] 6.1 End-to-end check against the dev corpus with a real product: profile builds, a part-attributed metric is returned, an aspect-attributed metric is returned, and the gap list is populated — done against `血压计` (not `Ventilator`, per the corpus actually loaded); required fixing two bugs first, see `KnowledgeStore/doc-repo/bugs/202609/2026091301-bug-product-metric-reviewer-e2e-crash-curation-gap-and-retrieval-precision.md`. That doc's Finding 3 (retrieval-precision defect) is still open.
- [x] 6.2 `go test ./...` in `ChenWeb/server` and `mise build-server` clean; frontend build clean — `go build ./...`, `go vet ./...`, `mise build-server`, and `bun run build` (web/) all clean; `go test ./api/product-reviews/...` green. Full-tree `go test ./...` surfaces 5 pre-existing failures unrelated to this change (`ontology/seed` sqlmock query drift, `ontology/semantic` `TEST_DATABASE_URL` format mismatch, `openmetadatahandler` external-service/config mismatch, `terminologyresourcehandler` FK constraint from stale test data, `qudt-import` fixture parse error) — none touch code this change added or modified.
- [x] 6.3 Record the knowledge change: what the app assumes about `kb.products.relation_type` and `kb.metrics.subject_concept_id`, which docs are now stale, and what was intentionally left undocumented (per workspace `CLAUDE.md` "Coding Best Practice")
- [x] 6.4 Commit through `jj` — migrations and backend as one commit, frontend as another; confirm `jj log` shows only the expected linear commits
