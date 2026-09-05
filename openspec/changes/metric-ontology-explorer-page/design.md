## Context

`home3/knowledge` is a single large route (`+page.svelte`, ~940 lines) that switches a content
area between ~25 section components by an `activeSection: KbSectionId` string, with a hardcoded
`menuItems` tree, a DB-backed label/visibility overlay (`kb.page_config`), and a per-group
open/close state (e.g. `ontologyOpen`). Sections are plain components rendered in an
`{#if}/{:else if}` ladder and given `{darkMode}` plus a few props. The `Ontology` group today has
one child, `Metric Ontology` (`kb-metric-ontology` → `MetricOntologyAnalysisView`).

The design brief (`KnowledgeStore/doc-repo/specs/202609/2026090501-spec-metric-graph-page-design.md`)
asks for a new graph-first page with a canvas, a tabbed content viewer, and a PDF viewer in
resizable panels. Prototype **B v2** at
`ChenWeb/prototypes/metric-ontology-explorer/metric-graph-B-atlas.html` is the interaction
reference: an elliptical "orrery" of a `Metric` centre plus nine satellites, seven of which
unfold a fixed downstream chain (one at a time; the active satellite swings to the reading
position), an editorial entry tab plus one closeable record tab per chain node, and a
book-styled source pane with evidence highlighting.

Constraints that shape this design:

- ChenWeb `CLAUDE.md`: minimum code, surgical edits to shared files, match existing style, no
  speculative abstraction, no un-requested configurability.
- The frontend has **no D3**; it uses `echarts` / `svelte-echarts` and Svelte 5 runes. A prior
  change (`redesign-metric-ontology-model-graph`) explicitly rejected adding a graph library for
  a diagram, hand-rolling inline SVG instead.
- A real PDF viewer already exists: `shared-pdf-viewer.svelte` (props `inputId` / `fileUrl`,
  `page`/`zoom` bindable, `renderHighlights` + `highlightVersion` snippet API) backed by
  `/api/v1/kb/inputs/:id/file`.
- Read endpoints already cover most of the record-tab tables (`/api/v1/kb/metrics*`,
  `/inputs*`, `/objects*`, `/object-nodes/:id`, `/keyword-concepts`, `/ontology/terms`,
  `/semantic-assertions`, `/metrics/ontology-analysis`). No read endpoint exists for
  `kb.class_contract_revisions`, `kb.assertion_evidence`, `kb.semantic_decision_candidates`, a
  `kb.metric_definitions` listing, or metric-scoped processor run logs.

## Goals / Non-Goals

**Goals:**

- A self-contained `MetricOntologyExplorerView` reachable at
  `home3/knowledge → Ontology → Metric Ontology Explorer`, wired into `+page.svelte` with the
  same small footprint as any other section (union member, menu child, import, `ontologyOpen`
  handling, one render branch).
- The three-pane resizable workspace with both arrangements and per-browser persistence.
- The canvas orrery — rectangular icon+label nodes, an enlarged `Metric` centre, pan/zoom, re-fit,
  the nine satellites, and the seven fixed chains with one-at-a-time unfold/fold and the swing.
- The content viewer's entry-tab + record-tab model, with record tabs reading live data where an
  endpoint exists and showing a schema panel otherwise.
- The source pane reusing `shared-pdf-viewer`, with evidence-span highlighting driven from the
  canvas / active record tab.
- Zero new npm dependencies; theme via the knowledge view's existing tokens.

**Non-Goals:**

- No backend change. The five missing read endpoints are a separate follow-up change; this
  change ships them as schema panels.
- No change to the existing `Metric Ontology` dashboard, its route, or `metricOntologyAnalysisService`.
- No reuse of the prototype's bespoke "Atlas" palette/typography (Fraunces/Spectral, warm paper).
  The real component adopts the app's existing knowledge-view tokens and light/dark modes.
- No editing/curation actions from this view — it is read + navigate only.
- No graph search, saved views, deep-linking to a canvas state, or a minimap in this change.

## Decisions

### D1. Hand-rolled inline SVG canvas in Svelte 5 — not D3, not ECharts

The topology is fixed (10 base nodes, ≤4 chain nodes) and the layout is deterministic (an ellipse
for the satellites; a vertical column for the open chain). The interaction set is: click-to-select,
click-to-unfold/fold, wheel-zoom, drag-pan, re-fit, and one eased rotation to bring the active
satellite to angle 0. None of that needs a force simulation or a scene graph.

