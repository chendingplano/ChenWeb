## Context

The Metric Ontology Explorer (`home3/knowledge → Ontology → Metric Ontology Explorer`) is a
graph-first workspace: a canvas orrery (`ontology-canvas.svelte`), a tabbed content viewer
(`content-viewer.svelte`), and a source-document pane, composed by
`metric-ontology-explorer-view.svelte`. Its data model (`model.ts`) is a fixed `Metric` centre,
nine satellites, and — for seven of them — an unfoldable downstream chain of "chain nodes". Each
chain node opens a **record tab** that shows rows from a single aggregate read,
`GET /api/v1/kb/metrics/:metric_id/graph` (`metric_graph_store.go`), keyed by chain-node id and
shaped client-side by `PROJECT` in `metricOntologyExplorerService.ts`. The `Analysis` satellite
(`id: 'analysis'`) has **no chain** today — it is one of `document / product / analysis / misc`.

The governed class machinery this change reads from went live in `metric-class-contracts` (jj
`be4d`): every metric class term now gets a real `kb.ontology_class_contract_revisions` row
(`identity_only`, promoted to `partially_defined` by
`classfoundation.SynthesizeContractFromObservations` on unambiguous multi-document agreement),
with the current revision pointed to by `kb.ontology_term_headers.current_contract_revision_id`.
A metric's class is **not** a column on `kb.metrics`; it is reached through
`kb.semantic_decision_candidates` (`source_artifact_type='metric'`, `source_artifact_id=<metric_id>`)
→ `resulting_assertion_id` → `kb.semantic_assertions.instance_of_term_id`. `metric_graph_store.go`
already walks exactly this path for its `mdef__contract` node.

Hybrid search in this codebase means Reciprocal Rank Fusion (`rrfK = 60`) of a lexical candidate
list (`ts_rank_cd` over a tsvector, or ParadeDB BM25) and a semantic candidate list (pgvector
`embedding <=> query::vector` cosine), as implemented for `kb.search_artifacts` in
`search_registry.go` (`queryHybridSearchResults`) and, on-the-fly, in
`artifact_indexing.go` (`FindSimilarArtifactsOnTheFly`). `kb.search_artifacts` is **record-
partitioned** (`input_record_id`) and **instance-grained** (one row per metric / topic / …), gated
by `SEARCH_SEMANTIC_ENABLED`; embeddings are `vector(1536)` (`text-embedding-3-small`), computed
best-effort by `computeQueryEmbedding` / `newSearchQueryEmbedder` from `EMBEDDING_MODEL_NAME`.
**No ontology table carries an embedding.**

Constraints (ChenWeb `CLAUDE.md`): minimum code, surgical edits, match existing style, no
speculative abstraction, no hard-coded prompts, every schema change via a goose migration. The
workspace runs a live `mise dev` / air server that auto-applies new migrations.

## Goals / Non-Goals

**Goals:**

- The `Analysis` satellite unfolds a two-node chain — `Metrics of Same Class`,
  `Metrics of Similar Classes` — behaving like the other chained satellites (canvas unfold/fold,
  breadcrumb, a record tab per node).
- A read endpoint `GET /api/v1/kb/metrics/:metric_id/related-metrics?scope=&limit=` returning a
  ranked metric list for one of the two cohorts of the selected metric.
- `same_class` = every metric resolving to the selected metric's class term (cap 200).
- `similar_class` = top **N (default 20)** metrics that are instances of class contracts
  **hybrid-matched** (RRF lexical + pgvector) as similar to the selected metric's class contract.
- A dedicated class-contract hybrid-search index (`kb.ontology_class_contract_search`) — one row
  per governed class term, tsvector + `vector(1536)` — kept current on contract-revision
  create/promote and backfillable via an admin endpoint.
- Each result row is clickable and recenters the whole explorer on that metric
  (`?metric_id=` deep-link, the same mechanism the Search tab uses).

**Non-Goals:**

- No change to `GET /api/v1/kb/metrics/:metric_id/graph`, its store, or the twelve existing chain
  nodes.
- No "similar class contracts" tab in its own right (pre-instance-expansion) — the cohort tabs
  list **metrics**, not classes. (Open Question.)
- No ParadeDB/pg_search BM25 index on the contract index (tsvector is enough at this corpus size;
  Open Question if it grows).
- No curation UI for the match results, no persisted metric↔metric edges (the list is computed
  live, like `FindSimilarArtifactsOnTheFly`).
