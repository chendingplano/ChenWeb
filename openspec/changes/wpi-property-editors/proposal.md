## Why

The WPI Editor's Properties panel is really eleven separate property editors: one for the page,
one for panes, one for the panel, and one per element code module (text, input, select, button,
list, menu, chat-composer, chat-panel). They were written one at a time and have drifted apart.
Some have a title and some do not, some show help tips and some do not, many properties that
should be pulldowns are free-text fields, and typing records one undo step per keystroke. KnowledgeStore
devdoc 2026102701 §1.3.9 sets one set of rules for all of them; this change builds them.

## What Changes

- One shared property editor draws every entity's properties while the app runs, from a
  definition written as data in code (`web/src/lib/wpi/properties/`). No entity has its own
  Svelte editor any more.
- A definition lists the entity's groups and, for each property, its value editor and the data
  that editor needs (pulldown choices, units, limits), normally taken from the entity's class.
  A property the definition does not describe gets a common-sense value editor chosen from its
  name and default value.
- Reusable value editors in `web/src/lib/components/shared-ui/value-editors/`: text, multi-line
  text, localized text, number, switch, value picker, ordered multi-picker, comma-separated list,
  color picker, length editor, data source, sort order, column list, validation rules, read-only value and JSON. Only
  the editors that today's properties need are built; date, time and button-type pickers and
  standalone database/schema pickers wait for a property that needs one.
- Every property editor looks the same: a title naming the entity, groups that fold and unfold
  when their heading is clicked, and for each property a label, a value editor and a help tip.
  A non-empty help tip shows as an icon at the upper right of the value editor.
- The Properties panel shows the editor of the one selected entity (element, page, pane or
  panel) and nothing otherwise.
- A changed value takes effect on the canvas at once, and canvas changes (page and pane
  resizing, element and pane positions) show in the editor at once.
- Element and pane editors show the entity's position, read-only.
- Undo records one step per focus: everything typed or chosen in one property before leaving it
  is one step, not one step per keystroke.
- Invalid typed values are not applied; the editor says why and, on leaving the field, restores
  the stored value.
- **BREAKING (editor only):** `PageConfigPanel.svelte` and `TextProperties.svelte` and the WPI
  branches of `PropertiesPanel.svelte` are removed. The Flow page keeps `PropertiesPanel` for its
  own nodes. No descriptor format, API or stored data changes.
- Each code module document in `KnowledgeStore/doc-repo/code_modules/` (and the page, page
  layouter and flow layouter documents) gains a *Property editor* section listing every
  attribute's group and value editor.

## Capabilities

### New Capabilities

- `wpi-property-editors`: how the WPI Editor shows and edits the properties of the selected
  page, pane, panel or element: definitions, value editors, layout, help tips, selection,
  immediate effect, two-way sync, positions, validation and undo.

### Modified Capabilities

None in `openspec/specs/`. The Properties-panel wording in the pending changes
`wpi-code-modules` and `wpi-create-element-class` (*Element node properties*), `wpi-text-r2`
(*Text authoring and layout*) and `wpi-pages-own-panes` (*Page and layouter controls*, *Pane
editing*) describes the editors this change replaces; where they differ, this capability wins.
Reconcile those requirements when the pending changes are archived.

## Impact

- **Web (ChenWeb `web/`):** new `src/lib/wpi/properties/`, `src/lib/components/shared-ui/property-editor/`
  and `src/lib/components/shared-ui/value-editors/`; `wpi-editor.svelte` (panel selection, undo per
  focus, position reporting); `canvas/PagePreview.svelte` (reports the selected element's and pane's
  position); `canvas/PropertiesPanel.svelte` (WPI branches removed); `PageConfigPanel.svelte` and
  `TextProperties.svelte` removed; new Paraglide keys in `messages/en.json` and `messages/zh-cn.json`.
- **Server:** none.
- **Documents:** devdoc 2026102701 (§1.2, §1.3, §1.4), the code module documents, and reusable-module
  catalogue entries for the property editor and the non-trivial value editors.
