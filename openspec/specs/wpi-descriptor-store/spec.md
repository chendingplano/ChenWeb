# wpi-descriptor-store Specification

## Purpose
Storage and validation of WPI descriptors — apps and classes in `kb.wpi_descriptors`, pages
in `kb.frontend_pages`: versioned rows where the latest active version wins, the admin write
endpoint, the validation rules, and the app bundle that returns an app with its pages and the
classes they use.
## Requirements
### Requirement: Versioned descriptor storage
The system SHALL store app, element and action descriptors in `kb.wpi_descriptors`, one row
per version, identified by `(kind, descriptor_id, version)`, and pages in
`kb.frontend_pages` (capability `wpi-pages`). The descriptor in use SHALL be the highest
`version` whose `status` is `active`.

#### Scenario: Latest active version wins
- **WHEN** page `customer-form` has version 1 `active` and version 2 `active`
- **THEN** the system uses version 2

#### Scenario: Retired and draft versions are ignored
- **WHEN** page `customer-form` has version 1 `active` and version 2 `draft`
- **THEN** the system uses version 1

#### Scenario: No pages in the descriptor table
- **WHEN** a row of kind `page` is inserted into `kb.wpi_descriptors`
- **THEN** the database rejects it

### Requirement: Admins write descriptors as new versions
`PUT /api/v1/wpi/descriptors/:kind/:descriptorId` SHALL validate the body's structure and
capabilities and, if valid, store it as version `max(version)+1` with status `active`,
recording the acting user. For an element or action row, the body is a class row. For a
page, its element and action instances SHALL be checked as far as possible without other
descriptors: each names a class that has code, sets only attributes that code defines, and
has valid values. References to other descriptors and checks against class rows SHALL NOT
be made at this point. Only admins (admin flag, owner, or `admin` role) SHALL be allowed.

For page writes, missing required element or action instance values SHALL be allowed so an
incomplete page can be saved. Supplied values, class copies, page structure and conflicting
values SHALL still be checked. Runtime bundle validation SHALL continue to require complete
instances. Saving creates the latest active page version even if it is incomplete; that
version cannot be run until it is completed.

#### Scenario: Save an incomplete list
- **WHEN** an admin saves a page containing a list without `source` or `columns`
- **THEN** the page is stored and can be reopened for editing
- **AND** runtime validation reports the missing configuration until it is supplied

#### Scenario: Valid descriptor is stored
- **WHEN** an admin PUTs a valid page descriptor `customers` that has no earlier versions
- **THEN** the response is 200 with `version` 1 and the row is stored as `active`

#### Scenario: Invalid descriptor is rejected
- **WHEN** an admin PUTs a page whose instance is `{"element": "slider"}`
- **THEN** the response is 400 listing the error and nothing is stored

#### Scenario: Non-admin is refused
- **WHEN** a logged-in user who is not an admin PUTs a descriptor
- **THEN** the response is 403 and nothing is stored

#### Scenario: Body and URL disagree
- **WHEN** the body's `descriptor_id` differs from the URL's `descriptorId`
- **THEN** the response is 400 and nothing is stored

### Requirement: App bundle
`GET /api/v1/wpi/apps/:appId` SHALL return the app descriptor, every page reachable from
it, and the element and action classes those pages use, keyed by ID, after validating the
whole set, including that every statement a `query` names and every file a `load-file`
names is registered. It SHALL load all active classes of each kind with one query per kind.
It SHALL require a logged-in user.

#### Scenario: Valid app
- **WHEN** a logged-in user requests `customer-admin`
- **THEN** the response is 200 with the app, its pages, the classes they use, and any warnings

#### Scenario: Unknown app
- **WHEN** a logged-in user requests an app ID with no active descriptor
- **THEN** the response is 404

#### Scenario: Invalid app
- **WHEN** a page reachable from the app has an instance of a class with no active class row
- **THEN** the response is 422 listing each error with the descriptor's kind, ID, the path
  inside it, and a message

#### Scenario: Reference cycles terminate
- **WHEN** page A opens page B and page B opens page A
- **THEN** each descriptor appears once in the bundle and the request completes

#### Scenario: Broken unused class
- **WHEN** an active class row that no page of the app uses is invalid
- **THEN** the app's bundle is still returned

#### Scenario: Unregistered statement
- **WHEN** a page reachable from the app has a `query` whose `sql_stmt_id` is not in `kb.stored_stmts`
- **THEN** the response is 422 with an error at that instance's `/sql_stmt_id`

### Requirement: Validation rules
The validator SHALL check, for format `schema_version` `"0.2"`:

- structure: required attributes present, no unknown attributes, values of the allowed
  kinds and enumerations, `descriptor_id` matching `^[a-z0-9][a-z0-9-]{0,127}$`;
- capabilities: every layout template and layouter is one the interpreter supports, and
  every element and action class — of a class row or named by an instance — has code;
- class rows: as in capability `wpi-classes`;
- instances (bundle only): every attribute its class row sets to `null` is set, and the
  merged instance has every attribute the code requires and passes the cross-attribute
  rules (a `text` element has exactly one of `text` and `data_binding`; a menu's
  `initial_item` is an item that opens a page);
- references: every referenced page and class exists (bundle only);
- panes: every pane named by the app or an `open-page` instance exists in the app's layout
  template (bundle only);
- tables: every table and column named by a `load` or `save` instance exists in the
  database (bundle only);
- bindings and references: well-formed paths, no `__proto__`, `constructor` or
  `prototype` segment, and each reference root used only where allowed;
- consistency: an attribute of a page's `save_action` that no `read-write` element on that
  page binds produces a warning, not an error.

#### Scenario: Unknown pane
- **WHEN** an `open-page` instance targets pane `side` in an app using `top-left-middle-right-bottom`
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
- **WHEN** a descriptor has `schema_version` `"0.1"`
- **THEN** validation reports that this interpreter does not support that version

#### Scenario: Unbound save attribute
- **WHEN** a page's `save_action` maps attribute `email` but no `read-write` element on the page binds `$input_doc.email`
- **THEN** the bundle is returned with a warning naming the page and the attribute

