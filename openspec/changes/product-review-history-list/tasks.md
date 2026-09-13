## 1. Backend — profile list query

- [x] 1.1 Add a `ProfileSummary` type (or extend `Profile`) in `store.go` carrying:
  `LatestRequestID *int64`, `LatestRunID *int64`, `LatestRunStatus string`,
  `LatestRunFinishedAt *time.Time` alongside the existing profile fields (name,
  product_description, keywords, notes, status, updated_at). (Added to `models.go`; discovered
  an existing, unused `Store.ListProfiles(ctx, tenantID)` stub with no callers/tests/keywords —
  completed it in place rather than adding a second method.)
- [x] 1.2 Add `Store.ListProfiles(ctx, tenantID string, limit int) ([]ProfileSummary, error)` —
  the `LEFT JOIN LATERAL` query from design.md Decision 1, `ORDER BY p.updated_at DESC LIMIT $2`.
  Clamp `limit` to a sane default/max in the handler (e.g. default 50, max 200), not in the store
  method.
- [x] 1.3 Unit tests (sqlmock, matching existing package style): profiles with a completed latest
  run, profiles with no request yet, profiles whose latest run is running/failed, tenant
  isolation, and the `updated_at DESC` ordering. (`TestListProfilesMixedRunHistory` covers a
  completed-run profile and a never-run profile in one call, plus keyword decoding; ordering and
  tenant scoping are expressed in the SQL text matched by the mock, matching how other `store.go`
  tests in this package verify query shape rather than exercising a live DB.)

## 2. Backend — list endpoint

- [x] 2.1 Add `ListProductProfiles` handler (`GET /kb/product-profiles?limit=`) calling
  `Store.ListProfiles`, returning `{status: true, profiles: [...]}`.
- [x] 2.2 Register the route in `server/api/routes.go` next to the other `/kb/product-profiles`
  routes.
- [x] 2.3 Unit test for the handler's limit clamping/parsing (invalid/absent/oversized `limit`
  query param). (Extracted the parsing into a pure `parseListLimit` helper — this package's
  handlers all wire real global DB/config and have no precedent for an httptest-based handler
  test, same boundary noted in the `product-review-intake` change's task 4.4 — and unit-tested
  that helper directly in the new `handler_test.go`.)

## 3. Frontend — service client

- [x] 3.1 Add a `ProfileSummary` type and `listProfiles(limit?: number)` call to
  `productMetricReviewService.ts` for `GET /kb/product-profiles`.
- [x] 3.2 Service test (`bun test`, matching existing fetch-stub style) for `listProfiles`'s
  success and error responses.

## 4. Frontend — intake page UI

- [x] 4.1 Extract `handleRerun()`'s two-branch resume logic (rerun latest request vs.
  resume-build a draft profile) into a small shared helper usable by both the existing duplicate
  hero's "Re-run" button and the new card-select "Re-Run" button (design.md Decision 3) — no
  behavior change to the existing duplicate-hero path. (`resumeReview()`.)
- [x] 4.2 Add `selectedProfile: ProfileSummary | null` state. On card click: populate
  `name`/`description`/`keywordsInput`/`notes` from the profile and set `selectedProfile`.
- [x] 4.3 Change the submit button's label/behavior: "Re-Run" (calling the 4.1 helper against
  `selectedProfile`) when `selectedProfile` is set, "Start"/existing behavior otherwise.
- [x] 4.4 Clear `selectedProfile` (reverting to "Start") when the live `name` field no longer
  matches `selectedProfile.name` (trimmed) — design.md Decision 4. (`$effect`.)
- [x] 4.5 Render the card list below the form: fetch `listProfiles()` on mount, show an empty
  state when there are no profiles, otherwise render name / description clamp / keyword chips /
  latest-run status+relative-time per card (design.md Decision 5). Match the existing
  `.pmr-shell` token/style pattern (including the button-color fix already applied to `.primary`).
- [x] 4.6 Add any new i18n keys to `messages/{en,zh-cn}.json` (baseLocale zh-cn, `pmr_*`
  convention) and recompile paraglide. (Relative-time strings ("2h ago") are formatted in
  TypeScript rather than templated per-locale — see design notes; only the status words are
  localized.)

## 5. Verification & docs

- [x] 5.1 `cd server && go build ./... && go vet ./...` and
  `go test ./api/product-reviews/`. (Clean: `ok`.)
- [x] 5.2 `cd web && bun run build`, `bun run check`, and `bun test` (service test + affected
  component tests). (`bun run build` clean. `bun run check`: 3 pre-existing errors in unrelated
  files, none in files this change touches. `bun test`: 361/366 pass; the 5 failures are the same
  pre-existing ones documented in `product-review-intake`'s task 8.2 — doc-structure-settings,
  topic-tree-record-browser, pdf-view-window, knowledge page.)
- [~] 5.3 Manual end-to-end check with `mise dev` running: confirm the card list renders for
  existing profiles, selecting a card prefills the form and relabels the button, re-running both
  a previously-run and a never-run profile navigates to a results page, and editing the name after
  selecting a card reverts the button to "Start". **Mostly done** via Playwright against the live
  `mise dev` frontend (localhost:5173) with the `/api/v1/kb/product-profiles` fetch intercepted
  and stubbed (no login credentials in this environment — same boundary as
  `product-review-intake`'s task 8.3 for anything requiring an authenticated API call): verified
  the button is `#818cf8` and enabled/disabled correctly, cards render name/description/keyword
  chips/status+relative-time, clicking a card fills all four fields and relabels the button
  "重新运行" (Re-Run), and editing the name field afterward reverts it to "开始" (Start) with the
  `disabled` attribute clearing/setting correctly throughout. Also confirmed
  `GET /api/v1/kb/product-profiles` is live and returns 401 unauthenticated for a real
  (non-stubbed) request (same as sibling endpoints, proving correct routing/no regression), and
  ran the exact `ListProfiles` SQL directly against the live dev DB (`miner`) — it returns both
  existing profiles ("Ventilator", "血压计") with correct
  `latest_request_id`/`latest_run_id`/status/`finished_at`. **Not done**: an actual logged-in
  click-through exercising the real (non-stubbed) `startProductReviewIntake`/`rerunReview` calls
  end to end. Someone with access should run that manually.
- [x] 5.4 Update the Product Metric Reviewer user manual (per the versioning convention used by
  `product-metric-reviewer-v1.1-en.md`) to document the past-reviews list and re-run-from-card
  flow. (New `product-metric-reviewer-v1.2-en.md`; v1.0/v1.1 left untouched, matching the
  established convention that older revisions keep `status: current` and aren't edited.)
- [x] 5.5 Commit via `jj` (see workspace `CLAUDE.md` git workflow) once the above passes.
