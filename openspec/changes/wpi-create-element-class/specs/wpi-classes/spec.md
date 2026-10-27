## MODIFIED Requirements

### Requirement: Classes are named after their code
Element classes SHALL be stored as `kind = 'element'` rows and action classes as
`kind = 'action'` rows. A class's `descriptor_id` SHALL be the name under which the
interpreter registers the code that implements it. The element classes SHALL be `text`,
`input`, `select`, `button`, `list` and `menu`; the action classes SHALL be `load`, `save`,
`open-page`, `close-page` and `save-page`. An `active` class row whose ID has no code in the
interpreter, and an instance naming such a class, SHALL be validation errors. An element
class with no code MAY be stored as a `draft` through the create endpoint (capability
`wpi-element-class-authoring`); a draft is never used by the interpreter.

#### Scenario: Class without code
- **WHEN** an admin PUTs an element class row `slider`
- **THEN** the response is 400 saying element class `slider` has no code in this interpreter

#### Scenario: Instance of an unknown class
- **WHEN** a page lists `{"element": "slider"}`
- **THEN** writing the page fails with an error at `/elements/<i>/element`

#### Scenario: Draft class without code
- **WHEN** an admin creates element class `slider` through `POST /api/v1/wpi/classes/element`
- **THEN** it is stored as a `draft`, and the interpreter does not use it

### Requirement: Class rows give defaults or require a value
A class row SHALL hold the class attributes `descriptor_id` and `schema_version` (required),
`description` and `implement_notes` (optional; `implement_notes` is the prompt for
implementing the class). Every other attribute in the row SHALL be an instance attribute
defined by the class's code. In an action class row (format `0.2`) its value SHALL be the
default, or `null` meaning every instance must set it, and an instance attribute the row
leaves out SHALL be optional with no default. In an element class row (format `0.3`) its
value SHALL be an object with only an optional `help_tip` (text for authors) and an
optional `default_value`: the default, or `null` meaning every instance must set it; an
attribute without `default_value`, or left out of the row, SHALL be optional with no
default. Each attribute the class's code requires SHALL be given a default or `null`.
Attributes that hold action instances (`on_click`, `on_row_click`, `source`, `items`, and
`input` of `open-page`) SHALL NOT have a default.

#### Scenario: Default used
- **WHEN** the `input` class row has `"binding_mode": {"default_value": "read-write"}` and an instance does not set `binding_mode`
- **THEN** the instance is editable

#### Scenario: Required attribute missing on an instance
- **WHEN** the `input` class row has `"data_binding": {"default_value": null}` and an instance on page `p` does not set `data_binding`
- **THEN** the app bundle is rejected with an error naming page `p` and the instance's path

#### Scenario: Optional attribute without default
- **WHEN** the `list` class row has `"on_row_click": {"help_tip": "…"}` and an instance does not set it
- **THEN** the list is valid and its rows are not clickable

#### Scenario: Code-required attribute without a default
- **WHEN** an admin PUTs an `input` class row whose `data_binding` has a help tip but no `default_value`
- **THEN** the response is 400 saying the row must give `data_binding` a `default_value` or `null`

#### Scenario: Default for an action-holding attribute
- **WHEN** an admin PUTs a `button` class row with `"on_click": {"default_value": {"action": "close-page"}}`
- **THEN** the response is 400

#### Scenario: Element class row in the flat format
- **WHEN** an admin PUTs an element class row with `"schema_version": "0.2"` or with `"label": null`
- **THEN** the response is 400: element class rows are format `0.3` and each attribute is an object

### Requirement: Text in several languages
The text attributes SHALL accept a non-empty string, shown in every language, or an object
mapping language codes to non-empty strings that includes `en`. They are `label` (an
instance attribute of `input`, `select` and `button`), `description` (a class attribute, and
a page attribute), `implement_notes` (a class attribute), `help_tip` (in an element class
row's attribute) and `page_name` (a page attribute). The interpreter SHALL show the text for
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

#### Scenario: Help tip without English
- **WHEN** an element class row sets `"label": {"help_tip": {"zh-cn": "标题"}, "default_value": null}`
- **THEN** writing the class fails with an error at `/label/help_tip`
