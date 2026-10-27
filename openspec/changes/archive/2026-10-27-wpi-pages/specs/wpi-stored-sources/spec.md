## ADDED Requirements

### Requirement: Stored statements and files
The system SHALL keep named SQL statements in `kb.stored_stmts` (`stmt_name`, `sql_stmt`,
`description`, `creator`, `access_controls`) and named JSON files in `kb.stored_files`
(`file_name`, `file_content`, `description`, `creator`, `access_controls`). Names SHALL
match `^[a-z0-9][a-z0-9-]{0,127}$`; `file_content` SHALL be a JSON object. The WPI API SHALL
offer no way to write either table, so SQL text and file contents never travel as request
text.

#### Scenario: Descriptor names a statement, not SQL
- **WHEN** a page's `query` instance is `{"action": "query", "sql_stmt_id": "wpi-demo-customer-directory", "result": "list"}`
- **THEN** the bundle sent to the browser contains the statement's name and no SQL text

### Requirement: Access controls
A row's `access_controls` SHALL list the user roles that may use it. `NULL`, an empty list,
or a list containing `all` SHALL let every logged-in user use it. Otherwise the user SHALL
have one of the listed roles; an admin (admin flag, owner, or `admin` role) SHALL count as
having the role `admin`. A user without access SHALL get 403 and nothing SHALL run.

#### Scenario: Open to everyone
- **WHEN** a statement has `access_controls` `{all}` and any logged-in user runs a `query` naming it
- **THEN** the statement runs

#### Scenario: Role required
- **WHEN** a statement has `access_controls` `{finance}` and a user whose roles are `{sales}` runs it
- **THEN** the response is 403 and the statement does not run

### Requirement: Query action
A `query` instance SHALL set `sql_stmt_id` and `result` (`record` or `list`) and MAY set
`params`, an array of values that may use `$args.<name>`. The server SHALL run the named
statement in a read-only transaction with a statement timeout, binding `params` in order to
`$1…$n` as query parameters, and SHALL return each row as an object keyed by the statement's
column names: `{"result": [rows]}` for `list` (at most 1000 rows), or `{"result": row}` for
`record` (the first row; 404 when there is none). `query` SHALL be allowed as a page's or an
`open-page`'s `input` and as a list's `source`.

#### Scenario: List source
- **WHEN** a list's `source` is a `query` naming `wpi-demo-customer-directory` with `"result": "list"`
- **THEN** the list shows the rows the statement returns

#### Scenario: Parameter from a row click
- **WHEN** a row click runs `open-page` with `"args": {"id": "$record.id"}` and an `input` `query` with `"params": ["$args.id"]` on customer 7
- **THEN** the statement runs with `$1` = 7 and the page opens with that customer as its input doc

#### Scenario: Statement that writes
- **WHEN** a stored statement tries to insert or update rows
- **THEN** the database refuses it, nothing changes, and the response is an error

#### Scenario: Unregistered statement
- **WHEN** a `query` names a statement that is not in `kb.stored_stmts`
- **THEN** the app bundle fails with an error at the instance's `sql_stmt_id`, and running it returns 400

### Requirement: Load-file action
A `load-file` instance SHALL set `file_id`. The server SHALL return the named file's content
as `{"result": file_content}`. `load-file` SHALL be allowed as a page's or an `open-page`'s
`input`.

#### Scenario: About page
- **WHEN** page `customer-admin-about` has `"input": {"action": "load-file", "file_id": "wpi-demo-about"}`
- **THEN** the page opens with the stored file's content as its input doc

#### Scenario: Unregistered file
- **WHEN** a `load-file` names a file that is not in `kb.stored_files`
- **THEN** the app bundle fails with an error at the instance's `file_id`, and running it returns 400

#### Scenario: Not a list source
- **WHEN** a list's `source` is a `load-file` instance
- **THEN** writing the page fails with an error naming the allowed classes
