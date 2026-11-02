## Context

The value editors are Svelte components in `web/src/lib/components/shared-ui/value-editors/`,
registered by ID in `shared-ui/property-editor/editors.ts` and used only by the Properties
panel. Devdoc 2026102701 §1.3.9 lists 28 of them. 15 have their own component. 5 are an
existing editor with fixed options: `class-definition.ts` already maps them to their base
editor (`aliases`). 3 are controls now used only inside the data-source editor:
`database-picker/TablePicker.svelte`, `FieldPicker.svelte` and
`value-editors/SortOrderEditor.svelte`. The last 5 are not built.
`class-editor.ts` already lists all 28 IDs (`VALUE_EDITOR_IDS`) and the 5 unbuilt ones
(`PENDING_EDITORS`).

Element classes are rows in `kb.wpi_descriptors`. Each row names the code module that draws
it (`code_module_id`). The frontend registers modules in `wpi/elements/index.ts`, and the
backend lists what each module accepts in `codeModules` (`server/api/wpi/validate.go`).
Several classes may share one module (devdoc 2026102601 §3.5.2). The palette's ELEMENTS
folder lists every draft and active element class from `GET /api/v1/wpi/classes/element`,
which returns `code_module_id`. A draft is listed but never run. A page that names one is
rejected (`wpi-element-class-authoring`).

## Goals / Non-Goals

**Goals:**
- An author can drag any built or variant value editor onto a page, bind it to an
  input-doc field, preview it and save the page.
- The palette shows them in an ELEMENTS → VALUE EDITORS sub-folder.
- The Properties panel and the Class tab work on them as on any element.

**Non-Goals:**
- Building the five unbuilt editors (date, time, button-type, database, schema pickers).
- Field rules such as `required` or input validation on value-editor elements.
- Changing how the Properties panel itself uses the value editors.
- Nested sub-folders anywhere except under ELEMENTS, and on the Flow page.

## Decisions

- **D1 One code module, many classes.** One new module, `value-editor`, draws every value
  editor. Each class fixes the editor it draws in a class-scope attribute `editor` and gives
  default options in `editor_options`. *Alternative:* one module per editor (28 modules).
  That would need 28 `kb.code_modules` rows, 28 backend entries and 28 Svelte files, all
  doing the same thing. Also, module ID `text` is already the text element. The
  shared-module design is what §3.5.2 of the WPI design allows for.

- **D2 Class IDs are the devdoc names.** The classes are called `text-editor`,
  `multiline-text-editor`, `localized-text-editor`, `number-editor`, `switch`, `value-picker`,
  `ordered-multi-picker`, `string-list`, `date-picker`, `time-picker`, `color-picker`,
  `length-editor`, `font-picker`, `alignment-picker`, `icon-picker`, `button-type-picker`,
  `database-picker`, `schema-picker`, `table-picker`, `field-picker`, `data-source-editor`,
  `column-list-editor`, `sort-order-editor`, `validation-editor`, `action-picker`,
  `layouter-picker`, `read-only-value` and `json-editor`. None clashes with an existing
  element class (`button`, `chat-composer`, `chat-panel`, `input`, `list`, `menu`, `select`,
  `text`). `editor` holds the ID used in definitions (`text`, `color`, `font-picker`, …).
  These are the 28 IDs in `VALUE_EDITOR_IDS`.

- **D3 Attributes.** The module requires `editor`, `label`, `data_binding` and
  `binding_mode`, like `input`, and accepts `editor_options` plus the shared size attributes.
  - `editor` is class scope and fixed.
  - `label` is localized text, drawn above the editor.
  - `data_binding` and `binding_mode` follow the Data binding requirement. An unbound editor
    keeps its value locally. `read-only` draws the `read-only` editor with the bound value.
  - `editor_options` is a JSON object passed to the editor as its options. It holds only
    JSON. Labels that are functions in the Properties panel (`choiceLabel`, `emptyLabel`)
    fall back to the editor's plain behaviour.
  - `editor_options` is instance scope: the class gives its defaults, and an instance may
    change them, for example `value-picker` choices or the `field-picker` table.
  - A value the editor unsets (`onChange(undefined)`) writes `null` to the binding.
  - Server field errors for the binding show under the editor, as for `input`.

