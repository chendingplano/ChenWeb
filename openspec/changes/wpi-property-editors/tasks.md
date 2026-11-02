## 1. Definitions and common sense

- [x] 1.1 `wpi/properties/types.ts`: `EditorDef`, `GroupDef`, `PropertyDef`, `ValueEditorId`, `EditorOptions`, `PropertySource`
- [x] 1.2 `wpi/properties/common-sense.ts` with tests (`bun test`) for every rule in design D3
- [x] 1.3 Element definitions: text, input, select, button, list, menu, chat-composer, chat-panel (groups, labels, editors, choices, visibility, validation and warnings carried over from `TextProperties`/`PropertiesPanel`), registered in `index.ts`; tests compare groups, choices and visibility with the old lists
- [x] 1.4 Page, pane and panel (flow layouter) definitions, with Paraglide labels and help tips; tests
- [x] 1.5 `wpi/properties/sources.ts`: element, page, pane and panel adapters (`get`, `isSet`, `fixed`, `required`, `helpTip`, `set`, `reset`, extra attributes); tests for set/reset/fixed and the special `page_layouter` and `pane_id` setters

## 2. Value editors

- [x] 2.1 Simple editors: text, multiline-text, number, switch, value-picker, read-only, json, sort-order — shared contract (design D4), draft text kept while focused, invalid text not applied and restored on blur
- [x] 2.2 localized-text, ordered-multi-picker, color-picker (none / one / light and dark), length (keywords, units, bare number → px)
- [x] 2.3 data-source (Pick Table, Pick Fields, limit, action, result, sort order), column-list, validation rules
- [x] 2.4 Pure helpers behind them (length parse/format, color mode, localized value, ordered list moves) with tests

## 3. Renderer and editor wiring

- [x] 3.1 `property-editor/PropertyEditor.svelte` (title, `<details>` groups, label, `*`, Reset, fixed, error/warning, Other group) and `HelpTip.svelte` (icon at the value editor's upper right, hover and focus)
- [x] 3.2 `canvas/edit-session.ts` (one undo step per focus) with tests; `wpi-editor.svelte` uses it for every property change
- [x] 3.3 `PaneStructure.svelte` (tree, split, add, merge, delete) for the pane editor
- [x] 3.4 `wpi-editor.svelte`: selection rule (design D6), page/pane/panel/element editors in the panel, *Panes* selects the root pane when none is selected
- [x] 3.5 `PagePreview.svelte`: report the selected element's and pane's position; editors show it read-only
- [x] 3.6 Remove `PageConfigPanel.svelte`, `TextProperties.svelte` and the WPI branches of `PropertiesPanel.svelte`; remove what that leaves unused; keep the Flow page working
- [x] 3.7 Paraglide keys in `messages/en.json` and `messages/zh-cn.json`; `bun run check` passes; i18n baseline not raised

## 4. Verification

- [x] 4.1 `bun test` for the canvas, properties and value-editor tests; `bun run check`
- [x] 4.2 Browser check in the WPI editor: every element module, page, pane and panel editor; fold/unfold; help-tip icon; invalid length; page resize sync; positions; one undo step per focus; empty panel for no or several selected nodes; Flow page properties still work

## 5. Documents

- [x] 5.1 Code module documents: a *Property editor* table (group, attribute, value editor, data source) in each element module document and in `interpreter-page.md`, `interpreter-layouter-page.md`, `interpreter-layouter-flow.md`
- [x] 5.2 Reusable-module catalogue entries (design D9)
- [x] 5.3 Devdoc 2026102701: §1.2 right panel, §1.3.1 files, §1.3.3 undo, §1.3.9 (drawn from definitions, no Svelte file per entity; help tip icon; undo per focus), §1.4

## 6. Invalid values (user decision, 2026-11-02)

- [x] 6.1 `DraftField.svelte`: on leaving a field with invalid text keep the text, show the reason under the value editor with *Abandon Edit* and *Stay Edit*; Paraglide keys; harness tests and spec updated
