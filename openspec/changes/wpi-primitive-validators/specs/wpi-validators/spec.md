## ADDED Requirements

### Requirement: Primitive validator classes
The system SHALL store nine active validator classes in `kb.wpi_descriptors` with kind
`validator`: `string`, `integer`, `decimal`, `number`, `boolean`, `enum`, `null`, `binary`
and `bigint`. Each class SHALL:
- name the code module of the same ID in `code_module_id`;
- have category `primitive`;
- have a description in English and Chinese;
- list every constraint its code accepts, each with a help tip in English and Chinese.

`kb.code_modules` SHALL list the nine modules with module type `validator`. The class row
SHALL also state these facts, and each SHALL agree with the code:
- `value_types`;
- `empty_values` and `empty_constraint`;
- `execution` (`sync`) and `runs_in` (`browser_and_backend`);
- `error_codes`, each with its `params`;
- each constraint's `type`, and `choices` for choice types.

#### Scenario: Validator classes are listed
- **WHEN** an author requests `GET /api/v1/wpi/classes/validator`
- **THEN** the response lists the nine classes, each with its whole row, and `integer` has `code_module_id` `integer`, category `primitive` and constraint `maximum` of type `integer`

#### Scenario: Class row disagrees with its code
- **WHEN** an admin stores a validator class whose `code_module_id` is `integer` and whose constraint `maximum` has type `string`
- **THEN** the write is rejected with an issue at `/constraints/maximum/type`

#### Scenario: Unknown constraint in a class
- **WHEN** an admin stores a validator class on code `integer` with a constraint `max_length`
- **THEN** the write is rejected with an issue at `/constraints/max_length`

### Requirement: Shared constraints
Constraints SHALL be implemented once per language and shared by the validators that
accept them. Each validator SHALL accept exactly the constraints below, with the given code
defaults ("—" means not applied until set). Names are snake_case JSON Schema names.

| Constraint | Type | Validators | Default | Meaning | Error code |
|---|---|---|---|---|---|
| `empty` | choice pass/fail/check | all but `null` (check: string, boolean, enum) | `check` for string, else `pass` | What an absent, `null` or `""` value does | `<v>.empty` |
| `trim` | choice none/both/start/end | string | none | Whitespace removed before the checks | — |
| `normalization` | choice none/nfc/nfd/nfkc/nfkd | string | none | Unicode normalization before the checks | — |
| `min_length`, `max_length` | non-negative integer | string | — | Length bounds | `string.min_length`, `string.max_length` |
| `length_unit` | choice code_points/utf16_units/utf8_bytes | string | code_points | What length counts | — |
| `non_blank` | boolean | string | false | Rejects text that is only whitespace | `string.blank` |
| `single_line` | boolean | string | false | Rejects line breaks | `string.line_break` |
| `pattern` | pattern | string | — | The whole value matches | `string.pattern`, `string.pattern_limit` |
| `prefix`, `suffix`, `contains` | string | string | — | Starts with, ends with, contains | `string.prefix`, `string.suffix`, `string.contains` |
| `accept_strings` | boolean | integer, number, boolean | true for integer and number, false for boolean | Strings are parsed | `<v>.type` |
| `accept_numbers` | boolean | decimal, bigint, boolean | true, true, false | JSON numbers are accepted | `<v>.type` |
| `minimum`, `maximum`, `exclusive_minimum`, `exclusive_maximum` | the validator's number type | integer, number, decimal, bigint | — | Bounds | `<v>.minimum` … |
| `sign` | choice any/positive/negative/non_negative/non_positive/non_zero | integer, number, decimal, bigint | any | Sign | `<v>.sign` |
| `multiple_of` | positive, the validator's number type | integer, number, decimal, bigint | — | Exactly divisible | `<v>.multiple_of` |
| `representation` | choice any/int8/int16/int32/uint8/uint16/uint32 (integer); unbounded/int64/uint64/int128/uint128 (bigint) | integer, bigint | any, unbounded | Fits a bounded integer type | `<v>.range` |
| `integer_only` | boolean | number | false | Must be whole | `number.integer` |
| `allow_non_finite` | boolean | number | false | Accepts `Infinity`, `-Infinity`, `NaN` strings | `number.finite` |
| `precision` | positive integer | decimal | — | Most digits in all; with `scale`, at most precision − scale before the point | `decimal.precision`, `decimal.integer_digits` |
| `scale` | non-negative integer | decimal | — | Most digits after the point | `decimal.scale` |
| `rounding` | choice reject/half_up/half_even/down/up/floor/ceiling | decimal | reject | With `scale`: reject or round extra digits | — |
| `min_digits`, `max_digits` | positive integer | bigint | — | Digit count | `bigint.min_digits`, `bigint.max_digits` |
| `allow_leading_zeros` | boolean | bigint | true | Accepts `007` | `bigint.leading_zeros` |
| `must_be` | choice either/true/false | boolean | either | Required value | `boolean.must_be` |
| `allowed_values` | JSON array | enum | null (must be set) | Allowed values | `enum.not_allowed` |
| `excluded_values` | JSON array | enum | — | Excluded values | `enum.excluded` |
| `case_sensitive` | boolean | enum | true | Strings compare with case | — |
| `type_sensitive` | boolean | enum | true | When false, `1` equals `"1"` and `true` equals `"true"` | — |
| `mode` | choice must_be_null/must_not_be_null | null | must_be_null | Null-ness | `null.must_be_null`, `null.must_not_be_null` |
| `absent_is_null`, `empty_string_is_null` | boolean | null | true, false | What counts as null | — |
| `encoding` | choice base64/base64url/hex | binary | base64 | Encoding of the bytes | `binary.encoding` |
| `padding` | choice optional/required/forbidden | binary | optional | Base64 `=` padding | `binary.padding` |
| `ignore_whitespace` | boolean | binary | false | Whitespace in the text is ignored | — |
| `hex_case` | choice any/lower/upper | binary | any | Case of hex digits | `binary.case` |
| `min_bytes`, `max_bytes` | non-negative integer | binary | — | Decoded size | `binary.min_bytes`, `binary.max_bytes` |