- **D4 One resolver for editor IDs.** A small pure module, `wpi/properties/editor-ids.ts`,
  maps a catalogue ID to what draws it:
  - a built editor → its component in `valueEditors`;
  - `font-picker`, `alignment-picker`, `icon-picker`, `action-picker`, `layouter-picker` → the
    base editor (today's `aliases`);
  - `table-picker`, `field-picker`, `sort-order-editor` → thin adapters over `TablePicker`,
    `FieldPicker` and `SortOrderEditor` that follow the `ValueEditorProps` contract;
  - an unbuilt ID → none.

  `class-definition.ts` and the new element both use it, so the two cannot disagree.
  *Alternative:* a second mapping in the element. It would drift from `class-definition.ts`,
  which is the drift the property-editors change removed.

- **D5 Variant choices.** `alignment-picker` defaults to `{"choices": ["left", "center",
  "right"]}`, and an instance may switch to `top`/`center`/`bottom`. `action-picker` and
  `layouter-picker` load their choices when drawn, from the existing
  `/api/v1/wpi/classes/action?scope=all` and `/api/v1/wpi/classes/page-layouter` endpoints,
  unless `editor_options.choices` is given. `field-picker` reads its table from
  `editor_options.table`. `font-picker` is the text editor. `icon-picker` is the value-picker
  with whatever choices its options give (Open Questions).

- **D6 Unbuilt editors are drafts.** The five unbuilt editors are `draft` rows that name the
  `value-editor` module and fix `editor`. The existing draft rules apply unchanged. They are
  listed and marked as drafts, and a page that names one is rejected. Building one later
  means: add its component, add it to the resolver, and activate the class with the existing
  update endpoint. Backend validation lets an active class or a page instance use only a
  built `editor`. A draft may name any of the 28.

- **D7 Palette sub-folders.** `NodePalette` gets an optional `subCategories` prop
  (`{ Elements: ['Value editors'] }`). The editor gives every element class whose
  `code_module_id` is `value-editor` the category `Value editors`. All other element classes
  keep `Elements`.
  - A sub-folder is drawn inside its parent, indented, and starts folded.
  - A parent's count includes its sub-folders.
  - While searching, a match opens the sub-folder and its parent.
  - Dropping works unchanged, because a node type's ID, not its category, decides what is
    created.
  - The Flow page passes no `subCategories`, so it is unchanged.

  *Alternative:* a `palette_folder` attribute in the class row. That would add a descriptor
  attribute just for the editor's layout, while `code_module_id` already says what the class
  is.

- **D8 Properties definition.** `elementEditors['value-editor']` has these groups:
  placement; General (`label`, `data_binding`, `binding_mode`); Editor (`editor` read-only,
  `editor_options` as JSON); size. The code-module document's *Property editor* table
  records the same.

- **D9 Seeding.** One goose migration inserts the `kb.code_modules` row and the 28 class
  rows (23 `active`, 5 `draft`), with descriptions and help tips in English and Chinese.
  Each insert is guarded with `WHERE NOT EXISTS`, and Down deletes rows by creator, as in
  `20261030000005_wpi_chat_elements.sql`. The rows use the attribute format current at
  implementation time. If `20261102000002_wpi_inline_attribute_definitions.sql` has landed,
  that means inline `scope` and `value_editor` on each attribute.

## Risks / Trade-offs

- [The value editors are styled for the 390 px Properties panel (11 px text, `--cv-*`
  colours with plain fallbacks)] → They render with the fallbacks on a page. They look
  small but work. Restyling them is left out, so the panel is not affected.
- [Another session has uncommitted WPI changes in the same files: `class-definition.ts`,
  `validate.go`, `api.ts`, `entries.ts` and the message files] → Implementation edits these
  files in place and commits only its own hunks.
- [`wpi-chat-elements` and `wpi-create-element-class` are not archived, so the main
  `wpi-page-rendering` spec still has the older Element types text] → This change's delta
  starts from the chat-elements version. Archive in order: chat-elements first.
- [Function-valued options cannot be stored] → Picker choices show raw values on pages. A
  `choice_labels` option can be added later if authors need it.

## Migration Plan

Apply the migration with `doc-service --migrate-only` (air does not watch `.sql`). Rollback
is the migration's Down plus reverting the frontend and backend commits. No existing page or
class changes.

## Open Questions

- **icon-picker choices:** no icon set exists in code. The class starts with no choices, so
  each instance must give `editor_options.choices` until an icon set is chosen.
- **font-picker:** the devdoc mentions a "font grammar", but none exists in code. This change
  uses the plain text editor.
