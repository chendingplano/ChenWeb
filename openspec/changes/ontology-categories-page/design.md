## Context

Category trees live in `kb.category_trees`, `kb.category_nodes` and `kb.category_bindings`
(devdoc 2026102003). The API under `/api/kb/category-trees` already provides:

- `GET /kb/category-trees`: the list of trees (`trees`).
- `GET /kb/category-trees/:name`: the current tree as a flat, depth-ordered `nodes` list with
  `depth`, `canonical_name`, `pref_labels`, `is_others` and `is_leaf`, plus `valid` and `problems`.
- `GET /kb/category-trees/:name/nodes/:node_id/members`: the bindings of a node, each with
  `artifact_type`, `artifact_id`, `record_id`, `stale` and `path`.

The Ontology section of `home3/knowledge` already hosts the Metric Ontology pages. The explorer
(`metric-ontology-explorer-view.svelte`) is the closest precedent for a page with its own service
module and a read-only fetch on selection. The ECharts `TreeChart` is already registered in
`tree-graph-view.svelte`.

## Goals / Non-Goals

**Goals:**
- A Categories page under Ontology that lists all trees and draws the chosen one.
- One ECharts tree component with a layout pulldown covering LR, RL, TB and BT.
- A right-side artifact panel for leaf clicks.

**Non-Goals:**
- Editing a tree, running classification, or changing bindings from this page.
- Resolving artifact names. The members endpoint returns IDs only, so the panel shows type, ID
  and record ID. Name resolution is a later change.
- Showing bindings on inner nodes. Only leaves open the panel, as requested.
- Changing the backend.

## Decisions

**Layout is one chart, four orientations.** The four ECharts examples differ only by the tree
series `orient` (`LR`, `RL`, `TB`, `BT`) and by label placement. One `TreeChart` option builder
takes the orientation and derives the label position from it: `left`/`right` for horizontal
layouts and `top`/`bottom` for vertical ones. Four separate components would duplicate the
option code. Alternative: copy each example verbatim. Rejected because the examples only differ
in these two fields.

**Fixed-spacing canvas, not a fitted tree.** ECharts' `tree` and `graph` series always fit the
whole tree into the viewport, so 1,570 nodes end up packed and overlapping. Instead, node
positions are computed in `layoutTree` with fixed gaps (horizontal: 320 px per level, 18 px per
leaf row; vertical: 200 px per level, 16 px per row). The ECharts option draws them on a cartesian
canvas: scatter for nodes, polyline `lines` for elbow connectors, and `dataZoom` for pan and zoom.
`filterMode: 'none'` keeps every edge drawn while the view moves. Alternatives: a hand-written SVG
(full control, but leaves ECharts) and ECharts `graph` with `layout: 'none'` (still fits to the
viewport). Rejected because ECharts' cartesian canvas already gives fixed spacing and pan/zoom.

**Labels.** Every node's label sits on the same side in a layout: right of the circle for LR, left
for RL, below for TB and above for BT. For the vertical layouts the text is rotated 90 degrees clockwise (`rotate: -90`) so it reads
top-down, and anchored just past the circle (`position: 'bottom'` for TB, `'top'` for BT,
`distance` 10); TB uses `align: 'left'` (text starts at the circle) and BT `align: 'right'` (text
ends at it), so the text runs away from the circle. The rotation pivots on the anchor
point and carries any pixel offset with it, so a `labelLayout.dy` (tried first) moved the labels
sideways instead of down; ECharts also ignores `labelLayout` on a data item. Labels
longer than 40 width units are truncated with "...". A folded node's label starts with its hidden child count, "(+N)  name", so the count stays next
to the circle. ECharts reports hovers only on the symbol, so
the chart component hit-tests the label boxes and shows the full name.

**Line style.** A second pulldown, "Line style", picks how a parent joins its children: "Broken
lines" (right-angled elbows) or "Curve" (the default; S-curves, as in the ECharts tree-vertical
example). A curve is the cubic Bézier whose control points are the elbow's four points (parent,
both bends, child), sampled into 16 segments and drawn by the same `lines` series, so both styles
share one layout and work in all four orientations.

**Folding.** Only the root is expanded by default, so two levels show. A folded node keeps
its datum with `collapsed` and `child_count`, and is drawn hollow with "(+N)" in its label.
Clicking a node toggles it; clicking a leaf opens the artifact panel. The zoom window is
kept in data units, so folding does not move the view.

**Miniature drives the view.** A small SVG overlay draws every node and the current zoom window.
Dragging its viewport rectangle dispatches `dataZoom` actions for both axes, and pressing outside
the rectangle centres the view there. The zoom window is read back from the `datazoom` event, so
the miniature stays in step with wheel and drag zoom on the canvas.

**Canvas extent.** The axes span the tree's bounds plus a margin equal to the tree's own size on
each side (at least 600 px). That is large but not unbounded: the view cannot be panned past it.

**Initial zoom is 100%.** The first window is sized to the chart's pixel size, so one data unit is
one CSS pixel and the root is visible at first. Layout changes reset to this window; resizing the
window keeps the current zoom.

**Leaf detection from the API.** Leaves are taken from the `is_leaf` field that
`GET /kb/category-trees/:name` returns. The frontend does not infer them from `children`,
because `others` leaves are leaves too and the API already marks them.

**Members fetched on leaf click, direct only.** A leaf has only direct memberships, so
`direct=true` is passed. This keeps the request to the node's own bindings. The `limit` stays at
the backend default (500). A tree that gets more than 500 bindings on one leaf would show a
truncated list, so the panel shows a "showing first 500" note when the count reaches the limit.

**Menu rename in code and in the database.** The code default is changed in `en.json` and
`zh-cn.json`, and the `kb.page_config` rows for `kb-metric-ontology` are updated too. Those rows
override the code default, so leaving them would keep the old name on screen.

**Section id `kb-ontology-categories`.** Added as a child of `kb-metric-ontology`. It is
excluded from `needsActiveStore`, since it reads the category API and not a knowledge store, the
same as the explorer.

**Tree list shows all trees, not only the active store.** Category trees are global (they are not
per knowledge store), so the list does not depend on `knowledgeStoreState`.

## Risks / Trade-offs

- [Large tree renders slowly] → Nodes are drawn as one scatter series and edges as one lines
  series, not one ECharts element per node. Check render time on `cat:general-tech-categories`
  (1,570 nodes).
- [Empty panel looks broken] → No classification has been run yet, so every leaf is empty. The
  panel shows an explicit empty state, and the proposal records this limit.
- [Label override drift] → The DB rows and the code default can diverge again. The task list
  checks both after the change.
- [Stale tree data after edits] → The page loads the tree on selection and reloads it when the
  tree is re-selected. There is no live refresh.

## Migration Plan

Frontend-only change, plus a two-row UPDATE on `kb.page_config`. Rollback: restore the previous
label on rows 453 and 454 and revert the commit. No data is created or deleted.

## Open Questions

- None blocking. Name resolution for artifacts is deferred to a later change.
