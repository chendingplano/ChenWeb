## 1. Backend: prompt template + composition

- [x] 1.1 Add `prompts/prompt-product-drawing-exploded-view-v1.md` with the
      exploded-view template (`{{PRODUCT_NAME}}` and `{{COMPONENTS_SENTENCE}}`
      placeholders).
- [x] 1.2 Replace `internalPartsInstruction`/`buildDrawingPrompt` in
      `server/api/productdrawings/prompt.go` with
      `ComposeExplodedViewPrompt(productName string, components []string) (string, error)`
      that loads the new template and fills placeholders, omitting the
      component-listing sentence when `components` is empty.
- [x] 1.3 Add unit tests in `prompt_test.go` covering: components present,
      components empty, product name substituted in both places.

## 2. Backend: preview endpoint + generate cleanup

- [x] 2.1 Add `ComposePrompt` handler: `POST /api/v1/product-drawings/compose-prompt`
      taking `{product_name, components: []string}`, returning
      `{prompt: string}`; no DB access, no image call.
- [x] 2.2 Register the route in `server/api/routes.go` next to the other
      `product-drawings` routes.
- [x] 2.3 Update `GeneratePending` in `handler.go` to use `prompt` as-is
      (drop the `buildDrawingPrompt(prompt)` wrapper).
- [x] 2.4 Update/extend `handler_test.go` to assert the image provider
      receives the prompt verbatim (no appended suffix) and add a test for
      the new compose-prompt handler.

## 3. Frontend: service layer

- [x] 3.1 Add `composeDrawingPrompt(productName: string, components: string[])`
      to `web/src/lib/services/productDrawingService.ts` calling the new
      endpoint.
- [x] 3.2 Add/extend tests in `productDrawingService.test.ts` for the new
      function.

## 4. Frontend: Product Review generator UI

- [x] 4.1 In `product-metric-review-view.svelte`, replace the local
      `buildDrawingPrompt()` string builder with a call to
      `composeDrawingPrompt(profileName, parts)`, where `parts` is derived
      from `nodes` (`node_kind === 'part' && status !== 'rejected'`, labels,
      capped at 15).
- [x] 4.2 Add a `drawingModel` state (`'Qwen' | 'OpenAI'`, default `'Qwen'`)
      and a `<select>` next to the prompt textarea with the same two options
      as `product-drawings-view.svelte`.
- [x] 4.3 Pass `model: drawingModel` through in the `generateProductDrawing()`
      call inside `generateDrawing()`.

## 5. Verification

- [x] 5.1 Run `cd server && go test ./api/productdrawings/...`.
- [x] 5.2 Run frontend unit tests covering `productDrawingService.ts`.
- [ ] 5.3 Manually exercise the Product Review page: open a profile with
      part nodes, confirm the prompt box pre-fills with the new template
      text including real component names, generate with each model option,
      keep the drawing, and confirm `kb.product_profiles.drawing_id` is set
      and the drawing reloads from cache on revisit (requirement 3 —
      verification only, no code change expected).
