## Why

Asking "which metrics apply to a ventilator?" cannot be answered by matching metric text against
the word *ventilator*. A product is a recursive structure — product → modules → parts → parts of
parts — and most metrics are attached to a **part**, not to the product: `Display Luminance` is a
metric of the *display*, `Battery Endurance` of the *battery*, `Leakage Current` of the *power
supply*. A product-name query misses all of them. The same query also misses the whole
**lifecycle envelope** the product must satisfy — storage, transportation, maintenance, usage
environment, safety, packaging, disposal — where the metric's subject is a condition, not a part.

Everything needed to answer the question is already in the knowledge base and unreachable:
`kb.products` records, per document, which products a document is *about* and under which relation
(`scope`, `component_of`, `storage_requirement`, `maintenance_requirement`, `usage_condition`, …);
`kb.metrics.subject_concept_id` gives each metric's subject a governed concept identity;
`kb.object_nodes` + accepted `core:part_of` / `core:component_of` assertions hold the reconciled
part graph; `kb.search_artifacts` provides hybrid lexical+vector retrieval over every artifact
family. No surface composes them into a product-scoped answer.

## What Changes

- **New app: Product Metric Reviewer.** Give it a product name or a product description; it returns
  every metric related to that product — across its parts and across its lifecycle aspects — with
  the document, the line spans, and the reason each metric was included.

- **New concept: a product scope profile.** A persisted, versioned, curatable tree of scope nodes
  (`product` root → `module` / `part`, recursive → `aspect`) built in three passes:
  1. **Propose** — an LLM decomposes the product from its name/description (prompt file, per
     `CLAUDE.md` §2; never hard-coded).
  2. **Ground** — every proposed node is resolved against `kb.object_nodes` (normalized names +
     hybrid search), `kb.keyword_concepts` (scope `metric_subject`, the space
     `kb.metrics.subject_concept_id` lives in), and governed ontology terms. Grounding is what
     later lets a metric be matched by identity rather than by string.
  3. **Expand** — from each grounded node, traverse accepted `core:part_of` / `core:component_of`
     assertions and `kb.products.relation_type IN ('component_of','contains_product')` to pull in
     real parts the model never named. Bounded by depth and node budget.
  A reviewer accepts, rejects, edits, or adds nodes; the profile is reusable across runs and
  across products in the same family.

- **Aspect nodes are first-class**, from a configured, i18n'd vocabulary (`storage`, `transport`,
  `maintenance`, `usage_environment`, `safety`, `packaging`, `disposal`, `emc`, …). Each aspect
  maps to the `kb.products.relation_type` values that already encode it, so aspect matching is a
  join, not a guess.

- **Two-path retrieval, unioned and deduped.**
  - *Document-first* (the pivot described in the request): score documents whose `kb.products`
    rows, summaries, or topics match any scope node — boosted, not gated, by
    the `document.doc_kind` facet (`da:doc_kind_standard` and kin) so standards rank first — then
    take the metrics of the documents in scope.
  - *Direct match*: metrics whose `subject_concept_id` equals a scope node's concept, or whose
    subject matches a node through hybrid search over `kb.search_artifacts_metric`, **regardless
    of document** — this is what catches a display metric sitting in a component datasheet that
    never says "ventilator".
  Each surviving hit records **every** path that found it, a relevance tier
  (`direct` | `part` | `aspect` | `document_scope`), a score, and a human-readable inclusion reason.

- **Retrieval is written over a registered artifact type**, keyed `(artifact_type, artifact_id)` —
  the same key `kb.search_artifacts` uses. v1 registers `metric` only; adding `provision`,
  `inventory_item`, or `test_method` later is a store adapter plus config, not a redesign.

- **Persisted request → run → results, with a report.** Runs are re-runnable and diffable, so a
  re-run after new documents land shows what is new. The report includes a **coverage gap list** —
  scope nodes with zero metrics — which is often the more actionable half of the answer.

- **New page** `/home3/product-metric-review`: scope-tree editor, results explorer grouped by node
  and tier, evidence drawer with line spans and PDF locator, report tab.

- **No change to any existing extraction, pipeline, or semantic-write path.** The app is a reader
  over the artifacts those paths already produce; the only writes are its own tables.

## Capabilities

### New Capabilities

- `product-scope-profile`: the scope-profile model (nodes, kinds, grounding refs, origin/status,
  versioning), the propose → ground → expand construction passes and their bounds, the aspect
  vocabulary and its mapping to `kb.products.relation_type`, and the curation endpoints.
- `product-artifact-retrieval`: document scoping (match paths, standards boost, per-document
  reason), the two retrieval paths, union/dedup/tiering/scoring rules, the inclusion-reason
  contract, and the registered-artifact-type indirection that keeps `metric` from being hard-wired.
- `product-metric-review-runs`: request/run lifecycle, result persistence with provenance, re-run
  and diff-against-previous-run, coverage and gap reporting, and the read/export endpoints.
- `product-metric-reviewer-page`: the `/home3/product-metric-review` surface — scope-tree editing,
  result exploration and filtering, evidence drill-down, report view, and its i18n.

### Modified Capabilities

<!-- None. The nine specs in openspec/specs/ (semantic-assertion-lifecycle, ontology-class-*,
     canonical-semantic-claims, lossless-semantic-processing, …) govern how assertions are
     produced and governed; this change only reads accepted rows and adds no requirement to any
     of them. `kb.products`, `kb.metrics`, `kb.search_artifacts` and the doc-processor registry
     are likewise read unchanged — no spec in openspec/specs/ covers them today. -->

## Impact

- **New migrations** (`ChenWeb/project_migrations/`) — six tables, all in `kb`:
  `product_profiles`, `product_profile_nodes`, `product_review_requests`, `product_review_runs`,
  `product_review_run_documents`, `product_review_results`. The report is a column pair on
  `product_review_runs` (JSONB + rendered Markdown), not a seventh table.
- **New package** `ChenWeb/server/api/product-reviews/` — profile builder (propose/ground/expand),
  document scorer, retrieval paths, run controller, report renderer, echo handlers
  (`CWB_KB_PMR_0xx`). Mirrors the layout of `server/api/doc-reviews/`.
- **Modified** `ChenWeb/server/api/routes.go` — the `/kb/product-reviews/...` and
  `/kb/product-profiles/...` route block, placed after the existing `/kb/products` routes.
- **New config** `ChenWeb/product-review.local.toml` — aspect vocabulary with `name_<locale>` /
  `desc_<locale>`, models, score thresholds, depth/node/document budgets. Same shape and locale
  resolution as `doc-review.local.toml`.
- **New prompts** `ChenWeb/prompts/prompt-product-structure-v1.md` and
  `prompt-product-aspect-mapping-v1.md`.
- **New frontend** `ChenWeb/web/src/routes/home3/product-metric-review/` plus components under
  `web/src/lib/components/home3/product-metric-review/` and a
  `web/src/lib/services/productMetricReviewService.ts`.
- **Reads, unchanged**: `kb.products`, `kb.metrics`, `kb.inputs`, `kb.doc_facets`,
  `kb.doc_facet_values`, `kb.object_nodes`, `kb.artifact_objects`, `kb.keyword_concepts`, `kb.semantic_assertions`
  (accepted only), `kb.search_artifacts`, and `kbsearch`'s embedding/RRF helpers.
- **No** new Go or npm dependency. **No** change to `go.work` or `shared/go`.
