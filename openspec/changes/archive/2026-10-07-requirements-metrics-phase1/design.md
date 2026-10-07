## Context

ADR `2026100603` separates requirements from metrics. Phase 1 covers only the work that needs
neither `extract_provisions` changes nor migration of existing rows (review decisions recorded in
the ADR change log).

Current state, verified 2026-10-06:

- `kb.metrics` has no link to `kb.provisions`. `kb.provisions.id` is `BIGINT NOT NULL`, and
  `prov_id` is a text key like `metric_id`.
- `metric_id` is `<record>_mtc_<seq>`. With `force_clear`, `FinalizeChunkBatch` renumbers from 1
  and calls `DeleteMetricsByInputRecordID`. The plain `Force` path also deletes. Benchmark code
  deletes in `doc-benchmark/metric_adapter.go` and `doc-benchmark/store_cleanup.go`.
- `kb.assertion_evidence` links metric occurrences by `(artifact_type='metric', artifact_id =
  metric_id, input_record_id)`. A partial unique index allows one active `supports` row per
  occurrence (`20260818000018`). `metric_lossless_writer.supersedeMetricSupportEvidence` replaces a
  link only when it runs again for the same `metric_id`. Between re-extraction and that run, or
  for a new row that never gets one, the old link silently points at a different metric.
- `EvidenceStore.DeleteEvidence` already soft-deletes one row and moves the assertion to
  `unsupported` when its last support goes (subject to `EvidenceLossTransitionAllowed`).
- The web metric types already carry `value_class`, `value_range_type`, `formula_or_definition`
  and `reasoning_tags`, which is everything the classifier needs.

## Goals / Non-Goals

**Goals:**
- `kb.metrics.provision_id` exists, is safe under provision re-extraction, and is exposed by the
  metric read APIs.
- After any deletion of a record's metric rows, no active metric evidence for that record remains.
- Customer-facing metric views label each row with its statement kind, in both languages.

**Non-Goals:**
- Filling `provision_id`. This waits for the `extract_provisions` change.
- Repairing or re-linking evidence that is already stale. Existing records are out of scope; they
  heal when re-extracted, because the retire step runs then.
- Changing `metric_id`'s format, the prompts, the ontology class logic, or admin-only views.
- Filtering or counting by kind on the server.

## Decisions

### D1 — `provision_id` references the surrogate key, nulls on delete

```sql
-- +goose Up
ALTER TABLE kb.metrics
  ADD COLUMN IF NOT EXISTS provision_id BIGINT NULL
    REFERENCES kb.provisions (id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_metrics_provision_id
  ON kb.metrics (provision_id) WHERE provision_id IS NOT NULL;
-- +goose Down
DROP INDEX IF EXISTS kb.idx_metrics_provision_id;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS provision_id;
```

- **Why `id`, not `prov_id`:** `prov_id` is a text key with the same reuse risk as `metric_id`.
  A foreign key on a reused key would recreate the evidence defect.
- **Why `ON DELETE SET NULL`:** `extract_provisions` deletes all rows of a record before
  re-extracting. A cascade would delete metrics; a restrict would block provision re-extraction.
- `MetricsSQLStore.ensureMetricsTable` also runs `ALTER TABLE … ADD COLUMN IF NOT EXISTS` for
  older columns. Add the column there too, matching that pattern, but without the foreign key:
  DDL inside the store must not depend on `kb.provisions` existing. The migration owns the
  constraint.
- The insert paths do not write the column, so it defaults to null. The read queries and
  JSON responses that list metric columns add `provision_id` (nullable number).

### D2 — Retire evidence by record before deleting metric rows

Add `EvidenceStore.RetireMetricEvidenceForRecord(ctx, inputRecordID, actor, reason) (int, error)`:

1. Select the ids of active evidence rows with `artifact_type = 'metric' AND input_record_id = $1
   AND NOT deleted`.
2. Call the existing `DeleteEvidence` for each id, so the "last support → unsupported" transition
   and its reason are recorded exactly as for a single deletion.
3. Return the count. The callers log it with the record ID.

Call it **before** the delete in `MetricsSQLStore.DeleteMetricsByInputRecordID`, which covers
the `Force` and `force_clear` paths, with reason `"metric rows deleted for re-extraction"`.

The two benchmark delete sites are left unchanged (decided during implementation).
`store_cleanup.go` always deletes the benchmark's own input right after its metrics, and
`kb.inputs` IDs are never reused, so its `metric_id` values cannot recur. `MetricAdapter.Cleanup`
has no non-test caller. Neither can cause the reuse defect. Evidence left by a deleted input falls
under the existing input-deletion retention policy (spec §10.12).

- **Why per-row `DeleteEvidence` and not one bulk `UPDATE`:** the state transition and its audit
  reason live in `DeleteEvidence`. A bulk update would bypass them, leaving assertions `active`
  with no support. Volume is small (at most a few hundred rows per record).
- **Why before, not after:** if the metric delete then fails, the evidence is retired but the
  metrics remain. The next lossless-writer run finds no active support and adds fresh links, so
  this order is self-healing. The reverse order can leave live links to deleted rows.
- **Why not in one transaction:** `DeleteEvidence` and the assertion store use their own `*sql.DB`,
  not a transaction. Making them transactional is a larger refactor that the ordering argument
  makes unnecessary.
