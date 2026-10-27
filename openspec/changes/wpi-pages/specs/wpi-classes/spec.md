## MODIFIED Requirements

### Requirement: Instances live in pages and are merged with their class
An element instance SHALL be written in a page's `elements` as
`{"element": "<class>", ...}`; an action instance SHALL be written where it is used as
`{"action": "<class>", ...}`: a button's or menu item's `on_click`, a list's `on_row_click`
or `source`, a page's `input` or `save_action`, or an `open-page`'s `input`. An instance
SHALL NOT set class attributes, and SHALL set only instance attributes its class's code
defines (plus `args` for action instances). The interpreter SHALL use the merged instance:
the class row's non-null defaults overlaid with the instance's own attributes; for an
element, a page-wide `binding_mode` fills in before the merge (capability `wpi-pages`). Each
position SHALL accept only these classes: triggers `save`, `open-page`, `close-page`,
`save-page`; `source` `load` or `query`; `input` static data (an object without `action`),
`load`, `query` or `load-file`; `save_action` `save`.

#### Scenario: Two instances of one class
- **WHEN** page `customer-form` lists `{"element": "input", "label": "User name", "data_binding": "$input_doc.user_name"}` and `{"element": "input", "label": "Phone", "data_binding": "$input_doc.phone_number"}`
- **THEN** both are drawn by the `input` code, each with its own label and binding and the class's default sizes

#### Scenario: Instance sets a class attribute
- **WHEN** a page lists `{"element": "input", "description": "x", ...}`
- **THEN** writing the page fails with an error at that attribute

#### Scenario: Wrong class in a position
- **WHEN** a list's `source` is `{"action": "save", ...}`
- **THEN** writing the page fails with an error naming the allowed classes `load, query`

#### Scenario: Static data at a trigger
- **WHEN** a button's `on_click` is `{"page": "customers"}`, an object without `action`
- **THEN** writing the page fails with a missing `action` error; only `input` positions take static data

### Requirement: Text in several languages
The text attributes SHALL accept a non-empty string, shown in every language, or an object
mapping language codes to non-empty strings that includes `en`. They are `label` (an
instance attribute of `input`, `select` and `button`), `description` (a class attribute, and
a page attribute) and `page_name` (a page attribute). The interpreter SHALL show the text for
the current locale, falling back to `en`.

#### Scenario: Chinese locale
- **WHEN** an instance has `"label": {"en": "Phone", "zh-cn": "电话"}` and the locale is `zh-cn`
- **THEN** the label shows "电话"

#### Scenario: Missing language falls back to English
- **WHEN** an instance has `"label": {"en": "Phone"}` and the locale is `zh-cn`
- **THEN** the label shows "Phone"

#### Scenario: Page name without English
- **WHEN** a page sets `"page_name": {"zh-cn": "客户"}`
- **THEN** writing the page fails with an error at `/page_name`
