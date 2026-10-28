## ADDED Requirements

### Requirement: Element classes name their code module
An element class row SHALL name the frontend code that draws it, its code module, in the
class attribute `code_module_id`. Several classes MAY name the same code module. The code
modules with code SHALL be `text`, `input`, `select`, `button`, `list` and `menu`, each
registered under its ID in the frontend registry `codeModules` and, with the attributes it
defines, in the backend validator. A `code_module_id` SHALL match
`^[a-z0-9][a-z0-9-]{0,127}$` and SHALL name an active element module in `kb.code_modules`.
An `active` class row SHALL set it, and its module SHALL also have code. A `draft` MAY leave
it out, and MAY name a module that has no code yet. An action class row SHALL NOT set
`code_module_id`. An instance SHALL NOT set it.

#### Scenario: Two classes share a code module
- **WHEN** active classes `simple-button` and `fancy-button` both set `"code_module_id": "button"`, and a page lists an instance of each
- **THEN** both are drawn by the `button` code, each with its own class's defaults

#### Scenario: Active class without a code module
- **WHEN** an admin PUTs an element class row through `PUT /api/v1/wpi/descriptors/element/:id` without `code_module_id`
- **THEN** the response is 400 with an issue saying `code_module_id` is missing

#### Scenario: Unregistered code module
- **WHEN** an admin PUTs an element class row with `"code_module_id": "slider"`
- **THEN** the response is 400 saying code module `slider` has no code in this interpreter

#### Scenario: Draft naming a module whose code is not written
- **WHEN** `slider` is an active element module in `kb.code_modules` with no code yet, and an admin creates draft `dial` with `"code_module_id": "slider"`
- **THEN** the draft is stored

#### Scenario: Module missing from kb.code_modules
- **WHEN** an admin creates draft `dial` with `"code_module_id": "slider"` and `kb.code_modules` has no active element row `slider`
- **THEN** the response is 400 saying code module `slider` is not an active element module in `kb.code_modules`

#### Scenario: Action class with a code module
- **WHEN** an admin PUTs action class `save` with `"code_module_id": "save"`
- **THEN** the response is 400 with an issue at `/code_module_id`

### Requirement: Code modules are listed in kb.code_modules
The table `kb.code_modules` SHALL hold one row per code module with these columns:

- `code_module_id`: unique, matching `^[a-z0-9][a-z0-9-]{0,127}$`;
- `module_name`: unique and not empty;
- `description`;
- `module_type`: `element`, `action`, `page` or `layouter`;
- `module_doc`: the file name of its document in `KnowledgeStore/doc-repo/code_modules`;
- `notes`;
- `status`: `active` or `suspended`;
- `creator`, `created_at` and `updated_at`.

A migration SHALL create it and store the six element modules, active, each with
`module_doc` `<id>.md`. Rows SHALL come from migrations or `psql`; there SHALL be no write
API. `GET /api/v1/wpi/code-modules` SHALL return every row as `code_modules` to a logged-in
user. An app SHALL NOT load while a class it uses names a module that is not an active
element module.

#### Scenario: Seeded modules
- **WHEN** the migration has run
- **THEN** `kb.code_modules` has active element rows `text`, `input`, `select`, `button`, `list` and `menu`, with documents `text.md` … `menu.md`

#### Scenario: Suspended module
- **WHEN** module `button` is `suspended` and app `customer-admin` uses a class drawn by it
- **THEN** `GET /api/v1/wpi/apps/customer-admin` returns 422 with an error at that class's `/code_module_id`

#### Scenario: Editor lists the modules
- **WHEN** a logged-in user requests `GET /api/v1/wpi/code-modules`
- **THEN** the response is 200 with every row of `kb.code_modules`

### Requirement: Classes fix attributes
An element class SHALL be able to fix an instance attribute for all its instances: the
attribute's object MAY hold `instance_modifiable`, `true` or `false`, and leaving it out
SHALL mean `true`. When it is `false` the attribute SHALL be a
class attribute: its `default_value` SHALL be present and not `null`, and SHALL be the value
of every instance; an instance that sets the attribute SHALL fail validation; an attribute
that holds actions SHALL NOT be fixed. A page's `binding_mode` SHALL NOT fill in a
`binding_mode` that the class fixes.

#### Scenario: Fixed value used
- **WHEN** class `fancy-button` has `"style": {"default_value": "fancy", "instance_modifiable": false}` and an instance does not set `style`
- **THEN** the merged instance has `style` `fancy`

#### Scenario: Instance sets a fixed attribute
- **WHEN** an instance of `fancy-button` on page `p` sets `"style": "plain"`
- **THEN** validation fails with an error at that instance's `/style`, saying the class fixes it

#### Scenario: Fixed without a value
- **WHEN** a class row has `"style": {"instance_modifiable": false}` or `"style": {"default_value": null, "instance_modifiable": false}`
- **THEN** writing the class fails with an issue at `/style`

#### Scenario: Fixed action
- **WHEN** a class row has `"on_click": {"default_value": {"action": "close-page"}, "instance_modifiable": false}`
- **THEN** writing the class fails with an issue at `/on_click`

#### Scenario: Fixed binding mode on a read-only page
- **WHEN** class `editable-input` fixes `binding_mode` to `read-write` and a page with `"binding_mode": "read-only"` lists an instance of it
- **THEN** the page is valid and the instance is editable

### Requirement: Attributes a code module does not support
An element class row SHALL be stored even when it names instance attributes that its code
module does not define. Those attributes SHALL have no effect on what is drawn. Each SHALL be reported as a
warning naming the class, the attribute and the code module: in the response of
`PUT /api/v1/wpi/descriptors/element/:id`, of `POST /api/v1/wpi/classes/element` (when the
draft names a registered module) and of `PUT /api/v1/wpi/classes/element/:id`, as `warnings`,
and in the app bundle's `warnings` for the classes the app uses. They SHALL NOT be errors.
The size attributes `width`, `height`, `min_width` and `max_width` SHALL count as defined by
every code module.

#### Scenario: Class edited with a new attribute
- **WHEN** an admin saves active `button` with `"label_hori_align": {"default_value": "center"}`
- **THEN** the response is 200 with a warning that code module `button` does not support `label_hori_align`

#### Scenario: App using that class
- **WHEN** `customer-admin` uses that `button` class
- **THEN** `GET /api/v1/wpi/apps/customer-admin` returns 200 with the bundle and that warning

#### Scenario: Instance sets the unsupported attribute
- **WHEN** an instance of that `button` sets `"label_hori_align": "left"`
- **THEN** the page is valid and the button is drawn as before

### Requirement: Element class rows move to format 0.4
A migration SHALL store, for every element class whose latest `draft` or `active` row is in
format `0.3`, a new version with the same status and body, `schema_version` `0.4` and, for an
active row, `code_module_id` equal to its class ID. It SHALL keep every other attribute of the
row, SHALL record `migration:wpi-code-modules` as its author, SHALL do nothing when run
again, and its down migration SHALL delete the rows it stored.

#### Scenario: Edited class keeps its edits
- **WHEN** active `button` is at version 3 in format `0.3` with an extra attribute `label_hori_align`
- **THEN** after the migration, version 4 is active in format `0.4` with `"code_module_id": "button"` and `label_hori_align`

#### Scenario: Draft gets no code module
- **WHEN** draft `slider` is at version 1 in format `0.3`
- **THEN** after the migration, version 2 is a draft in format `0.4` without `code_module_id`
