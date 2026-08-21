## 1. Design doc

- [x] 1.1 Write `KnowledgeStore/doc-repo/design/202608/2026082101-design-metric-ontology-analysis-page.md`
      capturing the model-graph layout (node/edge diagram, spoke groupings, ASCII layout), referencing
      ADR 2026082004, the prior design doc's §4, and `metric-ontology-v1.0-en.md` §5.
- [x] 1.2 Confirm the doc filename prefix (`2026082101`) doesn't collide with an existing doc before
      writing it.

## 2. Graph markup and styling

- [x] 2.1 Add the inline SVG "Metric Ontology Model" graph section to the dashboard-mode branch of
      `+page.svelte`: `Metrics` center node, `ontology_terms`/`semantic_assertions`/`assertion_evidence`
      surrounding nodes, edges from `Metrics` to each, and the normalization label on the
      `ontology_terms` → `semantic_assertions` relationship.
- [x] 2.2 Style the graph using the page's existing CSS custom properties so it matches light/dark
      mode (`.page-shell` / `.page-shell.dark` tokens) with no new tokens introduced.

## 3. Layout restructure

- [x] 3.1 Replace the `kpi-grid` → `dashboard-grid` → `recent-section` stack with a CSS Grid using
      `grid-template-areas` (`terms graph assertions` / `evidence evidence evidence`), per design.md.
- [x] 3.2 Move the "Ontology metrics" KPI, "Metric classes" KPI, and `mapping-panel` into the
      `terms` grid area, preserving their existing markup, classes, and bindings.
- [x] 3.3 Move the "Current instances" KPI, `coverage-panel`, and `errors-panel` into the
      `assertions` grid area.
- [x] 3.4 Move the "Occurrences", "With errors", "No detected errors" KPIs, `error-panel`, and
      `recent-section` into the `evidence` grid area.
- [x] 3.5 Update the `@media (max-width: 1050px)` (and 700px) rules so the grid collapses to a single
      reading-order column (graph, terms, assertions, evidence) instead of the prior grid's rules.

## 4. Verification

- [x] 4.1 Run the page locally (`mise dev` under `ChenWeb/`) and visually confirm the graph renders
      centered with the three clusters around it, in both light and dark mode.
- [x] 4.2 Confirm `?view=document` (Document Metrics) still renders unchanged.
- [x] 4.3 Confirm existing interactions still work post-move: KPI click-to-select, mapping-table row
      select, scope/reset buttons, recent-table search filter, loading/error notices.
- [x] 4.4 Check keyboard/tab order follows the new visual grouping (terms → graph → assertions →
      evidence) per design.md's accessibility risk note.
- [x] 4.5 Resize to a narrow viewport and confirm the single-column collapse reads top-to-bottom
      sensibly.

## 5. Model graph expansion (follow-up, 2026-08-21)

- [x] 5.1 Replace the four-node diagram with the full manual §5 model: three population lanes
      (ontology-born / corpus-level / record-born), the `kb.ontology_terms` shell with its metric
      identity, measurement science, and claim frame groups, and every §5.5 join as a labeled edge.
- [x] 5.2 Promote the graph from the centre column to a full-width hero band with accent border,
      gradient, kicker/title/lede, and a three-population legend; widen `.content` to
      `max-width: 1840px` with `padding-inline: clamp(18px, 2.4vw, 46px)`.
- [x] 5.3 Regroup the spokes beneath the band as `graph graph` / `terms assertions` /
      `evidence evidence`, keeping the §4 component-to-entity mapping unchanged.
- [x] 5.4 Verify the diagram in light and dark mode; fix the `color-mix(in oklch, …)` hue swing that
      rendered the record lane green and the corpus lane pink (mix accents with `transparent`, and
      use `in srgb` for opaque tints).
- [x] 5.5 Verify at 1880px, 1280px, and 900px that the diagram scrolls inside its own wrapper and the
      page acquires no horizontal scrollbar (`min-width: 0` on the graph grid cell).
- [x] 5.6 Update `2026082101-design-metric-ontology-analysis-page.md` §2 and §3 and its change log.
- [x] 5.7 Re-run `svelte-check`: no new errors or unused-selector warnings.
