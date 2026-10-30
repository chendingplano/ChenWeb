## Context

The guiding design (devdoc 2026102601, revised 2026-10-29, clarified 2026-10-30) moves panes
from the app into the page, and makes every use of a class a whole copy of the class. The
code follows the earlier model:

- `server/api/wpi`: apps have `layout_template` and `panes` (`templatePanes` in `types.go`);
  pages have `layouter`, `layouter_config` and a flat `elements` list; instances are merged
  with the stored class row (`againstClass`, `mergeInstance`); the bundle ships the class
  rows (`Bundle.Elements`, `Bundle.Actions`); actions are found by JSON Pointer under
  `/elements/<i>`.
- `web/src/lib/wpi`: `AppRuntime` keeps panes from the app's template, panels (one open page
  each), and menu bindings (`panels.ts`); `templates/` draws the two layout templates;
  `Panel.svelte` draws a page box with margins; `entries.ts` merges instances with
  `bundle.elements`.
- The editor (`wpi-editor.svelte` and `canvas/`) maps a page's `elements` to canvas nodes
  ordered by position, draws them in one flow area (`PagePreview.svelte`), and edits the
  page attributes and `layouter_config` (`PageConfigPanel.svelte`).
- Stored data is demo data: apps `customer-admin` and `customers-demo`, seven pages, six
  element classes, seven action classes, and one stray non-WPI `page` row in
  `kb.wpi_descriptors` (to be deleted).

Decisions already made with the user: the editor covers full multi-pane editing; stored data
is migrated and the old format is dropped; the server fills in class copies when a page is
written; the stray row is deleted.

## Goals / Non-Goals

**Goals:**
- Implement the revised descriptor formats, validation, bundle, server actions, runtime and
  editor, so that pages own their panes end to end.
- Keep one rule per concern on both sides: the Go validator and the TypeScript runtime merge
  instances the same way, and resolve panes the same way.
- Convert every stored app and page; after the migration only the new formats are valid.

**Non-Goals:**
- `open-page` targets `tab`, `new-tab` and `iframe`, loading a page by URL, age-out, timed
  refresh, page styles and policies (all later phases in the devdoc).
- Authoring layouter classes in the editor; only the three built-in classes exist.
- A way to refresh an instance's class copy from a newer class (re-placing the element does
  it).

## Decisions

### D1. Format versions

Apps, pages and layouter classes are written in `schema_version` `"0.5"`. Element class rows
stay `"0.4"` and action class rows `"0.2"`: their format does not change. `schemaVersionOf`
returns the one supported version per kind; anything else is an error, as today. A class copy
keeps the `schema_version` of the class it copies and is checked against that kind's version.

### D2. Copies: `class` beside the copy's own values

Every element instance, action instance and layouter copy is
`{ "<name-key>": "<class id>", "class": { <whole class row> }, ...own values }`, where the name
key is `element`, `action` or `layouter`, and must equal `class.descriptor_id`.

- Element and action instances: own values sit beside `class` and hold only what the instance
  sets. The merged instance is the copy's non-null defaults, overlaid with the page's
  `binding_mode` (when the copy names `binding_mode` and does not fix it), the instance's own
  values, then the copy's fixed values. This is today's `mergeInstance`/`withPageDefaults`
  with the copy in place of the stored row, so the existing tests carry over.
- Page layouter copy: `{layouter, class, root?}`. The effective tree is `root` if set, else
  `class.root` (whole replacement).
- Element layouter copy: `{layouter, class, config?}`. The effective config is `config` if
  set, else `class.config` (whole replacement), so a panel that changes one parameter lists
  all of them.
- `class` is reserved: it cannot be an attribute name in a class row.

Alternatives: flattening every class value into the instance (rejected 2026-10-30: help tips
and notes have no place, and fixed attributes cannot be enforced); per-key merge of `config`
(rejected for one uniform rule).

### D3. Validation against the copy

`ValidateDescriptor(page)` becomes complete for instances: it no longer needs class rows. For
each copy it checks the copy as a class row of its kind (element rows: structure, registered
code module, required attributes given; action rows: code exists; layouter rows: registered
code module of the right layouter type, valid `root` or `config`), then the instance against
the copy (the `againstClass` checks: only attributes the copy names plus sizes, none it fixes,
every `null` default set), then the merged instance (`elementComplete`, required action
attributes). `ValidatePageInstances` and `loadClasses` are removed from page writes and from
the bundle. What still needs the database stays in the bundle and the write path: code modules
active in `kb.code_modules`, tables and columns, stored statements and files, and pane
resolution across pages.

### D4. The server fills in missing copies on page write

`PUT /api/v1/wpi/descriptors/page/:id` first runs `fillClassCopies(page, load)`: for every
element instance, action instance (not static data) and layouter copy without `class`, it
copies the latest active class row of that kind and ID; an unknown or inactive class is an
error at the instance's path. Copies already present are left alone. It then validates and
stores the filled-in body, and returns it as `page` in the response, so the editor adopts the
copies and a later save does not copy newer classes into old instances. The same function
backs the editor's save (which uses this endpoint).

