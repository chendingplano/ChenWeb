## 1. Backend struct and queries (`server/api/kbhandler/metrics_handler.go`)

- [x] 1.1 Add `KeywordConceptID *string \`json:"keyword_concept_id,omitempty"\`` and
      `MetricDefinitionTermID *string \`json:"metric_definition_term_id,omitempty"\``
      to the `metricRecord` struct (near `MetricID`/`ValueRangeTypeError`, lines 23-68)
- [x] 1.2 In `ListMetrics`'s query (lines 150-179), add
      `m.keyword_concept_id, m.metric_definition_term_id, m.value_range_type_error` to
      the `SELECT` list
- [x] 1.3 In `ListMetrics`'s `rows.Scan` (lines 201-210), add
      `&r.KeywordConceptID, &r.MetricDefinitionTermID, &r.ValueRangeTypeError` matching
      the new SELECT column order
- [x] 1.4 In `fetchMetricByID`'s query (lines 263-291), add
      `m.keyword_concept_id, m.metric_definition_term_id, m.value_range_type_error` to
      the `SELECT` list
- [x] 1.5 In `fetchMetricByID`'s `QueryRow(...).Scan` (lines 301-310), add
      `&r.KeywordConceptID, &r.MetricDefinitionTermID, &r.ValueRangeTypeError` matching
      the new SELECT column order
- [x] 1.6 (not in original plan) Update `metrics_handler_test.go`'s sqlmock
      expectations (`TestUpdateMetricSuccess`, `TestListMetricsSortsByFirstSourceLineSpan`)
      to match the new SELECT column list and row shape — both tests broke against the
      new query without this

## 2. Frontend type (`web/src/lib/services/kbService.ts`)

- [x] 2.1 Add `metric_id?: string | null`, `keyword_concept_id?: string | null`,
      `metric_definition_term_id?: string | null`, `value_range_type_error?: string |
      null` to the `KbMetricRecord` type (lines 171-207)

## 3. Frontend display (`web/src/lib/components/home3/metric-mgmt-view.svelte`)

- [x] 3.1 In `buildMetricGroupAttrs`'s `metadata` array (lines 585-611), add four
      `textAttr(...)` entries — `metric_id` ("Metric ID"), `keyword_concept_id`
      ("Keyword Concept ID"), `metric_definition_term_id` ("Definition Term ID"),
      `value_range_type_error` ("Range Type Error") — each using `fmt(m.<field>)` /
      `has(m.<field>)` like the existing entries, placed after `explicit` so the
      existing seven entries keep their order and satellite positions. Used key
      `metric_artifact_id` (not `metric_id`) for the new "Metric ID" entry since
      `metric_id` was already taken by the existing "ID" entry (`m.id`).

## 4. Verification

- [x] 4.1 `cd server && go build ./... && go test ./api/kbhandler/...` — all metric
      tests pass; remaining 17 failing tests in the package are pre-existing
      (confirmed identical failure set with these changes stashed out)
- [x] 4.2 `cd web && bun run check` — no errors/warnings in either changed file; the
      one pre-existing error (`doc-processor-dashboard-state.test.ts`) is unrelated
- [ ] 4.3 Open Knowledge System → Metrics with `input_record_id=416`, select metric id
      32123 ("阳光房堆肥设施单室体积", has `keyword_concept_id` = `kwc_67d1b0596471` and
      `metric_definition_term_id` = `measurement:kwc_67d1b0596471` per a direct
      read-only query against the dev DB), confirm the Metadata group shows both;
      confirm a metric without these fields renders the group unchanged from today.
      NOT completed by the agent — the API route requires interactive Google OAuth
      login (`authmiddleware.AuthMiddleware` on `apiGroup`), which can't be scripted
      headlessly; needs a manual check in a logged-in browser session.
