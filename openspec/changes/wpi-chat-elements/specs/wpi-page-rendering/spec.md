## MODIFIED Requirements

### Requirement: Element types
The interpreter SHALL draw instances of these element classes: `text` (static text or a
bound value), `input` (with `input_type` `text`, `number` or `email`), `select` (options
from the instance), `button`, `list` (`rows` presentation, read-only), `menu` (a tree of
items and folders), `chat-composer` (message box and tool bar for an AI assistant) and
`chat-panel` (the Chat and Dialog tabs of the page's chat). Each instance SHALL be drawn by
the frontend code registered under its class copy's `code_module_id`. An instance whose
code module has no registered code SHALL be replaced by an error box naming the page and
the module.

#### Scenario: Instance overrides a default
- **WHEN** a page lists `{"element": "input", "label": "Email", "data_binding": "$input_doc.email", "binding_mode": "read-only"}` and the `input` class defaults `binding_mode` to `read-write`
- **THEN** the input is drawn and the user cannot edit it

#### Scenario: Menu folder
- **WHEN** a menu item has `children`
- **THEN** it is drawn as a folder that expands and collapses to show its child items

#### Scenario: Chat elements
- **WHEN** a page lists a `chat-composer` and a `chat-panel`
- **THEN** both are drawn and share the chat of the page instance
