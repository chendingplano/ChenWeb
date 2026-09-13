## Why

The Product Review page's "Generate 3D Drawing" button produces drawings that
don't match the requested product. The backend always appends a static,
ventilator-specific instruction string (mentioning "motors or pumps, valves,
filters", etc.) to every prompt, which biases the image model away from
whatever product was actually requested (e.g. a blood-pressure monitor). The
same flow also has no way to pick an image-generation model, even though the
underlying API already supports one.

## What Changes

- Replace the hardcoded, ventilator-biased prompt suffix
  (`internalPartsInstruction` / `buildDrawingPrompt` in
  `server/api/productdrawings/prompt.go`) with a product-agnostic template
  file (`prompts/prompt-product-drawing-exploded-view-v1.md`) that is filled
  in with the actual product name and a real, product-specific component list.
- Add `productdrawings.ComposeExplodedViewPrompt(productName, components)` and
  a new stateless `POST /api/v1/product-drawings/compose-prompt` endpoint that
  renders this template for preview purposes (no DB writes, no image call).
- `GeneratePending` (`POST /api/v1/product-drawings/generate`) stops
  auto-appending the old static suffix and uses whatever prompt it is given
  as-is.
- On the Product Review results page, the "Drawing prompt" textarea is now
  pre-filled by calling the new compose-prompt endpoint with the profile's
  name and its part-tier scope-tree node labels (`node_kind = 'part'`,
  excluding rejected nodes, capped at 15), instead of the old local
  `buildDrawingPrompt()` string builder. The field remains editable before
  generating.
- Add a model dropdown (`Qwen · Aliyun` / `OpenAI · ChatGPT Image 2.5`,
  mirroring the System Admin → Resources → Generate 3D Product Drawings page)
  to the Product Review drawing generator, wired into the existing
  `generateProductDrawing()` call.

## Capabilities

### New Capabilities
- `product-drawing-prompt-composition`: server-side composition of the
  exploded-view drawing prompt from a template, a product name, and a
  product-specific component list, exposed via a preview endpoint and used
  by prompt generation.

### Modified Capabilities
(none — no existing spec covers product-drawing generation)

## Impact

- `server/api/productdrawings/prompt.go`, `handler.go`, `models.go`
- `server/api/routes.go` (new route)
- `prompts/prompt-product-drawing-exploded-view-v1.md` (new)
- `web/src/lib/services/productDrawingService.ts`
- `web/src/lib/components/home3/product-metric-review-view.svelte`
