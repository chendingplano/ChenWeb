## 1. Contract

- [x] 1.1 Write the code-module document `KnowledgeStore/doc-repo/code_modules/interpreter-value-editor.md`: attributes, the 28 classes with their `editor` and default `editor_options`, the *Property editor* table, and the draft rule for unbuilt editors.

## 2. Backend

- [x] 2.1 `validate.go`: register `value-editor` in `codeModules` (required `editor`, `label`, `binding_mode`; optional `data_binding`, `editor_options`).
- [x] 2.2 Check `editor`: one of the 28 IDs. In an active class or a page instance, only the 23 that are drawn. `editor_options` must be an object. Add Go tests for an unknown editor, an active unbuilt editor, a draft unbuilt editor and options that are not an object.

## 3. Frontend

- [x] 3.1 `wpi/properties/editor-ids.ts`: resolve a catalogue ID to its editor (built, variant to base, the three data-source controls, unbuilt to none). Switch `class-definition.ts` to it. Add bun tests.
- [x] 3.2 Adapters that follow `ValueEditorProps`: `IconSearchPicker` + `Icon` from `@chendingplano/shared` (stores the icon name, `locale={getLocale()}`), `TablePicker`, `FieldPicker` (table from options) and `SortOrderEditor`. Remove the `icon-picker` → `value-picker` alias.
- [x] 3.3 `wpi/elements/ValueEditorElement.svelte`: label, editor, binding read and write (unset writes `null`), read-only mode, field errors, and choices loaded for `action-picker` and `layouter-picker`. Register it as `value-editor`.
- [x] 3.4 `elementEditors['value-editor']`: placement, General, Editor and size groups.
- [x] 3.5 Palette: add `Value editors` after `Elements` in `WPI_PALETTE_CATEGORIES`, give `value-editor` classes that category in `wpi-editor.svelte`, and add its label in `NodePalette.categoryLabel`. Add a bun test for the grouping.
- [x] 3.6 Messages `nodepalette_category_value_editors`, `wpi_value_editor_*` and `icon_search_placeholder` / `_empty` / `_error` in `en.json` and `zh-cn.json`. Run `bun run check`.
- [x] 3.7 Runtime (found by the browser check): `rt.read` subscribes to an input-doc attribute that does not exist yet, so a value written there later is shown. `getPath`'s `Object.hasOwn` on a Svelte state proxy does not subscribe to a missing key.

## 4. Data

- [x] 4.1 Goose migration: the `kb.code_modules` row and 28 class rows (23 active, 5 draft), with English and Chinese descriptions and help tips, guarded inserts and a Down that deletes rows by creator. Dry-run it, apply it with `doc-service --migrate-only`, and read the rows back.

## 5. Verification and docs

- [x] 5.1 Go tests, bun unit tests, `bun run check`.
- [x] 5.2 Browser check: the VALUE EDITORS folder and search. Drag `color-picker`, `value-picker` and `icon-picker` into a pane, set the binding and options, preview, change values (pick an icon by typing `reload`), and confirm the input doc. Save and reopen the page.
- [x] 5.3 Update devdoc 2026102701 (§1.2 palette, §1.3.2 node types, §1.3.9 value-editor table: element class per editor), the catalogued value-editor module documents' *used by*, and hand-off 2026110202 (Issues 1 and 2 closed).
