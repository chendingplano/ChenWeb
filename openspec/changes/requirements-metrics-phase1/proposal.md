## Why

Customers read our results as an expert's work. Today every row of `kb.metrics` is shown as a
"metric", although about half are requirements with nothing to measure (for example "垃圾转运车辆
应密闭"). ADR `2026100603` (Separate Requirements from Metrics) fixes the target model. This change
is its Phase 1: the parts that need no `extract_provisions` work and no migration of existing rows.

It also fixes a defect found while writing the ADR. Forced re-extraction deletes a record's metrics
and renumbers `metric_id` from `<record>_mtc_1`. Active `kb.assertion_evidence` rows keep pointing
at those reused IDs, so they silently attach old semantic assertions to unrelated new metrics. On
the two records that have metric evidence, every link is stale.

## What Changes

- Add a nullable `kb.metrics.provision_id` column referencing `kb.provisions(id)`
  `ON DELETE SET NULL`, with a partial index. It is not filled by any processor yet, and existing
  rows are not backfilled. It is the future link from a requirement's criterion to its provision
  clause (ADR DR2).
- When a record's metric rows are deleted for re-extraction, retire their
  active metric evidence through the existing evidence soft-delete path. Assertions that lose
  their last support become `unsupported` with a recorded reason. New metrics never inherit
  evidence by `metric_id` reuse.
- Add a read-time **statement-kind classifier** that maps a metric row to what an expert would
  call it: a requirement (with a criterion, with an open value, inspection-only, or delegated to a
  standard), a metric value, a test-method parameter, a metric definition, or another definition.
  It derives the kind from existing fields only and stores nothing.
- Show the kind as a label in the customer-facing metric views, in English and Chinese, so
  requirement rows stop being presented as metrics.

## Capabilities

### New Capabilities
- `metric-provision-link`: the `kb.metrics.provision_id` column, its referential behaviour and its
  null-until-linked contract.
- `metric-statement-kind`: classifying metric rows into statement kinds and showing that kind in
  customer-facing views.

### Modified Capabilities
- `metric-supporting-evidence-cardinality`: adds the requirement that deleting a metric
  occurrence retires its active supporting evidence, so a reused `metric_id` cannot inherit it.

## Impact

- **Database:** one Goose migration in `project_migrations/` (additive column, foreign key and
  index). No data rewrite.
- **Server:** `api/doc-processing/extract-metrics.go` (`MetricsSQLStore.DeleteMetricsByInputRecordID`)
  and `api/ontology/assertions/evidence_store.go` (a
  record-scoped retire helper built on `DeleteEvidence`). Also the `kb.metrics` read queries and
  response structs that should expose `provision_id`.
- **Web:** a new pure module under `web/src/lib/` for the classifier with unit tests. Labels go in
  `metric-wiki-view.svelte`, `artifact-category-panel.svelte`,
  `metric-ontology-explorer/metric-search-pane.svelte` and `metric-detail-groups.ts`. New
  Paraglide messages are added to both `messages/en.json` and `messages/zh-cn.json`.
- **Out of scope:** `extract_provisions`, prompt changes, filling `provision_id`, re-linking or
  cleaning existing evidence, and ontology class changes (ADR Phases 2–5).