- **Alternatives rejected:** changing `metric_id` to a non-reused form (it is a public identifier
  in URLs, the wiki, graph and review code); keying evidence by `kb.metrics.id` (touches decision
  candidates, the normalizer and every evidence reader); a database trigger (spec §10.12 forbids
  database-level cascades for evidence, and a trigger cannot run the state transition).

### D3 — A pure, shared web classifier

New module `web/src/lib/metric-statement-kind.ts`:

```ts
export type StatementKind =
  | 'requirement_with_criterion'  // requirement/target + lower/upper/exact/range
  | 'requirement_value_open'      // requirement + limit_absent
  | 'inspection_requirement'      // requirement + qualitative, no citation
  | 'delegated_requirement'       // reference class, or external_reference/cited_doc tags, non-numeric
  | 'test_parameter'              // reasoning_tags has test_condition
  | 'metric_definition'           // definition + formula_or_definition
  | 'definition'                  // definition without a formula (term definitions)
  | 'metric_value'                // observation / design_capability / numeric reference
  | 'observation'                 // non-numeric observation or design_capability
  | 'unclassified';
export type StatementGroup = 'requirement' | 'metric' | 'test' | 'definition' | 'unclassified';
export function classifyMetricStatement(row: {...}): { kind: StatementKind; group: StatementGroup };
```

The rules are evaluated in order. Values are lower-cased and trimmed first, and `reasoning_tags`
accepts `string[]`, a JSON string or unknown.

1. `test_condition` tag → `test_parameter`.
2. `value_class = definition` → `metric_definition` if `formula_or_definition` is non-empty,
   else `definition`.
3. `value_class = reference`, or a `cited_doc:`/`external_reference` tag: numeric range type →
   `metric_value`, otherwise → `delegated_requirement`.
4. `value_class` in {`requirement`, `target`}: numeric → `requirement_with_criterion`,
   `limit_absent` → `requirement_value_open`, `qualitative` → `inspection_requirement`.
5. `value_class` in {`observation`, `design_capability`}: numeric → `metric_value`, else
   `observation`.
6. Anything else → `unclassified`.

Numeric means `lower_bound | upper_bound | exact | range`.

- **Why web-side:** Phase 1 only labels rows. One pure function with unit tests is the smallest
  change. A server-side classifier would touch every metric endpoint. If a later phase needs
  filtering or counts, the rules move to Go with the same test table.
- **Why `target` counts as a requirement:** a target is a normative level the source sets ("应达到"),
  which an expert reads as a requirement. It stays distinguishable through `value_class`.
- **Why unknown values are `unclassified`, not "metric":** mislabelling a requirement as a metric
  is the defect this change exists to fix.

### D4 — Labels in customer-facing views

A small `StatementKindBadge.svelte` (one component, no text of its own beyond message keys) shows
the group, with the kind as its `title`. It is placed:

- in `metric-wiki-view.svelte` next to the metric name;
- in `artifact-category-panel.svelte` in the selected-item details, above the existing class row;
- in `metric-ontology-explorer/metric-search-pane.svelte` beside the existing `value_class` tag;
- in `metric-detail-groups.ts` as a "Statement kind" attribute next to `value_class`.

Added during implementation: the Explorer search response (`metric_search_handler.go`) did not
carry `value_range_type`, `formula_or_definition` or `reasoning_tags`, so it now returns them. The
wiki page is LLM-generated and cached and has no row fields, so `GET /kb/metrics/:id/wiki` now
adds a live, best-effort `statement` object (one indexed lookup; omitted on failure, never written
into the cached page). The badge is shown through a new optional `titleAside` snippet on
`artifact-wiki-article-view.svelte`. The category item gained `formula_or_definition`. Labels live
in `web/src/lib/metric-statement-kind-labels.ts`, shared by the badge and `metric-detail-groups.ts`.

Message keys: `metric_statement_group_<group>` and `metric_statement_kind_<kind>`, in both
`en.json` and `zh-cn.json`. Chinese wording: 要求 for requirement and 指标 for metric (ADR DR5), for
example `metric_statement_group_requirement` = 要求, `…_metric` = 指标, `…_test` = 试验参数,
`…_definition` = 定义. The badge text must stay selectable. Do not use `select-none`.

## Risks / Trade-offs

- [Benchmark-only tags] `test_condition`, `cited_doc:` and `external_reference` are set reliably by
  the benchmark skill and the v7+ prompt, but older production rows may lack them. → Those rows fall
  through to `value_class` rules. The worst case is a test setting shown as
  `requirement_with_criterion`. That is still a requirement, which is acceptable.
- [Retire makes assertions unsupported] After a re-extraction, the affected assertions read as
  `unsupported` until the lossless writer re-runs. → This is correct: their source is gone. The
  state and reason are visible in existing assertion views.
- [`provision_id` unused for now] A column that is always null can look like dead schema. → The
  column comment names ADR 2026100603 DR2, and the spec states the null-until-linked contract.

## Migration Plan

1. Apply the Goose migration (additive, through the normal startup migration path). Rollback is
   the `Down` block. No data depends on the column yet.
2. Deploy the server. The retire step takes effect on the next metric deletion for any record.
3. Deploy the web build. Labels appear immediately for all rows, existing ones included, because
   classification happens at read time.

## Open Questions

- Should admin-only views (`metric-mgmt-view.svelte`) also show the badge? This design leaves them
  unchanged. Adding the badge there is a one-line follow-up if wanted.
