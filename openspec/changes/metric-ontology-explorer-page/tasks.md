> Verification legend: tasks marked "(needs mise dev)" require the full stack
> (Go API + Postgres + Clickhouse) and were **not** run during implementation.
> Everything else was verified by `svelte-check`, the model unit test, or an
> isolated in-browser harness of the view mounted outside `home3/+layout`.

## 1. Wiring into home3/knowledge

- [x] 1.1 Add `'kb-metric-ontology-explorer'` to the `KbSectionId` union in
      `web/src/routes/home3/knowledge/+page.svelte`.
- [x] 1.2 Add a `Metric Ontology Explorer` child to the `kb-metric-ontology` menu group,
      immediately after the existing `Metric Ontology` child, with description
      "Explore a metric's ontology and pipeline on a canvas".
- [x] 1.3 Import `MetricOntologyExplorerView` and add an
      `{:else if activeSection === 'kb-metric-ontology-explorer'}` branch that renders it with
      `{darkMode}` and `metricId={metricOntologyExplorerId}` (derived from
      `page.url.searchParams.get('metric_id')`).
- [x] 1.4 Extend the `ontologyOpen` open-on-select and is-open checks so selecting the new child
      expands the `Ontology` group — the existing `selectSection` already special-cases any child
      of `kb-metric-ontology`, so the menu entry is sufficient; also added the new id to the
      `needsActiveStore` exclusion list so the page opens without a store selected.
- [x] 1.5 Add the `kb-metric-ontology-explorer` label to `config/knowledge-content/labels-zh-cn.toml`
      (`指标本体浏览器`); English keeps the hardcoded default per the labels-en.toml convention.
- [x] 1.6 Create `web/src/lib/components/home3/metric-ontology-explorer-view.svelte` — the real
      composer (not a stub): owns cross-pane state and mounts the shell + three panes.

## 2. Static ontology model

- [x] 2.1 Create `web/src/lib/components/home3/metric-ontology-explorer/model.ts` with the
      `Metric` centre and the nine satellites (id, label, glyph, edge verb, entry copy for
      Definition/Connection/Processing/Sources).
- [x] 2.2 Add the seven chains to `model.ts` exactly as specified: Object → Object Mention →
      Object Node; Keyword → Keyword Concept; Metric Definition → Ontology Term → Class Contract
      Revision; Processor → Extract Metrics → Normalize Assertion → Associate Semantics → Project
      Semantics; Evidence → Assertion Evidence → Decision Candidate → Semantic Assertion.
- [x] 2.3 Each chain node carries `{ table, columns[], description }`; `loaderKey` is set on the
      five with an existing endpoint (Object Mention, Object Node, Keyword Concept, Ontology Term,
      Semantic Assertion); `evidenceSpans` set on `Assertion Evidence`.
- [x] 2.4 `model.test.ts` asserts chain shapes/order, that exactly five chain nodes carry a
      `loaderKey` (live) and the other seven do not (schema panel), and the `evidenceSpans` hook.
      7/7 pass under `bun test`.

## 3. Panel shell

- [x] 3.1 `metric-ontology-explorer/panel-shell.svelte` accepts `canvas` / `content` / `source`
      snippet props plus `tokens`; owns the `arrangement` state.
- [x] 3.2 Left-Right template (left column: canvas over content; right: source) and Top-Bottom
      template (canvas band; bottom row: content | source) as nested flex with `leftW` /
      `canvasH` / `tabsW` pixel sizes.
- [x] 3.3 Drag splitters with deferred `setPointerCapture`, per-splitter min clamps; the canvas
      runs its own `ResizeObserver` on its host so it re-fits on any pane resize.
- [x] 3.4 Persist `{ arrangement, leftW, canvasH, tabsW }` to `localStorage` (`moe-explorer-layout`)
      via `browser`-guarded `try/catch` with defaults; arrangement toggle + "Reset layout" control.
- [x] 3.5 Verified in harness: switching arrangement keeps the canvas selection, open tabs and the
      loaded doc; splitter drag resizes and re-fits; reset restores default sizes only. (Persist
      is written on every change; a literal reload-restore was not exercised — the load path is a
      guarded `localStorage.getItem` + `JSON.parse`.)

## 4. Ontology canvas

- [x] 4.1 `metric-ontology-explorer/ontology-canvas.svelte`: `<svg>` + root `<g>` with a
      `translate(x,y) scale(k)` transform; layout invariants documented in the header comment.
- [x] 4.2 Enlarged `Metric` centre and nine satellite rectangles (glyph + label + kind caption)
      on the ellipse from a `$derived` positions array; Metric→satellite edges.
