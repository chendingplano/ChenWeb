## Why

The Product Review intake page (`apps-product-review`, `product-review-intake-view.svelte`) uses a
bespoke single-column design that ignores the app's standard 3-panel content styling (the rounded-card
look used by Document Review), wastes the panel's available width with a 720px content cap, and offers no
way to pick an image-generation model up front. Separately, 3D product drawings only ever get generated
through a manual step on the post-run Results page — a reviewer has to open a generator panel and click
through Generate → Keep after the review finishes. This is the second request to make drawing generation
automatic at intake time; it still isn't wired in, because `IntakeProductReview`
(`server/api/product-reviews/intake.go`) has no reference to `kb.product_drawings` at all.

## What Changes

- Restyle the Product Review intake page to match the standard middle-panel look used by Document Review:
  each block (the intake form, the past-reviews list) becomes its own full-width rounded-rectangle card
  (`border-radius: 12px`, the same `cardBg`/border convention as `document-review-view.svelte`), stretched
  to the middle panel's width with the standard side margins, replacing the current 720px centered column.
- Rearrange the intake form's fields (Product name, Short description, Keywords, Notes) into a two-column
  grid wide enough to fill the card, and add a new **Image Generation Model** pulldown (Qwen / OpenAI,
  same options as the Generate 3D Product Drawings admin page) so the model is chosen before Start.
- Wire automatic drawing lookup/generation into the intake (Start) submission path
  (`IntakeProductReview` / `server/api/product-reviews/intake.go`): given the product name, check
  `kb.product_drawings` for an existing row; if none exists, generate one synchronously using the selected
  model via the existing `productdrawings` generate+keep flow, then bind the resulting drawing id to
  `kb.product_profiles.drawing_id` via the existing `SetProfileDrawing` store method. If a drawing already
  exists for that product name, its id is bound without regenerating.
- The Results page's existing manual drawing panel is unchanged and still works (e.g. to regenerate with a
  different model later); this change only adds the automatic path at intake time.

## Capabilities

### New Capabilities

- `product-review-intake-layout`: the two-column, card-styled intake form (product name/description/
  keywords/notes/model fields arranged in a full-width grid inside rounded-card panels matching the
  Document Review page) and the full-width, card-styled past-reviews list.
- `product-review-auto-drawing`: automatic 3D product drawing lookup-or-generate-and-bind behavior that
  runs as part of the Product Review intake submission, given a product name and a chosen image generation
  model.

### Modified Capabilities

(none — `product-review-intake` and `product-review-results-layout` capability specs from earlier changes
were never merged into `openspec/specs/`, so there is no base spec to diff against; this change adds two
new, additive capabilities instead.)

## Impact

- Frontend: `web/src/lib/components/home3/product-review-intake-view.svelte` (layout rewrite: cards,
  two-column grid, model select), `web/src/lib/services/productMetricReviewService.ts` or the intake
  submission call site (pass `model` through to the intake request).
- Backend: `server/api/product-reviews/intake.go` (`IntakeProductReview`) gains a drawing lookup/generate/
  bind step; `server/api/product-reviews/store.go` (reuse `SetProfileDrawing`, add a
  find-drawing-by-name query if one doesn't already exist); reuses `server/api/productdrawings`
  (`GeneratePending`/`KeepPending` logic, or a direct internal call to the same provider/insert code to
  avoid an HTTP round-trip) — no new external API surface.
- Database: no schema change (`kb.product_drawings` and `kb.product_profiles.drawing_id` already exist);
  may add a lookup query (`SELECT ... FROM kb.product_drawings WHERE name = $1`).
- No changes to the Results page's manual drawing generator or to any other dashboard page's layout.
