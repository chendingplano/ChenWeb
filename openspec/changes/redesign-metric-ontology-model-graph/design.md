## Context

`+page.svelte` (dashboard mode) currently renders, in order: `kpi-grid` (6 cards), `dashboard-grid`
(a `1fr 1fr` CSS grid holding `error-panel`, `coverage-panel`, `mapping-panel`, `errors-panel`), then
`recent-section` (search + table). All markup, state (`$state`/`$derived`), and the `loadAnalysis()`
fetch against `metricOntologyAnalysisService.getMetricOntologyAnalysis` live in this one file; there
is no shared "panel" component to extend. The new KnowledgeStore design doc
`2026082101-design-metric-ontology-analysis-page.md` is the visual source of truth for the target
layout; this document covers how that layout is implemented inside the existing Svelte file.

## Goals / Non-Goals

**Goals:**
- Introduce one new section — the Metric Ontology Model graph — as the visual center of the
  dashboard, built as inline SVG using the page's existing CSS custom properties (`--bronze`,
  `--text`, `--border`, `--subtle`, dark-mode overrides already defined on `.page-shell.dark`) so it
  matches the page's theme with no new design tokens.
  Place `Metrics` at the center and `ontology_terms`, `semantic_assertions`, `assertion_evidence` as
  three surrounding nodes, with a labeled edge from `ontology_terms` into `semantic_assertions`
  annotated "normalizes value · value type · range type" (manual §9.4–9.5), matching the entity
  names as spelled in `metric-ontology-v1.0-en.md` §5.5, not paraphrased.
- Restructure the surrounding markup into a CSS Grid (`grid-template-areas`) with the graph in the
  center cell and three "spoke" clusters around it, regrouping the *existing* KPI cards, panels, and
  table by which model entity they belong to:
  - `ontology_terms` spoke: "Ontology metrics" KPI, "Metric classes" KPI, `mapping-panel`.
  - `semantic_assertions` spoke: "Current instances" KPI, `coverage-panel`, `errors-panel`.
  - `assertion_evidence` / `Metrics` spoke (bottom, full width): "Occurrences", "With errors", "No
    detected errors" KPIs, `error-panel` (donut), `recent-section` table.
- Preserve every existing behavior verbatim: `loadAnalysis()`, `selectMetric`, `resetFilters`,
  `filteredRows`, the loading/error notes, and the `?view=document` branch are untouched.
- Responsive collapse: below the existing `1050px` breakpoint, the grid stacks to a single column in
  reading order (graph, then the three spokes) — consistent with how `dashboard-grid` already
  collapses today, no new breakpoint behavior invented.

**Non-Goals:**
- No changes to `metricOntologyAnalysisService.ts`, the API response shape, or fixture data.
- No changes to Document Metrics or Ontology Metrics modes/routes.
- No new shared/reusable graph or panel component — this is a single-page layout change, and the
  project's simplicity guidance (ChenWeb `CLAUDE.md` §1.2) argues against extracting one for a
  single call site. If a second page later needs the same model graph, extract then.
- No new npm dependency for diagramming (e.g. no D3/force-graph). The topology is fixed and its
  coordinates hand-placed — inline SVG keeps it dependency-free and themeable with plain
  CSS variables, consistent with how the donut chart on this same page is already done (a hand-rolled
  `conic-gradient`, no charting library).

## Decisions

- **Fixed inline SVG, not a generated/force-directed layout.** Coordinates are hand-placed in a
  `0 0 1520 740` viewBox rather than computed: the topology is fixed and the readability depends on
  lane alignment (each ontology term group sits directly above the record that references it), which
  no automatic layout preserves. Alternative considered: a graph-layout library — rejected as
  unjustified weight for a static diagram, and as actively harmful to the lane alignment.
