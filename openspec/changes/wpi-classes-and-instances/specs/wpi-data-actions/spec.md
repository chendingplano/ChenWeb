## MODIFIED Requirements

### Requirement: Only stored actions run
`POST /api/v1/wpi/actions/run` SHALL run only `load` and `save` action instances written in
pages stored in `kb.wpi_descriptors`, named by `ref` as
`{"kind": "page", "id": page ID, "pointer": JSON Pointer}`. The system SHALL load the
instance's action class, check the instance against it and run the merged instance. The
request SHALL NOT be able to supply a table name, column name or action body. A logged-in
user SHALL be required.

#### Scenario: Action instance by pointer
- **WHEN** a logged-in user runs `{"ref": {"kind": "page", "id": "customer-form", "pointer": "/save_action"}, "input_doc": {...}}`
- **THEN** the system runs the page's `save` instance merged with the `save` class

#### Scenario: Nested instance
- **WHEN** a logged-in user runs `{"ref": {"kind": "page", "id": "customers", "pointer": "/elements/1/on_row_click/input"}, "args": {"id": 7}}`
- **THEN** the system runs the `load` instance nested in that `open-page` with `id` 7

#### Scenario: Other reference kinds
- **WHEN** `ref.kind` is `action` or `element`
- **THEN** the response is 400

#### Scenario: Extra fields are ignored for targeting
- **WHEN** the request body also contains `"table": "kb.users"`
- **THEN** the system ignores it and uses only the stored instance's table

#### Scenario: Action that is not load or save
- **WHEN** the pointer names an `open-page` instance
- **THEN** the response is 400

#### Scenario: Not logged in
- **WHEN** the request has no valid session
- **THEN** the response is 401 and nothing runs

### Requirement: Argument substitution
The system SHALL replace each `$args.<name>` value in the action with the request's
`args.<name>` value and SHALL pass every value to the database as a query parameter, never
as SQL text. In the browser, a trigger's `args` SHALL be resolved where the trigger runs; a
`load` instance nested as an `open-page` `input` SHALL receive the `open-page`'s resolved
arguments, or, if it has `args` of its own, those resolved with `$args` meaning the
`open-page`'s arguments.

#### Scenario: Missing argument
- **WHEN** a `load` instance uses `$args.id` and the request has no `id` argument
- **THEN** the response is 400 naming the missing argument

#### Scenario: Value containing SQL
- **WHEN** an argument value is `"1; DROP TABLE x"`
- **THEN** it is passed as a parameter value and no extra statement runs

#### Scenario: Nested load receives the opener's arguments
- **WHEN** a list row click runs `{"action": "open-page", "args": {"id": "$record.id"}, "input": {"action": "load", "where": {"id": "$args.id"}, ...}, ...}` on customer 7
- **THEN** the nested `load` runs with argument `id` 7
