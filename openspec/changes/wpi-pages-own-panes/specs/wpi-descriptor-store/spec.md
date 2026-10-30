## MODIFIED Requirements

### Requirement: Versioned descriptor storage
The system SHALL store app, element class, action class, page layouter class and element
layouter class descriptors in `kb.wpi_descriptors`, one row per version, identified by
`(kind, descriptor_id, version)`, with `kind` `app`, `element`, `action`, `page-layouter` or
`element-layouter`, and pages in `kb.frontend_pages` (capability `wpi-pages`). The descriptor
in use SHALL be the highest `version` whose `status` is `active`.

#### Scenario: Latest active version wins
- **WHEN** page `customer-form` has version 1 `active` and version 2 `active`
- **THEN** the system uses version 2

#### Scenario: Retired and draft versions are ignored
- **WHEN** page `customer-form` has version 1 `active` and version 2 `draft`
- **THEN** the system uses version 1

#### Scenario: No pages in the descriptor table
- **WHEN** a row of kind `page` is inserted into `kb.wpi_descriptors`
- **THEN** the database rejects it

#### Scenario: Layouter classes
- **WHEN** the system reads the active `page-layouter` rows
- **THEN** it finds `single-pane` and `top-left-middle-right-bottom`

### Requirement: Admins write descriptors as new versions
`PUT /api/v1/wpi/descriptors/:kind/:descriptorId` SHALL validate the body's structure and
capabilities and, if valid, store it as version `max(version)+1` with status `active`,
recording the acting user. `kind` SHALL be `app`, `page`, `element`, `action`,
`page-layouter` or `element-layouter`. For a page, the server SHALL first copy the active
classes into instances and layouter copies that have none (capability `wpi-classes`), then
check the page completely, including each instance against its class copy, and SHALL return
the stored page as `page`. References to other descriptors SHALL NOT be checked at this point.
Only admins (admin flag, owner, or `admin` role) SHALL be allowed.

#### Scenario: Valid descriptor is stored
- **WHEN** an admin PUTs a valid page descriptor `customers` that has no earlier versions
- **THEN** the response is 200 with `version` 1 and the stored page, and the row is stored as `active`

#### Scenario: Invalid descriptor is rejected
- **WHEN** an admin PUTs a page whose instance is `{"element": "slider"}` and `slider` has no active class
- **THEN** the response is 400 listing the error and nothing is stored

#### Scenario: Non-admin is refused
- **WHEN** a logged-in user who is not an admin PUTs a descriptor
- **THEN** the response is 403 and nothing is stored

#### Scenario: Body and URL disagree
- **WHEN** the body's `descriptor_id` differs from the URL's `descriptorId`
- **THEN** the response is 400 and nothing is stored

### Requirement: App bundle
`GET /api/v1/wpi/apps/:appId` SHALL return the app descriptor and every page reachable from it
(its `initial_page` and the pages `open-page` instances open), keyed by ID, with any warnings,
after validating the whole set: each page, every pane an `open-page` names (capability
`wpi-panes`), every table and column, every statement a `query` names and every file a
`load-file` names, and that every code module a class copy names is active. It SHALL NOT return
class rows: instances carry their class copies. It SHALL require a logged-in user.

#### Scenario: Valid app
- **WHEN** a logged-in user requests `customer-admin`
- **THEN** the response is 200 with the app, its pages and any warnings

#### Scenario: Unknown app
- **WHEN** a logged-in user requests an app ID with no active descriptor
- **THEN** the response is 404

#### Scenario: Invalid app
- **WHEN** a page reachable from the app has an instance whose class copy names a code module that is suspended
- **THEN** the response is 422 listing each error with the descriptor's kind, ID, the path
  inside it, and a message

#### Scenario: Reference cycles terminate
- **WHEN** page A opens page B and page B opens page A
- **THEN** each descriptor appears once in the bundle and the request completes

#### Scenario: Unregistered statement
- **WHEN** a page reachable from the app has a `query` whose `sql_stmt_id` is not in `kb.stored_stmts`
- **THEN** the response is 422 with an error at that instance's `/sql_stmt_id`

### Requirement: Validation rules
The validator SHALL check each descriptor in its kind's format: `schema_version` `"0.5"` for
apps, pages, page layouter classes and element layouter classes, `"0.4"` for element class
rows and `"0.2"` for action class rows:

- structure: required attributes present, no unknown attributes, values of the allowed
  kinds and enumerations, `descriptor_id` matching `^[a-z0-9][a-z0-9-]{0,127}$`;
- apps: `initial_page` names a page ID;
- pages and panes: as in capabilities `wpi-pages` and `wpi-panes`;
- capabilities: every active action class has code, every active element class names a code
  module that has code and is an active element module in `kb.code_modules`, and every
  layouter class names a code module of its layouter type that has code (an element class
  draft is checked for structure and against `kb.code_modules`, capability
  `wpi-element-class-authoring`);
- class rows and class copies: as in capabilities `wpi-classes` and `wpi-code-modules`; an
  attribute that a class's code module does not define produces a warning, not an error;
- instances (page write and bundle, checked against their class copies): as in capability
  `wpi-classes`; the merged instance has every attribute the code requires and passes the
  cross-attribute rules: a `text` element has exactly one of `text` and `data_binding`, and a
  menu's `initial_item` is an item that opens a page;
- references: every referenced page exists (bundle only);
- panes: every pane an `open-page` instance names resolves (bundle only, capability
  `wpi-panes`);
- tables: every table and column named by a `load` or `save` instance exists in the
  database (bundle only);
- bindings and references: well-formed paths, no `__proto__`, `constructor` or
  `prototype` segment, and each reference root used only where allowed;
- consistency: an attribute of a page's `save_action` that no `read-write` element on that
  page binds produces a warning, not an error.

#### Scenario: Unknown pane
- **WHEN** an `open-page` instance targets pane `side`, which no page in any chain it can be opened in has
- **THEN** the bundle request returns 422 naming the page, the instance's path and the pane

#### Scenario: Missing column
- **WHEN** a `save` instance maps an attribute to a column that does not exist in its table
- **THEN** the bundle request returns 422 naming the page, the table and the column

#### Scenario: Forbidden path segment
- **WHEN** an element instance's `data_binding` is `$input_doc.__proto__.x`
- **THEN** validation reports an error for that instance

#### Scenario: Reference root outside its place
- **WHEN** a `load` instance's `where` uses `$input_doc.id` instead of `$args.id`
- **THEN** validation reports an error for that instance

#### Scenario: Unsupported schema version
- **WHEN** a page has `schema_version` `"0.2"`
- **THEN** validation reports that this interpreter supports `"0.5"` for page descriptors

#### Scenario: Unbound save attribute
- **WHEN** a page's `save_action` maps attribute `email` but no `read-write` element on the page binds `$input_doc.email`
- **THEN** the bundle is returned with a warning naming the page and the attribute

#### Scenario: Element class row in an old format
- **WHEN** an element class row has `schema_version` `"0.3"`
- **THEN** validation reports that this interpreter supports `"0.4"` for element descriptors

#### Scenario: Page write checks instances against their copies
- **WHEN** an admin PUTs a page whose `input` instance does not set `data_binding`, whose default in the instance's copy is `null`
- **THEN** the response is 400 with an error at that instance's path