- No `can_compare` capability work — "similar" here is contract-document similarity, not a
  comparability guarantee (that is DR22, still unbuilt).
- No change to the `Analysis` entry-tab prose, the `Metric Ontology` dashboard, or
  `metricOntologyAnalysisService`.

## Decisions

### D1. Lazy per-tab fetch, not folded into `metric_graph`

The two Analysis tabs fetch their own data on open (keyed `(metric_id, scope)`), rather than
`metric_graph_store` gaining two more node keys.

- **Chosen** because `similar_class` issues an embedding call and two hybrid queries — putting it
  on every metric selection (which is what `/graph` is) would tax the canvas critical path for a
  tab the reader may never open. `same_class` is cheap but pairing the two on one mechanism keeps
  the content-viewer branch single. `/graph` stays byte-for-byte unchanged, so the
  `metric-scoped-explorer` change's endpoint and its tests are untouched.
- **Alternative — add `analysis__same_class` / `analysis__similar_class` to `/graph`.** Rejected:
  couples an expensive path to the eager read; `metric_graph_store.Load` would need an embedder
  dependency; the per-satellite count badge (derived from `/graph` node rows) would then fire an
  embedding call just to show a number.

### D2. One endpoint, `scope` selector, uniform row shape

`GET /api/v1/kb/metrics/:metric_id/related-metrics`

| param | values | default |
| --- | --- | --- |
| `scope` | `same_class`, `similar_class` | — (400 if missing/other) |
| `limit` | 1..200 | `20` for `similar_class`, `200` for `same_class` |

`metric_id` accepts the canonical `<record_id>_mtc_<seq>` and legacy `<record_id>_<seq>` forms
(reuse `parseMetricID` / `canonicalMetricID`), 404 on an unknown metric, 400 on a malformed id —
consistent with `/wiki` and `/graph`.

```jsonc
{
  "status": true,
  "metric_id": "416_mtc_1",
  "scope": "similar_class",
  "class_term_id": "measurement:collection_frequency_x7a1",   // the SELECTED metric's class; null if unresolved
  "results": [
    {
      "metric_id": "812_mtc_3",
      "metric_name": "…", "metric_name_en": "…",
      "input_record_id": 812,
      "class_term_id": "measurement:pickup_interval_9c2f",     // THIS row's class
      "class_label": "measurement:pickup_interval_9c2f",       // best available label; falls back to term_id
      "metric_value": "…", "metric_unit": "…",
      "source_filename": "std_1503937.pdf",
      "score": 0.031,                                          // similar_class only: matched-class RRF score
      "matched_class_term_id": "measurement:pickup_interval_9c2f"  // similar_class only
    }
  ]
}
```

For `same_class`, `score` / `matched_class_term_id` are omitted. The frontend `PROJECT` maps this
one shape to each node's `columns`.

- **Alternative — two endpoints.** Rejected: near-identical parse / resolve / shape code; one
  handler with a `switch scope` is smaller and matches how `SearchMetrics` handles its variants.

### D3. Resolve the selected metric's class once; no class is a state, not an error

The handler resolves `class_term_id` for the selected metric via the
`decision_candidates → resulting_assertion_id → semantic_assertions.instance_of_term_id` path
(one query; the `DISTINCT` non-empty `instance_of_term_id` among the metric's resulting
assertions — in practice one). If the metric has **no** resolved class (deferred candidate, no
assertion, Phase D never completed):

- `class_term_id: null`, `results: []`, HTTP 200.
- The record tab shows "This metric has no resolved governed class yet — no peers to show",
  distinct from "no rows" and from an error.

This mirrors `metric_graph_store`'s treatment of empty nodes (present, empty, not omitted).

### D4. `same_class` query

```sql
SELECT m.metric_id, m.metric_name, m.metric_name_en, m.input_record_id,
       m.metric_value, m.metric_unit, COALESCE(i.staging_filename,'') AS source_filename
FROM kb.semantic_assertions a
JOIN kb.semantic_decision_candidates dc
     ON dc.resulting_assertion_id = a.id
    AND dc.source_artifact_type = 'metric'
JOIN kb.metrics m ON m.metric_id = dc.source_artifact_id
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE a.instance_of_term_id = $1        -- selected metric's class term
  AND m.metric_id <> $2                 -- exclude the selected metric
GROUP BY m.metric_id, m.metric_name, m.metric_name_en, m.input_record_id,
         m.metric_value, m.metric_unit, i.staging_filename
ORDER BY m.input_record_id, m.metric_id
LIMIT $3                                -- default 200
```

