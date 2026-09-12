# Implementation notes — product-metric-reviewer

Backend (Phases 1–4) and frontend (Phase 5) are complete: 34/38 tasks. Phase 6.1 (e2e
against a real corpus) and 6.2 (full-tree `go test ./...`) are deferred — see the bottom.
`go test ./api/product-reviews/` = **50 green**, `go vet` + `go build ./...` clean;
`bun test src/lib/services/productMetricReviewService.test.ts` = **7 green**,
`bun run build` clean, `svelte-check` adds no new errors.

## Coding Best Practice (CLAUDE.md) — knowledge questions

**What knowledge changed?**

- **New app: Product Metric Reviewer.** Given a product profile (a versioned, curatable
  tree of scope nodes) it returns every metric across the product's parts and lifecycle
  aspects, with the document, line spans, tier, and a human-readable reason per hit.
- **Six new `kb` tables** (migrations `20260909000004`–`0006`, applied + recorded on
  `miner`): `product_profiles`, `product_profile_nodes`, `product_review_requests`,
  `product_review_runs`, `product_review_run_documents`, `product_review_results`. The
  report is a `report_json` + `report_md` column pair on `product_review_runs`, not a
  seventh table.
- **New package `server/api/product-reviews/`** (`productreviews`): config loader
  (`product-review.local.toml`) with `zh-cn`/`en` locale resolution and an
  `AspectVocabulary(locale)` reader; `Store` (profile/node CRUD, version-bump discipline,
  cycle rejection, single-`product`-root invariant); `Proposer` / `Grounder` / `Expander` /
  `AttachAspects` (the propose → ground → expand → aspects construction passes) and a
  `Builder` that composes them; an `ArtifactAdapter` registry (`metric` only in v1) +
  `DocumentScoper` (Path A/B/C) + `Retriever` (Path D/E, `assembleResults`); `RunStore` +
  `RunController` (request/run lifecycle, state machine, persistence); `BuildReport` +
  `DiffResults`; 17 Echo handlers.
- **New route block in `server/api/routes.go`** after `apiGroup.GET("/kb/products", …)`:
  `/kb/product-profiles/*` and `/kb/product-reviews/*` (17 endpoints — profile
  create/read/build/ready/node-mutate, aspect vocabulary with `?lang=`, review
  create/list/read/rerun, run read, results with node/tier/path/document/type filters +
  score sort, scoped documents, diff, CSV export).
- **New config `ChenWeb/product-review.local.toml`** — aspect vocabulary
  (`name_<locale>` / `desc_<locale>`, `relation_types` or `match_mode = "lexical"`), models,
  grounding thresholds, depth/node/document/run budgets, standards boost weight.
- **New prompts** `prompts/prompt-product-structure-v1.md`,
  `prompts/prompt-product-aspect-mapping-v1.md` (the only LLM calls; retrieval is pure
  SQL + Go).
- **Frontend**: `web/src/lib/services/productMetricReviewService.ts` (typed client),
  `web/src/routes/home3/product-metric-review/+page.svelte` +
  `web/src/lib/components/home3/product-metric-review-view.svelte` (scope-tree, results,
  evidence drawer, report tab — light + dark), 46 `pmr_*` i18n keys in `messages/en.json`
  + `messages/zh-cn.json`.

**What the app assumes about the substrate it reads (all read-only, unchanged):**

- **`kb.products.relation_type`** is a free-text column whose *effective* vocabulary is
  the 22 values the `prompt-enrich-product-relations-v1.md` prompt emits (`scope`,
  `regulated_object`, `requirement_target`, `component_of`, `contains_product`,
  `storage_requirement`, `maintenance_requirement`, `usage_condition`,
  `installation_requirement`, `testing_requirement`, `certification_requirement`,
  `risk_source`, …). Path A weights these via `[scoring.relation_type_weights]` in config;
  an unknown value falls back to weight `0.3`. Aspect nodes join on
  `(root product identity, relation_type ∈ aspect.relation_types)`. **On `miner` today
  `kb.products` has 0 rows** — Path A and aspect scoping are untested against real data.
- **`kb.metrics.subject_concept_id`** (FK → `kb.keyword_concepts`, scope `metric_subject`)
  is the identity Path E's concept-equality half matches on. **On `miner` today 0 of 56
  `kb.metrics` rows have it populated** — Path E currently rests entirely on the
  `metric_subject` normalized-name match and the RRF-over-`kb.search_artifacts_metric`
  anchor. The concept path is coded and unit-tested but has no live data to exercise.
- **`kb.search_artifacts_metric`** is joined to `kb.metrics` on `m.id = sa.source_row_id`
  (INNER) so the ~6500 orphaned search rows whose source metrics were re-extracted drop
  out automatically; `sa.artifact_id` (`<record>_mtc_<seq>`) is authoritative, no seq
  recomputation.
