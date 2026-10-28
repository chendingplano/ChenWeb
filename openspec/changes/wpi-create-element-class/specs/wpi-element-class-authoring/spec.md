## ADDED Requirements

### Requirement: Admins create element classes as drafts
`POST /api/v1/wpi/classes/element` SHALL take an element class row as its body, check it
by the draft rules, and store it in `kb.wpi_descriptors` as `kind = 'element'`, version 1,
`status = 'draft'`, recording the acting user. The class's code SHALL NOT be required. On
success the response SHALL be 201 with `descriptor_id`, `version` 1 and `status`
`draft`. Only admins (admin flag, owner, or `admin` role) SHALL be allowed.

#### Scenario: New class stored as a draft
- **WHEN** an admin posts `{"descriptor_id": "slider", "schema_version": "0.3", "description": {"en": "A slider"}, "implement_notes": "A range input", "label": {"help_tip": {"en": "Caption"}, "default_value": null}, "min": {"default_value": 0}}`
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
`0.3`, and a `description` that is a non-empty string or texts by language including `en`;
`implement_notes`, if present, SHALL be text in the same forms. Every other key SHALL be an
instance attribute: its name SHALL match `^[A-Za-z_][A-Za-z0-9_]*$` and SHALL NOT be
`element`; its value SHALL be an object with only an optional `help_tip` (text) and an
optional `default_value`. Attributes that hold actions (`on_click`, `on_row_click`,
`source`, `items`, `input`) SHALL have a `default_value` of `null` or none. Defaults of
`width`, `height`, `min_width` and `max_width` SHALL be lengths; of `label`, text; of
`data_binding`, a `$input_doc` path; of `binding_mode`, `read-only` or `read-write`. Other
attribute names SHALL be accepted. A draft that breaks a
rule SHALL be refused with 400 listing the issues.

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
whose status is `draft` or `active`, as `descriptor_id`, `version`, `status`,
`description` and `attributes` (the row's instance attributes, each with its `help_tip` and
`default_value`; class attributes left out), ordered by `descriptor_id`. Classes whose rows are all `retired` SHALL be
left out. It SHALL require a logged-in user.

#### Scenario: Drafts and active classes
- **WHEN** `input` is active and `slider` is a draft
- **THEN** the list holds `input` with status `active` and `slider` with status `draft`

#### Scenario: Instance attributes listed
- **WHEN** the latest row of `button` has `label` and `on_click`, each `{"help_tip": …, "default_value": null}`
- **THEN** the list's `button` entry has `attributes` with exactly `label` and `on_click`, as stored

#### Scenario: Not logged in
- **WHEN** a request without a session asks for the list
- **THEN** the response is 401

### Requirement: Create Element form
In the WPI editor, **Create Nodes → Create Element** SHALL open a form that defines an
element class as a list of its attributes. The list SHALL start with the class attributes
`descriptor_id` (empty), `schema_version` (`"0.3"`), `description`
(`{"en": "", "zh-cn": ""}`) and `implement_notes` (empty), which are marked as class
attributes in a column of their own and cannot be removed or renamed. An instance attribute
SHALL be entered as a name, a default value (empty for none) or "Every instance must set it"
(stored as `null`), and a help tip in English and optionally Chinese, and added with
**Add Attribute** to the list as `{"help_tip": …, "default_value": …}`; the list shows
every attribute's value (an instance attribute's default, or "no default") and help tip;
instance attributes can be removed. Double-clicking an attribute's value, an instance
attribute's help tip or an instance attribute's name SHALL edit it in place: Enter or
leaving the field keeps the edit, Escape drops it; `implement_notes` is edited in a
multi-line editor, where Ctrl/Cmd+Enter keeps the edit. A rename SHALL be refused for a
malformed, reserved or duplicate name. The author SHALL be able to resize the form by
dragging its corner. A typed value SHALL be
stored as JSON when it parses as JSON, otherwise as a string, except `descriptor_id` and
`schema_version`, which are always strings; an instance attribute cannot be edited to empty.
**Create Element** SHALL post the class, leaving out empty description texts and empty
`implement_notes`, after
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
- **THEN** the list shows `label: null` and `max: 100`, and the created row has `"label": {"default_value": null}, "max": {"default_value": 100}`

#### Scenario: Optional attribute with a help tip
- **WHEN** the author adds `on_change` with no default and the English help tip "Runs on change"
- **THEN** the list shows "no default", and the created row has `"on_change": {"help_tip": {"en": "Runs on change"}}`

