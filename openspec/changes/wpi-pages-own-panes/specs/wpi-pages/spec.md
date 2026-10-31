## MODIFIED Requirements

### Requirement: Page attributes
A page descriptor SHALL require `descriptor_id`, `schema_version` (`"0.5"`) and
`page_layouter`, and MAY set `panels`, `page_name`, `description`, `access_controls`,
`keywords`, `page_tags`, `page_policy`, `page_style`, `input`, `save_action`, `refresh`,
`binding_mode`, `hori_align`, `vert_align`, `hori_scrollbar`, `vert_scrollbar`, `bkgd_color`,
`frgd_color`, and the sizes `width`, `height`, `min_width`, `min_height`, `max_width` and
`max_height`. `page_layouter` and `panels` are as in capability `wpi-panes`. The validator
SHALL check:

- `page_name` (at most 100 characters) and `description` (at most 1000) are text attributes
  (a non-empty string, or non-empty strings keyed by language code including `en`);
- `access_controls`, `keywords` and `page_tags` are arrays of non-empty strings; `keywords`
  and `page_tags` hold at most 200 items;
- `page_policy` and `page_style` are strings and are otherwise ignored;
- `hori_align` is `left`, `center` or `right`; `vert_align` is `top`, `center` or `bottom`;
  `hori_scrollbar` and `vert_scrollbar` are `auto` or `hide`;
- a size is a non-negative number with an optional unit `px`, `rem`, `em` or `%` (`px` when
  omitted; spaces may precede the unit), a `%` value is within 0 to 100, and `width` and
  `height` also accept `auto`;
- `bkgd_color` and `frgd_color` are `auto` or a CSS color;
- `binding_mode` is `read-only` or `read-write`.

The page's status is the `status` column of `kb.frontend_pages`; a `status` attribute, the
former `page_alignment`, `layouter`, `layouter_config`, `elements`, the margins and any other
attribute SHALL be an error. Margins belong to panes (capability `wpi-panes`).

#### Scenario: Named and described page
- **WHEN** an admin PUTs a page with `"page_name": {"en": "Customers", "zh-cn": "客户"}` and `"description": "All customers"`
- **THEN** the page is stored

#### Scenario: Elastic page width
- **WHEN** a page sets `"width": "elastic"`
- **THEN** writing the page fails with an error at `/width`

#### Scenario: Unknown alignment
- **WHEN** a page sets `"hori_align": "justify"`
- **THEN** writing the page fails with an error at `/hori_align`

#### Scenario: Former attribute name
- **WHEN** a page sets `"page_alignment": "center"`
- **THEN** writing the page fails with an error at `/page_alignment`

#### Scenario: Margin on the page
- **WHEN** a page sets `"left_margin": "30px"`
- **THEN** writing the page fails with an error at `/left_margin`

#### Scenario: Length without a unit
- **WHEN** a page sets `"max_width": "640"` and `"width": "50 %"`
- **THEN** the page is stored and its maximum width is 640px

### Requirement: Page-wide binding mode
A page's `binding_mode` SHALL be the default for every element on it whose class copy names
`binding_mode`. An element instance's own `binding_mode` SHALL come first, then the page's,
then the default in the instance's class copy. A copy whose `binding_mode` default is `null`
SHALL be satisfied by the page's value.

#### Scenario: Read-only page
- **WHEN** a page with `"binding_mode": "read-only"` has an `input` instance that does not set `binding_mode`, and the instance's copy defaults it to `read-write`
- **THEN** the input is read-only

#### Scenario: Instance overrides the page
- **WHEN** the same page has an `input` instance with `"binding_mode": "read-write"`
- **THEN** that input is editable

### Requirement: Load a page into the editor
The WPI editor's Load Page button SHALL open a modal listing the latest active version
of each page, 20 per page, with descriptor ID, localized name and description, created
and updated timestamps. Authors SHALL be able to filter by descriptor ID, description,
and inclusive local calendar date ranges for creation and update, and navigate pages.
Loading SHALL fetch the full descriptor before replacing the canvas with one editable
node per element instance of every panel, recording the pane it belongs to and preserving
the order within each panel, repeated classes, class copies and all attributes.
The descriptor SHALL stay in memory and View JSON SHALL retain its page metadata, page
layouter and panels.
A nonempty canvas SHALL require confirmation before replacement; undo SHALL restore
the prior canvas and page metadata. Failed requests SHALL leave the canvas unchanged
and display a translated error. The dialog SHALL support English and Chinese,
keyboard focus containment, Escape dismissal and text selection.

#### Scenario: Load a page with two panes
- **WHEN** the author loads a page whose panels `list` and `detail` hold two and three elements
- **THEN** the canvas has five element nodes, two in pane `list` and three in pane `detail`, in their stored order

