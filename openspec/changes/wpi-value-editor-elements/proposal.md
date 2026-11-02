## Why

The WPI Editor's value editors (devdoc 2026102701 §1.3.9) exist only inside the Properties
panel. A page author cannot put a colour picker, a length editor or a data-source editor on
a WPI page, although these are exactly the controls admin and configuration pages need.
Making each value editor an element class lets an author drag it from the palette, bind it
to a field and run it like any other element. It reuses the 15 built editors instead of
writing new form controls.

## What Changes

- The palette gets a **VALUE EDITORS** folder, after ELEMENTS. It lists one element class
  for each of the 28 value editors in devdoc 2026102701 §1.3.9. These classes behave like
  the classes in ELEMENTS: they can be dropped into a pane and configured in the Properties
  panel and the Class tab.
- A new element code module, `value-editor`, draws any value editor on a page and binds it
  to an input-doc field (`label`, `data_binding`, `binding_mode`, like `input`). Each class
  fixes which editor it draws (`editor`) and gives its default options (`editor_options`).
- The classes are stored in `kb.wpi_descriptors` by one migration:
  - 15 built editors (text-editor … json-editor) are **active**.
  - 8 variants are **active** classes over an existing control:
    - font-picker uses the text editor with the font-family rule, until font management
      exists (a separate change, after this one; see design, "Follow-up: font management");
    - alignment-picker, action-picker and layouter-picker use the value-picker with fixed
      or loaded choices;
    - icon-picker uses the shared icon library's search picker (`IconSearchPicker`) and
      stores the icon name;
    - table-picker, field-picker and sort-order-editor use the controls now inside the
      data-source editor.
  - 5 unbuilt editors (date-picker, time-picker, button-type-picker, database-picker,
    schema-picker) are **drafts**: listed and marked as drafts, but not usable on a page
    until a later change builds them.
- The `icon-picker` editor ID now draws the icon library picker everywhere it is used,
  including attributes in the Properties panel that a class configures with it. This
  replaces the `icon-picker` → `value-picker` alias. It closes Issue 1 and Issue 2 of
  hand-off 2026110202.
- Backend validation accepts the `value-editor` module and checks `editor` and
  `editor_options`.
- No new endpoints. The existing class-list endpoint already reports `code_module_id`,
  which places a class in VALUE EDITORS.

## Capabilities

### New Capabilities
- `wpi-value-editor-elements`: the `value-editor` code module, the 28 value-editor element
  classes, and the palette's VALUE EDITORS folder.

### Modified Capabilities
- `wpi-page-rendering`: the element types now include the value-editor classes, drawn by
  the `value-editor` code module.

## Impact

- Frontend:
  - `web/src/lib/wpi/elements/ValueEditorElement.svelte` (new) and the element registry;
  - `wpi/properties/`: a new `value-editor` definition, and editor-ID resolution factored
    out of `class-definition.ts`;
  - `shared-ui/value-editors/`: adapters for the icon, table, field and sort-order controls;
  - `shared-ui/canvas/NodePalette.svelte` (folder label) and `shared-ui/wpi-editor.svelte`
    (palette grouping);
  - messages `nodepalette_*`, `wpi_value_editor_*` and `icon_search_*` in `en.json` and
    `zh-cn.json`.
- Shared library: uses `IconSearchPicker` and `Icon` from `@chendingplano/shared` as they
  are; no change to `shared/`.
- Backend: `server/api/wpi/validate.go` (`codeModules`, `editor`/`editor_options` checks)
  and tests.
- Database: one goose migration adding the `value-editor` row to `kb.code_modules` and 28
  rows to `kb.wpi_descriptors`. Production also needs `shared.icon_library` (hand-off
  2026110202, not yet applied there) before icon-picker works in production.
- Docs: new code-module document `KnowledgeStore/doc-repo/code_modules/interpreter-value-editor.md`.
  Devdoc 2026102701 (§1.2, §1.3.2, §1.3.9 value-editor table) is updated, as is hand-off
  2026110202 (Issues 1 and 2 closed).
- The Flow page (`canvas-01.svelte`) shares `NodePalette` but does not list WPI folders,
  so it is unchanged.
