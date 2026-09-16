# Past Review Card Details Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Store bilingual product names and render wider Past reviews cards with a left-side 3D drawing, explicit Keywords/Description/Metrics rows, and truncated-description tooltips.

**Architecture:** Keep `kb.product_profiles.name` as the compatibility lookup field and add `name_cn`/`name_en` as display data. Extend the existing profile-summary endpoint with bilingual names, drawing metadata, and latest run metric count so the Svelte page renders one response without per-card lookups. Extend the existing product-name typeahead with a selection callback so intake can persist catalog names.

**Tech Stack:** Go, PostgreSQL goose migrations, Echo/API store tests with sqlmock, Svelte 5, TypeScript, Vitest, ESLint, svelte-check.

---

## Chunk 1: Schema and backend profile data

### Task 1: Add bilingual profile columns and backfill existing rows

**Files:**
- Create: `project_migrations/20260916000001_add_bilingual_names_to_kb_product_profiles.sql`

- [ ] Add nullable-safe `name_cn TEXT NOT NULL DEFAULT ''` and `name_en TEXT NOT NULL DEFAULT ''` columns.
- [ ] Backfill `name_cn` from the existing `name`.
- [ ] Backfill `name_en` with a correlated `kb.product_names` lookup matching trimmed Chinese catalog name; use an empty string when no match exists.
- [ ] Add a down migration that removes only the new columns.
- [ ] Run the migration against the project migration test/check command if available.
- [ ] Commit with `jj` as `feat: persist bilingual product profile names`.

### Task 2: Extend Go profile models and store queries

**Files:**
- Modify: `server/api/product-reviews/models.go`
- Modify: `server/api/product-reviews/store.go`
- Test: `server/api/product-reviews/store_test.go`
- Test: `server/api/product-reviews/helpers_test.go` if shared profile row fixtures need new columns

- [ ] Add `NameCN` and `NameEN` JSON fields to `Profile`; retain `Name` unchanged.
- [ ] Add an optional latest metric count field to `ProfileSummary` (JSON `latest_metric_count`), derived from the latest run’s `result_count`.
- [ ] Update create/get/update profile SQL scan and write paths to select/store the bilingual fields while keeping legacy callers valid.
- [ ] Update `ListProfiles` to select `p.name_cn`, `p.name_en`, drawing ID, and latest run `result_count`; use `COALESCE`/nullable scan behavior so old or not-yet-migrated data does not panic.
- [ ] Update sqlmock rows and expectations for create, get, update, and list queries.
- [ ] Add tests proving a completed latest run returns its metric count, a profile with no run returns no count, and bilingual names round-trip.
- [ ] Run `go test ./server/api/product-reviews` and fix failures.
- [ ] Commit with `jj` as `feat: expose bilingual names and metric counts in review summaries`.

### Task 3: Accept and persist catalog English names during intake

**Files:**
- Modify: `server/api/product-reviews/intake.go`
- Modify: `server/api/product-reviews/store.go`
- Modify: `server/api/product-reviews/models.go`
- Test: `server/api/product-reviews/store_test.go`
- Test: `server/api/product-reviews/intake_drawing_test.go` or a focused intake test file

- [ ] Add an optional `product_name_en`/display-English input to the intake request model.
- [ ] On new profile creation, set legacy `name` from the Chinese catalog name, `name_cn` from the Chinese name, and `name_en` from the supplied English name.
- [ ] For manually typed requests with no English value, store the typed name in `name` and `name_cn`, with `name_en` empty.
- [ ] Preserve duplicate detection and resume behavior based on the existing normalized `name` field.
- [ ] Add tests for catalog-selected and manually typed intake requests.
- [ ] Run the focused product-review Go tests.
- [ ] Commit with `jj` as `feat: save catalog bilingual names during product intake`.

## Chunk 2: Frontend selection and card presentation

### Task 4: Return the selected product-name catalog entry from the typeahead

