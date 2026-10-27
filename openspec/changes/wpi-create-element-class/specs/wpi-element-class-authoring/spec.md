## ADDED Requirements

### Requirement: Admins create element classes as drafts
`POST /api/v1/wpi/classes/element` SHALL take an element class row as its body, check it
by the draft rules, and store it in `kb.wpi_descriptors` as `kind = 'element'`, version 1,
`status = 'draft'`, recording the acting user. The class's code SHALL NOT be required. On
success the response SHALL be 201 with `descriptor_id`, `version` 1 and `status`
`draft`. Only admins (admin flag, owner, or `admin` role) SHALL be allowed.

#### Scenario: New class stored as a draft
- **WHEN** an admin posts `{"descriptor_id": "slider", "schema_version": "0.2", "description": {"en": "A slider"}, "label": null, "min": 0}`
- **THEN** the response is 201 with version 1 and status `draft`, and the row is stored as `draft`

#### Scenario: Non-admin is refused
- **WHEN** a logged-in user who is not an admin posts a class
- **THEN** the response is 403 and nothing is stored

#### Scenario: Not logged in
- **WHEN** a request without a session posts a class
- **THEN** the response is 401 and nothing is stored

### Requirement: Only new class IDs are created
The create endpoint SHALL refuse a class ID for which any `kind = 'element'` row exists,
whatever its version or status, with 409 and the error `element class "<id>" already
exists`, and SHALL store nothing. A concurrent create of the same ID SHALL get the same
error.

#### Scenario: ID of an active class
- **WHEN** an admin posts a class `input`
- **THEN** the response is 409 saying element class `input` already exists

#### Scenario: ID of an existing draft
- **WHEN** draft `slider` exists and an admin posts a class `slider` again
- **THEN** the response is 409 and no second row is stored

### Requirement: Draft rules
A draft SHALL have `descriptor_id` matching `^[a-z0-9][a-z0-9-]{0,127}$`, `schema_version`
`0.2`, and a `description` that is a non-empty string or texts by language including `en`.
Every other key SHALL be an instance attribute: its name SHALL match
`^[A-Za-z_][A-Za-z0-9_]*$` and SHALL NOT be `element`; its value SHALL be a default or
`null`. Attributes that hold actions (`on_click`, `on_row_click`, `source`, `items`,
`input`) SHALL be `null`. Defaults of `width`, `height`, `min_width` and `max_width` SHALL
be lengths; of `label`, text; of `data_binding`, a `$input_doc` path; of `binding_mode`,
`read-only` or `read-write`. Other attribute names SHALL be accepted. A draft that breaks a
rule SHALL be refused with 400 listing the issues.

#### Scenario: Description missing
- **WHEN** an admin posts a draft without `description`
- **THEN** the response is 400 with an issue at `/description`

#### Scenario: Default for an action-holding attribute
- **WHEN** a draft sets `"on_click": {"action": "close-page"}`
- **THEN** the response is 400 with an issue at `/on_click`

#### Scenario: Reserved or malformed attribute name
- **WHEN** a draft has an attribute `element` or `max-value`
- **THEN** the response is 400 with an issue at that attribute

#### Scenario: Bad size default
- **WHEN** a draft sets `"width": "wide"`
- **THEN** the response is 400 with an issue at `/width`

#### Scenario: Attribute unknown to the interpreter
- **WHEN** a draft sets `"step": 5`
- **THEN** the draft is stored

### Requirement: Drafts are implemented through code and the existing write
A draft SHALL never be run: the interpreter SHALL use only `active` class rows. A draft
SHALL become active only when a developer has registered the class's code and an admin
stores the class row through `PUT /api/v1/wpi/descriptors/element/:id`. That write SHALL
still refuse a class with no code.

#### Scenario: Draft without code cannot be activated
- **WHEN** draft `slider` exists, no code is registered as `slider`, and an admin PUTs class `slider`
- **THEN** the response is 400 saying element class `slider` has no code in this interpreter

#### Scenario: Page naming a draft class
- **WHEN** a page lists `{"element": "slider"}` while `slider` is only a draft
- **THEN** writing the page fails with an error at `/elements/<i>/element`