### Requirement: Canvas page preview
When a page is loaded, the editor SHALL render its pane tree, and in each leaf pane its
panel's elements with the interpreter's layout and element components, inside a resizable
page surface on the canvas, initially centered and scaled to fit. Each rendered element SHALL
have a transparent selection overlay that selects its editor node and opens its properties
without triggering its page action; clicking a pane's empty area SHALL select the pane. An
element class dropped from the palette SHALL be added to the leaf pane under the drop point;
a drop outside every pane SHALL be refused with a message. The preview SHALL use the page's
stored data sources and preserve element instance attributes when properties change. When
flow storage supports persistence, the flow data carries the loaded page alongside its
editable elements.

#### Scenario: Drop into a pane
- **WHEN** the author drops the `button` class onto the preview over pane `detail`
- **THEN** a `button` node is added to pane `detail` and drawn at the end of its panel

### Requirement: Interactive page preview
The editor toolbar SHALL offer Preview for the selected tab, switching the canvas to
an interactive WPI runtime and providing an Edit control to return. Preview SHALL use the current page, whether named or unnamed, and load the pages
it opens. A draft preview SHALL NOT save the page; input and save actions tied to
the draft page SHALL report that saving is required. The page SHALL be the app's initial page, drawn in the app's
page area; panes that its `open-page` instances name but that only an outer page would have,
and other validation failures, SHALL be reported to the author. Switching tabs SHALL return
to editing.

#### Scenario: Preview a saved page
- **WHEN** the author previews saved page `customer-admin-home`
- **THEN** the page runs with its panes, and its menu opens pages in its `middle` pane

### Requirement: Page and layouter controls
When a page is loaded, the editor SHALL show a movable control frame at the upper right of the
canvas with the buttons *Page*, *Panes* and *Panel*. *Page* SHALL show the page's attributes
in the Properties panel; *Panes* SHALL show the page layouter's tree and the selected pane's
attributes; *Panel* SHALL show the element layouter of the selected leaf pane's panel and its
`config`. Selecting an element SHALL return the panel to that element. Edits SHALL update the
in-memory descriptor, refresh the preview and be undoable. The frame SHALL be movable by
pointer and keyboard and initially aligned immediately left of the canvas settings button. The
page frame SHALL have draggable right and bottom edges and a bottom-right corner, and the
corner SHALL also support the keyboard. Resizing the frame SHALL update the page's corresponding
width or height attributes. The pane area SHALL have matching side and bottom insets when it
fills the frame. These changes SHALL be undoable. Saving SHALL store the page with the
server's class copies and adopt the stored page the server returns.

#### Scenario: Resize a page from its edge
- **WHEN** the author drags the right edge of a 500px-wide page frame 100px to the right at 100% zoom
- **THEN** the frame is 600px wide, the page's width attribute becomes `600px`, and its height is unchanged

Under *Page* and *Panes*, the page layouter selector SHALL list the latest active page layouter classes
from `kb.wpi_descriptors`. Choosing a class SHALL apply its pane tree and update the page's class copy. Panels in matching
leaf panes remain in place; elements displaced from other panes move to the first leaf. The
change SHALL be undoable.

#### Scenario: Page configuration shows the page layouter
- **WHEN** the author opens *Page*
- **THEN** the page layouter selector shows the current class and allows choosing another active class

#### Scenario: Change a page layouter
- **WHEN** the author selects another active page layouter
- **THEN** the page uses that class copy and pane tree, and displaced elements move to its first leaf

#### Scenario: Edit a pane's width
- **WHEN** the author selects pane `left` under *Panes* and sets its `width` to `300px`
- **THEN** the preview draws `left` 300px wide, and undo restores 240px

## ADDED Requirements

### Requirement: Editing panes
The tree SHALL label its top pane *root* regardless of its stored `pane_id`. The selected pane's
`pane_id` remains visible and editable.

Under *Panes*, the editor SHALL let the author select a pane in the tree or the preview, edit
its attributes and its `pane_id`, split a leaf pane into rows or columns, add a pane to a split
pane, and delete a pane other than the root. Splitting leaf `x` SHALL make it a split pane
with leaf children `x-1` and `x-2`, and move `x`'s panel, elements and leaf-only attributes to
`x-1`. Deleting a pane SHALL remove its panes, panels and elements, after confirmation when it
holds elements; a split pane left with one child SHALL be replaced by that child, which keeps
the split pane's size attributes. Renaming a pane SHALL rename its panel key and move its
elements with it; a `pane_id` already used in the page SHALL be refused. The Properties panel
SHALL let the author move an element node to another leaf pane. Every pane edit SHALL be
undoable, and all controls SHALL have English and Chinese text.

#### Scenario: Split a pane
- **WHEN** the author splits the single-pane page's pane `main`, which holds three elements, into columns
- **THEN** `main` has children `main-1` and `main-2`, the three elements are in `main-1`, and `main-2` is empty

#### Scenario: Delete a pane with elements
- **WHEN** the author deletes pane `main-2`, which holds one element, and confirms
- **THEN** the element node is removed, and the root pane is now the leaf `main-1` with its panel and `main`'s size attributes

#### Scenario: Move an element to another pane
- **WHEN** the author selects an element in pane `left` and chooses pane `middle` in the Properties panel
- **THEN** the element is drawn at the end of `middle`'s panel
