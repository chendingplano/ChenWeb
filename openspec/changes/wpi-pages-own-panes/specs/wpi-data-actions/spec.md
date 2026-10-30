## MODIFIED Requirements

### Requirement: Only stored actions run
`POST /api/v1/wpi/actions/run` SHALL run only `load`, `save`, `query` and `load-file` action
instances written in stored pages, named by `ref` as
`{"kind": "page", "id": page ID, "pointer": JSON Pointer}`. The system SHALL check the
instance against the class copy it holds and run the instance merged with that copy; it SHALL
NOT read the stored action class. The request SHALL NOT be able to supply a table name, column
name, statement, file or action body. A logged-in user SHALL be required.

#### Scenario: Action instance by pointer
- **WHEN** a logged-in user runs `{"ref": {"kind": "page", "id": "customer-form", "pointer": "/save_action"}, "input_doc": {...}}`
- **THEN** the system runs the page's `save` instance merged with its copy of the `save` class

#### Scenario: Nested instance
- **WHEN** a logged-in user runs `{"ref": {"kind": "page", "id": "customers", "pointer": "/panels/main/elem_list/1/on_row_click/input"}, "args": {"id": 7}}`
- **THEN** the system runs the `load` instance nested in that `open-page` with `id` 7

#### Scenario: Other reference kinds
- **WHEN** `ref.kind` is `action` or `element`
- **THEN** the response is 400

#### Scenario: Extra fields are ignored for targeting
- **WHEN** the request body also contains `"table": "kb.users"`
- **THEN** the system ignores it and uses only the stored instance's table

#### Scenario: Action that does not run on the server
- **WHEN** the pointer names an `open-page` instance
- **THEN** the response is 400

#### Scenario: Static input is not an action
- **WHEN** the pointer names a page's `input` that is static data
- **THEN** the response is 400

#### Scenario: Not logged in
- **WHEN** the request has no valid session
- **THEN** the response is 401 and nothing runs
