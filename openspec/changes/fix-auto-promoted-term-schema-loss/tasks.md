## 1. Migration

- [x] 1.1 Create `project_migrations/20260822000001_replace_ontology_term_metric_columns_with_properties.sql`: add `properties JSONB` and drop `value_type`, `range_type`, `permitted_unit_term_ids` on `kb.ontology_terms` and `kb.ontology_term_revisions`.
- [x] 1.2 In the same migration, `CREATE OR REPLACE` `kb.sync_ontology_term_revision_after_insert()` to carry `properties` instead of the three dropped columns.
- [x] 1.3 In the same migration, `CREATE OR REPLACE VIEW kb.ontology_terms_current` to select `properties` instead of the three dropped columns.
- [x] 1.4 Write the down migration: re-add the three columns on both tables, best-effort backfill from `properties` via JSONB extraction, restore the prior trigger function and view, then drop `properties`.

## 2. terms_store.go

- [x] 2.1 Replace `Term.ValueType string`, `Term.RangeType string`, `Term.PermittedUnitTermIDs []string` with `Term.Properties map[string]any`.
- [x] 2.2 Update `termColumns` and `termRevisionColumns` to select `properties` instead of the three dropped columns.
- [x] 2.3 Update `scanTerm` to scan `properties` (JSONB bytes) into `Term.Properties` via `encoding/json`, mirroring the existing null-handling pattern used for `definition`/`scope`.
- [x] 2.4 Update `CreateTerm`, `CreateTermVersion`, and `insertTermChunk` to marshal `Term.Properties` (nil-safe: empty/nil map persists as SQL NULL) instead of passing the three old fields.
- [x] 2.5 Update the package doc comment on `Term` (currently describing `ValueType`/`RangeType`/`PermittedUnitTermIDs` as auto-promotion-only) to describe `Properties` the same way.

## 3. Auto-promotion synthesis

- [x] 3.1 In `alignment.go`, update `TermSynthesisInput` to carry `Properties map[string]any` (or keep the existing typed fields on the input struct and build the `properties` map only when constructing the `terms.Term{}` in `EnsureAcceptedOrCreate` — pick whichever keeps `TermSynthesisInput`'s doc comment about "transcribed, never guessed" fields intact) and pass it through to `CreateTerm` as `Properties`.
- [x] 3.2 In `extract-metrics.go` `resolveAll`, change `synth.Definition` to prefer `rep["metric_desc"]`, falling back to `rep["formula_or_definition"]` when `metric_desc` is empty.
- [x] 3.3 In `extract-metrics.go` `resolveAll`, always set the raw unit string (`raw_unit`) from `rep["metric_unit"]` when non-empty, independent of whether `MatchUnitLabel` resolves a `permitted_unit_term_ids` entry.

## 4. Sweep for other readers

- [x] 4.1 Re-run a repo-wide grep for `value_type`, `range_type`, `permitted_unit_term_ids` scoped to Go code touching `kb.ontology_terms`/`terms.Term` (not the unrelated `kb.metrics`-side `value_range_type` family) and confirm no other call site breaks. Fix any found. (Confirmed: all remaining hits are unrelated structs — `TermSynthesisInput`, `ontology_candidate_harvest.go`'s own metric-side fields, `metric_wiki_generate.go`, `metric_adapter.go` — none read/write `terms.Term`'s removed fields.)
- [x] 4.2 Check `server/api/kbhandler/metric_ontology_analysis_handler.go` and `server/api/terminologyresourcehandler/qudt_ontology.go` specifically (both matched the original broad grep) for any latent dependency on the dropped columns. (Both clear — matches were `value_range_type_error` on `kb.metrics`, unrelated to `kb.ontology_terms`.)

## 5. Verification

- [x] 5.1 `cd ChenWeb && go build ./...` (or the project's equivalent) to confirm the workspace compiles after the struct/column changes.
- [x] 5.2 `go vet ./...` in ChenWeb.
- [x] 5.3 Run/inspect existing tests covering `alignment.go` `EnsureAcceptedOrCreate` and `terms_store.go`, updating any that construct/assert on the removed `Term` fields. (Fixed mocks in terms_store_test.go, alignment_test.go, seed/seed_test.go, candidates/promote_test.go, comparison/store_test.go, modules/releases_store_test.go, kbhandler/ontology_comparison_{cells,scopes}_handler_test.go — several of the latter turned out to already be broken before this change, from a prior schema migration whose test mocks were never updated; fixing the column list made them pass. Added two new tests locking in the bug fixes: TestResolvingMetricsStoreAutoPromotePrefersMetricDescOverFormula and TestResolvingMetricsStoreAutoPromoteRetainsRawUnitOnResolverMiss.)
- [x] 5.4 Confirm `mise dev`/`air` picks up and applies the new migration cleanly against the local dev database (data already cleared, so no backfill data to verify — just confirm the migration runs without error and the new column/trigger/view exist). (Verified with a disposable scratch database via goose directly, not the live dev DB: `CREATE OR REPLACE VIEW` cannot drop/reorder columns (Postgres 42P16) and the original DROP COLUMN ordering violated a view dependency (2BP01) — both fixed in the migration: the view is now DROP+CREATE instead of CREATE OR REPLACE, and old columns are dropped only after the view/trigger no longer reference them. Verified Up and Down both apply cleanly and the trigger-mirrored `kb.ontology_terms_current` view carries `properties` correctly.)
