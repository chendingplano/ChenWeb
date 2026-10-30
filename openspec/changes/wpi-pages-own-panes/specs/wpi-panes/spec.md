## ADDED Requirements

### Requirement: Page layouter and pane tree
A page SHALL have a `page_layouter` whose effective tree has exactly one root pane, the whole
page. A pane SHALL have a `pane_id` (`^[a-z0-9][a-z0-9-]{0,63}$`), unique in the page. A pane
with `split` (`rows` or `columns`) SHALL have two or more `children`; a pane without `split` is
a leaf and SHALL NOT have `children`. With `rows` the children are stacked top to bottom and
divide the pane's height; with `columns` they sit left to right and divide its width. The
attributes that concern what a pane shows (`left_margin`, `right_margin`, `top_margin`,
`bottom_margin`, `hori_align`, `vert_align`, `hori_scrollbar`, `vert_scrollbar`) SHALL apply to
leaf panes only and SHALL be an error on a split pane. A page whose root pane has no children
is a single-pane page.

#### Scenario: Nested split
- **WHEN** a page's root is split into rows `top`, `middle-row` and `bottom`, and `middle-row` is split into columns `left`, `middle` and `right`
- **THEN** the page has the leaf panes `top`, `left`, `middle`, `right` and `bottom`

#### Scenario: Duplicate pane ID
- **WHEN** two panes of a page have `pane_id` `main`
- **THEN** writing the page fails with an error at the second pane

#### Scenario: Split pane with one child
- **WHEN** a pane has `"split": "rows"` and one child
- **THEN** writing the page fails with an error at that pane's `children`

#### Scenario: Margin on a split pane
- **WHEN** a split pane sets `left_margin`
- **THEN** writing the page fails with an error saying the attribute applies to leaf panes only

### Requirement: Pane attributes and geometry
A pane's `width` and `height` SHALL be a length (`px`, `rem` or `em`; `px` when the unit is
left out), a percentage of the containing pane (of the page for the root), `elastic` (the
default) or `auto`. `elastic` panes SHALL share what their fixed, percentage and `auto`
siblings leave, within their limits. `auto` SHALL size a leaf pane to its panel and a split
pane to its children. `min_width` and `min_height` (default `50px`), `max_width` and
`max_height` SHALL limit the size. The size of the root pane SHALL NOT be used: the page's own
sizes apply. A leaf pane SHALL place its content inside its four margins (each `0px` by
default); its content fits in a direction only if the content and the two margins of that
direction fit. When the content is smaller, `hori_align` (`left`, `center` default, `right`)
and `vert_align` (`top` default, `center`, `bottom`) SHALL place it; when it does not fit,
`hori_scrollbar` and `vert_scrollbar` SHALL decide: `auto` (default) shows a scroll bar, `hide`
shows none and clips. A `resizable` pane with a pixel size SHALL have a drag handle that
changes its size within its limits while the app runs.

#### Scenario: Elastic and fixed columns
- **WHEN** a 1000px wide row holds a 240px pane and two elastic panes
- **THEN** each elastic pane is 380px wide

#### Scenario: Margins count for scrolling
- **WHEN** a leaf pane 300px tall with `top_margin` and `bottom_margin` 20px shows a panel 280px tall and `vert_scrollbar` is `auto`
- **THEN** the pane shows a vertical scroll bar

#### Scenario: Resizing a pane
- **WHEN** pane `left` is 240px wide, `resizable`, with `min_width` 160px, and the user drags its edge to 100px
- **THEN** the pane is 160px wide

### Requirement: Panels
A page's `panels` SHALL map leaf pane IDs of its effective tree to panels; any other key SHALL
be an error. A panel SHALL have an `elem_layouter` and MAY have an `elem_list` of element
instances (default empty). A leaf pane without a panel SHALL be empty. A panel's size SHALL
come from its content only; `elastic` and percentage element widths SHALL be measured against
the pane's width inside its left and right margins, and SHALL act as `auto` in a pane whose
width is `auto`.

#### Scenario: Panel for a split pane
- **WHEN** a page has a panel keyed `middle-row`, a split pane
- **THEN** writing the page fails with an error at `/panels/middle-row`

#### Scenario: Elastic element in a pane with margins
- **WHEN** a panel's only element is `elastic` in a 600px pane with 20px left and right margins
- **THEN** the element is 560px wide

### Requirement: Layouter classes and copies
Page layouter classes SHALL be stored in `kb.wpi_descriptors` with `kind` `page-layouter` and
element layouter classes with `kind` `element-layouter`, in format `0.5`. A page layouter
class SHALL have `descriptor_id`, `schema_version`, `code_module_id` (an active module of type
`page-layouter` that has code) and `root`, and MAY have `description`. An element layouter
class SHALL have `descriptor_id`, `schema_version`, `code_module_id` (an active module of type
`element-layouter` that has code) and `config` (every parameter with its default), and MAY
have `description`. The classes `single-pane`, `top-left-middle-right-bottom` and
`flow-layouter` SHALL exist. A page's `page_layouter` SHALL be
`{"layouter": <class ID>, "class": <whole class row>, "root"?: <tree>}` and a panel's
`elem_layouter` SHALL be `{"layouter": <class ID>, "class": <whole class row>, "config"?: <parameters>}`;
`layouter` SHALL equal the copy's `descriptor_id`. The page's `root` and the panel's `config`,
when set, SHALL replace the copy's `root` or `config` whole. A change to a layouter class SHALL
NOT change pages that copied it.

