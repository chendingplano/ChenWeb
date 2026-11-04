## Context

The requirements are in KnowledgeStore `doc-repo/validators/validator-requirements.md`. This
change builds its §3.1 primitive validators: `string`, `integer`, `decimal`, `number`,
`boolean`, `enum`, `null`, `binary` and `bigint`.

What exists today:
- Classes are rows in `kb.wpi_descriptors`, with kinds `app`, `element`, `action`,
  `page-layouter` and `element-layouter`. Each kind has its own format. Element rows are
  format `0.4`, and their instance attributes are objects (`help_tip`, `default_value`,
  `instance_modifiable`).
- An instance holds a whole copy of its class in `class`. The server fills missing copies on
  a page PUT (`copies.go`) and checks each instance against its copy (`againstCopy`).
- Element classes name their code in `code_module_id`. That code must be an active row of
  `kb.code_modules` of the right `module_type`, and code registered in both `codeModules`
  maps (Go `validate.go`, TS `wpi/elements/index.ts`).
- The `input` element has its own rules: `required`, `max_length`, `validation` and
  `input_type`. They are checked in the browser (`input-validation.ts`, on blur and before a
  save) and in the backend (`checkInputValue`, on every `save` action). Failures return 422
  with `field_errors`: attribute → codes.
- The Properties panel is one renderer (`PropertyEditor.svelte`) drawing an `EditorDef`
  (groups of properties) through a `PropertySource`.
- The palette (`NodePalette.svelte`) has one level of folders. The WPI folders are Pages,
  Elements and Value editors. Element classes are dropped onto a leaf pane of the canvas
  page (`data-editor-pane`).
- A shared JSON file of test cases already serves both languages for text attributes
  (`server/api/wpi/text-attribute-cases.json`).

The user answered four of the requirements' open questions (§4) on 2026-11-04:
- Q1: drop a validator onto the element that uses it.
- Q2: the class names its code in `code_module_id`, with module type `validator`.
- Q4: constraint names are snake_case forms of JSON Schema keywords.
- Scope: full use by elements, in the browser and the backend.

## Goals / Non-Goals

**Goals:**
- Nine active validator classes, each configurable through many constraints.
- One Go and one TypeScript implementation that agree, proven by a shared file of cases.
- `input`, `select` and value-editor elements can use any number of validators. The
  validators run when editing ends, before a save, and again in the backend.
- Authors find validators in the palette under VALIDATORS → PRIMITIVE, drop them onto
  elements, and configure them in the element's Properties panel.
- Errors use the standard shape (code, JSON Pointer path, params), in English and Chinese.

**Non-Goals:**
- The other categories (§3.2–§3.11), asynchronous validators, and backend-only validators.
  The class format can describe them (`execution`, `runs_in`), but none is built.
- `anyOf`, `oneOf` and `not` composition. Only `allOf` is built (§2.6.4).
- Writing normalized values back (§4 Q5), warnings (§4 Q7), and context such as locale or
  timezone (§4 Q6). Results report the normalized value, but nothing uses it yet.
- A Class tab for validator classes. Admins change a class with the existing
  `PUT /api/v1/wpi/descriptors/validator/:id`.
- Changing the `input` element's own rules.

## Decisions

### D1 Storage: kind `validator`, format `0.1`, module type `validator`
- **Kind.** A migration widens the `kind` check of `kb.wpi_descriptors` to include
  `validator`.
- **Format.** Validator rows have their own format, `ValidatorSchemaVersion = "0.1"`. This
  follows the per-kind formats that `schemaVersionOf` already handles.
- **Code.** A validator class names its code in `code_module_id` (user decision, §4 Q2). The
  code must be an active `kb.code_modules` row of the new module type `validator`, and code
  registered in the validator registries of both languages.
- **Seed rows.** The migration seeds nine module rows and nine class rows, one per §3.1
  validator. Class ID, module ID and validator type are the same name. The names fit
  `idPattern`, because none contains `_`.
- **Why `code_module_id`:** several classes can share one code, such as a future `age`
  class on the `integer` code with fixed limits (§4 Q2).
- *Alternative:* name the code by the class ID, as action classes do. Rejected by the user.

