## 1. Backend formats and validation

- [x] 1.1 `types.go`: format versions per kind (`0.5` app, page, layouter classes; `0.4` element; `0.2` action), kinds `page-layouter` and `element-layouter`, layouter code registries, `Bundle` without class maps; remove `templatePanes` and `layouters`
- [x] 1.2 `validate.go`: app `0.5` (`initial_page`), page `0.5` attributes (no margins, scroll bars), pane trees (`paneTree`: one root, split/children, unique IDs, leaf-only attributes, sizes, margins), `panels` keyed by leaf IDs
- [x] 1.3 `validate.go`: layouter class rows (`page-layouter` with `root`, `element-layouter` with `config`) and layouter copies (`layouter`, `class`, `root`/`config` replacing the copy's), flow `config` checks moved from `layouter_config`
- [x] 1.4 `validate.go`: instances with class copies: name equals copy ID, copy valid as a class row of its kind, instance checked and merged against the copy (page `binding_mode` rule kept); `class` reserved as attribute name; `actionPositions` walks `panels/<id>/elem_list`
- [x] 1.5 Go tests for 1.1–1.4 (`validate_test.go`) with new-format fixtures

## 2. Backend write, bundle and actions

- [x] 2.1 `fillClassCopies`: copy the latest active class into instances and layouter copies without `class`; errors at the instance path for missing classes; unit tests
- [x] 2.2 `PutDescriptor`: kinds `page-layouter` and `element-layouter`; pages: fill copies, validate, store, return `page`; remove `ValidatePageInstances`/`loadClasses` from the page path; handler tests
- [x] 2.3 `bundle.go`: load app and `initial_page`, follow `open-page`, validate pages, code modules of class copies (element and layouter types), pane resolution over `(page, host chain)` with a depth cap, tables, stored sources; `BuildPagePreviewBundle` with `initial_page`; bundle tests
- [x] 2.4 `actions.go`: resolve the instance by pointer in panels, check and merge against its own copy; action tests updated
- [x] 2.5 `store.go`/catalog: module types `page-layouter`/`element-layouter` in the code module lookups used by validation
- [x] 2.6 Update the remaining Go tests (`store_test`, `stored_test`, `preview_test`, `customers_demo_integration_test`, `seed_test`) and run `go test ./server/api/wpi/`

## 3. Migration

- [x] 3.1 Write `20261030000001_wpi_pages_own_panes.sql`: delete stray `page` rows, kind check, module types and `page-layouter` module, layouter classes, class-copy PL/pgSQL helpers, page conversion, `customer-admin-home`, apps in `0.5`
- [x] 3.2 Seed test: run the conversion's output through the Go validator (every converted page and new row valid)
- [x] 3.3 Dry-run the Up section in `BEGIN … ROLLBACK` against the dev database, then apply `20261029000001` and the new migration with `go run . --migrate-only`

## 4. Frontend runtime

- [x] 4.1 `types.ts`: app, page, pane, panel, layouter copy, instance with `class`, bundle `{app, pages, warnings}`
- [x] 4.2 `pane-tree.ts`: effective tree, leaf panes, pane lookup by ID, pane and page-box styles (sizes, margins, alignment, scroll bars, page box height rule); unit tests
- [x] 4.3 `entries.ts`: merge element and action instances with their copies; pointers under `/panels/<pane>/elem_list/<i>`; unit tests
- [x] 4.4 `panel-cache.ts` (replaces `panels.ts`): sharing key, find cached, show, forget; unit tests
- [x] 4.5 `runtime.svelte.ts`: page instances, pane instances, `openPage` with outward pane lookup and the Panel Cache, `closePage` (nested first, own panel shown), refresh on show, saves and triggers, menu current-item check, pane sizes from dragging
- [x] 4.6 Components: `WpiApp.svelte`, `PageView.svelte`, `PaneView.svelte`, `PanelView.svelte`, layouter registries by code module ID, `FlowLayouter.svelte` with copy `config`, `MenuElement.svelte` without bindings, `ListElement.svelte` pointers; remove `templates/`, `Pane.svelte`, `Panel.svelte`
- [x] 4.7 Paraglide messages for new runtime text (English and Chinese)

## 5. Editor

- [x] 5.1 `canvas/wpi-json.ts`: node data with `pane` and `class`; `pageJson(nodes, page)` builds panels; `pageNodes(page)` reads every panel; blank page in format `0.5`; unit tests
- [x] 5.2 `canvas/pane-edit.ts`: select, update attributes, split rows/columns, add child, delete (collapse), rename, move node; unit tests
- [x] 5.3 `PagePreview.svelte`: draw the pane tree with the runtime styles, panels with element overlays, pane selection, drop target per leaf
- [x] 5.4 `wpi-editor.svelte`: drop into the pane under the pointer, pane selection state, *Page*/*Panes*/*Panel* modes, save adopts the returned page, View JSON with panels, undo for pane edits
- [x] 5.5 `PageControl.svelte`, `PageConfigPanel.svelte` (page attributes, pane tree and pane attributes with split/add/delete/rename, panel `config`), `PropertiesPanel.svelte` (*Pane* select for element nodes)
- [x] 5.6 Paraglide messages for the editor (English and Chinese); `bun run check` and `bun test src/lib`

## 6. Verification and documents

- [x] 6.1 Browser check: `/wpi/customer-admin` (menu, panes, list → right pane, form save, switching keeps edits), `/wpi/customers-demo`, editor load/edit/split/save/preview of a multi-pane page
- [x] 6.2 Documents: devdoc 2026102601 status, `interpreter-page.md` §4–§10, `interpreter-layouter-flow.md`, new `interpreter-layouter-page.md`
