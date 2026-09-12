## 1. Schema

- [x] 1.1 Add a goose migration adding `keywords` (`text[]`) and `notes` (`text`), both
  nullable, to `kb.product_profiles` (per `shared/go/api/goose/goose.md` conventions; verify
  `keywords`/`notes` don't collide with reserved words).
- [x] 1.2 Confirm the migration applies cleanly against the dev DB (`miner`) and is recorded in
  `project_db_migration`. (`air` auto-ran it against a stale intermediate version of the file
  before the `keywords` type was corrected to `jsonb` — manually re-applied the corrected column
  type by hand against the empty dev table since goose won't re-run a version already marked
  applied; verified `\d kb.product_profiles` now matches the committed migration.)

## 2. Backend — duplicate lookup

- [x] 2.1 Add `Store.FindProfileByName(ctx, tenantID, name string) (*Profile, error)` in
  `store.go` — normalized (`lower(trim(...))`) exact match, scoped to tenant; returns `nil, nil`
  on no match.
- [x] 2.2 Add a `Store` or `RunStore` helper to fetch a profile's latest request and, if any, that
  request's latest run (composing existing `RunStore.ListRequests` + `RunStore.ListRuns` — no new
  tables).
- [x] 2.3 Unit tests (sqlmock, matching existing package style) for: no match, exact match,
  case/whitespace-insensitive match, tenant isolation. Plus `LatestRequestAndRun` tests for all
  three states (no request, request with no run, request with a completed run).

## 3. Backend — profile keywords/notes

