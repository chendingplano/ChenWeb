## Why

The category trees built under the category-trees change (devdoc 2026102003) can be created and
classified through the API, but nothing in the UI shows them. Reviewers need to browse a tree,
see its shape, and open the artifacts filed under a leaf. The Ontology menu also still labels its
first item "Metric Ontology", which no longer describes the section.

## What Changes

- Rename the `home3/knowledge` menu item under Ontology from "Metric Ontology" to "Ontology"
  (EN) and "本体" (ZH). The `kb.page_config` label overrides for `kb-metric-ontology` (en and
  zh-cn) are updated to match, since they take precedence over the code defaults.
- Add a child menu item "Categories" / "类目" under Ontology, after the Metric Ontology Explorer.
- New page: lists every category tree (`GET /api/kb/category-trees`). Selecting a tree renders it
  as an ECharts tree. A pulldown chooses one of four layouts: left-right (`LR`), right-left
  (`RL`), top-bottom (`TB`) and bottom-top (`BT`), matching the ECharts `tree-basic`,
  `tree-orient-right-left`, `tree-vertical` and `tree-orient-bottom-top` examples.
- Clicking a leaf node opens a right-side panel listing every artifact bound to that node
  (`GET /api/kb/category-trees/:name/nodes/:node_id/members`): artifact type, artifact ID, record
  ID, and whether the binding is stale.
- All new UI text goes through Paraglide in `en.json` and `zh-cn.json`.

## Capabilities

### New Capabilities
- `ontology-categories-view`: the Categories page under Ontology, covering the tree list, the
  four-layout tree chart and the leaf artifact panel.

### Modified Capabilities

None. The menu label change touches menu data only, not an existing spec-level requirement.

## Impact

- Frontend: `web/src/routes/home3/knowledge/+page.svelte` (menu entries, section id, view branch,
  active-store exclusion), a new view component under `web/src/lib/components/home3/`, a new
  service module `web/src/lib/services/categoryTreeService.ts`, and `web/messages/{en,zh-cn}.json`.
- Database: `kb.page_config` rows 453 and 454 (`kb-metric-ontology`, en and zh-cn) get the new
  label. No schema change.
- Backend: none. The existing category-tree list, tree and members endpoints are reused.
- Known limit: no classification has been run, so the artifact panel is empty for every leaf until
  bindings exist. The panel shows the empty state correctly in the meantime.