- [x] 4.3 Wheel-zoom toward pointer and drag-pan writing `{tx,ty,k}` `$state` (pan captures the
      pointer only past a 3px threshold so node clicks are not swallowed); re-fit control and a
      `fitView()` that frames all visible nodes and runs on unfold/fold, re-fit and resize.
- [x] 4.4 Unfold/fold: clicking a chained satellite opens its chain (folding any other), clicking
      it again folds; the ring rotates (CSS transition) so the active satellite reaches angle 0;
      the chain is a vertical column right of that satellite; chainless satellites only focus.
- [x] 4.5 `onfocus(nodeId)` on any node click, `onopenrecord(chainNodeId)` on a chain-node click;
      a `Metric · <satellite> ▸ …` breadcrumb whose crumbs call `onopenrecord`.
- [x] 4.6 Keyed `{#each}` with `scale` in/out transitions for chain nodes/edges; folding clears
      `openChain` in the composer, which drops the chain nodes and closes their record tabs.

## 5. Content viewer

- [x] 5.1 `metric-ontology-explorer/content-viewer.svelte` with an always-present entry tab and a
      dynamic `openTabs` list; the entry tab renders the focused node's
      Definition/Connection/Processing/Sources via a section sub-nav.
- [x] 5.2 Record tabs open on `onopenrecord`, activate, de-duplicate, are individually closeable;
      closing the active one falls back to the entry tab. (De-dup + close owned by the composer.)
- [x] 5.3 Live renderer calls the node's loader via `LOADERS[loaderKey]`, shows loading / a
      contained error / a rows table with the config columns.
- [x] 5.4 Schema-panel renderer: table name pill + column list + description + "read endpoint
      pending" note, no rows.
- [x] 5.5 `web/src/lib/services/metricOntologyExplorerService.ts` — five loaders over the existing
      `/api/v1/kb/*` reads, reusing `objectManagerService.searchObjects`; no new endpoint;
      `git diff web/package.json` empty.
- [ ] 5.6 (needs mise dev) Confirm the five live tabs render real rows against a dev DB and the
      seven schema-panel tabs render structure only. Harness confirmed both code paths and the
      contained-error state (loaders 500 without an API).

## 6. Source-document pane

- [x] 6.1 `metric-ontology-explorer/source-pane.svelte` wraps `shared-pdf-viewer`; resolves the
      focused metric via `getMetricWiki(metricId)` →
      `page.in_this_corpus.source_document.record_id` → `inputId` + `/api/v1/kb/inputs/:id/file`.
- [x] 6.2 Evidence spans from the active chain node surface in the pane's bar as a count/label;
      a footnote states span-level highlighting lands with the `kb.assertion_evidence` endpoint
      (there is no bbox data in v1 — `Assertion Evidence` is a schema panel).
- [x] 6.3 Empty state when no metric is in context (`?metric_id=` hint) or the metric has no
      linked document — no error thrown.
- [ ] 6.4 (needs mise dev) Verify with a real `?metric_id=` deep link: the document loads in
      `shared-pdf-viewer` and the evidence bar shows for the `Assertion Evidence` tab.

## 7. Theming and responsiveness

- [x] 7.1 `darkMode` threads from `+page.svelte` into the view and every pane; all colour comes
      from `metric-ontology-explorer/theme.ts`, whose values mirror the knowledge shell palette;
      no new global token.
- [x] 7.2 Verified light and dark parity in harness for the canvas, panes, tabs, splitters and
      the schema panel.
- [ ] 7.3 (needs mise dev) Sweep ~1880 / ~1280 / ~900 px in the real page: panes usable, canvas
      re-fits, no page-level horizontal scrollbar. Verified at 1500px in the harness; panes use
      `min-width: 0` / `min-height: 0` throughout and the canvas re-fits via `ResizeObserver`.

## 8. Verification

- [ ] 8.1 (needs mise dev) `cd ChenWeb && mise dev`; open `home3/knowledge → Ontology → Metric
      Ontology Explorer` and walk every spec scenario end to end.
- [ ] 8.2 (needs mise dev) Confirm every other `home3/knowledge` section still selects and
      renders. (The `+page.svelte` edit is additive only — union member, menu child, import,
      render branch, one exclusion-list entry — and `svelte-check` is clean.)
- [x] 8.3 `bun run check` (svelte-check): the two errors and 44 warnings are the pre-existing
      repo baseline in unrelated files; **zero** in the new files or the `+page.svelte` edit.
- [x] 8.4 `git diff web/package.json` is empty — no dependency added.
- [x] 8.5 `openspec validate --changes metric-ontology-explorer-page --strict` → `✓`.
