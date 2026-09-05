## Why

`home3/knowledge → Ontology` has one page today — **Metric Ontology** (`kb-metric-ontology`), a
flat diagnostics dashboard. There is no place a reader can start from the concept "Metric" and
explore, visually, what a metric touches and how the pipeline builds it: which document it came
from, which object it measures, which keyword concept named it, which governed definition shapes
it, which processor stages produced it, and which evidence, decision candidates and semantic
assertions stand behind it. The design brief
`KnowledgeStore/doc-repo/specs/202609/2026090501-spec-metric-graph-page-design.md` calls for a
dedicated, graph-first explorer with a canvas, a tabbed content viewer, and a source-document
viewer arranged in resizable panels.

## What Changes

- Add a new knowledge section **Metric Ontology Explorer** (`kb-metric-ontology-explorer`) as a
  second child under the existing `Ontology` menu group in `home3/knowledge`, rendered by a new
  `MetricOntologyExplorerView` component. The existing `Metric Ontology` dashboard is unchanged.
- The view is a **three-pane workspace**:
  - **Canvas** — a pan/zoom "orrery": a `Metric` centre node with nine satellites (Document,
    Object, Keyword, Evidence, Metric Definition, Processor, Product, Analysis, Misc). Seven of
    the satellites unfold a downstream chain of nodes on click and fold on a second click; one
    chain is open at a time and its satellite rotates to the reading position as the chain
    unfurls. Nodes are rectangles carrying an icon and a label.
  - **Tabbed content viewer** — an editorial "entry" tab for the focused node, plus one closeable
    **record tab** per chain node the reader opens, showing rows from that node's `kb.*` table
    (or, for processor stages, its recent run log).
  - **Source-document viewer** — the existing `shared-pdf-viewer` bound to the focused metric's
    source input, with evidence spans highlighted from the canvas / content viewer.
- The three panes support **two arrangements** (Left-Right and Top-Bottom) and **drag-resizable
  splitters**, with sizes and arrangement persisted per browser.
- The canvas is **hand-rolled inline SVG** driven by Svelte 5 runes — no new frontend dependency
  (the repo has no D3; a fixed topology plus a deterministic layout does not need one).
- **Chain topology and the ontology diagram are static** (defined in the component). **Record
  tabs read live data** where a read endpoint already exists — `kb.metrics`, `kb.inputs`,
  `kb.artifact_objects`, `kb.object_nodes`, `kb.keyword_concepts`, `kb.ontology_terms`,
  `kb.semantic_assertions`, and the ontology-analysis summary. Where no read endpoint exists yet
  (`kb.class_contract_revisions`, `kb.assertion_evidence`, `kb.semantic_decision_candidates`,
  a `kb.metric_definitions` list, and processor run logs scoped to a metric) the record tab
  shows a **schema panel** — the table name, column list and a short description — and a
  follow-up change adds those endpoints.
- The menu label is localizable through the existing `[knowledge-content]` labels mechanism
  (`knowledge-menu-labels-i18n`); the hardcoded default label is `Metric Ontology Explorer`.

## Capabilities

### New Capabilities
- `metric-ontology-explorer`: the `home3/knowledge → Ontology → Metric Ontology Explorer` view —
  its menu placement, the three-pane resizable workspace and its two arrangements, the canvas
  orrery with its nine satellites and seven unfoldable chains, the entry/record tab model of the
  content viewer, the source-document pane binding, and the live-vs-schema rule for record tabs.

### Modified Capabilities
<!-- None. No existing openspec/specs/ capability governs this page or the home3 knowledge UI. -->

## Impact

- **New** `ChenWeb/web/src/lib/components/home3/metric-ontology-explorer-view.svelte` — the view.
- **New** `ChenWeb/web/src/lib/components/home3/metric-ontology-explorer/` — canvas, panel-shell,
  content-viewer, and the static ontology/chain model (split so each file stays focused).
- **New** `ChenWeb/web/src/lib/services/metricOntologyExplorerService.ts` — thin wrappers over the
  existing `/api/v1/kb/*` read endpoints the record tabs use (reusing `objectManagerService`,
  `terminologyResourceService`, `metricOntologyAnalysisService`, `metricWikiService` where they
  already cover a table).
- **Modified** `ChenWeb/web/src/routes/home3/knowledge/+page.svelte` — add
  `'kb-metric-ontology-explorer'` to `KbSectionId`, a menu child under `kb-metric-ontology`, the
  import, the `ontologyOpen` expansion handling, and an `{:else if}` render branch.
- **Reused, unchanged** `shared-pdf-viewer.svelte`, `pdf-view-window.svelte` and the
  `/api/v1/kb/inputs/:id/file` document endpoint.
- **No** new npm dependency. **No** backend change in this change; the missing read endpoints are
  a separate follow-up.
- Localization: one new entry key in the `[knowledge-content]` menu labels tables.
