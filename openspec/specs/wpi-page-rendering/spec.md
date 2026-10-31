# wpi-page-rendering Specification

## Purpose
Drawing WPI pages in the browser: element instances drawn by the code of their class, lists
that query their own records, the flow layouter and sizes, data binding to the input doc,
dirty tracking, the unsaved-changes prompt and save errors shown next to their fields.
## Requirements
### Requirement: Element types
The interpreter SHALL draw instances of these element classes: `text` (static text or a
bound value), `input` (with `input_type` `text`, `number` or `email`), `select` (options
from the instance), `button`, `list` (`rows` presentation, read-only) and `menu` (a tree of
items and folders). Each class SHALL be drawn by the frontend code registered under its
class ID. An instance whose class has no registered code SHALL be replaced by an error box
naming the page and the class.

#### Scenario: Instance overrides a default
- **WHEN** a page lists `{"element": "input", "label": "Email", "data_binding": "$input_doc.email", "binding_mode": "read-only"}` and the `input` class defaults `binding_mode` to `read-write`
- **THEN** the input is drawn and the user cannot edit it

#### Scenario: Menu folder
- **WHEN** a menu item has `children`
- **THEN** it is drawn as a folder that expands and collapses to show its child items

### Requirement: Lists query their own records
A `list` element SHALL run its `source` instance (`load` or `query`) when first drawn, show one row per
returned record with the configured columns, and on a row click run `on_row_click` with
`$record` set to that row's record. It SHALL show a loading state while querying, an empty
state when there are no records, and an error box when the query fails.

#### Scenario: Rows drawn
- **WHEN** the list's `source` returns three customers
- **THEN** the list shows three rows with user name, email and phone columns

#### Scenario: Row click passes the record
- **WHEN** the user clicks the row of customer 7 and `on_row_click` passes `{"id": "$record.id"}`
- **THEN** the triggered action receives argument `id` 7

### Requirement: List pagination and integer pixel sizes
A list SHALL accept optional `pagination` of `"yes"` or `"no"`, defaulting to `"yes"`.
Enabled pagination SHALL show at most 20 returned records per page with localized
previous/next controls and page and record counts. Boundary controls SHALL be disabled.
Pagination SHALL operate on the source result without making additional requests and
SHALL reset to page 1 after a successful source reload. Disabled pagination SHALL show
all returned records and no pagination controls.

List `width` and `height` SHALL additionally accept non-negative integer pixel values,
while preserving existing size strings. A fixed-height list SHALL scroll its records
and keep pagination controls visible. Paging a clickable list SHALL preserve the
clicked row's record as `$record`.

#### Scenario: Navigate a list
- **WHEN** the source returns 45 records and pagination is omitted
- **THEN** page 1 shows records 1–20, page 2 shows 21–40, and page 3 shows 41–45
- **AND** moving between pages does not rerun the source

#### Scenario: Disable pagination
- **WHEN** the source returns 45 records and `pagination` is `"no"`
- **THEN** all 45 records are shown without pagination controls

### Requirement: Flow layouter and sizes
The `flow-layouter` SHALL place a page's elements left to right and wrap to a new row when
the next element does not fit. A fixed width SHALL be used subject to the flow width limits; `elastic` widths SHALL
share the remaining row width within each element's `min_width` and `max_width`; `auto`
SHALL size the element to its content. Widths SHALL follow pane resizes.

#### Scenario: Elastic fills remaining space
- **WHEN** a row holds a 160px element and an elastic element with `max_width` 400px in a 500px pane
- **THEN** the elastic element takes the rest of the row: 500px minus 160px and the gap between the two elements

#### Scenario: Wrapping
- **WHEN** the next element's minimum width does not fit in what is left of the row
- **THEN** it starts a new row

### Requirement: Page box
The interpreter SHALL draw a page in a frame whose padding is the page's margins (each
`30px` by default). The page box SHALL use `width`, `height`, `min_width` and `min_height`
(each `50px` by default), `max_width` and `max_height`. Percentages SHALL resolve against the
pane's content area excluding the margins. Without `width` the page SHALL fill the frame's
width; without `height` it SHALL be as tall as its content. When `max_width` or `max_height`
is set and the content is larger, the page SHALL scroll. When the page is narrower than the
frame, `hori_align` (default `center`) SHALL place it; when it is shorter, `vert_align`
(default `top`) SHALL. `bkgd_color` and `frgd_color` set to `auto` SHALL inherit the
application theme, so the page follows light and dark mode.

#### Scenario: Centred form
- **WHEN** a page sets `"max_width": "640px"` in a pane 1200px wide with default margins
- **THEN** the page is 640px wide, centred in the space between the margins

#### Scenario: Long content under a maximum height
- **WHEN** a page sets `"max_height": "300px"` and its content is 500px tall
- **THEN** the page is 300px tall with a vertical scroll bar

### Requirement: Flow spacing and alignment
A panel MAY set `elem_layouter.config` for `flow-layouter`. It SHALL accept
`column_gap` and `row_gap` (nonnegative decimal lengths in `px` or `rem`, default
`0.75rem`), `horizontal_alignment` (`start`, `center`, `end`, `space-between`,
default `start`), `vertical_alignment` (`top`, `center`, `bottom`, `stretch`, default
`top`), and `rows_alignment` (`top`, `center`, `bottom`, `space-between`, default
`top`). It also accepts `min_row_height` and `max_row_height` (defaults `30px` and
`200px`), `min_elem_width` and `max_elem_width` (defaults `20px` and `300px`),
`max_num_elems` (default 50; zero is unlimited), and `row_separator` and
`elem_separator` (default `none`; thin, norm and bold solid or dash lines).
Dimensions are nonnegative `px` or `rem` lengths, and the count is a nonnegative
integer. Unknown keys, non-object config and unsupported values SHALL be rejected
at their descriptor paths. Missing settings use their defaults, including in older copies.

