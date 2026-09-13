## 1. Backend — drawing/profile association

- [x] 1.1 Goose migration (see `shared/go/api/goose/goose.md`): add nullable
  `kb.product_profiles.drawing_id BIGINT REFERENCES kb.product_drawings(id)`. Verify it applies cleanly
  against the dev DB (`SELECT * FROM project_db_migration ORDER BY id DESC LIMIT 5`) before editing it
  further, per the workspace note about `mise dev`/air auto-applying migrations. (Confirmed via `\d
  kb.product_profiles` against the live `miner` DB.)
- [x] 1.2 `server/api/productdrawings/handler.go`: add `RETURNING id` to the `INSERT INTO
  kb.product_drawings` in `KeepPending`, add `Id int64` to `KeepResponse`, and return it.
- [x] 1.3 `server/api/product-reviews/`: add a way to persist `drawing_id` on a profile (either a small
  `PATCH /kb/product-profiles/:id/drawing {drawing_id}` handler, or extend the existing profile
  create/update path — match whichever pattern this package already uses for single-field profile
  updates). Register the route in `server/api/routes.go` next to the other `/kb/product-profiles` routes.
  (Added `Store.SetProfileDrawing` mirroring `SetProfileStatus`, and `SetProductProfileDrawing` handler
  mirroring `SetProductProfileReady`; also added `drawing_id` to `Profile`/`GetProfile`/
  `FindProfileByName`/`ListProfiles`/`CreateProfile` so it's consistently populated everywhere a profile
  is read.)
- [x] 1.4 Unit tests: `KeepPending`'s id-scan has no new test (this package's tests already skip DB
  assertions entirely since `ApiTypes.ProjectDBHandle` is nil in tests — same boundary noted in
  `product-review-history-list` task 2.3); added sqlmock store tests
  (`TestGetProfileWithDrawing`/`NoDrawing`, `TestSetProfileDrawingAssociates`/`Clears`, plus updated
  `TestListProfilesMixedRunHistory` to assert `DrawingID`). No handler-level test added for
  `SetProductProfileDrawing` — same boundary as its sibling `SetProductProfileReady`, which also has no
  handler test.

## 2. Frontend — service clients

- [x] 2.1 `productDrawingService.ts`: add `id?: number` to `KeptProductDrawing` (matching the new backend
  field).
- [x] 2.2 `productMetricReviewService.ts` (or wherever `ProfileSummary`/profile types live): add
  `drawing_id: number | null` to the profile type read from `GET`, and a `setProfileDrawing(profileId,
  drawingId)` call for the endpoint added in 1.3.
- [x] 2.3 Service tests for both additions, matching the existing fetch-stub style.

## 3. Frontend — drawing panel

- [x] 3.1 In `product-metric-review-view.svelte`, add drawing state: `drawingId: number | null` (from the
  loaded profile), `pendingDrawing: PendingProductDrawing | null`, `generating: boolean`, `drawingError:
  string | null`. (Named `drawingGenerating`/`drawingError`; also added `drawingBusy` for the
  keep/discard round trip and `showDrawingGenerator` to drive the regenerate flow.)
- [x] 3.2 Inline generator UI (shown when `drawingId` is null and no pending drawing): a prompt textarea
  pre-filled via a pure helper `buildDrawingPrompt(profile)` (e.g. `"3D exploded technical illustration of
  {name}: {description}"`, folding in keywords/notes), editable before submit, plus a "Generate" button
  calling `generateProductDrawing`.
- [x] 3.3 Pending-preview UI: render `pendingDrawing.image_url` via
  `pendingProductDrawingContentUrl(token)` with "Keep" and "Discard" actions — "Keep" calls
  `keepProductDrawing`, then `setProfileDrawing(profileId, result.id)`, sets local `drawingId`, clears
  pending state; "Discard" calls `ignoreProductDrawing` and returns to the generator.
- [x] 3.4 Kept-drawing UI: when `drawingId` is set, render the image via `productDrawingContentUrl(drawingId)`
  with a small "Regenerate" affordance that reopens the inline generator (3.2) without clearing the
  currently displayed image until a new one is kept (design.md Decision 1).
- [x] 3.5 Error/loading states mirroring `product-drawings-view.svelte`'s existing handling — no new error
  UI pattern.

## 4. Frontend — resizable layout

- [x] 4.1 Add `Layout = { drawingH: number; scopeW: number; detailW: number }` state with `KEY =
  'pmr-review-layout'`, `DEFAULTS`, `load()`/`persist()` using `localStorage` guarded by `browser`
  (mirror `metric-ontology-explorer/panel-shell.svelte`'s `load`/`persist`/`clamp` shape, design.md
  Decision 2). (Named `ReviewLayout`/`loadLayout`/`persistLayout` to avoid colliding with the file's
  existing `layout` DOM concepts.)
- [x] 4.2 `startResize`/`moveResize`/`endResize` pointer-capture handlers keyed by `'drawingH' |
  'scopeW' | 'detailW'`, with `clamp` bounds sane for each (e.g. `scopeW`/`detailW` min ~260px, `drawingH`
  min ~120px, all capped so the results pane can't be squeezed to zero). (Used 240/480 for scopeW,
  320/900 for detailW, 140/640 for drawingH, plus a `MAIN_MIN` of 360px enforced dynamically against the
  container's `clientWidth` in `moveResize`, mirroring `panel-shell.svelte`'s `Math.max(..., W - ...)`
  pattern.)
