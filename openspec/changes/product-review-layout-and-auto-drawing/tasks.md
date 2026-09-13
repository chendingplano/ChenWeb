## 1. Backend: drawing lookup + generate-and-save helper

- [x] 1.1 Add `FindByName(ctx context.Context, name string) (*ProductDrawing, error)` to
      `server/api/productdrawings/store.go`, matching `Store.FindProfileByName`'s convention
      (`LOWER(TRIM(name)) = LOWER(TRIM($1))`, most recent row first, `nil, nil` on no match).
- [x] 1.2 Factor the PNG-validate + write-file + `INSERT INTO kb.product_drawings ... RETURNING id`
      tail of `KeepPending` (`handler.go`) into a small unexported helper (e.g.
      `saveDrawing(ctx, name, description, prompt, keywords, notes string, img []byte, selection string)
      (*ProductDrawing, error)`), and have `KeepPending` call it.
- [x] 1.3 Add `GenerateAndSave(ctx context.Context, in GenerateAndSaveInput) (*ProductDrawing, error)` to
      `productdrawings` (new exported entry point, no HTTP/echo.Context): resolves `cfg :=
      defaultConfig()`, normalizes the model selection, calls `cfg.Provider.Generate`, then calls the
      Task 1.2 helper to validate/write/insert. Returns the inserted `ProductDrawing` (with `ID`).
- [x] 1.4 Unit-test `FindByName` (match, case/whitespace-insensitive match, no match) and
      `GenerateAndSave` (success path with a fake `ImageProvider`, provider-error path, non-PNG-bytes
      path) using the package's existing test patterns (check for an existing `*_test.go` in
      `productdrawings/` first and follow its style).

## 2. Backend: wire automatic drawing into intake

- [x] 2.1 In `server/api/product-reviews/intake.go`, extend the `IntakeProductReview` request body with
      `Model string \`json:"model"\`` (values `"Qwen"`/`"OpenAI"`, default handled by
      `productdrawings.normalizeSelection`-equivalent logic already in that package).
- [x] 2.2 After the existing `if profile.Status != ProfileReady { ... }` block, add a step guarded by
      `if profile.DrawingID == nil`: call the new `productdrawings.FindByName(ctx, profile.Name)`; if
      found, `store.SetProfileDrawing(ctx, profile.ID, found.ID)`.
- [x] 2.3 If not found: load part-tier component labels via `store.LoadNodes(ctx, profile.ID)` filtered
      to `node_kind == "part" && status != "rejected"`, capped at 15 (mirror
      `product-metric-review-view.svelte`'s `drawingComponents()`), compose the prompt with
      `productdrawings.ComposeExplodedViewPrompt`, call `productdrawings.GenerateAndSave` with the
      request's `Model`, and on success `store.SetProfileDrawing(ctx, profile.ID, saved.ID)`.
- [x] 2.4 On any error from the Task 2.2/2.3 drawing steps, log via the handler's existing
      `rc.GetLogger()` and continue to `StartReview` without failing the request (design.md Decision 6)
      — do not return an error response for a drawing failure.
- [x] 2.5 Add/extend a Go test for `IntakeProductReview` (or the smallest testable unit around it)
      covering: new profile + no existing drawing → drawing generated and bound; new profile + existing
      drawing by name → bound without generating; profile with `drawing_id` already set → neither
      lookup nor generation runs; generation failure → intake still succeeds with `drawing_id` left null.

## 3. Frontend: pass the selected model through

- [x] 3.1 In `web/src/lib/services/productMetricReviewService.ts`, add `model?: 'Qwen' | 'OpenAI'` to
      `startProductReviewIntake`'s input type and pass it through in the request body.
- [x] 3.2 In `product-review-intake-view.svelte`, add `let model = $state<'Qwen' | 'OpenAI'>('Qwen');`
      and include `model` in the `startProductReviewIntake({...})` call inside `start()`.

## 4. Frontend: card layout and two-column form

- [x] 4.1 In `product-review-intake-view.svelte`, wrap the intake form (`.empty-hero`'s form content)
      and the `<section class="history">` block each in a `.pmr-card` div; add a `.pmr-card` style rule
      (`background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: ...`)
      matching design.md Decision 1.
- [x] 4.2 Remove `.content`'s `max-width: 720px; margin: 0 auto`, keeping its existing padding as the
      panel's left/right margins, so both cards stretch to the middle panel's width.
- [x] 4.3 Change `.intake-form` from a vertical flex column to a two-column CSS grid
      (`grid-template-columns: 1fr 1fr`, matching `product-drawings-view.svelte`'s `.form-grid`), with a
      `.wide` class (`grid-column: 1 / -1`) applied to the Short description and Notes fields.
- [x] 4.4 Add the Image Generation Model field to the form as one grid cell: a `<label>` wrapping
      `<select bind:value={model}><option value="Qwen">Qwen · Aliyun</option><option
      value="OpenAI">OpenAI · ChatGPT Image 2.5</option></select>`, styled via the existing
      `.intake-form input, .intake-form textarea` rule extended to include `select`.
- [x] 4.5 Verify the Start button (`.primary.wide`) still spans the full card width below the grid, and
      that the duplicate-name hero (`.empty-hero` shown when `duplicateProfile` is set) still reads
      correctly inside the new card wrapper.

## 5. Verification

- [x] 5.1 Run backend tests for the touched packages: `cd server && go test ./api/productdrawings/...
      ./api/product-reviews/...` (adjust path to this project's actual test invocation/module root).
- [ ] 5.2 Manually exercise the intake page in the running `mise dev` app: submit a brand-new product
      name, confirm a drawing is generated and `kb.product_profiles.drawing_id` is set (check via the
      Results page's drawing panel or a DB query); submit a name that already has a
      `kb.product_drawings` row, confirm it's bound without a new provider call; re-run an
      already-reviewed product, confirm no drawing call happens.
- [x] 5.3 Visually confirm the intake page's two blocks render as full-width rounded cards matching
      Document Review's look, in both dark and light mode, and that the form's fields lay out in two
      columns with description/notes spanning both.