`GROUP BY` dedupes a metric that reached the class through more than one assertion. `class_term_id`
on every row is `$1` (all identical for this scope). Ordering is stable, not relevance-ranked —
the set is a definite peer group, not a search result.

### D5. `similar_class` = match contracts, then expand to their instances

1. **Match** — `classcontractsearch.MatchSimilar(ctx, db, selectedClassTermID, K)` runs the RRF
   query over `kb.ontology_class_contract_search` (D6) using the selected class's own indexed row
   as the query (its `search_document` for the lexical half, its stored `embedding` for the
   semantic half — falling back to embedding `search_document` on the fly if the stored vector is
   absent, exactly as `FindSimilarArtifactsOnTheFly` does). Excludes the selected class itself.
   Returns up to `K = 30` `{class_term_id, rrf_score}` ordered by score.
2. **Expand** — list metrics whose resolved class term is in that matched set (same
   `assertions → decision_candidates → metrics` join as D4, `a.instance_of_term_id = ANY($classes)`),
   carrying each metric's own `class_term_id`.
3. **Rank & cut** — order metrics by their matched class's `rrf_score` desc, then
   `input_record_id`, `metric_id`; dedupe by `metric_id` (keep the highest-scoring class);
   exclude any metric whose class term equals the selected metric's class (those are the
   `same_class` tab's job); take the top `limit` (default 20).

`K = 30` candidate classes bounds the expansion; a per-class instance fan-in cap
(`limit` rows scanned per class, ordered by `input_record_id`) keeps a huge class from crowding
out the rest before the final sort.

- **Alternative — search sibling *metrics* directly (over `kb.search_artifacts`) and group hits by
  class.** Rejected per the change's explicit decision: the requested behaviour is
  "hybrid search of the **class contracts**", and a contract-level index also answers "which
  classes are near this one" for future callers (comparison matrix, curation).
- **Alternative — structured `contract_payload` facet overlap, no embeddings.** Rejected: payloads
  are mostly `{}` / one `(value_type, unit)` pair today; too little signal.

### D6. A dedicated `kb.ontology_class_contract_search` table, not `kb.search_artifacts`

```sql
CREATE TABLE kb.ontology_class_contract_search (
    class_term_id                 TEXT PRIMARY KEY,
    current_contract_revision_id  BIGINT,
    definition_state              TEXT NOT NULL DEFAULT 'identity_only',
    module_id                     TEXT,
    search_document               TEXT NOT NULL DEFAULT '',
    search_vector                 tsvector,
    embedding_text                TEXT,
    embedding                     vector(1536),
    instance_count                INT NOT NULL DEFAULT 0,
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_occs_search_vector ON kb.ontology_class_contract_search USING gin (search_vector);
CREATE INDEX idx_occs_embedding_hnsw ON kb.ontology_class_contract_search
    USING hnsw (embedding vector_cosine_ops);
```

- **Chosen** over reusing `kb.search_artifacts` because that table is partitioned by
  `input_record_id` and its rows are per-instance; a class term is global and has no record. Its
  writer (`kbsearch.InsertSearchRegistryRows`) and per-record reindex/delete helpers
  (`DeleteSearchRegistryRowsForRecord`) are all record-scoped. Shoehorning a
  `artifact_type='class_contract'` with a sentinel `input_record_id` would fight every one of
  those. The class corpus is small (hundreds today, a few thousand at most), so a plain table
  with a GIN + HNSW index is ample.
- The RRF query is a trimmed copy of `queryHybridSearchResults`'s postgres branch: a `lexical`
  CTE (`ROW_NUMBER() OVER (ORDER BY ts_rank_cd(search_vector, plainto_tsquery('simple',$1)) DESC)`),
  a `semantic` CTE (`ORDER BY embedding <=> $2::vector`), `FULL OUTER JOIN`, score
  `1/(60+lex.rnk) + 1/(60+sem.rnk)`. `hybridCandidateLimit = 200` per side is already larger
  than the whole corpus.
- **No ParadeDB path.** `SEARCH_LEXICAL_BACKEND` governs `kb.search_artifacts`; this index just
  uses `simple` tsvector. If the corpus ever justifies BM25 that is an additive follow-up.

### D7. `search_document` composition

Concatenate, newline-separated (all lowercased for the tsvector; the raw form kept in
`embedding_text`):

