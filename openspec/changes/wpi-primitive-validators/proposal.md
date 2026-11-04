## Why

WPI pages that collect values (forms with an age, an amount, a code) can check them only
with the `input` element's few built-in rules. The validator requirements
(KnowledgeStore `doc-repo/validators/validator-requirements.md`) define validators as a
special kind of WPI element: classes that are configured per use, run in the browser and
again in the backend, and report standard errors. This change builds the primitive
validators of its §3.1 (`string`, `integer`, `decimal`, `number`, `boolean`, `enum`, `null`,
`binary` and `bigint`) and makes them usable end to end.

## What Changes

- New descriptor kind **`validator`** in `kb.wpi_descriptors`, added by a migration. Nine
  active validator classes are seeded, one per §3.1 validator, each naming its code module
  in `code_module_id` (user decision, requirements §4 Q2). `kb.code_modules` gets module
  type `validator` and nine rows.
- A validator class row has its own JSON shape, format `0.1`. It holds the category, the
  value types it accepts, how it treats empty values, its constraints (name, type,
  default, whether an instance may change it, help tip), its error codes with their params,
  `execution` (`sync`) and `runs_in` (`browser_and_backend`).
- Constraints are implemented once in code and shared by the validators that accept them.
  They go well beyond the requirements' "Common constraints", for example trimming, Unicode
  normalization, length units, patterns, prefix/suffix, exclusive bounds, sign,
  `multiple_of`, integer representations (int8 … uint64, int128), rounding policy, string
  and number coercion, case- and type-insensitive enum matching, and binary encodings.
  Names are snake_case forms of JSON Schema keywords (`min_length`, `exclusive_maximum`,
  `multiple_of`), as the user decided (§4 Q4).
- Two equivalent implementations, Go (`server/api/wpi/validators`) and TypeScript
  (`web/src/lib/wpi/validators`), both pass one shared file of test cases.
- **Element use.** `input`, `select` and value-editor elements accept a `validators`
  attribute: a list of validator instances, each holding a whole copy of its class, its own
  constraint values and optional localized messages. All validators must pass (`allOf`).
  - They run in the browser when editing ends: when an input loses focus, a pulldown value
    is chosen, or a value editor applies a value. They also run before a save.
  - The backend runs them again on every save and refuses failing values with 422.
  - Errors show under the element, made from the error code and params in English and
    Chinese, unless the instance gives its own message.
- **WPI Editor.**
  - The palette gets a **VALIDATORS** folder after VALUE EDITORS. It is listed in two
    levels: category (PRIMITIVE), then the validators.
  - Dragging a validator onto an element on the canvas adds an instance to that element
    (user decision, §4 Q1).
  - The element's Properties panel lists its validators. An author can add, remove and
    reorder them, and edit each constraint with the value editor its type calls for. The
    panel is drawn from data: no Svelte component per validator.
- New endpoint `GET /api/v1/wpi/classes/validator` lists the active validator classes.
- What is built keeps working unchanged. The input element keeps `required`, `max_length`,
  `validation` and its `input_type` formats. Pages without validators are unaffected.

## Capabilities

### New Capabilities
- `wpi-validators`: validator classes and instances, the shared constraints, the nine
  primitive validators, their results and error codes, element use in the browser and the
  backend, and validator editing in the WPI Editor (palette and Properties panel).

### Modified Capabilities
- `wpi-descriptor-store`: `kb.wpi_descriptors` stores a fifth class kind, `validator`, with
  its own format and validation rules, and page saves fill validator class copies.

## Impact

- **Backend:** new package `server/api/wpi/validators`. Changes in `server/api/wpi`:
  `types.go` (kind and format), `validate.go` (class rows, `validators` on elements),
  `copies.go` (validator copies), `bundle.go` and `store.go` (validator modules),
  `input_rules.go` and `actions.go` (backend runs and the 422 response), `handler.go` (list
  endpoint). One route is added in `server/api/routes.go`.
- **Frontend:** new `web/src/lib/wpi/validators/`. Changes in:
  - `wpi/properties/` (validator groups and source);
  - a new `validator-list` value editor;
  - `InputElement`, `SelectElement`, `ValueEditorElement`, `runtime.svelte.ts`, `api.ts`
    and `types.ts`;
  - `canvas/NodePalette.svelte` (second folder level), `canvas/element-palette.ts`,
    `canvas/PagePreview.svelte` (drop target) and `wpi-editor.svelte`;
  - messages `wpi_vld_*` and `nodepalette_*` in `en.json` and `zh-cn.json`.
- **Database:** one goose migration. It widens two check constraints and inserts 9
  `kb.code_modules` rows and 9 `kb.wpi_descriptors` rows.
- **Docs:**
  - new code-module document `KnowledgeStore/doc-repo/code_modules/validator-primitive.md`;
  - updates to the validator requirements (open questions answered), devdoc 2026102601
    (§3.5, §6, §7), devdoc 2026102701 (§1.2, §1.3.2, a new section for validators), and the
    input, select and value-editor code-module documents.
