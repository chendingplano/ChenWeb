## 1. Promotion-policy table and store

- [x] 1.1 Add a goose migration creating `kb.keyword_source_promotion_policy` (`source TEXT PRIMARY KEY`, `auto_promote_enabled BOOLEAN NOT NULL DEFAULT TRUE`, `set_by TEXT`, `set_at TIMESTAMPTZ`, `create_time`, `modify_time`). Mutable — no immutability trigger.
- [x] 1.2 Add a `PromotionPolicyStore` in `server/api/ontology/keywords/` with `IsEnabled(ctx, source) (bool, error)` (returns `true` when no row exists) and `Set(ctx, source string, enabled bool, setBy string) error` (upsert).

## 2. Auto-promotion function

- [x] 2.1 Add `server/api/ontology/keywords/catalog_promotion.go` with `PromoteCatalogEntries(ctx, db DBX, source, release, scope string) (PromotionCounts, error)`: reads `kb.keyword_catalog_entries`/`kb.keyword_catalog_labels` for `(source, release)`, picks a preferred label per entry (English-preferred-first fallback chain), normalizes it, and calls the same create-or-converge sequence `KeywordFamily.autoCreateConcept` uses. Also skips `entry_status='deprecated'` rows (matching the equivalent QUDT-ontology-term work's convention).
- [x] 2.2 `PromotionCounts` reports `EntriesScanned`, `ConceptsCreated`, `ConceptsConverged`, `Errors` (per-entry errors are counted, not fatal to the pass).

## 3. Wire the background trigger into ApproveResource

- [x] 3.1 In `ApproveResource`, after the keyword-lexicon staging import succeeds (both fresh and replay branches, every resource), `triggerCatalogAutoPromotion` checks `PromotionPolicyStore.IsEnabled(source)` and, if true, launches `go func() { PromoteCatalogEntries(...) }()` capturing `db` and the handler's logger before dispatch.
- [x] 3.2 `scope` comes from the resource's `AllowedScopes[0]`; `release` prefers `st.Release` (the actually-persisted release, correct for dynamic-release resources like Wikidata) over the resource's static configured release.
- [x] 3.3 Promotion outcome (counts or error) is logged only; never surfaced in the HTTP response (fire-and-forget, confirmed by `catalog_promotion_test.go`'s policy test which observes the DB directly, not the response).

## 4. Admin API for the promotion-policy override

- [x] 4.1 `PUT /api/v1/terminology-resources/:source/promotion-policy` (`SetAutoPromotePolicy`), form body `enabled=true|false`, records `set_by` from the authenticated user.
- [x] 4.2 `resourceView`/`viewFor` now includes `auto_promote_enabled`, computed from `PromotionPolicyStore` directly (defaults `true` when the DB handle is unavailable or no row exists), so the frontend gets it in the existing list/approve/disapprove responses without an extra round trip.

## 5. Frontend

- [x] 5.1 `TerminologyResource.auto_promote_enabled: boolean` added to `terminologyResourceService.ts`.
- [x] 5.2 `setAutoPromoteEnabled(source, enabled)` added, calling the new PUT endpoint.
- [x] 5.3 A per-resource toggle switch added to `external-terminology-resources-view.svelte`'s resource card, between the size/cadence grid and the downloaded-metadata panel.

## 6. Tests

- [x] 6.1 `promotion_policy_store_test.go` (sqlmock): default-when-absent, reflects an existing row, `Set` upserts, nil-DB errors.
- [x] 6.2 `catalog_promotion_test.go` (keywords package, `TEST_DATABASE_URL`-gated): `PromoteCatalogEntries` against a real Postgres — creates flagged provisional concepts + surfaces for non-deprecated entries, skips deprecated ones, and converges (no duplicates) on a second pass. Runs against a plain `*sql.DB`, not a wrapping transaction — deliberately, since the collision-recovery logic needs each attempt's failure to be independently committed/visible, which Postgres does not allow inside one shared transaction after an aborted statement. `kb.keyword_sources`/`kb.keyword_catalog_entries`/`kb.keyword_catalog_labels` fixture rows are left in place (immutable by trigger, matching production reality) rather than cleaned up; only the mutable concept/surface rows are deleted in `t.Cleanup`.
- [x] 6.3 Covered by 6.2 (disabled/enabled via direct `PromoteCatalogEntries` calls is redundant with) and by 6.4's `TestTriggerCatalogAutoPromotionRespectsPolicy`, which exercises the real disabled→no-concepts / enabled→concept-appears path through the actual `triggerCatalogAutoPromotion` function against `terminology.ResourceUCUM` with a synthetic release.
- [x] 6.4 `TestTriggerCatalogAutoPromotionRespectsPolicy` (terminologyresourcehandler package): calls the real `triggerCatalogAutoPromotion`, polls for the async result. Fire-and-forget/non-blocking and error-isolation are structural properties of the `go func(){...}()` wrapping and the void error handling (verified by code inspection, not a dedicated runtime test — there's no code path connecting the goroutine's outcome back to the caller to test against).

## 7. Manual verification

- [x] 7.1 `go build ./...`, `go vet ./...`, and `mise build-server` all clean. `bun run check` (svelte-check) shows no new errors/warnings in either changed frontend file; the one pre-existing repo-wide error is unrelated (a `.test.ts` import-extension issue).
- [x] 7.2 Verified via the automated integration tests (6.2/6.4) against the real local `chenweb_test` Postgres instance rather than the live dev DB, since this change's core claim — a resource's approved catalog entries produce flagged provisional concepts without human action — is exactly what those tests assert against real rows, real triggers, and real constraints (including the immutable-staging-table discovery in task 6.2's notes).
- [x] 7.3 Covered by 6.4: toggling the policy off/on and re-invoking the trigger directly demonstrates the same behavior a re-approve through the UI would produce.