### D2 Class row shape
```json
{
  "descriptor_id": "integer", "schema_version": "0.1", "code_module_id": "integer",
  "category": "primitive",
  "description": {"en": "…", "zh-cn": "…"}, "implement_notes": {"en": "…"},
  "value_types": ["number", "string"],
  "empty_values": ["absent", "null", "empty_string"], "empty_constraint": "empty",
  "execution": "sync", "runs_in": "browser_and_backend",
  "error_codes": {"integer.maximum": {"params": ["maximum"]}, "…": {}},
  "constraints": {
    "maximum": {"type": "integer", "help_tip": {"en": "…", "zh-cn": "…"}},
    "empty": {"type": "choice", "choices": ["pass", "fail"], "default_value": "pass"}
  }
}
```
- **Constraint objects.** Each is shaped like an element attribute: `help_tip`,
  `default_value` (`null`: every instance must set it; absent: optional with no default)
  and `instance_modifiable`. It also restates `type` (and `choices` for choice types).
  `value_editor` may override the editor the Properties panel uses.
- **Facts stated for the reader.** `value_types`, `empty_values`, `empty_constraint`,
  `execution`, `runs_in`, constraint `type`/`choices` and `error_codes` describe the code.
  They are there so the row is self-describing (§2.3). Validation checks that they agree
  with the code, so they cannot drift.
- **What a class may choose.**
  - `category` can be any of the eleven categories (§3.1–§3.11). It is free because a class
    may sit in another category than its code, such as `age` (date/time) on `integer`
    (§2.3).
  - A class may list only some of its code's constraints. Instances may set only the ones
    listed.
  - A constraint a class does not list keeps the code default.
- **Constraints, error codes and order.** Constraints sit in a `constraints` object, apart
  from the class attributes, so a constraint name never collides with `description`,
  `category` and the like. Property order comes from the code, because `jsonb` does not
  keep key order.

### D3 Instance shape inside the element
```json
"validators": [
  {"validator": "integer", "class": {…copy…},
   "constraints": {"minimum": 0, "maximum": 150},
   "messages": {"integer.maximum": {"en": "Age is at most {maximum}.", "zh-cn": "年龄不超过 {maximum}。"}}}
]
```
- **Effective constraints.** In increasing priority: code defaults, then the copy's
  `default_value`s, then the instance's `constraints`. The instance may set only
  constraints the copy lists and does not fix.
- **Messages.** `messages` replaces the built-in text of the codes it names. The text may use
  `{param}` placeholders.
- **The list.** `validators` is the element's validator list, combined as `allOf`.
- **No class change needed.** `validators` is allowed on every instance of `input`, `select`
  and `value-editor`, like the size attributes, even when the element's class copy does not
  name it. So existing classes, copies and pages need no change (§1: "an add-on"). A class
  row may still name `validators` with a default list. Its items then hold their own copies.
- *Alternative:* add a `validators` attribute to every input, select and value-editor class
  row. That needs a new version of 31 classes, and old copies could never use validators.

### D4 Two implementations, one specification file, one cases file
- **Where the code lives.**
  - Go: package `server/api/wpi/validators`. It is pure (no database) and is used by
    `validate.go` and the save path.
  - TypeScript: `web/src/lib/wpi/validators/`.
- **One specification file.** `server/api/wpi/validators/modules.json` is the single
  definition of what each module accepts: category, value types, empty handling, constraint
  specs in order (name, type, choices, code default) and error codes with params. Go embeds
  it; TypeScript imports it, as `text-values.json` already is.
- **Duplicated logic.** Only the checking logic exists twice. Both sides must give the same
  result for each case in the cases file `server/api/wpi/validators/validator-cases.json`:
  - the error codes in order;
  - their params;
  - the normalized value.
- **Seed rows checked too.** A Go test checks the seeded class rows against `modules.json`.
- *Alternative:* keep the specifications in Go and generate TypeScript from them. That needs
  a generator in the build; JSON both sides can read is simpler.

### D5 Exact numbers, the same way on both sides
- **One decimal type.** `integer`, `number`, `decimal` and `bigint` compare, round and test
  `multiple_of` on one exact decimal type: a big-integer coefficient and an exponent
  (`math/big.Int` in Go, `BigInt` in TypeScript).
- **Numbers.** A JSON number becomes a decimal through its shortest round-trip form (Go
  `strconv.FormatFloat(f, 'g', -1, 64)`, JavaScript `String(n)`). This is the same float64
  that JSON parsing gives in the browser. So `0.1` compares as `0.1`, and `multiple_of: 0.1`
  accepts `0.3`.
- **Number strings.** `number` parses a string to float64 first (`strconv.ParseFloat`,
  `Number()`), after a syntax check that both sides share.
- **Decimal and bigint strings.** `decimal` and `bigint` read strings exactly, so
  `"0.1000000000000000000001"` keeps its digits.
