## Context

The results page lives in `web/src/lib/components/home3/product-metric-review-view.svelte`, rendered
either standalone at `/home3/product-metric-review` (bare, no dashboard chrome) or embedded inside the
`/home3` dashboard shell (`dashboard.svelte` → `content-panel.svelte`) when nav child id
`apps-product-review` is active — this is the screenshot showing the "APP STATUS" panel. The three panes
today are a fixed CSS grid (`.layout { grid-template-columns: minmax(280px,340px) minmax(320px,640px)
minmax(360px,1fr); }`) inside a `.content { max-width: 1720px; margin: 0 auto; }` wrapper — no drag/resize
code exists in this file. The "APP STATUS" panel is `context-shelf.svelte`, a generic right sidebar
rendered by `dashboard.svelte` for every nav item (gated only on a user-toggled `shelfOpen` boolean, not on
content type) — it is not specific to Product Review and must stay working for every other page.

"Generate 3D Product Drawings" (`server/api/productdrawings`, `productDrawingService.ts`,
`product-drawings-view.svelte`) already exists under System Admin → Resources. It POSTs
`{name, description, prompt, keywords, notes, model}` to `/api/v1/product-drawings/generate`, gets back a
`PendingProductDrawing {token, image_url, expires_at}`, and the user either `POST
.../pending/:token/keep` (persists a `kb.product_drawings` row + PNG file) or `DELETE
.../pending/:token` (discard). The output is a 2D PNG technical/exploded-view illustration, not a 3D mesh
— "3D" is the illustration style requested in the prompt, not an asset format. Critically, `KeepPending`'s
`INSERT INTO kb.product_drawings (...)` has no `RETURNING id`, and `KeepResponse` only carries
`{status, filename, path}` — there is currently no way for a caller to learn the persisted row's id, which
we need to associate a drawing with a review profile so it doesn't regenerate on every page visit.

The nearest resizable-panel prior art in this codebase is the hand-rolled pointer-capture splitter in
`metric-ontology-explorer/panel-shell.svelte` (`startResize`/`moveResize`/`endResize` via
`setPointerCapture`, a `clamp` helper, and a `{arrangement, leftW, canvasH, tabsW}` layout persisted to
`localStorage`), plus `dashboard.svelte`'s own rail/shelf drag handles. A shadcn/paneforge `Resizable*`
wrapper also exists under `web/src/lib/components/ui/resizable/`, but it is only wired into two
demo/scaffold routes (`example-left-right-panes`, `sidebar-01`) — it is not used anywhere in the real app,
so it is not the established pattern here despite being present in the tree.

## Goals / Non-Goals

**Goals:**
- Show a generated reference drawing of the reviewed product above the Results tab's three panes, reusing
  the existing product-drawings generate/keep/ignore flow and backend — no new generation mechanism.
- Cache the kept drawing against the review profile so it persists across visits/runs instead of
  regenerating every page load.
- Make the scope-tree / results / metric-details panes independently width-resizable via drag, and the
  drawing area independently height-resizable via drag, filling the full remaining window width, following
  this codebase's existing hand-rolled splitter pattern (not the unused paneforge wrapper).
- Hide the "APP STATUS" context panel and its toggle specifically while Product Review is the active
  dashboard content, without touching `context-shelf.svelte` or any other page's behavior.

**Non-Goals:**
- Not changing the Report tab's layout (still a fixed 2-column grid) — the user's ask is scoped to the
  Results tab's three panes.
- Not building a new image-generation backend, model selection UI beyond what `ProductDrawingsView`
  already exposes, or a 3D mesh/asset viewer.
- Not changing `context-shelf.svelte` itself or its behavior on any other nav item.
- Not adding automatic/background generation — generation stays an explicit, user-triggered action (same
  as the System Admin flow), since it's a paid LLM/image call and can need retries.

## Decisions

### 1. Drawing generation & profile association

