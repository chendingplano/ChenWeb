## Context

`product-review-intake-view.svelte` is a self-contained design (its own oklch CSS-variable theme
`.pmr-shell`/`.pmr-shell.dark`), centered at `max-width: 720px`, with a single-column flex form
(`.intake-form`) and a below-the-fold `.history-grid` of past reviews. `document-review-view.svelte`
uses a different, simpler theme (hex `cardBg`/`borderColor` tokens) with full-width `padding: 1.5rem`
wrapper and repeated inline `border-radius: 12px` cards per wizard step.

3D product drawings today are generated only through a **two-step, user-driven** flow
(`productdrawings.GeneratePending` → preview → `productdrawings.KeepPending`), built for the System
Admin "Generate 3D Product Drawings" page and reused, unchanged, by a manual panel on the Product
Review **Results** page (`product-metric-review-view.svelte`, `generateDrawing()`/`keepDrawing()`).
`IntakeProductReview` (`server/api/product-reviews/intake.go`) — the handler behind the intake page's
Start button — has no reference to drawings at all. `kb.product_profiles.drawing_id` and
`kb.product_drawings` already exist and are already read/written by the Results-page flow via
`Store.SetProfileDrawing`.

## Goals / Non-Goals

**Goals:**
- Restyle the intake page to look like the rest of the app (full-width rounded cards) without
  introducing a second, conflicting color-token system into the file.
- Let the reviewer pick an image-generation model before starting, reusing the exact Qwen/OpenAI
  options already hardcoded in `product-drawings-view.svelte`.
- Make drawing lookup-or-generation happen automatically as part of the Start submission, with no
  extra click, so `kb.product_profiles.drawing_id` is populated for every reviewed product without the
  reviewer ever visiting the manual generator panel.

**Non-Goals:**
- No changes to the Results page's existing manual drawing panel (still available, e.g. to regenerate
  with a different model later) or to the pending/keep two-step endpoints it uses.
- No schema changes — `kb.product_drawings` and `kb.product_profiles.drawing_id` already exist.
- No dedup/locking for the race where two intakes for a brand-new, identically-named product run
  concurrently (see Risks) — out of scope for a low-concurrency internal/staging tool.

## Decisions

**1. Reuse `.pmr-shell`'s existing theme tokens for the cards, not `document-review-view`'s hex tokens.**
Both files already express "a bordered, 12px-radius surface block" — `document-review-view.svelte` as
`background: {cardBg}; border: 1px solid {borderColor}; border-radius: 12px`, `.pmr-shell` as
`var(--surface)` / `var(--border)`. Importing a second color system into `product-review-intake-view`
would fight its existing dark-mode handling. Instead: wrap the intake form and the history section each
in their own `<div class="pmr-card">` with `background: var(--surface); border: 1px solid var(--border);
border-radius: 12px; padding: ...`, matching Document Review's visual language with this file's own
variables. `.content` drops `max-width: 720px; margin: 0 auto` in favor of full width (the existing
`padding: 26px clamp(...)` already supplies the left/right margins Document Review uses).

**2. Two-column grid mirrors `product-drawings-view.svelte`'s `.form-grid`, not a new pattern.**
That page already solves "arrange product-drawing fields in two columns wide enough to fill the panel"
(`grid-template-columns: 1fr 1fr`, textareas widened via a `.wide` class spanning both columns). Reuse
the same shape for `.intake-form`: Product name / Keywords / **Image Generation Model** each take one
column, Short description / Notes span both via `.wide`. The Start button stays full-width below the
grid, unchanged.

**3. Model select is a plain hardcoded `<select>`, copied verbatim from `product-drawings-view.svelte`.**
The admin page's options (`Qwen`/`OpenAI`) aren't sourced from a DB/config anywhere — they're inline
markup. Duplicating the same two `<option>`s (with a new `model` state field, default `'Qwen'`) is
simpler and more consistent than inventing a shared component for two hardcoded strings. `model` is
added to `startProductReviewIntake`'s request type/body (`'Qwen' | 'OpenAI'`, optional) — the backend
ignores it whenever an existing drawing is found by name.