1. `class_term_id` verbatim (the slug carries signal, e.g. `measurement:collection_frequency_x7a1`).
2. `module_id`.
3. The class term's `definition` / `scope` from the latest `kb.ontology_terms` row for
   `class_term_id`, if any (`DISTINCT ON (term_id) … ORDER BY version DESC`, as
   `metric_graph_store.loadOntologyTerms` does).
4. `contract_payload` facets flattened: `value_type`, each `permitted_unit_term_ids` entry
   (`definition_state` too).
5. Up to ~20 distinct `metric_name` / `metric_name_en` / `metric_subject` values of metrics
   resolved to this class (the join from D4). **This is what makes classes comparable while
   contract payloads are still sparse** — two "collection frequency" classes look alike mostly
   through their instances' wording today.

`embedding` is computed best-effort (`SEARCH_SEMANTIC_ENABLED` true **and** `EMBEDDING_MODEL_NAME`
set); otherwise the row is lexical-only and the RRF query degrades to the lexical CTE. Same posture
as `kb.search_artifacts`.

### D8. Index freshness: reindex after the metric write commits, plus a backfill

- **Backfill** — `POST /api/v1/kb/ontology/class-contracts/backfill-search?limit=&reembed_all=`,
  a near-copy of `BackfillSearchEmbeddings`: iterate `kb.ontology_term_headers` where
  `term_kind='class'`, `classcontractsearch.Reindex` each, report
  `scanned/embedded/skipped/failed/remaining`. This is how the index is first populated and how it
  is repaired.
- **Incremental** — after Phase D's metric semantic transaction **commits**, the caller reindexes
  the class term(s) touched by that write (`EnsureHeader` / `SynthesizeContractFromObservations`
  ran inside the tx; the reindex, which does network I/O for the embedding, must not). Concretely:
  `writeMetricLossless` already returns the assertion; its caller collects the
  `instance_of_term_id`(s) for the batch and calls `classcontractsearch.Reindex` per class after
  commit, best-effort and logged (a failure never fails the metric write). If threading a return
  value up is intrusive, the fallback is a small post-index sweep in the doc-processor step that
  reindexes classes whose `current_contract_revision_id` changed since
  `kb.ontology_class_contract_search.updated_at` — same effect, looser coupling. The exact call
  site is a task; the **requirement** is only "the index is kept current and a backfill exists".
- **Never inside `a.DB.BeginTx` in `writeMetricLossless`.**

### D9. Frontend wiring

- `model.ts` — add `CHAINS.analysis`:

  ```ts
  analysis: [
    { id: 'analysis__same_class',    label: 'Metrics of Same Class',    glyph: 'stage',
      table: 'kb.metrics · same class',    columns: ['metric id','name','value','unit','document'],
      description: 'Every metric whose governed class is the one this metric resolved to.',
      related: 'same_class' },
    { id: 'analysis__similar_class', label: 'Metrics of Similar Classes', glyph: 'stage',
      table: 'class-contract hybrid search', columns: ['metric id','name','class','matched via','score'],
      description: 'Metrics in class contracts hybrid-matched as similar to this metric’s class contract.',
      related: 'similar_class' },
  ]
  ```

  `related?: 'same_class' | 'similar_class'` is the new optional `ChainNode` field marking a node
  as lazy-fetched. `CHAINED_SATELLITES` / `chainFor` already derive from `CHAINS`, so the canvas
  unfolds `analysis` with no canvas-code change.
- `metricOntologyExplorerService.ts` — `getRelatedMetrics(metricId, scope, limit?)` (a `getJson`
  call, same helper as `getMetricGraph`), plus `PROJECT['analysis__same_class']` /
  `PROJECT['analysis__similar_class']` shaping the D2 row into the columns above.
- `content-viewer.svelte` — when `activeNode?.related` is set, take a lazy branch: a small
  `$state` cache keyed `${metricId}::${scope}` with `loading | error | rows`, fetched in an
  `$effect` on `(metricId, activeNode.related)`; render with the existing table shell and the same
  three-state pattern (`no metric` / `loading` / `error` / `no rows` / table). Rows in this branch
  are wrapped in a `<button class="row-link">` calling `onpickmetric(row.metric_id)` — the prop is
  already passed in from the view for the Search tab.
- `metric-ontology-explorer-view.svelte` — no change: `onpickmetric` already deep-links
  `?metric_id=`; `counts['analysis']` stays `undefined` (the badge deriver only looks at `/graph`
  node rows), so the `Analysis` satellite simply shows no badge.

### D10. `error-code` prefixes and route placement