**Files:**
- Modify: `web/src/lib/components/home3/product-name-field.svelte`
- Modify: `web/src/lib/components/home3/product-review-intake-view.svelte`
- Test: existing component/service tests if present; otherwise add focused behavior coverage alongside the component conventions

- [ ] Add an optional `onSelect` prop carrying `ProductNameEntry`.
- [ ] Invoke it when a suggestion is selected while preserving the existing bound Chinese `value`.
- [ ] Track the selected entry in the intake form and clear the English selection if the user edits the name away from the selected catalog value.
- [ ] Include `product_name_en` in the new-review intake request; keep resume requests unchanged.
- [ ] Map legacy summaries with empty `name_cn` to `name` for display and selection compatibility.
- [ ] Run the relevant frontend tests.

### Task 5: Render option-B Past reviews cards with drawing and attribute rows

**Files:**
- Modify: `web/src/lib/services/productMetricReviewService.ts`
- Modify: `web/src/lib/services/productDrawingService.ts` if a shared content URL helper import is preferred
- Modify: `web/src/lib/components/home3/product-review-intake-view.svelte`

- [ ] Extend `Profile`/`ProfileSummary` TypeScript types with `name_cn`, `name_en`, `drawing_id`, and `latest_metric_count`.
- [ ] Use `productDrawingContentUrl(drawing_id)` for the left image; render a neutral framed placeholder when no ID exists.
- [ ] Make the history grid cards wider and use a horizontal flex layout with a fixed-width framed preview on the left and flexible details on the right; stack at narrow widths.
- [ ] Render the title as `name_cn / name_en` only when English exists, otherwise omit the slash and English portion.
- [ ] Replace keyword chips as the primary presentation with name-value rows for `Keywords`, `Description`, and `Metrics`; show `—` for empty/missing values.
- [ ] Keep the status line and `View results` button behavior intact, including click propagation prevention and keyboard card selection.
- [ ] Use a two-line CSS-clamped description preview.
- [ ] Add a small reactive truncation check using a text element’s `scrollHeight > clientHeight`; set the native `title` only when truncated. Ensure no tooltip is shown for a short description.
- [ ] Add accessible image alt text based on the bilingual display title; mark the placeholder as decorative.
- [ ] Run ESLint on the changed Svelte/TypeScript files and run svelte-check.

### Task 6: Add frontend regression coverage and verify the integrated feature

**Files:**
- Modify or create: the nearest existing intake component test file under `web/src/lib/components/home3/`
- Modify: `web/src/lib/services/productMetricReviewService.test.ts` if response/type behavior needs service coverage

- [ ] Test bilingual title fallback, explicit attribute labels, metric count rendering, drawing URL rendering, and missing-value em dashes.
- [ ] Test that a long description receives a tooltip only after it is actually clamped.
- [ ] Test that selecting a catalog suggestion supplies the English name to intake and editing the name clears that selection.
- [ ] Run the focused frontend test command used by `ChenWeb/web`.
- [ ] Run `npx eslint --no-warn-ignored` for every changed frontend file.
- [ ] Run `npx svelte-check --output human --fail-on-warnings --threshold error`.
- [ ] Run `go test ./server/api/product-reviews` and any migration validation command.
- [ ] Inspect the final diff and run `jj status`; confirm only task files plus the previously existing server edits are present.
- [ ] Commit the feature changes with `jj` as `feat: improve past review card details`.

## Notes for implementation

- Do not edit `web/src/lib/types/go-types.ts`; these fields are API/service types, not generated database types.
- Do not remove the existing `name` column or alter duplicate/resume semantics.
- Do not add per-card API requests; the list endpoint should remain the single source for card data.
- Existing unrelated edits in `server/api/product-reviews/hybrid.go`, `retrieve.go`, and `retrieve_test.go` were present before this task and must be preserved, not reformatted or folded into feature changes.