- **`kb.object_nodes.reconcile_status`** CHECK allows only
  `active|merged|pending_review|rejected` — there is no literal `ambiguous` value in that
  table. The grounder copies whatever string is on the row; `NeedsReconcileReview()` and
  the UI flag treat `{ambiguous, pending_review}` as "needs review" so the spec's
  `ambiguous` scenario still works if the corpus ever uses it.
- **Grounding source #3 is `kb.ontology_term_labels`** (matched on `label`), not
  `kb.ontology_terms` — that table has no label column.
- **`document.doc_kind` facet** (`kb.doc_facet_values`, values `da:doc_kind_standard` /
  `_specification` / `_regulation`, fallback `kb.doc_facets.input_doc_type`) is a **boost,
  never a filter** (Path C). **On `miner` today there are no `document.doc_kind` rows** —
  Path C is untested against real data.
- Retrieval reuses the **node label embeddings stored on `kb.product_profile_nodes`** by
  the grounding pass; it never calls an LLM or an embedder, which is what makes
  Requirement-06 (determinism) hold.

**Which docs / specs / ADRs / tests are affected?**

- **Specs (new capabilities, this change):** `specs/product-scope-profile/spec.md`,
  `specs/product-artifact-retrieval/spec.md`, `specs/product-metric-review-runs/spec.md`,
  `specs/product-metric-reviewer-page/spec.md`. No MODIFIED delta — the nine promoted specs
  in `openspec/specs/` govern how assertions are produced/governed; this change only reads
  accepted rows and adds no requirement to any of them.
- **Tests added (Go, `server/api/product-reviews/`):** `store_test.go`, `config_test.go`,
  `propose_test.go`, `ground_test.go`, `expand_test.go`, `aspects_test.go`,
  `registry_test.go`, `metric_adapter_test.go`, `docscope_test.go`, `retrieve_test.go`,
  `run_store_test.go`, `run_controller_test.go`, `report_test.go`, `diff_test.go` (+
  `helpers_test.go`, `run_helpers_test.go`). 50 tests; every scenario in the four spec
  files has a corresponding test.
- **Tests added (frontend):** `web/src/lib/services/productMetricReviewService.test.ts`
  (7 tests, `node:test` + `fetch`-stub style, run via `bun test`).
- **Related ADRs (context, unchanged):** `2026062804` (doc-review request/run split —
  mirrored here); the design's D-series decisions live in `design.md`.

**Which docs were updated?**

- The four new spec files, `proposal.md`, `design.md`, `overview.md`, this notes file, and
  the running handoff
  `KnowledgeStore/doc-repo/hand-offs/202609/2026090901-handoff-product-metric-reviewer.md`.
- **No `HowTo.md` entry.** Consistent with the sibling `metric-scoped-explorer` /
  `analysis-node-related-metrics` changes, which added none. The endpoint contracts live
  in `design.md` + the spec files + the typed frontend service.
- `product-review.local.toml` carries its own header comment documenting every knob and the
  locale-resolution contract.

**Which docs are now stale?**

- None in a wrong sense. `overview.md` predates the implementation and describes intent, not
  final internals (e.g. it does not name `assembleResults` or the `rrfSearch` helper) —
  still accurate at the level it is written.
- The dev DB being near-empty means the tuning numbers in `product-review.local.toml` are
  unvalidated placeholders (see below) — a state note, not a stale doc.

**What was intentionally left undocumented?**

- **Every number in `product-review.local.toml` is a placeholder** — `object_embedding_min
  = 0.82`, `concept_label_min` / `ontology_label_min = 0.90`, `standards_boost = 0.25`,
  `max_depth = 3`, `max_nodes = 200`, `max_documents = 400`, `max_results = 2000`,
  `per_document_cap = 60`, all `relation_type_weights`. Tune against a real corpus in task
  6.1; they are config, not code, so no code doc.
- **Retrieval tuning constants** are named consts, not config: `rrfK = 60`,
  `hybridCandidateLimit = 200` (`hybrid.go`), the `document_first` base score `0.01` and
  `direct_concept` base score `1.0` and `tierBonus` weights in `retrieve.go`. Promote to
  config only if tuning demands it.
- **`profile_version` per run** is stashed in `report_json.profile_version` rather than a
  schema column (the migration puts `profile_version` on the request). `Rerun` re-pins the
  request; `DiffAgainstPrevious` compares the two runs' `report_json` versions. This is how
  "Diff flags a profile change" works despite the shared column — a design choice, noted in
  the handoff's Phase 4 section, not surfaced in a spec.