The layouter SHALL clamp wrapper dimensions to the configured limits. Overflowing
non-text elements SHALL scroll; text exceeding the row maximum SHALL wrap, truncate with
an ellipsis, and expose its full text on hover. The row limit SHALL apply after natural wrapping.
Horizontal separators span the widest row; vertical separators span their row. Thin,
normal, and bold lines measure 1, 2, and 3 pixels respectively.

Horizontal alignment SHALL distribute leftover space independently in each row after
elastic growth and width limits. Row breaks and source order SHALL be preserved.
Vertical alignment SHALL position unequal-height items within each row; stretch SHALL
fill auto-height wrappers and their rendered element roots while retaining explicit
heights. Definite page height SHALL reach the flow container. Rows alignment SHALL
distribute spare vertical space without stretching rows; overflowing rows SHALL fall
back to top placement. Content-height pages SHALL have no extra height to distribute.
Space-between SHALL retain configured minimum gaps; a single item or row SHALL start
at the start/top edge. Items larger than the containing panel SHALL retain normal overflow.

#### Scenario: Exact fit and resized pane
- **WHEN** seven 240px elements have 16px gaps in a 1008px layout
- **THEN** four elements fit in the first row and three in the second
- **WHEN** the usable width becomes 1007px
- **THEN** only three elements fit in the first row, preserving source order

#### Scenario: Centered last row
- **WHEN** three 240px elements form the last row in 1008px with 16px gaps and horizontal center
- **THEN** the row has 128px of space on each side

#### Scenario: Overflow remains reachable
- **WHEN** centered or bottom-aligned rows exceed a definite page height
- **THEN** the first row stays at the top and subsequent rows overflow normally

### Requirement: Data binding
An element with `data_binding` `$input_doc.<path>` SHALL show the value at that path. With
`binding_mode` `read-write` the user SHALL be able to edit it and each edit SHALL be written
to that path, creating missing attributes; with `read-only` it SHALL NOT be editable.
Changes to the input doc made by actions SHALL update every element bound to the changed
path.

#### Scenario: Empty doc grows through bindings
- **WHEN** the *New customer* form opens with input `{}` and the user types a user name
- **THEN** the input doc becomes `{"user_name": "<typed value>"}`

#### Scenario: Nested path
- **WHEN** an element binds `$input_doc.address.city` read-write on an empty doc and the user types "Paris"
- **THEN** the input doc becomes `{"address": {"city": "Paris"}}`

### Requirement: Input doc lifecycle and dirty tracking
Each panel SHALL keep a clean copy of its input doc taken when it was opened or last
loaded or saved. The panel SHALL be dirty exactly when its input doc differs from that copy.
After a successful save the saved doc SHALL become the new clean copy and the returned key
SHALL be written into the doc.

#### Scenario: Edit and undo is not dirty
- **WHEN** the user changes a field and then changes it back
- **THEN** the panel is not dirty

#### Scenario: Insert then save again
- **WHEN** a new customer is saved and then edited and saved again
- **THEN** the second save updates the row created by the first

### Requirement: Save errors next to fields
When a save returns `field_errors`, each error SHALL be shown next to the element bound to
that attribute, as a translated message for its error code. Errors for an attribute SHALL
clear when the user edits that element.

#### Scenario: Required email
- **WHEN** a save returns `{"field_errors": {"email": ["required"]}}`
- **THEN** the input bound to `$input_doc.email` shows the "required" message in the current language

### Requirement: Unsaved-changes prompt
Closing a dirty panel SHALL ask the user to save, discard, or keep editing. "Save" SHALL
run the page's `save_action` and close the panel only if the save succeeds; "discard"
SHALL close it without saving; "keep editing" SHALL leave it open.

#### Scenario: Save from the prompt fails validation
- **WHEN** the user chooses "save" and the save returns field errors
- **THEN** the panel stays open and shows the errors

#### Scenario: Close a clean panel
- **WHEN** the user clicks a `close_page` button on a panel that is not dirty
- **THEN** the panel closes without a prompt

### Requirement: Interpreter text is translated
The interpreter SHALL take its own UI text from Paraglide messages in English and Chinese,
including prompts, errors, loading/empty states, text help controls and text Properties.
Supported localized descriptor fields SHALL select current locale then English. This
includes text-module static content, caption and tooltip as specified by `wpi-text-element`.
Plain strings SHALL be shown as written; bound objects SHALL retain String conversion
rather than being interpreted as language maps. Other module-specific text retains its
existing contract.

#### Scenario: Chinese locale
- **WHEN** the locale is `zh-cn` and a text element has English/Chinese static content, caption and tooltip
- **THEN** its content and supporting text use Chinese and its help control has a Chinese accessible name

#### Scenario: Plain strings remain literal
- **WHEN** a text element's source is a plain string or a bound object
- **THEN** the string is not translated and the bound object is not interpreted as a language map

### Requirement: Save page action
A `save-page` action instance SHALL save the triggering panel's input doc with its page's
`save_action`, exactly as choosing *Save* in the unsaved-changes prompt does, including
writing back the new key and showing field errors next to their elements.

#### Scenario: Save button
- **WHEN** the user edits the *New customer* form and clicks a button whose `on_click` is `{"action": "save-page"}`
- **THEN** the page's `save_action` inserts the customer and the new `id` is written into the input doc

