## MODIFIED Requirements

### Requirement: Classes are named after their code
Element classes SHALL be stored as `kind = 'element'` rows and action classes as
`kind = 'action'` rows. An action class's `descriptor_id` SHALL be the name under which the
interpreter registers the code that implements it; the action classes SHALL be `load`,
`save`, `open-page`, `close-page`, `save-page`, `query` and `load-file`. An element class's
`descriptor_id` SHALL identify the class only; the class SHALL name the code that draws it,
its code module, in `code_module_id` (capability `wpi-code-modules`), and several element
classes MAY share one code module. An `active` action class row whose ID has no code, an
`active` element class row whose `code_module_id` has no code or is not an active element
module in `kb.code_modules`, and an instance naming a class that has no `active` row SHALL
be validation errors. An element class whose code
module has no code MAY be stored as a `draft` through the create endpoint (capability
`wpi-element-class-authoring`); a draft is never used by the interpreter.

#### Scenario: Class without code
- **WHEN** an admin PUTs an element class row `slider` with `"code_module_id": "slider"`
- **THEN** the response is 400 saying code module `slider` has no code in this interpreter

#### Scenario: Instance of an unknown class
- **WHEN** a page lists `{"element": "slider"}` and `slider` has no active row
- **THEN** writing the page fails with an error at `/elements/<i>/element`

#### Scenario: Draft class without code
- **WHEN** an admin creates element class `slider` through `POST /api/v1/wpi/classes/element`
- **THEN** it is stored as a `draft`, and the interpreter does not use it

### Requirement: Class rows give defaults or require a value
A class row SHALL hold the class attributes `descriptor_id` and `schema_version` (required),
`description` and `implement_notes` (optional; `implement_notes` is the prompt for
implementing the class) and, in an element class row, `code_module_id`. Every other
attribute in the row SHALL be an instance attribute. In an action class row (format `0.2`)
it SHALL be defined by the class's code, its value SHALL be the default, or `null` meaning
every instance must set it, and an instance attribute the row leaves out SHALL be optional
with no default. In an element class row (format `0.4`) its name SHALL match
`^[A-Za-z_][A-Za-z0-9_]*$` and SHALL NOT be `element`. Its value SHALL be an object with only
these optional parts:

- `help_tip`: text for authors;
- `default_value`: the default, or `null` meaning every instance must set it;
- `instance_modifiable`: `false` makes the attribute fixed (capability `wpi-code-modules`).

An attribute without `default_value`, or left out of the row, SHALL be optional with no
default. An element class row MAY name attributes its code module does not define; they are
warnings, not errors (capability `wpi-code-modules`). Each attribute the class's code or
code module requires SHALL be given a default or `null`. Attributes that hold action
instances (`on_click`, `on_row_click`, `source`, `items`, and `input` of `open-page`) SHALL
NOT have a default.

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

#### Scenario: Element class row in an old format
- **WHEN** an admin PUTs an element class row with `"schema_version": "0.3"` or with `"label": null`
- **THEN** the response is 400: element class rows are format `0.4` and each attribute is an object

### Requirement: Instances live in pages and are merged with their class
An element instance SHALL be written in a page's `elements` as
`{"element": "<class>", ...}`; an action instance SHALL be written where it is used as
`{"action": "<class>", ...}`. The places that hold action instances are:

- a button's or menu item's `on_click`;
- a list's `on_row_click` or `source`;
- a page's `input` or `save_action`;
- an `open-page`'s `input`.

An instance SHALL NOT set class attributes. An action instance SHALL set only instance
attributes its class's code defines, plus `args`. An element instance SHALL set only
instance attributes its class row names, plus `width`, `height`, `min_width` and
`max_width`, and none that its class fixes. The interpreter SHALL use the merged instance:
the class row's non-null defaults overlaid with the instance's own attributes, then the
class's fixed values. For an element, a page-wide `binding_mode` fills in before the merge
when the class names `binding_mode` and does not fix it (capability `wpi-pages`).

Each position SHALL accept only these classes:

- triggers: `save`, `open-page`, `close-page`, `save-page`;
- `source`: `load` or `query`;
- `input`: static data (an object without `action`), `load`, `query` or `load-file`;
- `save_action`: `save`.

#### Scenario: Two instances of one class
- **WHEN** page `customer-form` lists `{"element": "input", "label": "User name", "data_binding": "$input_doc.user_name"}` and `{"element": "input", "label": "Phone", "data_binding": "$input_doc.phone_number"}`
- **THEN** both are drawn by the `input` code module, each with its own label and binding and the class's default sizes

#### Scenario: Instance sets a class attribute
- **WHEN** a page lists `{"element": "input", "description": "x", ...}`
- **THEN** writing the page fails with an error at that attribute

#### Scenario: Instance sets an attribute its class does not name
- **WHEN** the `button` class row does not name `input_type` and a page lists `{"element": "button", "input_type": "email", ...}`
- **THEN** writing the page fails with an error at `/elements/<i>/input_type`

#### Scenario: Wrong class in a position
- **WHEN** a list's `source` is `{"action": "save", ...}`
- **THEN** writing the page fails with an error naming the allowed classes `load, query`

#### Scenario: Static data at a trigger
- **WHEN** a button's `on_click` is `{"page": "customers"}`, an object without `action`
- **THEN** writing the page fails with a missing `action` error; only `input` positions take static data
