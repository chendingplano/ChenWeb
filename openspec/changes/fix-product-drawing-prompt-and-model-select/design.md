## Context

`server/api/productdrawings/` backs two callers via the shared
`POST /api/v1/product-drawings/generate` (`GeneratePending`) endpoint:

- System Admin → Resources → Generate 3D Product Drawings (fully free-text
  prompt authored by an admin).
- Product Review results page (prompt pre-filled from a locally built string,
  editable, then sent to the same endpoint).

`GeneratePending` currently wraps whatever prompt it receives with
`buildDrawingPrompt()`, which unconditionally appends a hardcoded,
ventilator-specific instruction (`internalPartsInstruction` in `prompt.go`).
This is the source of the "random"/mismatched drawings reported on the
Product Review page: the appended text names ventilator parts (motors, pumps,
valves, filters) regardless of the actual product, biasing the image model.

`kb.product_profile_nodes` already holds a per-product scope tree (`product`
/ `module` / `part` node kinds); the Product Review page loads this tree into
memory (`nodes`) for its scope-tree UI, so part labels for the current
product are already available client-side.

## Goals / Non-Goals

**Goals:**
- Make the generated prompt reflect the actual requested product and its
  real components, not a static ventilator-shaped template.
- Let the Product Review generator pick an image model (Qwen / OpenAI), like
  the admin resource page already does.
- Keep the prompt box editable — auto-fill is a default, not a lock-in.

**Non-Goals:**
- Changing the admin resource page's authoring UX (still fully free-text).
- Changing how `kb.product_drawings` / `kb.product_profiles.drawing_id`
  caching works (already implemented; verified as part of this change, not
  modified).
- Building a general templating engine — one fixed template with two
  placeholders is enough for this use case.

## Decisions

**Template lives in `prompts/`, not in Go or TS source.** Per project
convention (ChenWeb `CLAUDE.md` §2, "NEVER hard-code prompts in code"), the
new exploded-view template is a file,
`prompts/prompt-product-drawing-exploded-view-v1.md`, loaded the same way the
existing canonical ventilator prompt already is (`loadPrompt`'s
`PROMPTS_DIR` resolution). Alternative considered: build the string inline in
`prompt.go` (rejected — same mistake as the code being replaced, and violates
the documented convention) or inline in the Svelte component (rejected — same
convention violation, and the frontend shouldn't own image-model prompt
wording).

**Composition happens server-side via a new preview endpoint, not client-side
string building.** `POST /api/v1/product-drawings/compose-prompt` takes
`{product_name, components: string[]}` and returns `{prompt}` by rendering
the template — no DB access, no image generation. The frontend already has
the product name and part labels in memory (`profileName`, `nodes`), so it
calls this endpoint instead of rebuilding a hardcoded string locally.
Alternative considered: have the frontend fetch parts itself and interpolate
the (still file-based) template client-side — rejected, since the frontend
would need the raw template text shipped to it, and the substitution rule
(dropping the "For instance..." sentence when there are no components) would
end up duplicated in two languages.

**`GeneratePending` stops appending any suffix.** It now sends whatever
`prompt` it's given, verbatim, to the image provider. This makes the admin
resource page's behavior slightly different (no more free automatic
exploded-view reminder appended to hand-typed prompts) but removes the double
"exploded-view requirement" paragraph (old boilerplate + new template) that
would otherwise appear for Product Review requests and reintroduce the same
ventilator bias. `buildDrawingPrompt` and `internalPartsInstruction` are
deleted as they become unused (only caller is `GeneratePending`; the older
`Generate`/`NewHandler` endpoint never called them).

**Component selection: `node_kind = 'part'`, excluding `status = 'rejected'`,
capped at 15.** Rejected nodes are explicitly things the reviewer said don't
belong; proposed nodes are still plausible parts and are worth including.
15 is a soft cap to keep the prompt a reasonable length for the image model;
if a profile has fewer parts, all are used and the sentence just lists fewer
items.

**Model dropdown reuses the existing `model` field.** `GeneratePending`
already accepts `model: "qwen"|"openai"` and defaults sensibly when absent
(`normalizeSelection`). No backend change needed for model selection itself
— only the Product Review UI needs a `<select>` wired into the existing
`generateProductDrawing()` payload, mirroring
`product-drawings-view.svelte`'s options exactly (`Qwen · Aliyun` /
`OpenAI · ChatGPT Image 2.5`, default `Qwen`).

## Risks / Trade-offs

- **[Risk]** Admin resource page prompts lose the automatic exploded-view
  reminder they used to get for free → **Mitigation**: that page's UX already
  asks the admin to "describe the 3D technical drawing and the parts it
  should show" themselves; the placeholder text already implies a
  self-contained prompt is expected. Flagged in the proposal for visibility.
- **[Risk]** A product with zero accepted/proposed part nodes (e.g. a brand
  new profile) yields a prompt with no component examples → **Mitigation**:
  template renders the sentence-less variant gracefully instead of a broken
  sentence; this is an explicit, tested case.
- **[Risk]** 15-item cap could drop relevant parts for very large product
  trees → **Mitigation**: acceptable per YAGNI; can be revisited if reported.

## Migration Plan

No data migration. Deploy is a normal code + prompt-file rollout:
1. Add the new prompt file and backend endpoint (additive, no behavior change
   yet).
2. Switch `GeneratePending` to stop appending the old suffix and delete the
   dead code.
3. Switch the Product Review frontend to call the new endpoint and add the
   model dropdown.
Rollback is a plain revert; no persisted state format changes.