- **Size limits.** Strings longer than 1 000 characters or with an exponent beyond ±1 000
  are not numbers. This keeps alignment of exponents cheap.
- **Integer limits.** `integer` accepts only safe integers (±(2⁵³−1)). Larger values belong
  to `bigint`, because the browser cannot hold them exactly.
- *Alternative:* plain float64 arithmetic with a tolerance, as the input's `step` does today.
  It disagrees between languages near the tolerance and cannot express `decimal` scale.

### D6 Empty values are explicit
- **One constraint.** Every validator except `null` has the constraint `empty`, with
  choices from:
  - `pass`: accepted with no other check;
  - `fail`: rejected with `<type>.empty`;
  - `check`: read as the type's empty value and checked like any other.
- **What counts as empty.** An empty value is absent, `null` or `""` (§2.6.5).
- **Defaults per validator.**
  - `string`: choices pass, fail and check; default `check`. The empty string is "a string
    like any other" (§2.6.5), so `min_length: 1` rejects it.
  - `boolean`: choices pass, fail and check; default `pass`. `check` reads an empty value as
    `false`, so `must_be: true` rejects it, as for an unticked box.
  - `enum`: choices pass, fail and check; default `pass`. `check` compares the value with the
    allowed values.
  - The others: choices pass and fail; default `pass`.
- **`null`** checks null-ness itself, configured by `absent_is_null` and
  `empty_string_is_null`.
