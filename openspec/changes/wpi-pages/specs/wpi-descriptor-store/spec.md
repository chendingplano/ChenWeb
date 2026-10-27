## MODIFIED Requirements

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
