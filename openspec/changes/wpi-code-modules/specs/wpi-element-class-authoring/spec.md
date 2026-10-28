## MODIFIED Requirements

### Requirement: Admins create element classes as drafts
`POST /api/v1/wpi/classes/element` SHALL take an element class row as its body, check it
by the draft rules, and store it in `kb.wpi_descriptors` as `kind = 'element'`, version 1,
`status = 'draft'`, recording the acting user. The class's code SHALL NOT be required. On
success the response SHALL be 201 with `descriptor_id`, `version` 1, `status` `draft` and
`warnings`: when the draft names a registered code module, one warning for each attribute
that module does not define (capability `wpi-code-modules`), otherwise none. Only admins
(admin flag, owner, or `admin` role) SHALL be allowed.

#### Scenario: New class stored as a draft
- **WHEN** an admin posts `{"descriptor_id": "slider", "schema_version": "0.4", "description": {"en": "A slider"}, "implement_notes": "A range input", "label": {"help_tip": {"en": "Caption"}, "default_value": null}, "min": {"default_value": 0}}`
- **THEN** the response is 201 with version 1, status `draft` and no warnings, and the row is stored as `draft`

#### Scenario: Draft of an existing code module
- **WHEN** an admin posts draft `fancy-button` with `"code_module_id": "button"` and an attribute `glow`
- **THEN** the response is 201 with a warning that code module `button` does not support `glow`

#### Scenario: Non-admin is refused
- **WHEN** a logged-in user who is not an admin posts a class
- **THEN** the response is 403 and nothing is stored

#### Scenario: Not logged in
- **WHEN** a request without a session posts a class
- **THEN** the response is 401 and nothing is stored

### Requirement: Draft rules
A draft SHALL have:

- `descriptor_id` matching `^[a-z0-9][a-z0-9-]{0,127}$`;
- `schema_version` `0.4`;
- a `description` that is a non-empty string or texts by language including `en`;
- `implement_notes`, if present, as text in the same forms;
- `code_module_id`, if present, matching `^[a-z0-9][a-z0-9-]{0,127}$` and naming an active
  element module in `kb.code_modules`. The module need not have code yet.

Every other key SHALL be an instance attribute. Its name SHALL match
`^[A-Za-z_][A-Za-z0-9_]*$` and SHALL NOT be `element`. Its value SHALL be an object with only
an optional `help_tip` (text), an optional `default_value` and an optional
`instance_modifiable` (`true` or `false`). An attribute with `instance_modifiable` `false`
SHALL have a non-null `default_value` and SHALL NOT hold actions. Attributes that hold
actions (`on_click`, `on_row_click`, `source`, `items`, `input`) SHALL have a
`default_value` of `null` or none.

Defaults SHALL have these forms:

- `width`, `height`, `min_width` and `max_width`: lengths;
- `label`: text;
- `data_binding`: a `$input_doc` path;
- `binding_mode`: `read-only` or `read-write`.

Other attribute names SHALL be accepted. A draft that breaks a rule SHALL be refused with
400 listing the issues.

#### Scenario: Description missing
- **WHEN** an admin posts a draft without `description`
- **THEN** the response is 400 with an issue at `/description`

#### Scenario: Default for an action-holding attribute
- **WHEN** a draft sets `"on_click": {"default_value": {"action": "close-page"}}`
- **THEN** the response is 400 with an issue at `/on_click/default_value`

#### Scenario: Attribute that is not an object
- **WHEN** a draft sets `"max": 1`
- **THEN** the response is 400 with an issue at `/max`

#### Scenario: Reserved or malformed attribute name
- **WHEN** a draft has an attribute `element` or `max-value`
- **THEN** the response is 400 with an issue at that attribute

#### Scenario: Bad size default
- **WHEN** a draft sets `"width": {"default_value": "wide"}`
- **THEN** the response is 400 with an issue at `/width`

#### Scenario: Attribute unknown to the interpreter
- **WHEN** a draft sets `"step": {"help_tip": "Step size", "default_value": 5}`
- **THEN** the draft is stored

#### Scenario: Malformed code module ID
- **WHEN** a draft sets `"code_module_id": "Slider_1"`
- **THEN** the response is 400 with an issue at `/code_module_id`

### Requirement: Drafts are implemented through code and the existing write
A draft SHALL never be run: the interpreter SHALL use only `active` class rows. A draft
SHALL become active when an admin stores the class row through
`PUT /api/v1/wpi/descriptors/element/:id` with a `code_module_id` that names an active
element module in `kb.code_modules` that has code. If the class names a code module that
already has code, no new code is needed; otherwise a developer writes the module's code
first. That write SHALL refuse a class whose code module has no code. After a draft is
created, the editor SHALL say that pages can use it once an admin makes it active.