- **Why one choice and not a "non-empty" boolean** (the requirements' example): a boolean
  cannot express `check`, and `check` is the JSON Schema reading of `""` that §2.6.5 asks
  for strings. With all three, each validator states its handling, and an author can choose.
  `fail` is the "non-empty" flag.

### D7 Errors and their text
- **Results.** A result is `{valid, errors, value}`. Each error is
  `{code, path, params, validator}`:
  - `path` is the JSON Pointer of the bound value (`$input_doc.address.city` →
    `/address/city`);
  - `validator` is the index of the validator in the element's list, so the browser can find
    the instance's `messages`.
- **Order.** Errors come in a fixed order, the same in both languages. A type error stops the
  checks. Otherwise every failing constraint is reported.
- **Params.** Params hold the constraint values involved. For counts (lengths, bytes,
  digits, scale) they also hold `actual`.
- **Text.** The browser makes the text from Paraglide messages `wpi_vld_e_*`. It looks up the
  full code first (`integer.type`), then the shared part after the dot (`maximum`). Some
  messages pick their wording from a param, such as the unit for lengths or the chosen
  `sign`.
- **Backend 422.** The backend returns
  `{"field_errors": {…}, "validation_errors": {"age": [error, …]}}`. `field_errors` is
  unchanged, so existing clients keep working.

### D8 When elements run validators
The browser runs validators at these points:
- `input`: on blur, then on every keystroke once touched (as its rules do now), and before
  a save.
- `select`: after a value is chosen, and before a save.
- `value-editor`: after the editor applies a value, and before a save.

Rules that apply in all three:
- `select` and `value-editor` now register with the runtime's pre-save check, as `input`
  already does.
- A `read-only` binding is not checked, as for the input's rules.
- The backend runs the same validators for every element of those modules on each `save`
  action, after the input rules. Failures are logged with their codes (§2.6.8).

### D9 Editor: palette, drop and Properties panel
- **Two-level palette.** `NodeType` gets an optional `subcategory`. `NodePalette` shows the
  items of a folder in sub-folders when they have one, with their own fold state, and opens
  them while searching. The validator entries are:
  - ID `wpi-validator:<class>`;
  - folder `Validators`, after `Value editors`;
  - sub-folder: the class's `category`.

  Category labels are messages for all eleven categories, in §3 order.
- **Drop target.** `PagePreview` marks each element box with `data-editor-element`. The drop
  handler finds the element under the pointer. If its module accepts validators, the handler
  appends `{validator, class: <the listed row>}` to the node's `attrs.validators`, selects
  the node and records one undo step. Elsewhere it shows a message. The copy is taken at
  drop time, from the class list the palette already has. The server still fills missing
  copies for pages written by hand.
- **Properties panel.** It is still drawn from definitions:
  - **List group.** A "Validators" group holds one property, `validators`, drawn by a new
    value editor, `validator-list`. It lists the attached validators (move up, move down,
    remove) and has an "Add validator" pulldown of the active classes.
  - **Per-validator groups.** `withValidatorGroups(def, attrs)` adds one group per
    instance, titled "Validator *n*: *class*", with the class description as its help.
    - Each constraint the copy lists is a property, `validators.<n>.<constraint>`. Its value
      editor comes from the constraint type (D10), or from the copy's `value_editor`.
    - Each group ends with `validators.<n>.messages`, drawn as JSON.
  - **Source wrapper.** A `PropertySource` wrapper serves the `validators.<n>.*` names. A
    help tip, a default, a required flag or a fixed flag comes from the copy. Reset removes
    the instance's own value.
- **What comes from where.**
  - Constraint labels and choice labels are Paraglide messages keyed by constraint name.
    The constraints are code, shared by all validators.
  - Help tips come from the class row, like an element attribute's.
- *Alternative:* one Svelte "validator editor" that draws constraints itself. It would
  duplicate the renderer's help tips, Reset, invalid-value handling and per-focus undo, and
  the requirements say a validator needs no component of its own (§2.3).

### D10 Constraint types and their value editors

| Type | Value | Value editor |
|---|---|---|
| `boolean` | `true`/`false` | switch |
| `integer`, `non_negative_integer`, `positive_integer` | JSON integer | number (whole; min 0 or 1) |
| `number`, `positive_number` | JSON number | number (positive checked on apply) |
| `decimal`, `positive_decimal`, `bigint`, `positive_bigint` | decimal or integer text, or a JSON number | text, checked on apply |
| `string` | string | text |
| `pattern` | string, a regular expression in the common RE2/ECMAScript subset | text, checked on apply (compiled with the `u` flag) |
| `choice` | one of `choices` | value-picker |
| `json_array` | array of JSON values | json |

### D11 Regular expressions
- **Matching.** A pattern matches the whole value: Go `\A(?:p)\z`, ECMAScript `^(?:p)$`
  with the `u` flag.
- **On save,** a pattern must compile in Go. It must also avoid the constructs the input's
  rule already rejects: inline flags and groups `(?…`, and `\A \z \Q \E \p \P`.
- **In the Properties panel,** a pattern must also compile in the browser.
- **Length limit.** Values longer than 10 000 code points are not matched. They fail with
  `string.pattern_limit` on both sides, so the browser cannot hang on a long value (§2.6.6).

### D12 Whitespace and length are defined once
- **Whitespace.** "Whitespace" (trim, `non_blank`, number parsing) is one fixed set in both
  languages:
  - the Unicode White_Space characters;
  - U+FEFF.

  `strings.TrimSpace` and `String.prototype.trim` differ, so neither is used.
- **Line breaks** are `\n`, `\r`, U+2028 and U+2029.
- **Length units.** Length is counted in code points by default (§2.6.7). `length_unit` may
  choose UTF-16 code units or UTF-8 bytes instead. Grapheme clusters are left out, because
  the Go library and `Intl.Segmenter` may follow different Unicode versions.

## Risks / Trade-offs

- [Two implementations can drift] → `modules.json` is shared. Every module has cases in the
  shared file, and both suites run them. A Go test ties the seeded rows to `modules.json`.
- [Class copies make pages larger: each validator instance carries its row, about 4–8 KB
  with help tips] → It is the same model as element copies. The bundle may later drop help
  tips (devdoc 2026102601 §3.5.1).
- [`enum` with `case_sensitive: false` lowercases with `toLowerCase`/`strings.ToLower`,
  which differ for a few characters, such as U+0130 İ] → Documented. The cases cover ASCII,
  Greek and Cyrillic.
- [Unicode normalization tables may differ between Go's `x/text` and the browser's ICU
  version for newly added characters] → Accepted. The backend's result counts.
- [ECMAScript patterns backtrack; a nasty pattern can still be slow under the length
  limit] → The limit bounds it. Authors are admins.
- [Another session may edit the same WPI files] → Commit with explicit paths and hunks only
  (`feedback-jj-commit-explicit-paths`).

## Migration Plan

- One goose migration, `20261104000002_wpi_primitive_validators.sql`:
  - It widens `wpi_descriptors_kind_check` and `code_modules_module_type_check`.
  - It inserts the nine modules and nine classes, guarded so it can run twice.
  - Down deletes the rows and restores both checks.
- Dry-run the Up section in a rolled-back transaction, then apply it with `doc-service
  --migrate-only`.
- Rollback is the Down section and reverting the commits. No existing row changes.

## Open Questions

- Requirements §4 Q3 (types versus constraints), Q5 (normalized values), Q6 (other
  context) and Q7 (severity) are left open, as listed under Non-Goals. This change follows
  the proposed rule of Q3: a type says what a value is, a constraint narrows it. So
  `pattern`, `length`, `range` and the like are constraints here.