#### Scenario: Implementation notes
- **WHEN** the author double-clicks `implement_notes`, types two lines and presses Ctrl+Enter
- **THEN** the list shows both lines, and the created row has them in `implement_notes`

#### Scenario: Duplicate attribute name
- **WHEN** the author adds `label` twice
- **THEN** the second add is refused with a message and the list keeps one `label`

#### Scenario: Class ID already taken
- **WHEN** the author creates a class with ID `input`
- **THEN** the form stays open and says that element class `input` already exists

### Requirement: ELEMENTS folder lists element classes
The editor's palette SHALL list the element classes from the class list in its ELEMENTS
folder, marking drafts as not yet implemented. Dragging a class onto the canvas SHALL add
an element node labelled with the class ID. The new node SHALL be the canvas selection, SHALL
have no connection points, and SHALL be resizable by its corners and edges.

#### Scenario: New draft appears
- **WHEN** the author creates class `slider`
- **THEN** the ELEMENTS folder lists `slider`, marked as a draft, without reopening the editor

#### Scenario: Drag a class onto the canvas
- **WHEN** the author drags `input` from the ELEMENTS folder onto the canvas
- **THEN** an element node labelled `input` is added where it was dropped

#### Scenario: Dropped node is selected and resizable
- **WHEN** the author drops `button` on the canvas
- **THEN** the node shows its resize grips without a click, has no connection points, and
  dragging a corner or an edge resizes it

### Requirement: Element node properties
When an element node is selected, the Properties panel SHALL show its class and list the
class's instance attributes from the class list, each with its help tip and an editable
value. A value SHALL be read as JSON when it parses, otherwise as a string, and SHALL be
kept in the node's `data.attrs`; empty text SHALL unset it. An unset attribute SHALL show
its default ("Default: …"), "No default", or, for `default_value` `null`, that every
instance must set it. A field SHALL keep the text as typed until the author leaves it. When
the node's class is not in the class list, the panel SHALL say so instead of listing
attributes.

#### Scenario: Edit an attribute
- **WHEN** the author selects a `button` node and types `{"action": "save-page"}` into `on_click`
- **THEN** the node's `data.attrs.on_click` is the object `{"action": "save-page"}` and its name is unchanged

#### Scenario: Defaults shown
- **WHEN** the author selects an `input` node whose `input_type` is unset
- **THEN** the `input_type` field is empty and shows "Default: text"

### Requirement: View JSON
The editor's tool bar SHALL have **View JSON**, which opens a read-only view of WPI
descriptor JSON. With one node selected it SHALL show that node: an element node as the
element instance a page holds (`element`, then the attributes set in `data.attrs`); any other
node's data as it is, saying it has no WPI descriptor yet. With no node selected it SHALL show
the canvas as a page: `schema_version` `"0.2"`, `layouter` `"flow-layouter"` and `elements`,
the element nodes' instances in reading order (top to bottom, then left to right), with no
`descriptor_id`. Escape or a click outside SHALL close it.

#### Scenario: Node selected
- **WHEN** an `input` node with `label` `Name` is selected and the author clicks View JSON
- **THEN** the view shows `{"element": "input", "label": "Name"}`

#### Scenario: Nothing selected
- **WHEN** a `button` node sits below an `input` node, nothing is selected, and the author clicks View JSON
- **THEN** the view shows a page whose `elements` are the `input` instance, then the `button` instance

### Requirement: Node tab
In the WPI editor the properties panel SHALL have two tabs, **Properties** (the panel
described above) and **Node**, and SHALL start 390 px wide. With one node selected, the Node
tab SHALL list the node's descriptor, the JSON View JSON shows for it, one attribute per row
with an editable name and value, and SHALL let the author add, rename, change and remove
attributes. A value SHALL be read as JSON when it parses, otherwise as a string, and the
field SHALL keep the text as typed until the author leaves it. A name SHALL start with a
letter or `_` and contain only letters, digits and `_`, and SHALL NOT be one the descriptor
already has; a refused rename SHALL put the old name back and show the problem. For an
element node the edits SHALL go to `data.element` and `data.attrs`, leaving the node's name
unchanged, and `element` SHALL be neither renamed nor removed, its value always text. Any
other node's data SHALL be edited as it is, saying it has no WPI descriptor yet. The Flow
page's properties panel SHALL have no tabs.

#### Scenario: Attribute added
- **WHEN** a `button` node is selected and the author adds `on_click` with the value `{"action": "save-page"}` on the Node tab
- **THEN** the node's `data.attrs.on_click` is the object `{"action": "save-page"}`, listed last, and the Properties tab shows it

