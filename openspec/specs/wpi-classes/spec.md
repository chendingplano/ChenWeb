# wpi-classes Specification

## Purpose
Element and action classes and their instances in WPI: a small, stable set of classes whose
IDs name the code that implements them, class rows that give instance attributes defaults or
require them, instances written inside the pages that use them and merged with their class,
and text attributes given in several languages.
## Requirements
### Requirement: Classes are named after their code
Element classes SHALL be stored as `kind = 'element'` rows and action classes as
`kind = 'action'` rows. A class's `descriptor_id` SHALL be the name under which the
interpreter registers the code that implements it. The element classes SHALL be `text`,
`input`, `select`, `button`, `list` and `menu`; the action classes SHALL be `load`, `save`,
`open-page`, `close-page` and `save-page`. A class row whose ID has no code in the
interpreter, and an instance naming such a class, SHALL be validation errors.

#### Scenario: Class without code
- **WHEN** an admin PUTs an element class row `slider`
- **THEN** the response is 400 saying element class `slider` has no code in this interpreter

#### Scenario: Instance of an unknown class
- **WHEN** a page lists `{"element": "slider"}`
- **THEN** writing the page fails with an error at `/elements/<i>/element`

### Requirement: Class rows give defaults or require a value
A class row SHALL hold the class attributes `descriptor_id` and `schema_version` (required)
and `description` (optional). Every other attribute in the row SHALL be an instance
attribute defined by the class's code; its value SHALL be the default, or `null` meaning
every instance must set it. An instance attribute the row leaves out SHALL be optional with
no default. Each attribute the class's code requires SHALL appear in the row. Attributes
that hold action instances (`on_click`, `on_row_click`, `source`, `items`, and `input` of
`open-page`) SHALL NOT have a default.

#### Scenario: Default used
- **WHEN** the `input` class row has `"binding_mode": "read-write"` and an instance does not set `binding_mode`
- **THEN** the instance is editable

#### Scenario: Required attribute missing on an instance
- **WHEN** the `input` class row has `"data_binding": null` and an instance on page `p` does not set `data_binding`
- **THEN** the app bundle is rejected with an error naming page `p` and the instance's path

#### Scenario: Optional attribute without default
- **WHEN** the `list` class row leaves out `on_row_click` and an instance does not set it
- **THEN** the list is valid and its rows are not clickable

#### Scenario: Code-required attribute left out of the row
- **WHEN** an admin PUTs an `input` class row without `data_binding`
- **THEN** the response is 400 saying the row must give `data_binding` a default or `null`

#### Scenario: Default for an action-holding attribute
- **WHEN** an admin PUTs a `button` class row with an `on_click` default
- **THEN** the response is 400

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

