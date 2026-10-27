## MODIFIED Requirements

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
A `list` element SHALL run its `source` `load` instance when first drawn, show one row per
returned record with the configured columns, and on a row click run `on_row_click` with
`$record` set to that row's record. It SHALL show a loading state while querying, an empty
state when there are no records, and an error box when the query fails.

#### Scenario: Rows drawn
- **WHEN** the list's `source` returns three customers
- **THEN** the list shows three rows with user name, email and phone columns

#### Scenario: Row click passes the record
- **WHEN** the user clicks the row of customer 7 and `on_row_click` passes `{"id": "$record.id"}`
- **THEN** the triggered action receives argument `id` 7

## ADDED Requirements

### Requirement: Save page action
A `save-page` action instance SHALL save the triggering panel's input doc with its page's
`save_action`, exactly as choosing *Save* in the unsaved-changes prompt does, including
writing back the new key and showing field errors next to their elements.

#### Scenario: Save button
- **WHEN** the user edits the *New customer* form and clicks a button whose `on_click` is `{"action": "save-page"}`
- **THEN** the page's `save_action` inserts the customer and the new `id` is written into the input doc
