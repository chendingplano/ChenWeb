## ADDED Requirements

### Requirement: Versioned descriptor storage
The system SHALL store app, page, element and action descriptors in `kb.wpi_descriptors`,
one row per version, identified by `(kind, descriptor_id, version)`. The descriptor in use
SHALL be the highest `version` whose `status` is `active`.

#### Scenario: Latest active version wins
- **WHEN** page `customer-form` has version 1 `active` and version 2 `active`
- **THEN** the system uses version 2

#### Scenario: Retired and draft versions are ignored
- **WHEN** page `customer-form` has version 1 `active` and version 2 `draft`
- **THEN** the system uses version 1

### Requirement: Admins write descriptors as new versions
`PUT /api/v1/wpi/descriptors/:kind/:descriptorId` SHALL validate the body's structure and
capabilities and, if valid, store it as version `max(version)+1` with status `active`,
recording the acting user. References to other descriptors SHALL NOT be checked at this
point. Only admins (admin flag, owner, or `admin` role) SHALL be allowed.

#### Scenario: Valid descriptor is stored
- **WHEN** an admin PUTs a valid page descriptor `customers` that has no earlier versions
- **THEN** the response is 200 with `version` 1 and the row is stored as `active`

#### Scenario: Invalid descriptor is rejected
- **WHEN** an admin PUTs an element descriptor with `element_type` `"slider"`
- **THEN** the response is 400 listing the error and nothing is stored

#### Scenario: Non-admin is refused
- **WHEN** a logged-in user who is not an admin PUTs a descriptor
- **THEN** the response is 403 and nothing is stored

#### Scenario: Body and URL disagree
- **WHEN** the body's `descriptor_id` differs from the URL's `descriptorId`
- **THEN** the response is 400 and nothing is stored

### Requirement: App bundle
`GET /api/v1/wpi/apps/:appId` SHALL return the app descriptor and every page, element and
action descriptor reachable from it, keyed by ID, after validating the whole set. It SHALL
require a logged-in user.

#### Scenario: Valid app
- **WHEN** a logged-in user requests `customer-admin`
- **THEN** the response is 200 with the app, its pages, elements and actions, and any warnings

#### Scenario: Unknown app
- **WHEN** a logged-in user requests an app ID with no active descriptor
- **THEN** the response is 404

#### Scenario: Invalid app
- **WHEN** a page reachable from the app references an element descriptor that does not exist
- **THEN** the response is 422 listing each error with the descriptor's kind, ID, the path
  inside it, and a message

#### Scenario: Reference cycles terminate
- **WHEN** page A opens page B and page B opens page A
- **THEN** each descriptor appears once in the bundle and the request completes

### Requirement: Validation rules
The validator SHALL check, for format `schema_version` `"0.1"`:

- structure: required attributes present, no unknown attributes, values of the allowed
  kinds and enumerations, `descriptor_id` matching `^[a-z0-9][a-z0-9-]{0,127}$`;
- capabilities: every layout template, layouter, element type and action type is one the
  interpreter supports;
- references: every referenced page, element and action exists (bundle only);
- panes: every pane named by the app or an `open_page` action exists in the app's layout
  template (bundle only);
- tables: every table and column named by a `load` or `save` action exists in the database
  (bundle only);
- bindings and references: well-formed paths, no `__proto__`, `constructor` or
  `prototype` segment, and each reference root used only where allowed;
- consistency: an attribute of a page's `save_action` that no `read-write` element on that
  page binds produces a warning, not an error.

#### Scenario: Unknown pane
- **WHEN** an `open_page` action targets pane `side` in an app using `top-left-middle-right-bottom`
- **THEN** the bundle request returns 422 naming the action and the pane

#### Scenario: Missing column
- **WHEN** a `save` action maps an attribute to a column that does not exist in its table
- **THEN** the bundle request returns 422 naming the action, the table and the column

#### Scenario: Forbidden path segment
- **WHEN** an element's `data_binding` is `$input_doc.__proto__.x`
- **THEN** validation reports an error for that element

#### Scenario: Reference root outside its place
- **WHEN** a `load` action's `where` uses `$input_doc.id` instead of `$args.id`
- **THEN** validation reports an error for that action

#### Scenario: Unsupported schema version
- **WHEN** a descriptor has `schema_version` `"0.2"`
- **THEN** validation reports that this interpreter does not support that version

#### Scenario: Unbound save attribute
- **WHEN** a page's `save_action` maps attribute `email` but no `read-write` element on the page binds `$input_doc.email`
- **THEN** the bundle is returned with a warning naming the page and the attribute