- [x] 3.1 Extend `Profile` struct and `Store.CreateProfile`'s input/insert to carry `Keywords
  []string` and `Notes string`.
- [x] 3.2 Extend `CreateProductProfile` handler's request struct to accept `keywords` and `notes`
  and pass them through.
- [x] 3.3 Update/add unit tests covering the new fields round-tripping through create + get
  (`TestCreateProfileWithKeywordsAndNotes`; also updated the shared `profileRows` test helper and
  `TestCreateProfile`'s arg list for the new columns).

## 4. Backend — orchestrated intake endpoint

- [x] 4.1 Add `POST /kb/product-reviews/intake` handler (new file, e.g. `intake.go`) accepting an
  optional `resume_profile_id`. If `resume_profile_id` is set, skip the name lookup and drive
  `Builder.Build` → `Store.SetProfileStatus(ready)` → `RunController.StartReview` against that
  profile id directly (this is the "Re-run" path for a still-`draft` profile — see design.md
  Decision 3). Otherwise: look up an existing profile by name (2.1); if found, return the
  duplicate response shape from design.md Decision 3 (`duplicate: true`, profile,
  `latest_request_id`, `latest_run`) without creating anything; if not found, call
  `Store.CreateProfile` → `Builder.Build` → `Store.SetProfileStatus(ready)` →
  `RunController.StartReview` in sequence and return `{status: true, duplicate: false, profile,
  run}`.
- [x] 4.2 On any step failing after profile creation, return the error but leave the created
  (draft) profile in place — do not attempt cleanup/rollback (matches existing "failure leaves
  the request re-runnable" behavior).
- [x] 4.3 Register the route in `server/api/routes.go` next to the other `/kb/product-reviews`
  routes.
- [x] 4.4 Unit tests for the testable seam: `duplicateProfileResponse` (extracted from the
  handler specifically so it's sqlmock-testable without the global LLM/DB wiring) covers duplicate
  with a completed run (offers both view-results and re-run), duplicate with no run yet (offers
  re-run only, no `latest_run` key), and no match (returns nil so the caller proceeds to create).
  **Not covered by an automated test**, consistent with this package's existing boundary (no
  handler in `handler.go` has a unit test either — `newBuilder()`/`newRunController()` wire real
  config/LLM/DB globals that aren't mocked anywhere in this package): the full
  `IntakeProductReview` handler's happy path, its `resume_profile_id` branch, and a mid-pipeline
  failure. These are exercised by 8.3's manual end-to-end check instead.

## 5. Frontend — service client

- [x] 5.1 Add `startProductReviewIntake(input)` and any response types to
  `productMetricReviewService.ts` for `POST /kb/product-reviews/intake`.
- [x] 5.2 Add a call for "re-run from duplicate choice" reusing the existing rerun client call
  (confirm one already exists; add if not). (`rerunReview` already existed and is reused as-is.)
- [x] 5.3 Service tests (`bun test`, matching existing fetch-stub style) for the new call's
  success, duplicate, and error responses.

## 6. Frontend — intake page

- [x] 6.1 Add `routes/home3/product-review/+page.svelte` (thin wrapper, same `?dark=`/theme
  pattern as `routes/home3/product-metric-review/+page.svelte`).
- [x] 6.2 Add `lib/components/home3/product-review-intake-view.svelte`: form (name, description,
  keywords, notes), Start button (disabled without a name), loading state while the intake
  request is in flight, and the duplicate-choice UI (View results / Re-run), matching the visual
  style of `product-metric-review-view.svelte` (`.pmr-shell` token pattern, DM Mono + Manrope).
- [x] 6.3 On success (non-duplicate) or "View results"/"Re-run", navigate to
  `/home3/product-metric-review?run=<id>&dark=...`.
- [x] 6.4 Add any new i18n keys to `messages/{en,zh-cn}.json` (baseLocale zh-cn, matching
  existing `pmr_*` key convention) and recompile paraglide (`bun run build` or the `paraglide-js
  compile` command).

## 7. Navigation

- [x] 7.1 Add `{ id: 'apps-product-review', label: 'Product Review' }` to the
  `applications.children` array in `nav-rail.svelte`.
- [x] 7.2 Add a `child?.id === 'apps-product-review'` branch in `selectItem` opening
  `/home3/product-review?dark=...` via `window.open`, matching the existing `kb-metrics`/
  `kb-chunks` pattern. No `content-panel.svelte` change needed.

## 8. Verification & docs

- [x] 8.1 `cd server && go build ./... && go vet ./...` and `go test ./api/product-reviews/`.
  Also ran full-workspace `go build ./...`, `go vet ./...`, `go test ./...`: clean except 4
  pre-existing failing packages unrelated to this change (`ontology/keywords`, `ontology/names`,
  `ontology/seed`, `cmd/qudt-import`) — confirmed via `jj status` that none of their files were
  touched this session.
- [x] 8.2 `cd web && bun run build` and `bun test productMetricReviewService.test.ts` (and any
  new intake service test file). Also ran `bun run check` (svelte-check, clean for the new files)
  and the full `bun test` suite: 358/363 pass, the 5 pre-existing failures are all in unrelated
  files (doc-structure-settings, topic-tree-record-browser, pdf-view-window, knowledge page).
- [~] 8.3 Manual end-to-end check with `mise dev` running: submit a brand-new product name and
  confirm it lands on a completed run's results page; resubmit the same name and confirm the
  duplicate choice appears with working "View results" and "Re-run". **Partially done**: verified
  the new route and API endpoint are correctly wired and reachable on the live dev server —
  `GET /home3/product-review` and `POST /api/v1/kb/product-reviews/intake` both return 401
  unauthenticated, identically to the existing sibling page/endpoint (`/home3/product-metric-review`,
  `POST /api/v1/kb/product-profiles`), proving no regression and correct routing. **Not done**:
  the actual logged-in click-through (fill the form, submit, see the duplicate choice) — the app
  requires an authenticated session and I don't have login credentials for this dev environment.
  Someone with access should run this manually.
- [x] 8.4 Update the Product Metric Reviewer user manual (used the `user-manual-writer` skill).
  Per this doc repo's own versioning convention (see the `metric-assertion-semantic-processing-v1.x`
  series — each revision is a new file, old ones untouched), wrote
  `KnowledgeStore/doc-repo/user-manuals/product-metric-reviewer-v1.1-en.md`: §2 no longer
  requires an administrator hand-off, §4 rewritten to cover starting a new review, the
  duplicate-choice behavior, and opening a specific run directly, §11 drops the two now-resolved
  boundaries (no in-page create flow / not in the nav), §12 gained two troubleshooting rows.
  v1.0 left untouched.
- [x] 8.5 Commit via `jj` (see workspace `CLAUDE.md` git workflow) once the above passes.
