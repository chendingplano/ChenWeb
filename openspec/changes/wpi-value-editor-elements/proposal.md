## Why

The WPI Editor's value editors (devdoc 2026102701 §1.3.9) exist only inside the Properties
panel. A page author cannot put a colour picker, a length editor or a data-source editor on
a WPI page, although these are exactly the controls admin and configuration pages need.
Making each value editor an element class lets an author drag it from the palette, bind it
to a field and run it like any other element. It reuses the 15 built editors instead of
writing new form controls.

## What Changes

- The palette's ELEMENTS folder gets a sub-folder, **VALUE EDITORS**, listing one element
  class for each of the 28 value editors in devdoc 2026102701 §1.3.9.
- A new element code module, `value-editor`, draws any value editor on a page and binds it
  to an input-doc field (`label`, `data_binding`, `binding_mode`, like `input`). Each class
  fixes which editor it draws (`editor`) and gives its default options (`editor_options`).
- The classes are stored in `kb.wpi_descriptors` by one migration:
  - 15 built editors (text-editor … json-editor) are **active**.
  - 8 variants are **active** classes over an existing control. Five use a base editor with
    fixed options: font-picker, alignment-picker, icon-picker, action-picker and
    layouter-picker. Three use the controls now inside the data-source editor: table-picker,
    field-picker and sort-order-editor.
  - 5 unbuilt editors (date-picker, time-picker, button-type-picker, database-picker,
    schema-picker) are **drafts**: listed and marked as drafts, but not usable on a page
    until a later change builds them.
- Backend validation accepts the `value-editor` module and checks `editor` and
  `editor_options`.
- The Properties panel gets a definition for the module. The Class tab works on these
  classes as on any element class.
- No new endpoints. The existing class-list endpoint already reports `code_module_id`,
  which places a class in the sub-folder.

## Capabilities

### New Capabilities
- `wpi-value-editor-elements`: the `value-editor` code module, the 28 value-editor element
  classes, and the palette's VALUE EDITORS sub-folder.

### Modified Capabilities
- `wpi-page-rendering`: the element types now include the value-editor classes, drawn by
  the `value-editor` code module.

## Impact

- Frontend: `web/src/lib/wpi/elements/ValueEditorElement.svelte` (new) and the element
  registry; `wpi/properties/` (new `value-editor` definition, shared editor-ID resolution
  factored out of `class-definition.ts`); `shared-ui/canvas/NodePalette.svelte` (nested
  folders); `shared-ui/wpi-editor.svelte` (palette grouping); messages `nodepalette_*`,
  `wpi_value_editor_*` in `en.json` and `zh-cn.json`.
- Backend: `server/api/wpi/validate.go` (`codeModules`, `editor`/`editor_options` checks)
  and tests.
- Database: one goose migration adding the `value-editor` row to `kb.code_modules` and 28
  rows to `kb.wpi_descriptors`.
- Docs: new code-module document `KnowledgeStore/doc-repo/code_modules/interpreter-value-editor.md`.
  Devdoc 2026102701 (§1.2, §1.3.2, §1.3.9 value-editor table) is updated.
- The Flow page (`canvas-01.svelte`) shares `NodePalette` but passes no sub-folders, so it
  is unchanged.
