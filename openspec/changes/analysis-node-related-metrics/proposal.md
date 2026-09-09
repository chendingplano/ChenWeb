## Why

The Metric Ontology Explorer's `Analysis` satellite is a leaf today — it carries the P1–P8
diagnostics prose but no chain, so a reader on a selected metric cannot pivot from it to the
other metrics that share its governed class, nor to metrics in *neighbouring* classes. Those two
cohorts are exactly what "analysis in aggregate" needs: the same-class set is the metric's direct
peer group (identical contract), and the similar-class set is the wider comparison frontier the
class-contract machinery (`kb.ontology_class_contract_revisions`, live since `metric-class-contracts`)
was built to support but has no read surface for.

## What Changes

- **`Analysis` satellite gains a two-node chain** in the explorer canvas, unfolding like the
  other chained satellites: `Metrics of Same Class` and `Metrics of Similar Classes`.
- **New read endpoint** `GET /api/v1/kb/metrics/:metric_id/related-metrics?scope=same_class|similar_class&limit=N`
  — returns a ranked list of metrics for one of the two cohorts of the selected metric. The
  selected metric's governed class is resolved through the existing
  `semantic_decision_candidates → resulting_assertion_id → semantic_assertions.instance_of_term_id`
  path. `same_class` returns every metric whose resolved class term equals the selected metric's
  (capped at 200, the explorer's standard node cap). `similar_class` returns the top **N (default
  20)** metrics that are *instances of class contracts hybrid-matched as similar* to the selected
  metric's class contract.
- **New class-contract hybrid-search index** `kb.ontology_class_contract_search` — one row per
  governed class term (label + definition + flattened `contract_payload` facets + module as the
  lexical document, plus a `vector(1536)` embedding), with a tsvector GIN index and a pgvector
  HNSW index. Queried with Reciprocal Rank Fusion of lexical `ts_rank_cd` and pgvector cosine,
  the same fusion the `kb.search_artifacts` hybrid path uses. Kept current by a best-effort
  reindex when a class's contract revision is created or promoted (outside the metric write
  transaction), and backfillable via a new admin endpoint
  `POST /api/v1/kb/ontology/class-contracts/backfill-search` (mirrors `BackfillSearchEmbeddings`).
- **The two new record tabs render a navigable metric table** — same tab/table shell as the other
  chain nodes, but each row is clickable and re-deep-links `?metric_id=` so the whole explorer
  recenters on the chosen metric. `similar_class` rows also show the matched class term and the
  fusion score.
- **Lazy fetch** — unlike the twelve `metric_graph` nodes (served eagerly by one
  `GET /kb/metrics/:metric_id/graph` call), these two tabs fetch on open, keyed by
  `(metric_id, scope)`. The `similar_class` path issues an embedding call, so it stays off the
  canvas's critical path. `GET /kb/metrics/:metric_id/graph` is unchanged.
- No change to the `Analysis` entry-tab prose, the `Metric Ontology` dashboard, or the other
  eleven satellites.

## Capabilities

### New Capabilities

- `metric-analysis-neighbors`: the `Analysis` satellite's two-node chain (`Metrics of Same Class`,
  `Metrics of Similar Classes`), the `GET /api/v1/kb/metrics/:metric_id/related-metrics` read for
  both cohorts and its class-resolution and ranking rules, the navigable-row behaviour that
  recenters the explorer, and the lazy per-`(metric_id, scope)` fetch.
- `class-contract-hybrid-search`: the `kb.ontology_class_contract_search` index (schema, lexical
  document composition, embedding column, tsvector + HNSW indexes), the RRF lexical+vector match
  query over it, the best-effort reindex on contract-revision create/promote, and the
  `POST /api/v1/kb/ontology/class-contracts/backfill-search` admin endpoint.

### Modified Capabilities

<!-- None promoted to openspec/specs/. The `Analysis`-satellite chain and the record-tab
     renderer being extended belong to the still-unarchived `metric-scoped-explorer` change
     (spec not yet in openspec/specs/). "What Changes" and design.md record how this layers on
     it; the spec-lineage housekeeping is a task, mirroring how `metric-scoped-explorer` handled
     its own overlap with `metric-ontology-explorer-page`. -->

## Impact

- **New migration** `ChenWeb/project_migrations/<ts>_create_kb_ontology_class_contract_search.sql`
  — `CREATE TABLE kb.ontology_class_contract_search`, a GIN index on its `search_vector`, and an
  HNSW (`vector_cosine_ops`) index on its `embedding`. `CREATE EXTENSION vector` already applied
  by `20260603000001`.
- **New** `ChenWeb/server/api/ontology/classcontractsearch/` (package) — `Reindex(ctx, db, classTermID)`,
  `BackfillClassContractSearch(...)`, and `MatchSimilar(ctx, db, classTermID, limit)` (the RRF
  query). Reuses `kbsearch.SemanticSearchEnabled` / `kbsearch.FormatVectorLiteral` /
  `kbsearch.ConfiguredEmbeddingDim` and the `newSearchQueryEmbedder` / `computeQueryEmbedding`
  helpers' pattern.
- **New** `ChenWeb/server/api/kbhandler/related_metrics_handler.go` +
  `related_metrics_store.go` — `GetRelatedMetrics` (echo handler, `CWB_KB_RELM_0xx`), the
  class-resolution query, the `same_class` list query, and the `similar_class` compose (call
  `classcontractsearch.MatchSimilar`, then list instances ranked by matched-class score).
- **New** `ChenWeb/server/api/kbhandler/class_contract_search_backfill_handler.go` —
  `BackfillClassContractSearch` (echo handler, `CWB_KB_CCSB_0xx`).
- **Modified** `ChenWeb/server/api/routes.go` — `GET /kb/metrics/:metric_id/related-metrics`
  after the `/graph` line; `POST /kb/ontology/class-contracts/backfill-search` near the other
  `/kb/ontology/...` and backfill routes.
- **Modified** contract write path — one best-effort `classcontractsearch.Reindex(...)` call
  after a class's contract revision is appended/promoted, invoked *after* the metric write
  transaction commits (call site in the Phase D `metric_lossless_writer` caller / doc-processor
  post-index step; never inside the tx). Guarded so a failure only logs.
- **Modified** `ChenWeb/web/src/lib/components/home3/metric-ontology-explorer/model.ts` — add
  `CHAINS.analysis` with the two nodes and a per-node `related: 'same_class' | 'similar_class'`
  marker; their `columns`.
- **Modified** `ChenWeb/web/src/lib/services/metricOntologyExplorerService.ts` — add
  `getRelatedMetrics(metricId, scope, limit?)` and `PROJECT` entries for the two new node ids.
- **Modified** `ChenWeb/web/src/lib/components/home3/metric-ontology-explorer/content-viewer.svelte`
  — a lazy branch for the two `analysis__*` tabs (own loading / empty / error state keyed on
  `(metricId, scope)`) and clickable rows that call `onpickmetric`.
- **Modified** `ChenWeb/web/src/lib/components/home3/metric-ontology-explorer/ontology-canvas.svelte`
  — no logic change beyond `CHAINED_SATELLITES` now including `analysis` (already derived from
  `CHAINS`); verify the chain column renders for the `analysis` satellite position.
- **No** new npm dependency. **One** goose migration. **No** change to
  `GET /kb/metrics/:metric_id/graph` or `metricOntologyExplorerService.getMetricGraph`.
