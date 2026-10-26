## ADDED Requirements

### Requirement: Only stored actions run
`POST /api/v1/wpi/actions/run` SHALL run only `load` and `save` actions taken from
descriptors stored in `kb.wpi_descriptors`: either a stored action (`{"action": id}`) or an
inline action inside a stored page (`{"page": id, "pointer": JSON Pointer}`). The request
SHALL NOT be able to supply a table name, column name or action body. A logged-in user
SHALL be required.

#### Scenario: Stored action by ID
- **WHEN** a logged-in user runs `{"action": "load-customer", "args": {"id": 7}}`
- **THEN** the system runs the stored `load-customer` action with `id` 7

#### Scenario: Inline action by pointer
- **WHEN** a logged-in user runs `{"page": "customer-form", "pointer": "/elements/3/on_click"}` and that place holds an inline `save` action
- **THEN** the system runs that inline action

#### Scenario: Extra fields are ignored for targeting
- **WHEN** the request body also contains `"table": "kb.users"`
- **THEN** the system ignores it and uses only the stored descriptor's table

#### Scenario: Action that is not load or save
- **WHEN** the referenced action is an `open_page` action
- **THEN** the response is 400

#### Scenario: Not logged in
- **WHEN** the request has no valid session
- **THEN** the response is 401 and nothing runs

### Requirement: Argument substitution
The system SHALL replace each `$args.<name>` value in the action with the request's
`args.<name>` value and SHALL pass every value to the database as a query parameter, never
as SQL text.

#### Scenario: Missing argument
- **WHEN** `load-customer` uses `$args.id` and the request has no `id` argument
- **THEN** the response is 400 naming the missing argument

#### Scenario: Value containing SQL
- **WHEN** an argument value is `"1; DROP TABLE x"`
- **THEN** it is passed as a parameter value and no extra statement runs

### Requirement: Any existing table, checked and quoted
`load` and `save` SHALL be allowed on any table, written `name` or `schema.name`. Before
running, the system SHALL confirm that the table and every named column exist in the
database catalog, and SHALL quote them as identifiers.

#### Scenario: Table dropped after validation
- **WHEN** a stored action names a table that no longer exists
- **THEN** the response is 400 naming the table and nothing runs

### Requirement: Load
A `load` action SHALL select the mapped columns, filtered by `where` equality conditions,
and return them renamed to their attributes. With `result` `record` it SHALL return one
object; with `result` `list` it SHALL return an array ordered by `order_by`, limited to
`limit` (default 200, maximum 1000).

#### Scenario: One record
- **WHEN** `load-customer` runs with `id` 7 and that row exists
- **THEN** the response is 200 with `result` an object with attributes `id`, `user_name`, `phone_number`, `email`

#### Scenario: Record not found
- **WHEN** `load-customer` runs with an `id` that matches no row
- **THEN** the response is 404

#### Scenario: List with limit cap
- **WHEN** a list `load` action sets `limit` 5000
- **THEN** at most 1000 records are returned

### Requirement: Save with server-side validation
A `save` action SHALL validate the input doc against its fields' `required`, `max_length`,
`pattern` and `format` (`email`) rules on the server. If any fail, it SHALL return 422 with
`field_errors` mapping each failing attribute to error codes and SHALL write nothing.
Otherwise it SHALL write the mapped attributes to their columns.

#### Scenario: Required attribute missing
- **WHEN** `save-customer` runs with an input doc lacking `user_name`
- **THEN** the response is 422 with `field_errors.user_name` containing `required` and no row changes

#### Scenario: Several failures
- **WHEN** `email` is `"not-an-email"` and `user_name` is longer than its `max_length`
- **THEN** `field_errors` has `format` for `email` and `max_length` for `user_name`

### Requirement: Save modes and key write-back
`mode` `insert` SHALL insert a row and return the new key; `update` SHALL update the row
whose key column equals the input doc's key attribute; `upsert` SHALL insert when the key
attribute is missing or empty and update otherwise. A successful save SHALL return
`key_attribute` and `key_value`. Updates SHALL NOT check for concurrent changes: the last
save wins.

#### Scenario: Upsert inserts a new record
- **WHEN** `save-customer` (`upsert`, key `id`) runs with an input doc without `id`
- **THEN** a row is inserted and the response returns `key_attribute` `id` and the new `key_value`

#### Scenario: Upsert updates an existing record
- **WHEN** `save-customer` runs with an input doc whose `id` is 7
- **THEN** row 7 is updated and the response returns `key_value` 7

#### Scenario: Update of a missing row
- **WHEN** an `update` runs with a key that matches no row
- **THEN** the response is 404 and nothing is written

#### Scenario: Last save wins
- **WHEN** two users load row 7, both edit `email`, and both save
- **THEN** row 7 holds the email from the save that ran last

### Requirement: Action logging
Every action run SHALL be logged with the action reference, the user, the table, the mode
or result kind, the number of rows, the duration and any error.

#### Scenario: Failed run is logged
- **WHEN** a `save` fails because the database rejects the value
- **THEN** a log entry records the action reference, user, table and the error, and the response is an error, not success