#### Scenario: Rename refused
- **WHEN** the author renames `label` to `on_click` on a node that has both
- **THEN** `label` keeps its name and the panel says the node already has an attribute `on_click`

#### Scenario: Class key fixed
- **WHEN** an element node is selected on the Node tab
- **THEN** its `element` row has a read-only name and no remove button

### Requirement: Admins edit element classes
`GET /api/v1/wpi/classes/element/:classId` SHALL return the class's latest row whose status
is `draft` or `active` as `version`, `status` and `class` (the whole row), or 404 when it
has none; it SHALL require a logged-in user. `PUT` on the same path SHALL take an edited
class row and update its latest draft or active row in place, preserving its version,
status, ID and creation audit fields. It SHALL update the body, schema version, updated
user and updated timestamp without inserting any record. If the selected row no longer
exists when saving, it SHALL return 404 without creating a replacement. Both SHALL be checked for valid class structure, attribute names, help tips,
action-default restrictions and shared defaults, independently of the current interpreter's
attribute list. Admins SHALL be able to add, remove and rename instance attributes and edit
their defaults and help tips, including attributes required or unknown by the current code.
Drafts SHALL still require a description; active rows MAY omit one. Interpreter validation
SHALL continue to check descriptors against code when publishing or loading a bundle.
The response SHALL be 200 with `descriptor_id`, `version` and `status`. A body whose `descriptor_id` differs from the path SHALL be refused with 400,
a class with no draft or active row with 404, and a row that breaks a rule with 400 listing
the issues; nothing SHALL be stored then. Only admins SHALL be allowed.

#### Scenario: Draft edited
- **WHEN** draft `slider` is at version 1 and an admin PUTs an edited `slider` row
- **THEN** the existing version 1 row is updated with status `draft`, no row is inserted, and the response says version 1, status `draft`

#### Scenario: Active class edited
- **WHEN** `input` is active at version 2 and an admin PUTs it with a changed help tip
- **THEN** the existing version 2 row is updated with status `active` and no row is inserted

#### Scenario: Attributes the code lacks
- **WHEN** an admin adds `label_horizontal_align` and `label_vertical_align` to active `button`, both with default `center`, and saves
- **THEN** the existing active row stores both attributes without an unknown-attribute error

#### Scenario: Attributes removed or renamed
- **WHEN** an admin removes `on_click` or renames `label` to `caption` in active `button` and saves
- **THEN** the existing active row stores the edited row without requiring those code-defined attributes

#### Scenario: Malformed attribute
- **WHEN** an admin saves a class with an invalid attribute name or a scalar instead of an attribute object
- **THEN** the response is 400 listing the structural issue and nothing is stored

#### Scenario: Class ID changed
- **WHEN** an admin PUTs a body with `descriptor_id` `slider` to `/classes/element/dial`
- **THEN** the response is 400 and nothing is stored

### Requirement: Node tool bar
A selected node on the WPI editor's canvas and on the Flow page SHALL show a tool bar with
**Duplicate** and **Delete**, and no Configure button. Duplicate SHALL add a copy of the
node, with its data and size, one grid cell right of and below it (further while another
node sits there), and select the copy; edges SHALL NOT be copied. Delete SHALL remove the
node and every edge that touches it. Both SHALL be undoable. On the WPI editor, an element
node's tool bar SHALL also have **Edit class**, which opens the Create Element form on the
node's class: the class's latest row, with `descriptor_id` read-only, a note saying whether
the class is a draft or implemented, and **Save** in place of Create Element. Saving SHALL
store the edited row through the class PUT, close the form, say the class was saved, and
reload the class list; a refused save SHALL show the server's issues in the form.

#### Scenario: Duplicate an element node
- **WHEN** an `input` node with `label` `Name` is selected and the author clicks Duplicate
- **THEN** a second `input` node with `label` `Name` and the same size appears one grid cell down-right and is selected

#### Scenario: Delete a wired node
- **WHEN** a page node with an edge to an action node is selected and the author clicks Delete
- **THEN** the page node and the edge are removed, and undo brings both back

#### Scenario: No edit for pages
- **WHEN** a page node is selected
- **THEN** its tool bar has Duplicate and Delete only

#### Scenario: Edit a draft class from its node
- **WHEN** the author clicks Edit class on a node of draft `slider`, changes a default and saves
- **THEN** draft version 2 of `slider` is stored with the change and the form closes
