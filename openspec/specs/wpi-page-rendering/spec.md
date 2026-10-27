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

### Requirement: Flow layouter and sizes
The `flow-layouter` SHALL place a page's elements left to right and wrap to a new row when
the next element does not fit. A fixed width SHALL be used as given; `elastic` widths SHALL
share the remaining row width within each element's `min_width` and `max_width`; `auto`
SHALL size the element to its content. Widths SHALL follow pane resizes.

#### Scenario: Elastic fills remaining space
- **WHEN** a row holds a 160px element and an elastic element with `max_width` 400px in a 500px pane
- **THEN** the elastic element takes the rest of the row: 500px minus 160px and the gap between the two elements

#### Scenario: Wrapping
- **WHEN** the next element's minimum width does not fit in what is left of the row
- **THEN** it starts a new row

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
The interpreter SHALL take all text it shows itself from Paraglide messages in English and
Chinese: the unsaved-changes prompt, error boxes, field error messages, loading and empty
states. Text written inside descriptors SHALL be shown as written.

#### Scenario: Chinese locale
- **WHEN** the locale is `zh-cn` and the unsaved-changes prompt appears
- **THEN** its title and buttons are in Chinese, while page labels from descriptors are unchanged

### Requirement: Save page action
A `save-page` action instance SHALL save the triggering panel's input doc with its page's
`save_action`, exactly as choosing *Save* in the unsaved-changes prompt does, including
writing back the new key and showing field errors next to their elements.

#### Scenario: Save button
- **WHEN** the user edits the *New customer* form and clicks a button whose `on_click` is `{"action": "save-page"}`
- **THEN** the page's `save_action` inserts the customer and the new `id` is written into the input doc

