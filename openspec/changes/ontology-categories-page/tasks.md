## 1. Menu rename

- [x] 1.1 Change `home3_knowledge_metric_ontology` to "Ontology" in `web/messages/en.json` and to "本体" in `web/messages/zh-cn.json`
- [x] 1.2 Update `kb.page_config` rows 453 (en) and 454 (zh-cn) for `kb-metric-ontology` to label "Ontology" / "本体", and read them back to confirm
- [ ] 1.3 Verify the Ontology child label renders "Ontology" in the menu for both locales

## 2. Service and messages

- [x] 2.1 Create `web/src/lib/services/categoryTreeService.ts` with `listCategoryTrees`, `getCategoryTree(name)` and `listCategoryMembers(name, nodeId)` (direct=true), using the same JSON fetch pattern as `metricOntologyExplorerService.ts`
- [x] 2.2 Add Paraglide keys for the menu item ("Categories" / "类目"), the menu description, the page title, the layout pulldown options (left-right, right-left, top-bottom, bottom-top), empty and error states, the panel header, the stale flag, the truncation note and the leaf prompt, in both `en.json` and `zh-cn.json`
- [x] 2.3 Run `bun run check` from `web/` and confirm the message files agree and no new hard-coded text was added

## 3. Tree chart

- [x] 3.1 Create `web/src/lib/components/home3/category-tree-chart.svelte`: an ECharts `TreeChart` (orthogonal, expandAndCollapse, roam) that takes the nodes and an orientation prop (`LR`, `RL`, `TB`, `BT`) and sets label position from the orientation
- [x] 3.2 Build the tree data from the flat `nodes` list (depth-ordered, linked by `parent_id`) with each node's preferred label in the active language and a `leaf` flag
- [x] 3.3 Emit a click event carrying the node ID for leaves only; inner-node clicks emit nothing
- [x] 3.4 Unit tests for the orientation-to-label mapping, the four layouts and the leaf datum in `web/src/lib/components/home3/category-tree-model.test.ts` (7 passing via `bun test`). The leaf-only click rule lives in the Svelte component and is covered by 6.3 instead

## 4. Categories view

- [x] 4.1 Create `web/src/lib/components/home3/category-trees-view.svelte` with a tree list on the left, a layout pulldown (default LR), the chart in the centre and the artifact panel on the right
- [x] 4.2 Load the tree list on mount; load a tree on selection; redraw without refetch when the pulldown changes
- [x] 4.3 On leaf click, fetch members for that node and show them in the right panel: type, artifact ID, record ID, stale flag, with the empty-state and truncation note
- [x] 4.4 Show the validation problems above the chart when `valid` is false

## 5. Wiring the page

- [x] 5.1 In `web/src/routes/home3/knowledge/+page.svelte`, add child `{ id: 'kb-ontology-categories', label: m.home3_knowledge_categories(), description: ... }` after the explorer under the `kb-metric-ontology` parent
- [x] 5.2 Add `kb-ontology-categories` to `KbSectionId` and to the `needsActiveStore` exclusion list
- [x] 5.3 Add the render branch `{:else if activeSection === 'kb-ontology-categories'}` with `<CategoryTreesView {darkMode} />`
- [x] 5.4 Set `select-text` on the new view's root so text remains selectable (CLAUDE.md frontend rule)

## 7. Fixed-spacing canvas and miniature (follow-up)

- [x] 7.1 Replace the fit-to-viewport tree series with `layoutTree`: fixed level and row gaps per orientation, leaves on consecutive rows, inner nodes centred on their children
- [x] 7.2 Draw nodes (scatter) and elbow connectors (polyline lines) on a cartesian canvas with `dataZoom` pan and zoom and `filterMode: 'none'`
- [x] 7.3 Add the miniature overlay: whole-tree dots, viewport rectangle, drag to move, click to centre
- [x] 7.4 Make the Categories page fill the window height, with the chart taking the remaining space and the phone layout at 60vh
- [x] 7.5 Unit tests for the layout (gaps, centring, four orientations), canvas extent and initial window

## 8. Folding, two-level default and miniature (follow-up 2)

- [x] 8.1 Node symbol doubled to 12px and label font raised to 14px; spacing widened to keep labels clear
- [x] 8.2 Default view shows two levels: the root and its direct children; the children are folded
- [x] 8.3 Clicking a node expands or folds it; a folded node is hollow and shows its hidden child count; clicking a leaf still opens the artifact panel
- [x] 8.4 Folding keeps the view in data units, so the view does not jump
- [x] 8.5 Remove scrollbars from the chart pane; the miniature is a fixed 200x240 rectangle in the bottom-right corner that maps the whole visible tree, and dragging its viewport moves the view
- [x] 8.6 Unit tests for the default expansion, folding and the non-mutating visible tree

## 9. Label placement and truncation (follow-up 3)

- [x] 9.1 Labels sit on one side per layout for every node: right (LR), left (RL), below (TB), above (BT); vertical layouts keep the rotated text
- [x] 9.2 Labels longer than 40 width units (a CJK character counts as two) are truncated with "..."
- [x] 9.3 Hovering a truncated label shows its full name next to the pointer (hit-tested in the chart component; ECharts' own tooltip is off)
- [x] 9.4 Unit tests for placement, truncation (Latin and CJK) and folded-node label text

## 10. Vertical label placement (follow-up 4)

- [x] 10.1 TB labels sit below their circles and BT labels above them, rotated to run along the tree direction, using a measured pixel offset per label
- [x] 10.2 Unit tests for the offset sign and its growth with label length
- [x] 10.3 Replace the pixel offset with anchored placement (`position` bottom/top, `align: 'right'`, rotated): a `labelLayout.dy` is rotated with the text and moved TB/BT labels sideways; verified by rendering both layouts with ECharts 5.6; TB text reads top-down (`rotate: -90`, `align: 'left'`), like BT
- [x] 10.4 Widen the circle-to-label gap (`distance` 4 → 10) and put a folded node's count first: "(+N)  name"

## 11. Line style (follow-up 5)

- [x] 11.1 Add a "Line style" pulldown next to Layout with "Broken lines" and "Curve" (default), keys in `en.json` and `zh-cn.json`
- [x] 11.2 Draw a curve as the cubic Bézier through the elbow's points (`edgePath`), passed to `buildCanvasOption`
- [x] 11.3 Unit tests for `edgePath` and for the edge style reaching the lines series

## 6. Verification

- [x] 6.1 Run `bun run check` and the frontend unit tests from `web/`
- [ ] 6.2 Open `home3/knowledge` in the running dev server and check: Ontology label, Categories page, tree list, all four layouts on `cat:general-tech-categories`, a leaf click with the empty panel, and the zh-cn locale
- [ ] 6.3 Confirm an inner node click sends no members request (network panel)
- [ ] 6.4 Check the page at phone width with no horizontal scroll
- [ ] 6.5 Update `openspec` status to complete and note in the devdoc that the Categories page is built (devdoc 2026102003 §9)