- **`concept_label_min` is currently unused** — `kb.keyword_concepts` has no embedding
  column, so grounding source #2 is an exact (case-insensitive) `pref_label` match. The
  threshold stays configured for a future fuzzy match.
- **The metric CSV export omits value / unit** — `Result` carries only what
  `kb.search_artifacts_metric` + the retrieval path produce; value/unit would need a
  `kb.metrics` join. The evidence drawer shows the document number, not its title, for the
  same reason. Add a title/value to `ResultRow` server-side (join `kb.inputs` /
  `kb.metrics`) if a consumer needs them.
- **`testing` aspect** was added to `product-review.local.toml` beyond design D3's list of
  8 (because `testing_requirement` is a real relation type); `transport` / `packaging` /
  `disposal` / `emc` use `match_mode = "lexical"` because the relation-type enum has no
  matching value. Config, not code.

## Deferred / needs a live environment (Phase 6)

- **6.1 e2e** — dev DB `miner` is near-empty (`kb.products` 0 rows, `kb.metrics` 56 with 0
  `subject_concept_id`, no `document.doc_kind` facet rows). Needs a `Ventilator` corpus
  loaded, then: create profile → build → curate + mark ready → create review → confirm a
  part-attributed metric, an aspect-attributed metric, and a populated gap list. Path A / C
  and Path E's concept half have no live data to exercise until then.
- **6.2 full `go test ./...`** — the whole `ChenWeb/server` tree (not just the new package,
  which is green) plus `mise build-server`. `go build ./...` is already clean.
- The frontend page is **not registered in `nav-rail.svelte`** — reachable directly at
  `/home3/product-metric-review?run=<id>`. Add a nav entry (page-config system) when the
  app is ready for discovery.

## Update — 2026-09-13: 6.1 completed, two bugs fixed, one still open

Since this note was written, a self-service intake page shipped on top of this change
(`IntakeProductReview` / `intake.go`, `/home3` "Product Review" nav entry — not covered
above since it postdates this note) and the corpus stopped being near-empty. Task 6.1 was
finally run for real; full root-cause and fix detail is in
`KnowledgeStore/doc-repo/bugs/202609/2026091301-bug-product-metric-reviewer-e2e-crash-curation-gap-and-retrieval-precision.md`.
Summary:

- **`docscope.go` pathC crashed every review run outright** (`COALESCE(value, '')` against
  a `jsonb` column — `''::jsonb` is invalid JSON, fails at parse time regardless of data).
  Fixed: `COALESCE(value #>> '{}', '')`. This is why the one real run on record before today
  (`kb.product_review_runs.id=1`) failed in 0.5s having done nothing.
- **The self-service intake flow never curates its proposed nodes**, so part-tier
  attribution was structurally impossible through that page (retrieval only loads
  `status='accepted'` nodes, by design — this is *not* a relaxation of that design). Fixed:
  new `Store.AcceptAllProposed`, called only from `IntakeProductReview` between `Build` and
  `SetProfileStatus(ready)`; the manual curation page's accept/reject gate is untouched.
- **6.1 is done against `血压计` (blood pressure monitor), not `Ventilator`.** The dev
  corpus loaded today is BP-monitor documents; testing against a `Ventilator` profile (which
  a prior intake attempt had created when the DB was empty) produces domain-mismatched
  results — not a fair verification. All three of 6.1's literal conditions are met: 36
  part-tier + 1 aspect-tier attributed results, 20-entry gap list.
- **Still open: retrieval precision — deeper than a tuning knob.** Spot-checking the part-tier
  results shows real hits mixed with wrong-domain ones (a `主机`/"host unit" node attributed a
  composting-facility metric). Added a similarity floor (`ScoringConfig.HybridSimilarityMin`,
  `hybrid.go`'s `sem` CTE) and product-root-anchored query text for module/part nodes
  (`docscope.go` pathB, `retrieve.go` Path E) — both shipped and verified live. But re-running
  the `血压计` review proved these don't fix precision: for a short generic label like `主机`,
  real and wrong-domain matches sit interleaved in the same 0.23–0.32 cosine-similarity band
  (measured against the live corpus), so no threshold separates them, and the wrong-domain
  documents don't lexically co-occur with the product name at the chunk level either. A real
  fix needs product-identity propagated into the chunk-level index — out of scope here; full
  evidence and the remediation options are in the bug doc.
- Corrections to two of this note's original "what the app assumes" claims, now that
  `kb.products` and `kb.metrics` are populated: Path A (`kb.products` identity match) and
  Path C (`document.doc_kind` boost) both exercised against real data during the `血压计`
  run — Path C's underlying bug (above) is why it was never actually exercised until today,
  not because the facet rows didn't exist by the time of the run.