- **The graph is a full-width hero band above the spokes, not the centre column between them.**
  *(Supersedes the original three-column decision and the `position: sticky` card below it.)* The
  diagram carries the whole of manual §5 — three population lanes and roughly twenty entities — and
  needs ~1500px to stay legible, which the centre column of a three-column band cannot supply. The
  band earns "page centre" through prominence and reading order instead of geometry: accent top
  rule, warm gradient, larger shadow, and a kicker/title/lede/legend header. The spokes below keep
  their entity grouping. `.content` widens to `max-width: 1840px` to suit.
  ```
  grid-template-columns: 1fr 1fr;
  grid-template-areas:
    "graph    graph"
    "terms    assertions"
    "evidence evidence";
  ```
- **Three colour-coded population lanes as the primary axis, the pipeline as the secondary one.**
  Manual §5.1's populations are the distinction users most need and the one a flat entity-relationship
  diagram hides, since all three look alike in a listing. Lanes make "what does reprocessing do to
  this row" readable at a glance, and let every reference edge run vertically while every stage
  transition runs horizontally. Alternative considered: a single flat ER diagram — rejected because
  it teaches the joins without teaching the lifetimes.
- **Accent tokens mix with `transparent`, never with `--border` or `--surface`, under
  `color-mix(in oklch, …)`.** oklch interpolates hue along the shorter arc, so mixing `--blue`
  (hue 245) into `--border` (hue 75) renders green and `--violet` into `--border` renders pink —
  both observed during implementation. Where an opaque tint is needed (the `kb.metrics` fill, the
  card gradient) use `in srgb`, which does not interpolate hue.
- **Regroup existing markup in place, don't extract new components.** The four panels and KPI cards
  keep their existing markup/classes and are simply moved into the three spoke `<div>`s; this is a
  template reorganization, not a rewrite, per ChenWeb `CLAUDE.md` §1.3 (surgical changes, match
  existing style).
- **Graph nodes are static, not click-to-filter.** The design doc's ADR (2026082004) scopes this
  page as read-only diagnostics with no new interactions; making the graph nodes clickable
  filters/scroll-anchors would be a real interaction change beyond "redesign the layout." Left as an
  open question below in case the user wants it as a fast follow.

## Risks / Trade-offs

- [Narrow viewports squeeze the grid before 1050px] → Mitigation: reuse the existing
  `@media (max-width: 1050px)` breakpoint to collapse `grid-template-areas` to a single column,
  matching how `.dashboard-grid` already collapses today.
- [A ~1500px diagram is unreadable on a narrow screen] → Mitigation: the canvas scrolls inside its
  own `overflow-x: auto` wrapper with a `min-width: 1080px` floor rather than shrinking its labels,
  and the `.model-graph` grid cell sets `min-width: 0` so that floor does not push the whole page
  into horizontal scroll. Verified at 1880px, 1280px, and 900px.
- [Hand-placed SVG coordinates are harder to adjust than a computed layout] → Mitigation: the
  topology is fixed, so this is a one-time cost; §2 of the design doc records the lane geometry and
  the alignment invariant for anyone editing it.
- [Regrouping panels changes their reading/tab order] → Mitigation: DOM order matches the band's
  visual order (graph → terms → assertions → evidence) at both the wide and the collapsed
  breakpoint, so keyboard/screen-reader traversal stays sensible; call out in tasks.md as a check.
- [The two spoke columns are unequal in height, leaving the left one short] → Mitigation: the
  mapping-inventory panel takes `flex: 1` so it fills its column rather than leaving bare page
  background beneath it.

## Migration Plan

Single-file UI change behind no flag (matches how this prototype page has shipped changes so far —
no feature-flag precedent found for this route). Deploy via normal `mise dev`/build; rollback is a
plain revert of the one changed file. No data migration.

## Open Questions

- Should graph nodes become clickable jump-links to their spoke (or to Document/Ontology Metrics)?
  Left out of this change's scope; the ADR's read-only-diagnostics framing suggests keeping it
  static for now, but flagging for the user to confirm.
