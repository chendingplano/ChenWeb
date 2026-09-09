> Verification legend: tasks marked "(needs mise dev)" require the full stack
> (Go API + Postgres) and are verified against a running dev server. Everything
> else is verified by `go test` / `bun run check` / `bun test` / an isolated
> in-browser harness of the view.

## 1. Backend — metric-graph store

- [x] 1.1 Added `server/api/kbhandler/metric_graph_store.go` — `metricGraphStore{ DB *sql.DB }`,
      `Load(ctx, recordID int64, metricID string) (MetricGraph, error)`, `MetricGraph`
      (`Metric MetricGraphMetric` + `Nodes map[string]MetricGraphNode`), `MetricGraphNode{ Rows
      []map[string]any }`. Every node id is always present with an array (never null). Small
      scan/shape helpers local to the file.
- [x] 1.2 `object__mention` — `kb.artifact_objects` WHERE `artifact_type='metric' AND
      artifact_id=$1` `LIMIT 200`; fields id, object_name, object_name_en, object_id,
      reconcile_status. Collects the mentions' object_ids for 1.3.
- [x] 1.3 `object__node` — `kb.object_nodes` WHERE `object_id = ANY($1)` (`pq.Array`), skipped
      when there are no object_ids; fields id, object_id, canonical_name, canonical_name_en,
      object_type, reconcile_status.
- [x] 1.4 `keyword__concept` — `kb.keyword_concepts` WHERE `concept_id = $1` from
      `kb.metrics.keyword_concept_id`, skipped when null; fields concept_id, pref_label, status,
      scope, gloss.
- [x] 1.5 `mdef__term` — `SELECT DISTINCT ON (term_id) … FROM kb.ontology_terms WHERE term_id =
      ANY($1) ORDER BY term_id, version DESC` for `metric_definition_term_id` + the
      unit/quantity-kind/assertion-kind term ids carried on the resulting assertion; fields
      term_id, term_kind, module_id, status, definition.
- [x] 1.6 `mdef__contract` — reused `classfoundation.ContractStore.Current(ctx, termID)` for each
      distinct assertion `instance_of_term_id`; fields id, term_id, revision, definition_state,
      effective_from.
- [x] 1.7 `ev__dc` — reused `assertions.DecisionCandidateStore.List` with
      `SourceArtifactType="metric"`, `SourceArtifactID=metricID`; fields id, candidate_kind,
      logical_identity_key, status, resulting_assertion_id. Collects resulting_assertion_ids.
- [x] 1.8 `ev__sa` — `assertions.AssertionStore.GetByID` per resulting_assertion_id; fields id,
      subject, predicate_term_id, object, assertion_kind_term_id, confidence.
- [x] 1.9 `ev__ae` — reused `assertions.EvidenceStore.ListAdmin` with `ArtifactType="metric"`,
      `ArtifactID=metricID` (its default excludes deleted); fields id, assertion_id,
      input_record_id, artifact_object_id, evidence_quote, source_line_spans.
- [x] 1.10 Processor nodes per design D10 (not a run log): `proc__extract` = one
      `kb.metrics`-derived row (model_name, is_explicit_metric, confidence, location_type,
      created_at); `proc__normalize` = the `semantic:stage_normalize` active
      `kb.semantic_processing_outcomes` row(s); `proc__associate` = the `stage_class_resolution`
      + `stage_associate` active rows; `proc__project` = one row per resulting assertion (status,
      value_state, conformance_state, contract-revision id). Empty nodes return `[]`.
- [x] 1.11 `metric` block — first query in `Load`; `SELECT … FROM kb.metrics WHERE
      input_record_id=$1 AND metric_id=$2`; returns `errMetricNotFound` (handler → 404) on
      `sql.ErrNoRows`.
- [x] 1.12 Covered by `metric_graph_handler_test.go` (see 2.3): the httptest-level `200` case
      drives `Load` end to end with every downstream query mocked empty and asserts all twelve
      node keys present as arrays and `proc__extract` carrying its one row. A fully-linked and a
      Phase-D-incomplete fixture are exercised at 7.4 against the dev DB.

