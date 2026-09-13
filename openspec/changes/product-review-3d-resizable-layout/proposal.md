## Why

The Product Review results page (`/home3/product-metric-review`, embedded under the `apps-product-review`
nav item) renders the scope tree, results list, and metric-details pane as a fixed-width CSS grid capped
at 1720px, and shares the dashboard shell with the generic "APP STATUS" context panel even though this
page never uses it — wasting the panel's width and leaving no visual identification of the product under
review. Reviewers want a generated reference drawing of the product they're reviewing, the freedom to
resize the three working panes to their liking, and the full window width for a data-dense review
surface.

## What Changes

- The results page generates (once per profile, cached) a 3D-style product drawing for the reviewed
  product, reusing the existing "Generate 3D Product Drawings" flow (`productDrawingService.ts` /
  `server/api/productdrawings`) rather than a new generation mechanism, and displays it above the
  three-pane layout.
- The scope tree, results list, and metric-details panes become three independent, user-resizable panels
  with draggable vertical splitters between them, reusing the existing `resizable` UI primitives
  (`ResizablePaneGroup`/`ResizablePane`/`ResizableHandle`) already used elsewhere in the app (e.g.
  `metric-ontology-explorer-view.svelte`). The page's content width cap is removed so the three panels
  expand to fill the full remaining window width.
- A draggable horizontal splitter between the drawing area and the three-panel row lets the user resize
  the drawing's height.
- The generic app-shell "APP STATUS" context panel and its "Open/Close panel" toggle are hidden while
  Product Review (`apps-product-review`) is the active dashboard content. This is done by conditioning
  `dashboard.svelte`'s/`content-panel.svelte`'s rendering on the active nav child id — `context-shelf.svelte`
  itself is untouched and every other page keeps the panel exactly as it works today.
- Panel sizes (drawing height, pane widths) persist per-viewer via `localStorage`, mirroring the existing
  pattern in `panel-shell.svelte` (metric-ontology-explorer).

## Capabilities

### New Capabilities

- `product-review-results-layout`: the resizable drawing/scope-tree/results/metric-details layout for the
  Product Review results page, including on-demand 3D drawing generation for the reviewed product and
  suppression of the app-shell context panel while this page is active.

### Modified Capabilities

(none — the existing `product-metric-reviewer-page` capability's spec file was never merged into
`openspec/specs/` when its change was archived, so there is no base spec to diff against; this change adds
a new, additive capability instead of editing that page's core scope-tree/results/evidence behavior, which
is unchanged.)

## Impact

- Frontend: `web/src/lib/components/home3/product-metric-review-view.svelte` (layout, drawing panel),
  `dashboard.svelte` and `content-panel.svelte` (context-panel suppression), new use of
  `web/src/lib/services/productDrawingService.ts` and the `web/src/lib/components/ui/resizable/*`
  primitives.
- Backend: no new endpoints — reuses `/api/v1/product-drawings/*`. One additive DB migration to associate
  a generated drawing with a review profile (nullable FK), see design.md Decision 1.
- No changes to any other dashboard page's context-panel behavior.