- **Chosen:** an `<svg>` with a root `<g>` carrying a `translate(x,y) scale(k)` transform;
  nodes/edges from `{#each}` over `$derived` position arrays; pan/zoom from ~40 lines of
  `pointerdown`/`pointermove` + `wheel` handlers writing `{x,y,k}` `$state`; the swing from a
  `requestAnimationFrame` ease on a single `rot` value (or a CSS transition on the `<g>` transform
  where a jump is acceptable); enter/leave via Svelte's keyed-each transitions.
- **Alternative — add D3** (`d3-zoom`/`d3-selection`/`d3-transition`/`d3-timer`, ~30 KB): a near
  1:1 port of the prototype, fastest to working code. Rejected: a new dependency and stack
  divergence for behavior we can express directly, against the prior change's stated position.
- **Alternative — ECharts `graph`/custom series:** matches `tree-graph-view.svelte`, gets `roam`
  for free. Rejected: rectangular icon+label nodes, the swing, and per-click chain add/remove all
  fight ECharts' animation and data-update model; the result is more custom-series code than the
  hand-rolled SVG, and less controllable.

### D2. Component decomposition

`metric-ontology-explorer-view.svelte` is the entry component (props: `darkMode`, optional
`metricId`). It owns cross-pane state (focused node, open chain, open tabs, arrangement) and
composes:

- `metric-ontology-explorer/panel-shell.svelte` — the two arrangements, splitters, min-sizes,
  `localStorage` persistence, and a `ResizeObserver` that notifies the canvas.
- `metric-ontology-explorer/ontology-canvas.svelte` — the SVG orrery, pan/zoom, layout math,
  unfold/fold + swing; emits `focus(nodeId)` and `openRecord(chainNodeId)`.
- `metric-ontology-explorer/content-viewer.svelte` — the entry tab (Definition / Connection /
  Processing / Sources) and the dynamic record tabs; renders live rows or the schema panel per
  node config.
- `metric-ontology-explorer/source-pane.svelte` — a thin wrapper over `shared-pdf-viewer` with
  the evidence-highlight snippet and the empty state.
- `metric-ontology-explorer/model.ts` — the static ontology: the nine satellites, their entry
  copy, and the seven chains, each chain node carrying
  `{ id, label, icon, table?, columns, description, loader?, evidenceSpans? }`.

Splitting keeps each file focused (ChenWeb `CLAUDE.md` §1.2) and each pane independently
testable. Nothing here is extracted for reuse beyond this view.

### D3. The ontology graph and chains are static data, fetched nothing

`model.ts` defines the diagram. This matches the prior graph change's "hand-placed topology"
decision and the brief's framing of the canvas as a teaching diagram. Only **record tabs** and the
**PDF** touch the network.

### D4. Layout math

- Satellites on an **ellipse** `rx = clamp(hostW*0.32, 240, 430)`, `ry = clamp(hostH*0.30, 88,
  150)`, evenly spaced by index; an ellipse (not a circle) suits the wide, short canvas band the
  Top-Bottom arrangement produces and spreads the wider rectangles with less overlap.
- On unfold, `rot` eases so the active satellite reaches angle 0 (the 3-o'clock reading
  position); the chain is a **vertical column** offset to the right of that satellite,
  `x = sx + STEP`, `y = sy + (k - (n-1)/2) * ROW`, so wide chain rectangles never overlap each
  other regardless of chain length (the Processor chain has four).
- `fitView()` computes the bounding box of all visible nodes (± half-extent) and sets `{x,y,k}`
  to frame it with padding, clamped to `[0.42, 1.35]`; it runs on unfold/fold, on re-fit, and on
  `ResizeObserver`.

### D5. Panel shell without DOM re-parenting

The three panes are three components rendered in both arrangement templates behind an
`{#if arrangement === 'lr'}`; each arrangement is a nested flex with JS-controlled pixel sizes
(`leftW`, `canvasH`, `tabsW`). Splitters use `setPointerCapture` and write clamped sizes to
`$state`; `endResize` persists. State shape `{ arrangement, leftW, canvasH, tabsW }` is written to
`localStorage` under `moe-explorer-layout`, read through a `try/catch` with defaults so SSR and a
blocked/empty store both fall back cleanly.

### D6. Record tabs: one config, two renderers

Each chain-node config either has a `loader` (an async `() => rows[]` over an existing service) →
the tab renders loading / error / a rows table; or has none → the tab renders a **schema panel**
(`table`, `columns`, `description`, no rows). Live in this change:

| Node | Source | Service |
| --- | --- | --- |
| Object Mention | `kb.artifact_objects` | `objectManagerService.searchObjects` |
| Object Node | `kb.object_nodes` | `/api/v1/kb/object-nodes/:id` |
| Keyword Concept | `kb.keyword_concepts` | `/api/v1/kb/keyword-concepts` |
| Ontology Term | `kb.ontology_terms` | `/api/v1/kb/ontology/terms` |
| Semantic Assertion | `kb.semantic_assertions` | `/api/v1/kb/semantic-assertions` |