#### Scenario: Panel keeps the defaults
- **WHEN** a panel's `elem_layouter` is a `flow-layouter` copy without its own `config`
- **THEN** the panel uses the copy's `config`: 0.75rem gaps, start and top alignment

#### Scenario: Page changes the tree
- **WHEN** a page copies `top-left-middle-right-bottom` and sets its own `root` without `bottom`
- **THEN** the page has no bottom pane

#### Scenario: Layouter name differs from the copy
- **WHEN** a `page_layouter` has `"layouter": "single-pane"` and a copy whose `descriptor_id` is `top-left-middle-right-bottom`
- **THEN** writing the page fails with an error at `/page_layouter/layouter`

### Requirement: Opening a page in a pane
When a page P is opened in a leaf pane A of another page, A SHALL take the place of P's root
pane B, whatever the shape of P's tree. A's geometry, margins, `hori_align`, `vert_align`,
`hori_scrollbar` and `vert_scrollbar` SHALL apply to P, in place of B's and of P's own
alignment and scroll bars; B's attributes SHALL NOT be used. P's `width`, `height`, limits and
colors SHALL still apply, measured inside A. If P is a single-pane page, its panel keyed by B's
`pane_id` SHALL be drawn in A; otherwise B's children SHALL be created inside A. In the app's
page area, P SHALL be drawn with its own attributes and its root pane B.

#### Scenario: Wide page in a narrow pane
- **WHEN** a page with `"width": "500px"` is opened in a pane 300px wide whose `hori_scrollbar` is `auto`, and the page's root pane has `"hori_scrollbar": "hide"`
- **THEN** the pane shows a horizontal scroll bar

#### Scenario: Host pane alignment
- **WHEN** a single-pane page whose root pane has `"hori_align": "left"` is opened in a pane with `"hori_align": "center"`
- **THEN** the page's panel is centred in the pane

#### Scenario: Multi-pane page in a pane
- **WHEN** a page whose root is split into columns `list` and `detail` is opened in pane `middle`
- **THEN** the `list` and `detail` panes are drawn side by side inside `middle`

### Requirement: Outward pane lookup
An `open-page` with target `pane` SHALL name a leaf pane by `pane_id`, looked up first in the
page that holds the triggering element and then, if that page was opened in a pane of another
page, in that page, and so on up to the page in the app's page area; the nearest page with a
leaf pane of that ID SHALL win. When a page was opened in a pane, its root pane's `pane_id`
SHALL name that pane. The app bundle SHALL check every `open-page` instance against every
chain of pages it can be opened in, starting from the app's initial page, and SHALL report a
`pane` that resolves to no pane as an error at the instance's `/pane`.

#### Scenario: Pane of the home page
- **WHEN** a list on page `customers`, opened in pane `middle` of `customer-admin-home`, opens a customer in pane `right`
- **THEN** the customer opens in the home page's `right` pane

#### Scenario: Unresolved pane
- **WHEN** an `open-page` instance names pane `side` and neither its page nor any page it can be opened in has a leaf pane `side`
- **THEN** the bundle request returns 422 with an error at that instance's `/pane`

### Requirement: Panel Cache
The frontend SHALL keep the page instances opened in panes as
`(page-instance-id, pane-instance-id)` entries. A pane SHALL show one thing at a time: its own
panel, or a page instance opened in it. Opening a page in a pane SHALL hide the page instance
the pane shows, if any, and never close it; then show the cached page instance of the same
page with the same resolved input in that pane, without rendering or reloading it, or else
render the page and add an entry. Hiding SHALL keep the page instance in memory with its input
doc and unsaved changes, and SHALL NOT ask the user anything. Closing a page instance SHALL
first close the page instances opened in its panes, each after the unsaved-changes check, then
remove the instance and its entry; the pane SHALL then show its own panel, or nothing.
`close-page` from the page in the app's page area SHALL do nothing.

#### Scenario: Switching back keeps the page
- **WHEN** the user types into the *New customer* form, clicks *Customers*, then clicks *New customer* again
- **THEN** the same form is shown with the typed values, without reloading and without a prompt

#### Scenario: Same customer twice
- **WHEN** the user opens customer 7 from a list row, then customer 9, then customer 7 again, in pane `right`
- **THEN** customer 7's page instance is shown again with its edits

#### Scenario: Close shows the pane's own panel
- **WHEN** a pane with its own panel shows an opened page and the user runs `close-page` in that page
- **THEN** the page instance is removed and the pane shows its own panel