**4. Auto-drawing runs synchronously inside `IntakeProductReview`, after the scope tree is Ready.**
Placed as a new step after the existing `if profile.Status != ProfileReady { ... }` block (so it also
backfills a drawing for a *resumed* profile that reached Ready before this feature existed), guarded by
`if profile.DrawingID == nil`:
1. `productdrawings.FindByName(ctx, profile.Name)` — trimmed, case-insensitive exact match, mirroring
   `Store.FindProfileByName`'s existing matching convention (`LOWER(TRIM(name)) = LOWER(TRIM($1))`,
   most recent row on ties). If found, `store.SetProfileDrawing(ctx, profile.ID, found.ID)` and stop —
   no image call.
2. Otherwise, generate one: load the profile's accepted part-tier node labels via the existing
   `Store.LoadNodes(ctx, profile.ID)` (same filter the Results-page prompt already uses —
   `node_kind == "part" && status != "rejected"`, capped at 15 labels), compose the prompt with the
   existing `productdrawings.ComposeExplodedViewPrompt(promptDir, profile.Name, components)`, then call
   a new `productdrawings.GenerateAndSave(ctx, GenerateAndSaveInput{Name, Description, Keywords, Notes,
   Prompt, Selection}) (*ProductDrawing, error)`.
3. `store.SetProfileDrawing(ctx, profile.ID, saved.ID)`.

Run synchronously (blocking the intake HTTP response) rather than as a background goroutine: the intake
request already blocks on synchronous LLM calls to build the scope tree (`builder.Build`), so this adds
one more blocking external call of the same shape rather than introducing a new async/polling pattern.

**5. New `productdrawings.GenerateAndSave` bypasses the pending/token dance — there's no one to review
the image before it's kept.** The existing `GeneratePending`/`KeepPending` split exists so a human can
preview the image before committing it to the library; the auto-intake path has no reviewer, so
`GenerateAndSave` does the equivalent of "generate, then immediately keep" in one call: validate the PNG
via the same magic-byte check, write straight to `cfg.OutputDir` (not `PendingDir`), then run the same
`INSERT INTO kb.product_drawings ... RETURNING id` `KeepPending` already runs. The insert/validate/write
logic is factored out of `KeepPending`'s tail into a small unexported helper both call, so the SQL and
column list live in exactly one place.

**6. A failed drawing generation does not fail the review.** If the provider call errors (missing API
key, network, rate limit), log it via the handler's existing `rc.GetLogger()` and continue to
`StartReview` with `drawing_id` left null — the reviewer still gets their review, and can generate a
drawing manually from the Results page afterward exactly as today. This matches goal "make it automatic"
without making drawing generation a hard dependency of the review flow it wasn't part of before.

## Risks / Trade-offs

- **[Risk]** Two intake requests for the same brand-new product name, submitted concurrently, race
  `FindByName` and both generate+insert a drawing (two rows, same name; the later `SetProfileDrawing`
  wins). → **Mitigation:** none added — acceptable for a low-concurrency internal tool where each
  reviewer normally works on distinct products; not worth a DB constraint or locking for this change.
- **[Risk]** Synchronous generation adds image-provider latency (several seconds) to every *new-product*
  Start click. → **Mitigation:** none needed beyond what the existing LLM-driven tree-build step already
  costs; acceptable for a staging tool, and generation is skipped entirely on the common "re-run same
  product" / resume paths once a drawing exists.
- **[Risk]** `GenerateAndSave` duplicates `KeepPending`'s insert statement if not factored out carefully,
  risking drift between the two code paths' column lists. → **Mitigation:** Decision 5's shared helper
  keeps exactly one INSERT.

## Migration Plan

No schema or data migration — `kb.product_drawings` and `kb.product_profiles.drawing_id` already exist
and are unchanged. Deploy is a normal code push (frontend + backend); rollback is reverting the commit,
with no data cleanup required (any drawings auto-generated before rollback remain valid rows, just no
longer auto-bound on future intakes).

## Open Questions

None blocking — `FindByName`'s matching semantics (trimmed, case-insensitive exact match) are fixed by
mirroring `Store.FindProfileByName`'s existing, already-shipped convention rather than inventing a new
one.