Reuse `productDrawingService.ts` directly from the results view: a compact inline generator (prompt
textarea pre-filled from the profile's `name`/`product_description`/`keywords`/`notes` — e.g. `"3D exploded
technical illustration of {name}: {description}"` — editable before submit) replaces the drawing area when
the profile has no drawing yet. `Generate` calls `generateProductDrawing`, shows the pending image inline
with `Keep`/`Regenerate`/`Ignore`, mirroring `ProductDrawingsView`'s existing preview step so a bad
generation can be discarded before it's persisted.

To let the page remember which drawing belongs to which profile:
- Add `Id int64` to the Go `KeepResponse` struct and `RETURNING id` to the `INSERT INTO
  kb.product_drawings` in `KeepPending` (`server/api/productdrawings/handler.go`) — small additive backend
  change, required because there is currently no way to learn the persisted row's id.
- Add a nullable `kb.product_profiles.drawing_id BIGINT REFERENCES kb.product_drawings(id)` column (goose
  migration). On `Keep`, the frontend calls a small new `PUT`-style profile update (or a dedicated
  `PATCH /kb/product-profiles/:id/drawing`) to persist the returned id against the profile.
- On page load, if the profile has a `drawing_id`, fetch/display it via the existing
  `productDrawingContentUrl(id)`; otherwise show the inline generator.

**Alternative considered:** store `filename` instead of an id (avoids the backend change). Rejected —
there's no "fetch drawing content by filename" endpoint, only by numeric id, so this would need the same
kind of backend addition anyway while being less relational than a proper FK.

### 2. Resizable panels — hand-rolled splitters, not the paneforge `ui/resizable` wrapper

Add layout state local to `product-metric-review-view.svelte`:
```ts
type Layout = { drawingH: number; scopeW: number; detailW: number };
const KEY = 'pmr-review-layout';
const DEFAULTS: Layout = { drawingH: 320, scopeW: 320, detailW: 420 };
```
with the same `startResize`/`moveResize`/`endResize` pointer-capture + `clamp` + `localStorage` persistence
shape as `panel-shell.svelte`. The results (middle) pane stays the flexible `grow` column — only
`scopeW`/`detailW`/`drawingH` are drag-controlled, matching the "one flexible pane, N draggable panes"
approach already used there. A horizontal splitter sits between the drawing area and the pane row; vertical
splitters sit between scope-tree/results and results/metric-details.

**Alternative considered:** adopt `ui/resizable` (paneforge). Rejected for this change — it's present in
the tree but unused in any real view, so using it here would introduce a second, inconsistent resizable-panel
pattern alongside the one already proven in `panel-shell.svelte`, contradicting "match existing style."

### 3. Full width, scoped to the Results tab

`.content`'s `max-width: 1720px` stays for the Report tab (keeps report text readable) but is bypassed for
the Results tab's `.layout` row specifically (e.g. render `.layout` with `width: 100%; max-width: none;`
when `tab === 'results'`, or move it outside the capped wrapper for that tab), so the three panes actually
grow to fill the remaining window/embedded-content width as asked.

### 4. Context-panel suppression is nav-id-scoped, not component-level

`dashboard.svelte` gates both the `ContextShelf` render and its resize divider on
`activeMenu?.childId !== 'apps-product-review'` (in addition to the existing `shelfOpen` check);
`content-panel.svelte` hides the "Open/Close panel" toggle button under the same condition. `shelfOpen`
state itself is left untouched, so switching to another nav item restores whatever open/closed state the
user last had elsewhere. `context-shelf.svelte` is not edited.

### 5. Responsive collapse preserved

The existing `@media (max-width: 1000px)` single-column collapse in `product-metric-review-view.svelte`
stays; the new drag handles are disabled/hidden below that breakpoint (same as `panel-shell.svelte` not
being expected to drag on narrow viewports).

## Risks / Trade-offs

- **Generation cost/latency per profile** → mitigated by generating once and caching via `drawing_id`;
  regeneration is an explicit user action, not automatic.
- **Panes could be dragged to unusably small sizes** → clamp `scopeW`/`detailW`/`drawingH` to sane min/max
  bounds (mirroring `panel-shell.svelte`'s `clamp` calls), same as the existing pattern.
- **Backend `KeepResponse` shape change** → additive field only (`Id`), no existing consumer reads or
  breaks; `productDrawingService.ts`'s `KeptProductDrawing` type gains an optional `id` field.
- **Image-gen failures (Qwen/OpenAI outage, bad output)** → reuse `ProductDrawingsView`'s existing
  pending/error handling rather than reinventing error UI.

## Migration Plan

1. Goose migration: add `kb.product_profiles.drawing_id BIGINT NULL REFERENCES kb.product_drawings(id)`.
   Purely additive; existing profiles get `NULL` and simply show the inline generator on next visit.
2. Backend: add `RETURNING id` / `KeepResponse.Id`, and a small endpoint (or extend the existing profile
   update path) to persist `drawing_id` on a profile.
3. Frontend: layout/splitter state, drawing panel, context-panel suppression — additive UI, no data
   migration.
4. Rollback: drop the `drawing_id` column; frontend changes are UI-only and revert with a normal code
   revert.

## Open Questions

- None blocking — the only deferred item is whether old/replaced drawings (after "Regenerate") should ever
  be garbage-collected; out of scope here since `ProductDrawingsView` has no such cleanup today either.