Besides these, each validator SHALL report `<v>.type` for a value of the wrong type.
`integer` SHALL also report `integer.unsafe` beyond ±(2⁵³−1), and `bigint` SHALL report
`bigint.unsafe_number` for a JSON number beyond that range.

#### Scenario: Shared bound
- **WHEN** an `integer` validator with `maximum` 150 checks 151, and a `decimal` validator with `maximum` "150" checks "150.5"
- **THEN** the first reports `integer.maximum` and the second `decimal.maximum`, each with param `maximum`

#### Scenario: Exact multiple of a decimal step
- **WHEN** a `number` validator with `multiple_of` 0.1 checks 0.3
- **THEN** the value is valid

#### Scenario: Decimal scale with rounding
- **WHEN** a `decimal` validator with `scale` 2 and `rounding` `half_even` checks `"2.345"`
- **THEN** the value is valid and its normalized value is `"2.34"`

#### Scenario: Empty string is a string
- **WHEN** a `string` validator with `min_length` 1 and the default `empty` checks `""`
- **THEN** it reports `string.min_length` with params `min_length` 1 and `actual` 0

#### Scenario: Optional field
- **WHEN** an `integer` validator with `minimum` 18 and the default `empty` checks an absent value
- **THEN** the value is valid

### Requirement: Validation results
Validating a value SHALL produce `{valid, errors, value}`:
- `valid` is true when there are no errors;
- `value` is the normalized value, which is reported but not written back;
- each error is `{code, path, params, validator}`. `path` is the JSON Pointer of the bound
  value, `params` names the constraint values involved (plus `actual` for counts), and
  `validator` is the validator's index in the element's list.

A type error SHALL end the checks. Otherwise every failing constraint SHALL be reported, in
a fixed order that is the same in both languages.

#### Scenario: Several failures
- **WHEN** a `string` validator with `min_length` 5 and `pattern` `[a-z]+` checks `"AB"` at `$input_doc.code`
- **THEN** it reports `string.min_length` then `string.pattern`, both with path `/code`

### Requirement: Browser and backend agree
Every validator SHALL run the same way in Go and TypeScript. Both test suites SHALL pass one
shared file of cases, `server/api/wpi/validators/validator-cases.json`. Each case gives:
- a validator and its constraints;
- a value;
- the expected error codes with params, in order;
- the normalized value, where one is expected.

The constraint specifications SHALL come from one file, `server/api/wpi/validators/modules.json`,
which both languages read.

#### Scenario: Shared cases
- **WHEN** the Go tests and the TypeScript tests run
- **THEN** both pass every case in the shared file

### Requirement: Validator instances on elements
`input`, `select` and `value-editor` element instances SHALL accept `validators`: an array
of validator instances. This holds even when the element's class copy does not name it.
Each validator instance is
`{"validator": <class>, "class": <copy>, "constraints"?: {...}, "messages"?: {...}}`.
- An instance's effective constraints are, in increasing priority, the code defaults, the
  copy's defaults and its own `constraints`.
