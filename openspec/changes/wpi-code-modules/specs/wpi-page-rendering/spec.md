## MODIFIED Requirements

### Requirement: Element types
The interpreter SHALL draw instances of element classes with these code modules: `text`
(static text or a bound value), `input` (with `input_type` `text`, `number` or `email`),
`select` (options from the instance), `button`, `list` (`rows` presentation, read-only) and
`menu` (a tree of items and folders). Each instance SHALL be drawn by the frontend code
registered under its class's `code_module_id`. An instance whose class's code module has no
registered code SHALL be replaced by an error box naming the page and the code module.

#### Scenario: Instance overrides a default
- **WHEN** a page lists `{"element": "input", "label": "Email", "data_binding": "$input_doc.email", "binding_mode": "read-only"}` and the `input` class defaults `binding_mode` to `read-write`
- **THEN** the input is drawn and the user cannot edit it

#### Scenario: Menu folder
- **WHEN** a menu item has `children`
- **THEN** it is drawn as a folder that expands and collapses to show its child items

#### Scenario: Class drawn by a shared code module
- **WHEN** class `fancy-button` has `"code_module_id": "button"`
- **THEN** its instances are drawn by the `button` code