- `related_metrics_handler.go` — `CWB_KB_RELM_0xx`, route
  `apiGroup.GET("/kb/metrics/:metric_id/related-metrics", kbhandler.GetRelatedMetrics)` right
  after the `/graph` line (routes.go ~558).
- `class_contract_search_backfill_handler.go` — `CWB_KB_CCSB_0xx`, route
  `apiGroup.POST("/kb/ontology/class-contracts/backfill-search", kbhandler.BackfillClassContractSearch)`
  next to `POST /kb/search/backfill-embeddings`.

## Risks / Trade-offs

- **Sparse contract payloads → weak `similar_class` ranking until the corpus matures.** →
  Mitigation: D7 point 5 folds instance-metric wording into the lexical document, which is the
  strongest signal available now; the semantic half improves automatically as more classes are
  promoted and re-embedded; `same_class` (the deterministic tab) is unaffected.
- **Embedding model off / not configured (`SEARCH_SEMANTIC_ENABLED` false).** → The index rows are
  lexical-only and `MatchSimilar` runs the lexical CTE alone. The endpoint still returns a ranked
  list, just tsvector-ranked. Documented, not an error.
- **A class with thousands of instances dominates the expansion.** → K=30 candidate classes and a
  per-class fan-in cap before the final sort; final `limit` default 20.
- **Incremental reindex hook missed for a class.** → That class is absent from / stale in the
  index, so it may not surface as "similar"; `backfill-search` (or the `updated_at` sweep) repairs
  it. Never blocks a metric write.
- **HNSW index on a tiny table is arguably overkill.** → Kept for parity with
  `20260603000001` and because the corpus will grow; a seq-scan cosine sort would also be fine and
  the query planner will pick sensibly.
- **`kb.metrics.metric_id` join in D4/D5.** `dc.source_artifact_id` holds the canonical metric_id
  string; confirm it is always the `_mtc_` form (it is, post `metric-scoped-explorer`), else
  normalise in SQL.
- **Row-click navigation re-runs the whole `/graph` fetch for the new metric.** → Expected and
  already how the Search tab behaves; acceptable.
- **Spec lineage.** `metric-scoped-explorer` (which owns the `Analysis` satellite and the
  record-tab renderer) is not yet promoted to `openspec/specs/`. This change adds a **new**
  capability rather than a MODIFIED delta; at archive time the promoted
  `metric-ontology-explorer` spec must absorb the `Analysis` chain as a non-conflicting addition.
  Called out as a task, mirroring how `metric-scoped-explorer` handled its overlap with
  `metric-ontology-explorer-page`.

## Migration Plan

1. Land the goose migration
   `project_migrations/<ts>_create_kb_ontology_class_contract_search.sql` (Up: table + GIN + HNSW;
   Down: `DROP TABLE`, leave the `vector` extension). The live `mise dev` / air server applies it
   on reload.
2. Ship the `classcontractsearch` package, the two handlers, the two routes, and the post-commit
   reindex hook.
3. Run `POST /api/v1/kb/ontology/class-contracts/backfill-search?limit=500` repeatedly until
   `remaining=0` to populate the index (and again with `reembed_all=true` after the embedding
   model is confirmed configured).
4. Ship the frontend hunks (`model.ts`, service, `content-viewer.svelte`). Additive — the other
   satellites and `/graph` are untouched.

**Rollback:** revert the frontend hunks and the Go files, drop the two routes, run the migration's
Down (`DROP TABLE kb.ontology_class_contract_search`). The explorer returns to an `Analysis`
satellite with no chain; nothing else regresses.

## Open Questions

- **A "Similar Class Contracts" tab of its own** (classes, not their instances) — natural once the
  index exists; deferred until asked for.
- **ParadeDB BM25 on the contract index** if the class corpus grows past where `simple` tsvector
  ranks well.
- **`class_label` source** — class terms have no reliable human label today (`class_term_id` is a
  slug; `kb.ontology_terms.definition` is often null for synthesized classes). v1 falls back to
  the term_id. Revisit if a class-label projection lands.
- **Tuning** — `K = 30` candidate classes, `minCosine` for the semantic half (start from
  `metricConnectMinCosine()` = 0.75), default `N = 20`. All env-overridable? Start hard-coded with
  a named const, promote to env only if tuning demands it.
- **Reindex hook site** — thread `instance_of_term_id` up from `writeMetricLossless`'s caller vs.
  an `updated_at`-vs-`current_contract_revision_id` sweep in the doc-processor post-index step.
  Pick during implementation; the sweep is the lower-risk default.