- An instance SHALL set only constraints its copy lists and does not fix (`instance_modifiable: false`).
- A constraint whose default in the copy is `null` SHALL be set before the page runs. A
  page that lacks one MAY still be saved.
- `messages` keys SHALL be error codes of the copy, and its values text in one or two
  languages.
- Constraint values and cross-constraint rules SHALL be checked when the page is saved:
  minimum ≤ maximum, `min_length` ≤ `max_length`, patterns valid.
- Other element modules SHALL reject `validators`.
- A page save SHALL fill a missing copy from the latest active class.

#### Scenario: Validator on a text element
- **WHEN** a page's `text` element instance sets `validators`
- **THEN** the page is rejected with an issue at that instance's `validators`

#### Scenario: Fixed constraint
- **WHEN** a validator instance sets `maximum` and its class copy gives `maximum` with `instance_modifiable: false`
- **THEN** the page is rejected with an issue at that instance's `constraints/maximum`

#### Scenario: Inverted bounds
- **WHEN** a validator instance sets `minimum` 10 and `maximum` 5
- **THEN** the page is rejected with an issue at that instance's `constraints/maximum`

#### Scenario: Missing copy
- **WHEN** an admin saves a page whose input has `"validators": [{"validator": "integer"}]`
- **THEN** the stored page holds a copy of the active `integer` class in that instance's `class`

### Requirement: Validators run in the browser and the backend
In the browser, an element's validators SHALL run when editing ends:
- an `input` when it loses focus, and after that on every change;
- a `select` after a value is chosen;
- a `value-editor` after the editor applies a value.

They SHALL also run before a save, and a failure SHALL stop the save. All validators of an
element SHALL pass for its value to be valid (`allOf`). A `read-only` element SHALL not be
checked. Errors SHALL be shown under the element:
- an instance's own message for the code, if it has one, with `{param}` placeholders filled
  in;
- otherwise the built-in text for the code and params, in English or Chinese.

The backend SHALL run the validators of every `input`, `select` and `value-editor` element
of the page on each `save` action, after the `input` rules. Failing values SHALL be
refused with 422 and
`{"field_errors": {...}, "validation_errors": {<attribute>: [error, ...]}}`, and each
failure SHALL be logged.

#### Scenario: Age above the limit in the browser
- **WHEN** an input bound to `$input_doc.age` has an `integer` validator with `maximum` 150, and the user types `200` and leaves the field
- **THEN** the input shows "Must be at most 150." (or the Chinese text) and does not save

#### Scenario: Backend refuses a value the browser did not check
- **WHEN** a `save` action receives `{"age": 200}` for that page
- **THEN** the response is 422 with `validation_errors.age[0].code` `integer.maximum` and path `/age`

#### Scenario: Own message
- **WHEN** the validator instance has `"messages": {"integer.maximum": {"en": "Age is at most {maximum}."}}` and the value 200 fails
- **THEN** the input shows "Age is at most 150."

### Requirement: Validators in the WPI Editor
The palette SHALL have a **VALIDATORS** folder after VALUE EDITORS. It SHALL list the active
validator classes in sub-folders by category, in the order of requirements §3, starting
with PRIMITIVE. Search SHALL open the folders and sub-folders that have a match.

Dropping a validator onto an `input`, `select` or `value-editor` element on the canvas page
SHALL:
- append an instance with a copy of the class to that element's `validators`;
- select the element;
- be one undo step.

Dropping one anywhere else SHALL add nothing and show a message.

The element's Properties panel SHALL have a Validators group, which lists the element's
validators and can add, remove and reorder them. Each validator SHALL have a group of its
own:
- each constraint its copy lists, drawn with the value editor of the constraint's type;
- the constraint's help tip from the copy;
- Reset to the default;
- fixed constraints shown read-only, and required ones marked;
- a JSON property for its `messages`.

#### Scenario: Drop onto an input
- **WHEN** an author drags `integer` from VALIDATORS → PRIMITIVE onto an input on the canvas
- **THEN** the input's `validators` gains an `integer` instance with a class copy, and its Properties panel shows a group "Validator 1: integer" with its constraints

#### Scenario: Drop onto a button
- **WHEN** an author drops a validator onto a button
- **THEN** nothing is added and a message says validators go onto inputs, pulldown menus and value editors

#### Scenario: Edit a constraint
- **WHEN** an author sets `maximum` to 150 in that group
- **THEN** the instance's `constraints.maximum` becomes 150, and Reset removes it again