### Requirement: Element class list
`GET /api/v1/wpi/classes/element` SHALL return, for each element class, its latest version
whose status is `draft` or `active`, as `descriptor_id`, `version`, `status` and
`description`, ordered by `descriptor_id`. Classes whose rows are all `retired` SHALL be
left out. It SHALL require a logged-in user.

#### Scenario: Drafts and active classes
- **WHEN** `input` is active and `slider` is a draft
- **THEN** the list holds `input` with status `active` and `slider` with status `draft`

#### Scenario: Not logged in
- **WHEN** a request without a session asks for the list
- **THEN** the response is 401

### Requirement: Create Element form
In the WPI editor, **Create Nodes → Create Element** SHALL open a form that defines an
element class as a list of its attributes. The list SHALL start with the class attributes
`descriptor_id` (empty), `schema_version` (`"0.2"`) and `description`
(`{"en": "", "zh-cn": ""}`), which are marked as class attributes in a column of their own
and cannot be removed or renamed. An instance attribute SHALL be entered
as a name and either a default value or "Every instance must set it" (stored as `null`), and
added with **Add Attribute** to the list, which shows every attribute with its stored value;
instance attributes can be removed. Double-clicking an attribute's value SHALL edit it in
place, and double-clicking an instance attribute's name SHALL rename it in place: Enter or
leaving the field keeps the edit, Escape drops it. A rename SHALL be refused for a
malformed, reserved or duplicate name. The author SHALL be able to resize the form by
dragging its corner. A typed value SHALL be
stored as JSON when it parses as JSON, otherwise as a string, except `descriptor_id` and
`schema_version`, which are always strings; an instance attribute cannot be edited to empty.
**Create Element** SHALL post the class, leaving out empty description texts, after
checking that `descriptor_id` is a valid class ID and `description` has an English text. On success the form SHALL close and the class SHALL appear
in the ELEMENTS folder. On failure the form SHALL stay open and show the error, including
"already exists" for a 409. All of the form's text SHALL be available in English and
Chinese.

#### Scenario: Class attributes listed from the start
- **WHEN** the form opens
- **THEN** the list shows `descriptor_id`, `schema_version` `0.2` and `description`, with no way to remove them

#### Scenario: Value edited by double-click
- **WHEN** the author double-clicks `descriptor_id`, types `slider` and presses Enter
- **THEN** the list shows `slider`, and the created row has `"descriptor_id": "slider"`

#### Scenario: Instance attribute renamed
- **WHEN** the author double-clicks the name `max` and types `maximum`
- **THEN** the list shows `maximum` with the same value, and the created row has `maximum`, not `max`

#### Scenario: Class attribute name is read-only
- **WHEN** the author double-clicks the name `description`
- **THEN** nothing becomes editable

#### Scenario: Class ID missing
- **WHEN** the author presses Create Element without setting `descriptor_id`
- **THEN** nothing is sent and the form says `descriptor_id` must be the class ID

#### Scenario: Attributes listed as they are added
- **WHEN** the author adds `label` with "Every instance must set it" and then `max` with default `100`
- **THEN** the list shows `label: null` and `max: 100`, and the created row has `"label": null, "max": 100`

#### Scenario: Duplicate attribute name
- **WHEN** the author adds `label` twice
- **THEN** the second add is refused with a message and the list keeps one `label`

#### Scenario: Class ID already taken
- **WHEN** the author creates a class with ID `input`
- **THEN** the form stays open and says that element class `input` already exists

### Requirement: ELEMENTS folder lists element classes
The editor's palette SHALL list the element classes from the class list in its ELEMENTS
folder, marking drafts as not yet implemented. Dragging a class onto the canvas SHALL add
an element node labelled with the class ID.

#### Scenario: New draft appears
- **WHEN** the author creates class `slider`
- **THEN** the ELEMENTS folder lists `slider`, marked as a draft, without reopening the editor

#### Scenario: Drag a class onto the canvas
- **WHEN** the author drags `input` from the ELEMENTS folder onto the canvas
- **THEN** an element node labelled `input` is added where it was dropped
