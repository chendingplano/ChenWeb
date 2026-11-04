## 1. Validator specification and shared cases

- [x] 1.1 Write `server/api/wpi/validators/modules.json`: for the nine modules, category, value types, empty handling, constraint specs in order (name, type, choices, code default) and error codes with params (design D4, spec "Shared constraints")
- [x] 1.2 Write `server/api/wpi/validators/validator-cases.json`: cases for every module and constraint, including empty handling, coercion, exact decimals, rounding, representations, patterns and normalized values

## 2. Go validators package

- [x] 2.1 `server/api/wpi/validators`: embed `modules.json`; exact decimal type (D5); whitespace, line-break and length helpers (D12); portable pattern check and matching (D11)
- [x] 2.2 Implement the nine modules, the shared constraints and `Run(module, constraints, value, path)` returning `{valid, errors, value}`
- [x] 2.3 Implement `CheckConstraints` for constraint values and cross-constraint rules, used by descriptor validation
- [x] 2.4 Go tests: every shared case passes; constraint-check tests

## 3. Backend: descriptors, pages and saves

- [x] 3.1 `types.go`: `KindValidator`, `ValidatorSchemaVersion` "0.1", `validKind`, `schemaVersionOf`
- [x] 3.2 `validate.go`: validator class rows (D2) agree with the code; `validators` on input, select and value-editor instances (D3) checked against their copies, other modules rejected; allow-incomplete for null-required constraints
- [x] 3.3 `copies.go`: validator instances get class copies on page PUT; `bundle.go`/`store.go`: validator module refs checked against `kb.code_modules` type `validator`; `handler.go` `checkModule` for validator writes
- [x] 3.4 `GET /api/v1/wpi/classes/validator` (handler + route) listing the active validator classes, whole rows
- [x] 3.5 `input_rules.go`/`actions.go`: run validators on save, 422 with `validation_errors`, log failures (D7, D8)
- [x] 3.6 Backend tests: class rows, instances, copies, save with validators, seed rows match `modules.json`

## 4. Database

- [x] 4.1 Migration `20261104000002_wpi_primitive_validators.sql`: widen both checks, insert nine `kb.code_modules` rows and nine validator classes with English and Chinese help tips; Down restores
- [x] 4.2 Dry-run in a rolled-back transaction, then apply with `doc-service --migrate-only`; check the list endpoint

## 5. TypeScript validators

- [x] 5.1 `web/src/lib/wpi/validators/`: decimal, text helpers, constraints, nine modules, `runValidators` (allOf, error index), reading `modules.json`
- [x] 5.2 Error text from codes and params (`wpi_vld_e_*`), instance `messages` with `{param}`; constraint and choice labels (`wpi_vld_c_*`)
- [x] 5.3 Bun tests: every shared case passes; text and messages tests

## 6. Elements and runtime

- [x] 6.1 `types.ts`/`api.ts`/`runtime.svelte.ts`: validator instance types, `listValidatorClasses`, `validation_errors` in `WpiError`, per-page validation errors cleared on write
- [x] 6.2 `InputElement`, `SelectElement`, `ValueEditorElement`: run validators when editing ends and before a save; show client and server errors (D8)

## 7. WPI Editor

- [x] 7.1 `NodeType.subcategory`; `NodePalette` second folder level; VALIDATORS folder with category sub-folders (D9)
- [x] 7.2 `PagePreview` `data-editor-element`; drop a validator onto an element (pure helper + tests); message elsewhere
- [x] 7.3 Properties: `validator-list` value editor; `withValidatorGroups` and the validator source; Validators group in the input, select and value-editor definitions; constraint editors by type (D10)
- [x] 7.4 Bun tests for palette grouping, drop helper, validator groups and source; `bun run check` passes

## 8. Docs and verification

- [x] 8.1 KnowledgeStore: `code_modules/validator-primitive.md`; validator requirements (status, answered questions); devdoc 2026102601 (§3.5, §6, §7) and 2026102701 (§1.2, §1.3.2, new §1.3.11); input/select/value-editor code-module docs (`validators`)
- [x] 8.2 Run Go tests (`server/api/wpi/...`), `bun test` for the touched suites and `bun run check`
- [x] 8.3 Browser check in the editor: the palette folder, a drop onto an input, configuring a constraint, the preview showing the error
- [x] 8.4 Commit ChenWeb and KnowledgeStore with explicit paths