#### Scenario: Draft without code cannot be activated
- **WHEN** draft `slider` exists, no code module `slider` is registered, and an admin PUTs class `slider` with `"code_module_id": "slider"`
- **THEN** the response is 400 saying code module `slider` has no code in this interpreter

#### Scenario: Draft of an existing module activated
- **WHEN** draft `fancy-button` names code module `button` and an admin PUTs it with its `label` and `on_click` attributes
- **THEN** it is stored as active and pages can use it

#### Scenario: Page naming a draft class
- **WHEN** a page lists `{"element": "slider"}` while `slider` is only a draft
- **THEN** writing the page fails with an error at `/elements/<i>/element`

### Requirement: Create Element form
In the WPI editor, **Create Nodes → Create Element** SHALL open a form that defines an
element class as a list of its attributes.

The list SHALL start with the class attributes, which are marked as class attributes in a
column of their own and cannot be renamed:

- `descriptor_id`, empty;
- `schema_version`, `"0.4"`;
- `code_module_id`, empty;
- `description`, `{"en": "", "zh-cn": ""}`;
- `implement_notes`, empty.

`code_module_id`, `description` and `implement_notes` can be cleared. While
`code_module_id` is being edited, the form SHALL list the active element modules of
`kb.code_modules`.

An instance attribute SHALL be entered in the attribute fields:

- a name;
- a default value, empty for none;
- "Every instance must set it", stored as `null`;
- "Fixed by the class", stored as `"instance_modifiable": false`, which needs a default and
  excludes "Every instance must set it";
- a help tip in English and optionally Chinese.

**Add Attribute** adds it to the list as an attribute object. The list SHALL show every
attribute's value (an instance attribute's default, or "no default"), a "fixed" mark on
fixed attributes, and the help tip. Instance attributes can be removed. Double-clicking an
attribute, or its edit icon, SHALL load it into the fields, and **Modify** SHALL replace it;
an edited class keeps its `descriptor_id`. A name SHALL be refused if it is malformed,
reserved or a duplicate, and a fixed attribute without a default SHALL be refused. The
author SHALL be able to resize the form by dragging its corner. A typed value SHALL be
stored as JSON when it parses as JSON, otherwise as a string, except `descriptor_id`,
`schema_version`, `code_module_id` and `implement_notes`, which are always strings.

**Create Element** SHALL post the class, leaving out empty description texts, empty
`code_module_id` and empty `implement_notes`, after checking that `descriptor_id` is a valid
class ID and `description` has an English text. On success the class SHALL appear in the
ELEMENTS folder. If the server returned warnings, the form SHALL first show "Saved" with the
warnings and close when the author confirms; otherwise it SHALL close at once. On failure
the form SHALL stay open and show the error, including "already exists" for a 409. All of
the form's text SHALL be available in English and Chinese.

#### Scenario: Class attributes listed from the start
- **WHEN** the form opens
- **THEN** the list shows `descriptor_id`, `schema_version` `0.4`, `code_module_id`, `description` and `implement_notes`, with no way to rename them

#### Scenario: Code modules offered
- **WHEN** `kb.code_modules` holds the six seeded element modules and the author edits `code_module_id`
- **THEN** the form lists `button`, `input`, `list`, `menu`, `select` and `text`

#### Scenario: Fixed attribute added
- **WHEN** the author adds `style` with default `fancy` and "Fixed by the class"
- **THEN** the list shows `style` marked fixed, and the created row has `"style": {"default_value": "fancy", "instance_modifiable": false}`

#### Scenario: Fixed attribute without a default
- **WHEN** the author adds `style` with "Fixed by the class" and no default
- **THEN** the add is refused with a message

#### Scenario: Save with warnings
- **WHEN** the author saves class `fancy-button` with code module `button` and an attribute `glow`
- **THEN** the form shows "Saved" and a warning about `glow`, and closes when the author clicks OK

#### Scenario: Attributes listed as they are added
- **WHEN** the author adds `label` with "Every instance must set it" and then `max` with default `100`
- **THEN** the list shows `label: null` and `max: 100`, and the created row has `"label": {"default_value": null}, "max": {"default_value": 100}`

#### Scenario: Duplicate attribute name
- **WHEN** the author adds `label` twice
- **THEN** the second add is refused with a message and the list keeps one `label`

#### Scenario: Class ID already taken
- **WHEN** the author creates a class with ID `input`
- **THEN** the form stays open and says that element class `input` already exists

