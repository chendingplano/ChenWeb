# wpi-data-actions Specification

## Purpose
Server-side actions for WPI apps (`load`, `save`, and the `query` and `load-file` of
capability `wpi-stored-sources`): only action instances written in stored pages run, the
browser supplies argument values but never table or column names, values go to the database
as parameters, and saves are validated on the server with per-attribute errors.
## Requirements
### Requirement: Only stored actions run
`POST /api/v1/wpi/actions/run` SHALL run only `load`, `save`, `query` and `load-file` action
instances written in stored pages, named by `ref` as
`{"kind": "page", "id": page ID, "pointer": JSON Pointer}`. The system SHALL load the
instance's action class, check the instance against it and run the merged instance. The
request SHALL NOT be able to supply a table name, column name, statement, file or action
body. A logged-in user SHALL be required.

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

#### Scenario: Action that does not run on the server
- **WHEN** the pointer names an `open-page` instance
- **THEN** the response is 400

#### Scenario: Static input is not an action
- **WHEN** the pointer names a page's `input` that is static data
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

### Requirement: Any existing table, checked and quoted
`load` and `save` SHALL support `name` or `schema.name` in the project database, and
`database.schema.name` in a database listed in `[wpi].databases_to_show`. Before running,
the system SHALL confirm that the table and every named column exist in the selected
database's catalog, and SHALL quote schema, table and column identifiers. A database
outside the configured list SHALL be rejected. The editor SHALL display the configured
databases, then their schemas, then their tables in a cascading picker. Under the table
picker, the editor SHALL offer a field picker that lists the selected table's columns in
catalog order. Selecting a column SHALL add a mapping with the column name as its default
attribute; users SHALL be able to deselect columns and edit column and attribute names.
The field dialog SHALL be movable and resizable using a pointer and keyboard. Existing mappings
SHALL be preserved until the user changes them.

#### Scenario: Select a configured database table
- **WHEN** an author chooses table `people` in schema `public` of configured database `shared`
- **THEN** the picker stores `shared.public.people` and the action runs against `shared`

#### Scenario: List preview on a page without an ID
- **WHEN** a configured list is previewed on a page without a valid stored page ID
- **THEN** the editor prompts the user to load a saved page and does not send an action request

#### Scenario: Unconfigured database
- **WHEN** an action names a database outside `[wpi].databases_to_show`
- **THEN** the action is rejected and no query runs

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