Alternative: the client sends copies (rejected by the user: hand-written pages would need
whole class rows).

### D5. Pane trees

A pane is `{pane_id, split?, children?, ...attributes}`. `split` is `rows` or `columns` and
needs two or more `children`; a pane without `split` is a leaf and has no `children`. Pane IDs
are unique in the page. Sizes: a length (`px`, `rem`, `em`, unit optional), a percentage,
`elastic` or `auto`; limits: a length or percentage; margins: a length. The leaf-only
attributes (margins, `hori_align`, `vert_align`, `hori_scrollbar`, `vert_scrollbar`) on a
split pane are errors. `panels` keys must be leaf pane IDs of the effective tree. One Go
function `paneTree(root)` returns the leaf IDs and the issues; the TypeScript side has the
matching `leafPanes(tree)`.

### D6. Code module types and the `page-layouter` module

`kb.code_modules.module_type` allows `element`, `action`, `page`, `page-layouter` and
`element-layouter`; `flow-layouter` becomes `element-layouter`, and a new row `page-layouter`
(type `page-layouter`) is added with its document `interpreter-layouter-page.md`. The Go
validator keys layouter code by module ID (`pageLayouterModules = {"page-layouter"}`,
`elementLayouterModules = {"flow-layouter": flow parameters}`), as `codeModules` does for
elements. The frontend registries are keyed the same way.

### D7. Rendering

Three components replace `Pane.svelte`, `Panel.svelte` and `templates/`:

- `PageView` draws a page instance. In the app's page area it draws the page frame (the page's
  `hori_align`, `vert_align`, scroll bars) around the page box (sizes, limits, colors), and
  the root pane inside the box. Embedded in a host pane A, it draws only the page box; A's
  container already applies A's margins, alignment and scroll bars, and the box aligns its
  content with A's alignment. Inside the box it draws the root's children (split root) or
  the root's panel (leaf root); the root's own attributes are not used.
- `PaneView` draws one pane of a page instance: a split pane is a flex row or column of child
  `PaneView`s sized by `paneStyle`; a leaf pane is a container with its margins as padding,
  its alignment, and `overflow` from its scroll bars, showing its own `PanelView` or, when a
  page instance opened in it is visible, that instance's `PageView` (hidden ones stay mounted
  with `display: none`).
- `PanelView` draws a panel with the element layouter its copy names.

A page box's height is its `height` if set; otherwise 100% of the page area when the root is
split into `rows` with an `elastic` child or split into `columns`, else its content height.
A resizable pane with a pixel size gets a drag handle on the edge facing its next sibling (the
previous one for the last child); the dragged size is runtime state per pane instance.

### D8. Runtime state and the Panel Cache

`AppRuntime` keeps:

- page instances `{id, pageId, hostPane: paneInstanceId | null, doc, snapshot, input, ready,
  loading, error, fieldErrors, shown, key}`;
- pane instances, keyed `"<pageInstanceId>/<paneId>"`, `{opened: pageInstanceId[] (least
  recently shown first), visible: pageInstanceId | null}`; `visible: null` shows the pane's
  own panel.

`panel-cache.ts` (replacing `panels.ts`) holds the pure rules: `sharingKey(page, input)`,
`findCached(pane, instances, key)`, `show(pane, id)` and `forget(pane, id)`. `openPage` hides
the visible instance, shows a cached one with the same key or creates one, and runs the
refresh-on-show rule when an instance goes from hidden to shown. `closePage(id)` closes the
page instances opened inside it first (each with its own unsaved-changes check, stopping if
the user keeps editing), then the instance itself; its pane then shows its own panel. A
`close-page` from the app's initial page instance does nothing. Menu bindings are removed: a
menu item is shown as current when its target pane's visible instance has the item's key.

### D9. Outward pane lookup

Runtime: `resolvePane(pageInstanceId, paneId)` walks from the instance outward. In each page
instance it accepts a leaf pane of the effective tree; the root's ID, when the instance is
embedded, names its host pane. If the page has no such pane, it moves to the page instance
that owns the host pane. The result is a pane instance ID, or an error shown in place.

Bundle: a breadth-first walk over `(page, host chain)` pairs, starting at the initial page with
an empty chain, checks every `open-page` in the page against the page and its chain, and
derives the opened page's chain from where the pane resolved. Each pair is visited once and
chains are capped at 8 pages, so cycles end. An unresolved pane is a 422 error at the
instance's `/pane`, naming the chain.

### D10. Bundle and server actions

`Bundle` becomes `{app, pages, warnings}`. `BuildBundle` loads the app, its `initial_page`,
then pages named by `open-page` instances, round by round as today. `BuildPagePreviewBundle`
builds an app whose `initial_page` is the page. `actionPositions` walks `input`,
`save_action` and every panel's `elem_list`, so pointers become
`/panels/<paneId>/elem_list/<i>/...`. `resolveAction` checks the instance against its own
copy and merges with it; it no longer reads action class rows.

