## Why

The guiding design of the Web Page Interpreter was revised on 2026-10-29 and clarified on
2026-10-30 (devdoc `KnowledgeStore/doc-repo/devdocs/202610/2026102601-devdoc-web-page-interpreter.md`):
an app no longer divides the screen; each page divides its own space into panes with a page
layouter, and a panel holds the elements of one pane. Every use of a class copies the class,
so a later change to a class reaches only new copies. The code still follows the earlier
model, in which the app's layout template held the panes and pages were flat element lists,
so the design cannot be used yet.

## What Changes

- **BREAKING** App descriptors (format `0.5`) hold only `initial_page`; `layout_template` and
  `panes` are removed. The app opens its initial page in the app's page area.
- **BREAKING** Page descriptors (format `0.5`) have a `page_layouter` (a pane tree with exactly
  one root pane) and `panels` keyed by leaf pane ID, each with an `elem_layouter` and an
  `elem_list`. `layouter`, `layouter_config`, `elements` and the four page margins are
  removed; pages gain `hori_scrollbar` and `vert_scrollbar`.
- Panes have sizes (length, percentage, `elastic`, `auto`), limits, `resizable`, the four
  margins (default `0px`, counted when deciding whether to scroll), alignment and scroll bars.
- **BREAKING** Every element instance, action instance and layouter copy holds a whole copy of
  its class in `class`. Instances are merged with, and validated against, their copy, never
  the stored class. When a page is written, the server copies the active class into every
  instance or layouter copy that has no `class` yet.
- Page layouter and element layouter classes are stored in `kb.wpi_descriptors` with kinds
  `page-layouter` and `element-layouter`: `single-pane`, `top-left-middle-right-bottom` and
  `flow-layouter`. Code modules gain the types `page-layouter` and `element-layouter`, which
  replace `layouter`; a new `page-layouter` code module draws pane trees.
- Opening a page in a pane puts that pane in place of the page's root pane, for pages with
  any number of panes; the pane's margins, alignment and scroll bars apply, and the page's
  size and colors still apply.
- An `open-page`'s `pane` is looked up outward: the page holding the element, then the page
  it was opened in, and so on. The bundle checks that every `pane` resolves.
- **BREAKING** The Panel Cache keeps page instances opened in panes, as
  `(page-instance-id, pane-instance-id)` entries. A pane that switches pages hides the one it
  showed and never closes it; menu bindings are removed. Closing a page shows the pane's own
  panel again.
- **BREAKING** The app bundle returns the app and its pages only; the `elements` and
  `actions` maps are removed, since instances carry their classes.
- The WPI editor edits pages with any number of panes: the canvas preview draws the pane tree,
  elements are dropped into a pane and can be moved between panes, panes can be split,
  resized, edited and deleted, and each panel's element layouter can be configured.
- A migration converts the stored apps and pages to the new format with class copies, adds
  the layouter classes and code module types, turns `customer-admin` into an app whose
  initial page `customer-admin-home` holds the header, the menu and the panes, and deletes the
  one non-WPI `page` row from `kb.wpi_descriptors` so the kind check matches the spec again.

## Capabilities

### New Capabilities
- `wpi-panes`: page layouters and pane trees, pane attributes and geometry, page layouter
  and element layouter classes and their copies, opening a page in a pane, the outward pane
  lookup, and the Panel Cache.

### Modified Capabilities
- `wpi-app-shell`: the app opens its initial page in the app's page area; layout templates,
  menu bindings and closing on replace are replaced by pages that own panes and the Panel
  Cache.
- `wpi-classes`: instances and layouter copies hold a whole copy of their class, are merged
  with and validated against it, and the server fills in missing copies on write.
- `wpi-descriptor-store`: new descriptor kinds and formats, the page write that fills in
  class copies, the bundle without class maps, and the validation rules for panes and
  pane lookup.
- `wpi-pages`: page attributes of format `0.5` (page layouter, panels, scroll bars, no
  margins), and the editor's load, preview and controls for pages with panes.
- `wpi-page-rendering`: the page box, flow spacing and the flow layouter move to panels and
  panes.
- `wpi-data-actions`: action instances run on the server are resolved by their place in a
  panel and merged with their class copy.

## Impact

- Backend `ChenWeb/server/api/wpi/`: `types.go`, `validate.go` (formats, pane trees, class
  copies), `bundle.go` (initial page, pane resolution, no class maps), `handler.go` and
  `store.go` (page write with class fill-in, layouter kinds), `actions.go` (resolution against
  copies), and their tests.
- Frontend `ChenWeb/web/src/lib/wpi/`: types, runtime (page instances, pane instances, Panel
  Cache, outward lookup), rendering (page, pane tree, panel), layouters, entries and menu.
  `templates/` is removed.
- Editor `ChenWeb/web/src/lib/components/shared-ui/`: `wpi-editor.svelte`, `canvas/wpi-json.ts`,
  `PagePreview.svelte`, `PageConfigPanel.svelte`, `PageControl.svelte`, `page-placement.ts`,
  `PropertiesPanel.svelte` and new pane editing helpers; Paraglide messages in English and
  Chinese.
- Database: one goose migration in `ChenWeb/project_migrations/` (kinds, module types,
  layouter classes, converted apps and pages, new home page, stray row removed).
- Documents: the devdoc's status, `KnowledgeStore/doc-repo/code_modules/interpreter-page.md`
  (§4–§10), `interpreter-layouter-flow.md`, and a new code module document for the
  `page-layouter` module.