## 2. Backend — endpoint + route

- [x] 2.1 Added `server/api/kbhandler/metric_graph_handler.go` — `GetMetricGraph`:
      `EchoFactory.NewFromEcho("CWB_KB_MGRAPH_001")`, `parseMetricID` → 400 (`CWB_KB_MGRAPH_010`)
      with no query on malformed, `canonicalMetricID`, `metricGraphStore.Load`, 404
      (`CWB_KB_MGRAPH_020`) on `errMetricNotFound`, 500 (`CWB_KB_MGRAPH_030`) on other error, else
      200 `{ status:true, metric, nodes }`.
- [x] 2.2 Added `apiGroup.GET("/kb/metrics/:metric_id/graph", kbhandler.GetMetricGraph)` to
      `server/api/routes.go`, immediately after the `/kb/metrics/:metric_id/wiki` line.
- [x] 2.3 `metric_graph_handler_test.go` — httptest cases: malformed id → 400 with
      `mock.ExpectationsWereMet()` (no DB call); unknown metric → 404 (only the `kb.metrics`
      lookup runs); legacy `416_1` → normalised to `416_mtc_1`, 200, all twelve node keys, name
      populated. 3/3 pass.
- [x] 2.4 `go build ./...` clean, `go vet ./api/kbhandler/` clean. `go test ./api/kbhandler/`:
      the 3 new tests pass; the 15 pre-existing failures (ParadeDB/pgvector search-registry,
      `ARTIFACT_WEB_DIR`/summary-graph, topic-category parsing, ontology-candidate fingerprint)
      are unrelated env/infra cases and predate this change.

## 3. Frontend — service + model

- [x] 3.1 `web/src/lib/services/metricOntologyExplorerService.ts` — deleted the five `load*`
      functions, the `LOADERS` map, `RecordRows`, and the `objectManagerService` import; added
      `MetricGraph` / `MetricGraphMetric` / `MetricGraphRow` types and
      `getMetricGraph(metricId: string): Promise<MetricGraph>` (GET
      `/api/v1/kb/metrics/${encodeURIComponent(metricId)}/graph`, same `getJson` error handling);
      keep `Cell`, `dash`, `clip`.
- [x] 3.2 Added `PROJECT: Record<string, (r) => Cell[]>` (keys = chain-node ids); each projector
      shapes a raw graph row into the node's `columns` order using local `dash` / `clip` / `firstOf`
      helpers.
- [x] 3.3 `model.ts` — deleted `loaderKey` and the `LoaderKey` type; `ChainNode` keeps `table` /
      `columns` / `description` + `evidenceSpans`; every node's `columns` updated to match its
      projector (proc nodes retabled per design D10). Top comment updated to name the graph
      endpoint.
- [x] 3.4 `model.test.ts` — dropped the two `loaderKey` tests; added "every chain node has a
      PROJECT entry that fills its columns" (`project({}).length === columns.length`) and "PROJECT
      has no entry without a matching chain node". `bun test …/metric-ontology-explorer/` → 14/14.

## 4. Frontend — view composer

- [x] 4.1 `metric-ontology-explorer-view.svelte` — added `graph` / `graphError` `$state` and a
      cancel-guarded `$effect` keyed on `metricId` calling `getMetricGraph`; both clear when
      `metricId` is empty.
- [x] 4.2 Derived `metric = graph?.metric ?? null` and `counts` (`{ satelliteId:
      nodes[firstChainNodeId].rows.length }` over `CHAINS`); passed both to `OntologyCanvas`.
- [x] 4.3 Passed `metric`, `nodeRows = graph?.nodes ?? null`, `graphError` to `ContentViewer`;
      still passes `metricId` for the empty-state check.
- [x] 4.4 `SourcePane` unchanged (still `{tokens} {metricId} {evidenceSpans} {evidenceLabel}`).