### D11. Migration

One goose migration `20261030000001_wpi_pages_own_panes.sql`, run after the pending
`20261029000001` (which renames `page_alignment`):

1. Delete `kind = 'page'` rows from `kb.wpi_descriptors`; set the kind check to `app`,
   `element`, `action`, `page-layouter`, `element-layouter`.
2. Module types: allow `page-layouter` and `element-layouter`, move `flow-layouter` to
   `element-layouter`, insert the `page-layouter` module, drop `layouter`.
3. Insert the layouter classes `single-pane`, `top-left-middle-right-bottom` (kind
   `page-layouter`) and `flow-layouter` (kind `element-layouter`), version 1, `active`.
4. Convert every page's latest active version into a new version: a `single-pane` copy whose
   root pane `main` carries the page's margins (or the old default `30px`), a `main` panel
   with a `flow-layouter` copy (its `config` from `layouter_config`), and `elem_list` from
   `elements`; class copies are added to every element and action instance; `layouter`,
   `layouter_config`, `elements` and the margins are removed.
5. Add page `customer-admin-home` (`top-left-middle-right-bottom`: `top` 48px, `left` 240px
   resizable, `middle` and `right` elastic, no `bottom`) with the header's text and static
   title in `top` and the menu in `left`; store app `customer-admin` as
   `{initial_page: "customer-admin-home"}` and `customers-demo` as
   `{initial_page: "demo-customers"}`.

Copies are made by temporary PL/pgSQL functions (`pg_temp`) that walk the known action
positions (page `input` and `save_action`; an element's `on_click`, `source`,
`on_row_click`, menu `items` and their `children`; an `open-page`'s `input`), the same
positions `actionPositions` walks. Down restores nothing: the old format is no longer
supported by the code.

### D12. Editor

- A page is held as `loadedPage` (attributes, `page_layouter`, `panels` with their
  `elem_layouter`) plus element nodes. A node's data is `{label, element, attrs, class?,
  pane}`; `pageJson` builds each panel's `elem_list` from the nodes whose `pane` is that leaf,
  in reading order, and `pageNodes` does the reverse.
- `PagePreview` draws the pane tree with the runtime's style functions. Each leaf is a drop
  target (`data-editor-pane`); clicking a pane's empty area selects the pane; each element has
  its selection overlay. A drop outside any pane is refused with a toast.
- `PageControl` offers *Page*, *Panes* and *Panel*. *Panes* lists the tree (select a pane),
  shows the selected pane's attributes, and offers *Split into rows*, *Split into columns*,
  *Add pane* (to a split pane), *Delete pane* (not the root; its elements are removed after
  confirmation) and renaming its ID. Splitting leaf X makes it a split pane with children
  `X-1`, `X-2`; X's panel, elements and leaf attributes move to `X-1`. Deleting a child that
  leaves its parent with one child replaces the parent by that child, keeping the parent's
  size attributes. *Panel* edits the selected leaf's element layouter `config`.
- The Properties panel shows a *Pane* select for an element node, to move it to another leaf.
- A new page starts as a `single-pane` page with a `flow-layouter` panel `main` and no class
  copies; saving fills them in (D4) and the editor adopts the returned page. Node class copies
  are kept in node data, so View JSON and saves keep them.
- Every edit takes an undo snapshot, as today.

### D13. Element module documents and docs

The `page-layouter` code module gets `KnowledgeStore/doc-repo/code_modules/interpreter-layouter-page.md`.
`interpreter-layouter-flow.md` and `interpreter-page.md` are updated to the new shapes, and the
devdoc's status records the implementation.

## Risks / Trade-offs

- [Pages grow with class copies; the bundle carries help tips and notes] → Acceptable for
  now; the devdoc notes that the bundle can leave author-only text out later.
- [Running apps break when the format changes] → Apps reload their bundle on open; the
  migration converts every stored page in one transaction.
- [Old pointers in a running browser session point at `/elements/<i>`] → They fail with a
  clear 400; reloading the app fixes it.
- [Static pane checks cannot know every way a page is reached] → The bundle only follows
  `open-page` instances, which is the only way to open a page in a pane; chains are capped,
  and the runtime still shows an error in place if a pane does not resolve.
- [The editor change is large] → Pure helpers (`pane-tree.ts`, `wpi-json.ts`) carry the logic
  and are unit tested; the Svelte components stay thin.
- [The migration's PL/pgSQL copy walk could miss a position] → It mirrors `actionPositions`,
  and a seed test validates every converted page with the Go validator.

## Migration Plan

1. Apply `20261029000001` and `20261030000001` (`go run . --migrate-only` from
   `server/cmd/doc-service`), after a dry run of the new migration's Up inside
   `BEGIN … ROLLBACK`.
2. Restart the server and reload `/wpi/customer-admin` and `/wpi/customers-demo`.
3. Rollback: restore the database backup; the old code cannot read the new format.

## Open Questions

None.
