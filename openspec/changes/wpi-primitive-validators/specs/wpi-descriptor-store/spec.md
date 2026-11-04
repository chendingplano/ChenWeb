## ADDED Requirements

### Requirement: Validator class rows
`kb.wpi_descriptors` SHALL accept kind `validator`. A validator class row SHALL have format
`schema_version` `"0.1"` and SHALL name, in `code_module_id`, a validator module that has
code in this interpreter. When the row is stored, that module SHALL be an active
`validator` module in `kb.code_modules`.

A row SHALL hold `category`, one of `primitive`, `text`, `network`, `datetime`,
`collection`, `identifier`, `file`, `geographic`, `numeric`, `security` and `relational`
(requirements §3.1–§3.11). It SHALL also hold a description and a `constraints` object, and
it MAY hold `implement_notes`, `value_types`, `empty_values`, `empty_constraint`,
`execution`, `runs_in` and `error_codes`. Any other attribute SHALL be reported as an issue.

`PUT /api/v1/wpi/descriptors/validator/:id` SHALL store a new active version, as for the
other class kinds. Page saves SHALL fill missing validator class copies. The app bundle
SHALL refuse a page whose validator copies name a code module that is not an active
`validator` module.

#### Scenario: Validator kind is accepted
- **WHEN** a row of kind `validator` is inserted into `kb.wpi_descriptors`
- **THEN** the database accepts it

#### Scenario: Unsupported validator format
- **WHEN** an admin stores a validator class with `schema_version` `"0.4"`
- **THEN** the write is rejected with an issue saying this interpreter supports format `0.1` for validators

#### Scenario: Suspended validator module
- **WHEN** code module `integer` is suspended and an app's page uses an `integer` validator
- **THEN** the bundle request returns 422 naming the copy's `code_module_id`