## 5. Frontend — canvas centre node + badges

- [x] 5.1 `ontology-canvas.svelte` — added `metric` and `counts` props (defaulted `null` / `{}`).
- [x] 5.2 Centre node: with a metric, `label = truncateCore(metric_name)`,
      `sub = metric_id`, `w = min(320, max(178, 120 + coreWidthUnits*8.5))` (CJK counted 1.7×);
      full name (+ `metric_name_en` when it differs) in an SVG `<title>`. No metric → literal
      `Metric` / `ONTOLOGY CORE` / `w:178`.
- [x] 5.3 Satellite badge: `n.badge = counts[s.id] > 0 ? counts[s.id] : undefined`; rendered as an
      accent circle + count at the rect's top-right for `sat` nodes only; absent with no metric /
      no count.
- [x] 5.4 `fitView()` unchanged — it bounds all visible nodes' `x ± w/2`, so the wider core is
      framed automatically; ellipse and chain-column math untouched.

## 6. Frontend — content viewer record tabs

- [x] 6.1 `content-viewer.svelte` — added `nodeRows` / `graphError` props (kept `metricId`);
      deleted the `cache` state, the `LOADERS` `$effect`, and the `activeNode.loaderKey` branch;
      added `activeRows` `$derived` = `nodeRows[activeNode.id].rows.map(PROJECT[activeNode.id])`.
- [x] 6.2 Record-tab body renders one of: no `metricId` → "Pick a metric from the *Search* tab"
      (button switches to the Search tab); `graphError` → contained error line; `!nodeRows` →
      "Loading rows…"; `activeRows.length === 0` → "No {table} rows for this metric"; else the rows
      table over `columns` + `activeRows`.
- [x] 6.3 Removed the schema-panel markup and its `.schema` / `.schema-head` styles plus the now
      unused `td.empty` / `code` rules; added a small `.linklike` button style. Rows-table styles
      kept.
- [x] 6.4 Entry tab, Search tab, breadcrumb, and tab open/close/de-dup (owned by the composer) are
      untouched.

## 7. Verification

- [x] 7.1 `bun run check`: 2 errors + 44 warnings, all pre-existing baseline
      (`doc-processor-dashboard-state.test.ts`, `keyword-rewrite-rules-client.test.ts`, home5/home6
      deprecations) — **zero** in any changed file. `web/package.json` unchanged.
- [x] 7.2 `go build ./...` clean; `go vet ./api/kbhandler/` clean; `gofmt -l` clean on the new
      files. `go test ./api/kbhandler/ -run TestGetMetricGraph` → 3/3. Full `go test ./api/kbhandler/`
      has 15 unrelated pre-existing failures (search-registry / `ARTIFACT_WEB_DIR` / topic-parsing)
      — none touch metric-graph. Full `go test ./...` not run here (large; the changed package is
      green bar the documented pre-existing cases).
- [ ] 7.3 In-browser harness of `metric-ontology-explorer-view` with a mocked `getMetricGraph`
      (centre node name+id, per-node rows, no-rows state, rejected-fetch error state, no-`metricId`
      pick-a-metric state). Not run this session — needs the harness wired.
- [ ] 7.4 (needs mise dev) `mise dev`; pick a real metric and walk every spec scenario incl. the
      four `proc__*`, `mdef__contract`, `ev__ae`, `ev__dc` tabs and the satellite badges.
- [ ] 7.5 (needs mise dev) `curl` the endpoint: canonical id, legacy `<record>_<seq>`, malformed
      (400), unknown metric (404).
- [x] 7.6 `openspec validate --changes metric-scoped-explorer --strict` → `✓`.

## 8. Spec lineage housekeeping

- [ ] 8.1 Before archiving either change, drop the superseded requirements from
      `metric-ontology-explorer-page`'s delta spec (the schema-panel record-tab rule and the fixed
      centre-node label) so the promoted `metric-ontology-explorer` spec is consistent; note the
      preferred archive order (sibling first) in that change's tasks.
