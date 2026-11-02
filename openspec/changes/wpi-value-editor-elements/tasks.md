## 1. Contract

- [ ] 1.1 Write the code-module document `KnowledgeStore/doc-repo/code_modules/interpreter-value-editor.md`: attributes, the 28 classes with their `editor` and default `editor_options`, the *Property editor* table, and the draft rule for unbuilt editors.

## 2. Backend

- [ ] 2.1 `validate.go`: register `value-editor` in `codeModules` (required `editor`, `label`, `data_binding`, `binding_mode`; optional `editor_options`).
- [ ] 2.2 Check `editor`: one of the 28 IDs. In an active class or a page instance, only the 23 that are drawn. `editor_options` must be an object. Add Go tests for an unknown editor, an active unbuilt editor, a draft unbuilt editor and options that are not an object.

## 3. Frontend

- [ ] 3.1 `wpi/properties/editor-ids.ts`: resolve a catalogue ID to its editor (built, variant to base, the three data-source controls, unbuilt to none). Switch `class-definition.ts` to it. Add bun tests.
- [ ] 3.2 Adapters for `TablePicker`, `FieldPicker` (table from options) and `SortOrderEditor` that follow `ValueEditorProps`.
- [ ] 3.3 `wpi/elements/ValueEditorElement.svelte`: label, editor, binding read and write (unset writes `null`), read-only mode, field errors, and choices loaded for `action-picker` and `layouter-picker`. Register it as `value-editor`.
- [ ] 3.4 `elementEditors['value-editor']`: placement, General, Editor and size groups.
- [ ] 3.5 `NodePalette.svelte`: `subCategories` prop (nested, starts folded, parent count includes children, search opens both). In `wpi-editor.svelte`, give `value-editor` classes the `Value editors` category and pass `{ Elements: ['Value editors'] }`. Add a bun test for the grouping.
- [ ] 3.6 Messages `nodepalette_category_value_editors` and `wpi_value_editor_*` in `en.json` and `zh-cn.json`. Run `bun run check`.

## 4. Data

- [ ] 4.1 Goose migration: the `kb.code_modules` row and 28 class rows (23 active, 5 draft), with English and Chinese descriptions and help tips, guarded inserts and a Down that deletes rows by creator. Dry-run it, apply it with `doc-service --migrate-only`, and read the rows back.

## 5. Verification and docs

- [ ] 5.1 Go tests, bun unit tests, `bun run check`.
- [ ] 5.2 Browser check: the palette sub-folder, search, and dragging `color-picker` and `value-picker` into a pane. Set the binding and options, preview, change values, and confirm the input doc. Save the page.
- [ ] 5.3 Update devdoc 2026102701 (§1.2 palette, §1.3.2 node types, §1.3.9 value-editor table: element class per editor) and the catalogued value-editor module documents' *used by*.