### Requirement: Element node properties
When an element node is selected, the Properties panel SHALL show its class and list the
class's instance attributes from the class list, each with its help tip and an editable
value. A value SHALL be read as JSON when it parses, otherwise as a string, and SHALL be
kept in the node's `data.attrs`; empty text SHALL unset it. An unset attribute SHALL show
its default ("Default: …"), "No default", or, for `default_value` `null`, that every
instance must set it. An attribute the class fixes SHALL show its value read-only, marked
"Fixed by the class", and SHALL NOT be set in `data.attrs`. A field SHALL keep the text as
typed until the author leaves it. When the node's class is not in the class list, the panel
SHALL say so instead of listing attributes.

#### Scenario: Edit an attribute
- **WHEN** the author selects a `button` node and types `{"action": "save-page"}` into `on_click`
- **THEN** the node's `data.attrs.on_click` is the object `{"action": "save-page"}` and its name is unchanged

#### Scenario: Defaults shown
- **WHEN** the author selects an `input` node whose `input_type` is unset
- **THEN** the `input_type` field is empty and shows "Default: text"

#### Scenario: Fixed attribute
- **WHEN** the author selects a node of a class that fixes `style` to `fancy`
- **THEN** the `style` field shows `"fancy"`, cannot be edited and says "Fixed by the class"

### Requirement: Admins edit element classes
`GET /api/v1/wpi/classes/element/:classId` SHALL return the class's latest row whose status
is `draft` or `active` as `version`, `status` and `class` (the whole row), or 404 when it
has none; it SHALL require a logged-in user. `PUT` on the same path SHALL take an edited
class row and update its latest draft or active row in place, preserving its version,
status, ID and creation audit fields. It SHALL update the body, schema version, updated
user and updated timestamp without inserting any record. If the selected row no longer
exists when saving, it SHALL return 404 without creating a replacement.

Both SHALL be checked for valid class structure, attribute names, help tips, fixed
attributes, action-default restrictions and shared defaults, independently of the current
interpreter's attribute list. Admins SHALL be able to add, remove and rename instance
attributes and edit their defaults, help tips and whether they are fixed, including
attributes the code requires or does not define.

Drafts SHALL still require a description; active rows MAY omit one. An active row SHALL set
`code_module_id` to a code module that has code and MAY change it; a draft MAY leave it out.
A `code_module_id`, in either, SHALL name an active element module in `kb.code_modules`.
Interpreter validation SHALL continue to check descriptors against code when publishing or
loading a bundle.

The response SHALL be 200 with `descriptor_id`, `version`, `status` and `warnings` (one for
each attribute the class's code module does not define, when the module has code). A row
SHALL be refused, and nothing stored, when:

- its `descriptor_id` differs from the path: 400;
- the class has no draft or active row: 404;
- it breaks a rule: 400, listing the issues.

Only admins SHALL be allowed.

#### Scenario: Draft edited
- **WHEN** draft `slider` is at version 1 and an admin PUTs an edited `slider` row
- **THEN** the existing version 1 row is updated with status `draft`, no row is inserted, and the response says version 1, status `draft`

#### Scenario: Active class edited
- **WHEN** `input` is active at version 2 and an admin PUTs it with a changed help tip
- **THEN** the existing version 2 row is updated with status `active`, no row is inserted, and there are no warnings

#### Scenario: Attributes the code lacks
- **WHEN** an admin adds `label_horizontal_align` and `label_vertical_align` to active `button`, both with default `center`, and saves
- **THEN** the existing active row stores both attributes, and the response has a warning for each

#### Scenario: Attributes removed or renamed
- **WHEN** an admin removes `on_click` or renames `label` to `caption` in active `button` and saves
- **THEN** the existing active row stores the edited row without requiring those code-defined attributes

#### Scenario: Active class without a code module
- **WHEN** an admin saves active `button` without `code_module_id`, or with one that has no code
- **THEN** the response is 400 with an issue at `/code_module_id` and nothing is stored

#### Scenario: Suspended code module
- **WHEN** module `button` is `suspended` in `kb.code_modules` and an admin saves a class naming it
- **THEN** the response is 400 saying code module `button` is not an active element module, and nothing is stored

#### Scenario: Malformed attribute
- **WHEN** an admin saves a class with an invalid attribute name or a scalar instead of an attribute object
- **THEN** the response is 400 listing the structural issue and nothing is stored

#### Scenario: Class ID changed
- **WHEN** an admin PUTs a body with `descriptor_id` `slider` to `/classes/element/dial`
- **THEN** the response is 400 and nothing is stored