Schema panel in this change: `Class Contract Revision` (`kb.class_contract_revisions`),
`Assertion Evidence` (`kb.assertion_evidence`), `Decision Candidate`
(`kb.semantic_decision_candidates`), a `kb.metric_definitions` listing, and the four processor
stages' run logs. A follow-up change replaces each schema panel with a `loader` as its endpoint
lands — no other change to this view.

### D7. Source pane and the "focused metric"

The explorer opens from the menu with **no metric selected**; it also accepts `?metric_id=` (the
pattern `kb-metric-wiki` already uses via `page.url.searchParams`) so other surfaces (metric
list, search, the future record-row click) can deep-link one in. With a metric in context the
source pane resolves its `record_id` (`kb.inputs.id`) and loads `/api/v1/kb/inputs/:id/file` in
`shared-pdf-viewer`; evidence spans from the active node/tab are passed through the viewer's
`renderHighlights` + `highlightVersion` API. With no metric in context the source pane and any
instance-level entry content show an explicit empty state, and record tabs still work at table
level.

### D8. Theming

`darkMode` threads from `+page.svelte` into the view and every pane. Colors, borders, surfaces,
and the canvas node/edge strokes use the knowledge view's existing CSS custom properties and their
`.dark` overrides; no new token is introduced. This is a deliberate step back from the
prototype's bespoke palette to keep the page consistent with the rest of `home3/knowledge`.

### D9. `+page.svelte` edit is minimal and pattern-matching

Add `'kb-metric-ontology-explorer'` to `KbSectionId`; add the menu child under
`kb-metric-ontology` after `Metric Ontology`; add the import; extend the `ontologyOpen`
open-on-select / is-open checks that already special-case the Ontology group's children; add one
`{:else if activeSection === 'kb-metric-ontology-explorer'}` branch. No refactor of the ladder.

## Risks / Trade-offs

- **[Two panes show "no data" until a metric is chosen]** the source pane and instance-level
  entry content are empty on a cold open from the menu. → Mitigation: explicit empty states that
  name what's missing and how to get there (open with `?metric_id=`); record tabs remain useful
  cold; a metric picker is flagged as an Open Question, not shipped.
- **[Five record tabs are schema panels, not data]** a reader clicking `Assertion Evidence`,
  `Decision Candidate`, `Class Contract Revision`, the metric-definitions list, or a processor
  run log gets structure, not rows. → Mitigation: the panel states plainly that the endpoint is
  pending; the follow-up change is scoped to exactly these five; no fabricated rows.
- **[Hand-rolled pan/zoom and layout math is bespoke code to maintain]** → Mitigation: it is
  small and isolated in `ontology-canvas.svelte`; the layout invariants (ellipse params, chain
  column offsets, fit clamps) are documented at the top of that file.
- **[The Top-Bottom canvas band is short; a 4-node chain plus the ellipse can crowd it]** →
  Mitigation: `fitView` reframes on every unfold and on resize; the chain column is vertical so
  its width is one node; Left-Right remains available for a taller canvas.
- **[Prototype look is not carried over]** stakeholders who liked the Atlas palette see a plainer
  page. → Mitigation: called out as a Non-Goal; theme consistency with `home3/knowledge` is the
  intended outcome, and node shape / icons / the orrery structure are preserved.
- **[`+page.svelte` is large and central]** any edit risks the section ladder. → Mitigation:
  additive-only, following the `kb-metric-ontology` precedent line-for-line; covered by a
  post-change check that every existing section still selects and renders.

## Migration Plan

Additive front-end-only change, no flag (consistent with how other `home3/knowledge` sections
have shipped). Deploy via the normal `cd ChenWeb && mise dev` / build. Rollback is a revert of the
new files plus the `+page.svelte` hunk; no data migration, no API change. The one new
`[knowledge-content]` label key is optional — the hardcoded default renders if it is absent.

## Open Questions

- **Metric picker in the canvas pane?** A control to choose/search a metric so the source pane and
  instance content work without a `?metric_id=` deep link. Left out of this change; likely a fast
  follow once the page exists.
- **Record-row → focus a metric?** Clicking a row in a live record tab (e.g. an `Object Mention`)
  could set the focused metric and drive the source pane. Deferred with the picker.
- **Chain node icons** — reuse the small glyph set from the prototype, or pull from the icon set
  `+page.svelte` already imports (`NetworkIcon`, etc.)? Cosmetic; decide during implementation.
- **Processor run-log scoping** — once an endpoint exists, is a run log filtered to the focused
  metric, or the latest N runs of the stage overall? Belongs to the follow-up change.