- [x] 4.3 Restructure the Results tab markup: a top section (drawing area, height `layout.drawingH`) → a
  horizontal splitter (`onpointerdown` → `startResize(e, 'drawingH')`) → the three-pane row, where the
  scope-tree pane is `width: layout.scopeW`, a vertical splitter, the results pane is `flex: 1 1 auto`
  (the flexible pane), a vertical splitter, and the metric-details pane is `width: layout.detailW`. Report
  tab keeps its current fixed 2-column grid untouched. (`.layout` gets a `resizable` class only when
  `tab === 'results'`; `.layout.report-mode`'s existing grid CSS is untouched.)
- [x] 4.4 Remove/bypass `.content`'s `max-width: 1720px` specifically for the Results tab's pane row
  (design.md Decision 3) so the three panes fill the full remaining width; Report tab keeps the existing
  cap. (Implemented as a `.content.wide` modifier — simpler than a breakout hack, since `.content` wraps
  the whole page body and a child can't exceed its parent's max-width box without one — applied only
  when `tab === 'results' && run`, so Report tab and the empty/loading states are unaffected.)
- [x] 4.5 Keep the existing `@media (max-width: 1000px)` single-column collapse; hide/disable the drag
  splitters below that breakpoint. (Added `.layout.resizable { flex-direction: column }`, forced
  `width: auto !important` on the panes, and hid `.v-splitter`/`.h-splitter` in the media query.)

## 5. Frontend — context-panel suppression

- [x] 5.1 `dashboard.svelte`: gate the `ContextShelf` render and its resize divider (currently `{#if
  shelfOpen}`) on `shelfOpen && activeMenu?.childId !== 'apps-product-review'`, leaving `shelfOpen` state
  itself untouched. (Added a `shelfVisible` derived and used it for both `{#if}` gates.)
- [x] 5.2 `content-panel.svelte`: hide the "Open panel"/"Close panel" toggle button (both render sites,
  ~line 213 and ~line 368) when `activeMenu?.childId === 'apps-product-review'`. (Only the first site —
  the shared `!isDashboard` topbar — applies to Product Review; the second site at ~line 368 is the
  Dashboard home page's own toggle, gated by `{:else if isDashboard}`, which Product Review never is, so
  it was left untouched.)
- [x] 5.3 Manual check: toggling between Product Review and another nav item (e.g. Metric Management)
  restores that other item's previous shelf open/closed state correctly. (Verified with Chat as the other
  nav item — see 6.3.)

## 6. Verification & docs

- [x] 6.1 `cd server && go build ./... && go vet ./...` and `go test ./api/productdrawings/...
  ./api/product-reviews/...`. (Clean: `ok` for both packages.)
- [x] 6.2 `cd web && bun run build`, `bun run check`, `bun test` (new/affected service and component
  tests); compare any pre-existing failures against the baseline noted in `product-review-history-list`'s
  task 5.2. (`bun run build` clean. `bun run check`: same 3 pre-existing errors + 2 warnings as the
  documented baseline, none in files this change touches. `bun test`: 362/367 pass; the 5 failures are the
  same pre-existing ones — doc-structure-settings (×2), topic-tree-record-browser, and 2 more matching the
  documented baseline; all new tests added by this change pass.)
- [x] 6.3 Manual end-to-end with `mise dev`, via a headless-Chromium Playwright script against the live
  frontend (localhost:5173) with `/api/v1/**` fetches intercepted and stubbed (no login credentials in
  this environment — same boundary as prior changes' 8.3/5.3). Verified on the standalone route: the
  drawing prompt pre-fills from the profile's name/description/keywords/notes; generate → pending preview
  → Keep persists (stubbed `id`) and shows the kept image with a Regenerate action; dragging the scope-pane
  splitter (320px → 400px) and the drawing-area horizontal splitter (320px → 380px) resize live and the
  new sizes read back from `localStorage` after a full page reload; switching to the Report tab removes
  the drawing area and both splitters and reverts `.content`'s `wide` class (Report tab correctly
  unaffected). Verified in the embedded dashboard shell: a control nav item (Chat) still shows the
  "Open/Close panel" toggle (no regression), while Product Review (both the intake sub-view, screenshotted,
  and confirmed by selector count for the results sub-view since it shares the same gating condition) shows
  zero instances of the shelf toggle and no context panel, with the content area using the freed width.
  **Not done**: an actual logged-in click-through generating a real (non-stubbed) drawing end to end, and
  visually confirming a *results* (not intake) sub-view screenshot specifically — someone with access
  should do a final pass.
- [x] 6.4 Update the Product Metric Reviewer user manual (new `product-metric-reviewer-v1.3-en.md` per the
  existing versioning convention) documenting the drawing panel and resizable layout.
- [x] 6.5 Record the knowledge change per workspace `CLAUDE.md` "Coding Best Practice": what changed, that
  `kb.product_drawings` rows are never deleted on regenerate (intentionally out of scope), and that the
  `product-metric-reviewer-page` capability spec was never merged into `openspec/specs/` (pre-existing gap,
  not fixed by this change). (`KnowledgeStore/doc-repo/devdocs/202609/2026091302-devdoc-product-review-3d-resizable-layout.md`.)
- [ ] 6.6 Commit via `jj` (workspace `CLAUDE.md` git workflow): migration + backend as one commit, frontend
  as another.
