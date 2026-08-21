## Why

The Metric Dashboard at `home3/knowledge → Ontology → Metric Ontology` (`/home3/ontology-metric-analysis`,
dashboard mode) presents its KPIs and panels as a flat top-to-bottom stack. A reader has no visual
anchor for how the underlying entities relate to one another — which panel is about governed
vocabulary versus which is about the stored claim versus which is about the supporting evidence.
Design doc `2026082005-design-metric-ontology-analysis-page.md` (ADR 2026082004) specified the
dashboard's content but not this relationship. `metric-ontology-v1.0-en.md` §5 ("The Metric
Ontology model") now gives a clear, documented entity model — `Metrics` at the center, joined to
`ontology_terms`, `semantic_assertions`, and `assertion_evidence` — that the dashboard can use as
its organizing visual, making the page teach its own data model instead of presenting an
unexplained grid of cards.

## What Changes

- Add a central "Metric Ontology Model" graph to the dashboard-mode view, rendering the whole of
  manual §5 as three colour-coded population lanes — ontology-born, corpus-level identity, and
  record-born — with the processing pipeline of §9 as the horizontal axis, the governed term kinds
  grouped inside a `kb.ontology_terms` shell, and each join of §5.5 drawn as a labeled edge.
  `kb.metrics` is emphasised as the point of contact between the three populations. The graph is a
  read-only diagram; it introduces no new interactions beyond what the relocated panels already have.
- Present the graph as a full-width hero band above the relocated panels rather than as the middle
  column between them: a diagram carrying the full §5 model needs roughly 1500px to stay legible,
  which a centre column cannot supply. The page shell widens to `max-width: 1840px` to suit.
- Re-lay-out the dashboard's existing KPI cards, the four panels (error presence, coverage state,
  mapping inventory, errors by type), and the recent-occurrences table beneath that graph, grouped
  by which model entity each belongs to, instead of the current linear KPI-row → 2x2-panel-grid →
  table stack. No panel, KPI, or table is added, removed, or rewired to different data.
- Purely presentational: the existing `getMetricOntologyAnalysis` response shape, fixture fallback,
  filter/scope/search behavior, and the Document Metrics / Ontology Metrics modes are unchanged.

## Capabilities

### New Capabilities
- `metric-ontology-dashboard-layout`: the dashboard-mode view's page composition — the central
  Metric Ontology Model graph and the model-grouped placement of its KPI/panel/table components.

### Modified Capabilities
(none — no existing `openspec/specs/` capability currently governs this page's layout)

## Impact

- `ChenWeb/web/src/routes/home3/ontology-metric-analysis/+page.svelte` — restructure the
  dashboard-mode markup/CSS into a model-graph layout; add the inline SVG diagram. Data loading
  (`loadAnalysis`), state, and the Document Metrics branch are untouched.
- `ChenWeb/web/src/lib/components/home3/metric-ontology-analysis-view.svelte` — no change expected
  (it only re-exports the page, embedded); verify it still renders correctly.
- New design doc `KnowledgeStore/doc-repo/design/202608/2026082101-design-metric-ontology-analysis-page.md`,
  amending §4 ("Page 1 — Metric Dashboard") of the prior design doc for the new layout.
