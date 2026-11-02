## Context

The WPI Editor's right-hand Properties panel is drawn by three components with eleven separate
editors inside them:

- `canvas/PageConfigPanel.svelte` (333 lines): the page (five fixed groups, raw attribute names as
  labels), panes (page layouter pulldown, pane tree, split/merge/add/delete, `pane_id` rename, pane
  attributes) and the panel (flow layouter `config` and panel colors). Chosen with the *Page*,
  *Panes* and *Panel* buttons (`PageControl.svelte`, `configMode`).
- `canvas/TextProperties.svelte` (315 lines): the `text` element (six groups, localized text,
  validation, origin hints).
- `canvas/PropertiesPanel.svelte` (622 lines): every other element module, with per-module
  branches (`input` choices and visibility, chat token lists, `list` source/columns/order, button
  `on_click`), plus the Flow page's nodes and the *Node* tab.

Element class rows already carry `help_tip`, `default_value` and `instance_modifiable` for every
instance attribute (formats 0.3/0.4), and a page's element instance holds a copy of its class.
Pages have no classes. The Go validator (`server/api/wpi/validate.go`) is the authority on valid
values and is not changed.

Every edit currently calls `updateNodeData` or `onUpdate(page)`, which take an undo snapshot first,
so a typed word costs one undo step per character.

## Goals / Non-Goals

**Goals:**

- One renderer draws every page, pane, panel and element editor from a definition; no per-entity
  Svelte file.
- Definitions are data in code, one per entity kind, and name a value editor plus the data it
  needs for each property; an undescribed property gets a common-sense editor.
- Reusable value editors, with one prop contract, for every kind of value today's properties hold.
- Same layout everywhere: title, folding groups, label, value editor, help-tip icon.
- Selection rule, immediate effect, two-way sync, read-only positions, one undo step per focus.

**Non-Goals:**

- Descriptor-driven editors interpreted by WPI (devdoc §1.3.9 calls this later work).
- Editing classes; the Create Element / Edit class form is unchanged.
- The manual layouter, and editable positions.
- Date, time and button-type pickers, standalone database and schema pickers (no property uses
  them yet).
- The Flow page's node properties and the WPI editor's flow-only nodes (`wpi-page`, `wpi-action`,
  palette AI/Data/Actions nodes): they stay on `PropertiesPanel`.
- Server, descriptor format or API changes.

## Decisions

### D1. Definitions are TypeScript data, one file per entity kind

`web/src/lib/wpi/properties/` holds `page.ts` (the page, pane and flow-layouter panel definitions),
`text.ts`, `input.ts`, `elements.ts` (select, button, list, menu) and `chat.ts` (chat-composer,
chat-panel), registered in `index.ts` by code module ID. A definition is:

```ts
type EditorDef = { groups: GroupDef[] };
type GroupDef = { id: string; label: () => string; properties: PropertyDef[] };
type PropertyDef = {
  name: string;
  label?: () => string;          // Paraglide; raw name for properties no definition lists
  helpTip?: () => string;        // page/pane/panel; elements use the class row's help_tip
  editor?: ValueEditorId;        // omitted → common sense (D3)
  options?: EditorOptions;       // the data the editor needs: choices, units, keywords, limits
  read?: (stored) => edited;     // e.g. on_click {action:id} ↔ id, scrollbar 'hide' ↔ false
  write?: (edited) => stored;
  visible?: (ctx) => boolean;    // e.g. label_vertical_gap only for top/bottom captions
  validate?: (value, ctx) => string | null;  // refuse: the value is not applied
  warn?: (value, ctx) => string | null;      // show, but apply (width above max_width)
};
```

Choices come from the entity's class where a class exists: action classes for `on_click`
(`listTriggerActions`), page layouter classes for `page_layouter`, `text-values.json` for icons,
the chat module's token lists. Labels for value-picker choices are Paraglide functions.

*Alternative:* `editor`/`group`/`choices` fields in the class rows. Rejected by the user for now:
pages, panes and panels have no class rows, and it needs migrations and Go validation.

Code module documents (`KnowledgeStore/doc-repo/code_modules/`) gain a *Property editor* table
(group, attribute, value editor, where its data comes from) that the definition files follow.

### D2. One renderer, entity adapters

`shared-ui/property-editor/PropertyEditor.svelte` takes `{ title, def, source, onEditStart,
onEditEnd }`. `source` is an adapter with `get(name)` (effective value), `isSet(name)` (own
value present), `fixed(name)`, `required(name)`, `helpTip(name)`, `set(name, value)`,
`reset(name)` and an optional `extra` list (attributes the definition does not list, shown in an
*Other* group). Adapters live in `wpi/properties/sources.ts`:

- element: own values in `node.data.attrs`, class from the instance's copy or the class list,
  module fallbacks (`textDefaults`, `flowDefaults`); `set` → `updateNodeData`;
- page: `loadedPage`; `page_layouter` → `setPageLayouter`;
- each adapter says per property whether *Reset* applies (not for `page_layouter` or `pane_id`);
- pane: `setPaneAttr`; `pane_id` → `renamePane` (validated by `renameProblem`);
- panel: flow `config` via `setPanelConfig`, colors via `setPanelAttr`.

Pane structure controls (tree, split, add, merge, delete) are commands, not properties. They are
drawn by `PaneStructure.svelte` in a slot between the pane editor's title and its groups.

The *Node* tab stays for element nodes; `PropertiesPanel` keeps only the tab strip, the Node tab
and its Flow-page branches, and renders `PropertyEditor` in the Properties tab for elements.

### D3. Common-sense value editor

`commonSenseEditor(name, defaultValue)` in `wpi/properties/common-sense.ts`, first match wins:
fixed by the class → read-only; boolean default → switch; number default → number; name is
`width` or `height` or ends `_width`, `_height`, `_gap`, `_margin`, `_size` or `_thickness` →
length; name ends `color` → color picker; name is `label`,
`text`, `description`, `placeholder`, `help_text`, `help_tip`, or the default is a
`{en, …}` object → localized text; any other object or array default → JSON; otherwise text.

### D4. Value editors share one contract

`shared-ui/value-editors/<Name>.svelte`, props `{ id, value, options, invalid, onChange(value |
undefined) }`. `undefined` means unset. Editors that hold typed text (text, length, number,
JSON, localized) keep the text as typed while focused and call `onChange` only with values that
pass the editor's own grammar and the definition's `validate`. Invalid text is not flagged while
the author types (a half-typed `200p` on the way to `200px` is normal). On blur or Enter with
invalid text, the text stays in the field, unapplied, and under it the editor shows why
(`aria-invalid`, `role="alert"`) with two buttons (user decision, 2026-11-02): *Abandon Edit*
drops the typed text and shows the stored value; *Stay Edit* hides the message and puts focus
back in the field. A change from elsewhere (undo, the canvas) also drops the typed text.
Warnings (`warn`) describe an applied value and are shown at once (`role="status"`). Built: text, multiline-text,
localized-text, number, switch, value-picker, ordered-multi-picker, string-list (comma-separated
words: keywords, tags, access controls), color-picker, length, data-source (uses the existing
Pick Table and Pick Fields modules and `SortOrderEditor`), sort-order, column-list, validation,
read-only, json. Alignment, icon, action and layouter
pickers are the value-picker with their own choices and labels; font family is the text editor
with the font grammar.

### D5. Layout and help tips

Title (entity kind and ID, e.g. "Element · first-name"), then each group as `<details open>`
whose `<summary>` folds it. Each property: label (with `*` when every instance must set it),
*Reset* when it has its own value, the value editor, then any error or warning. When the help
tip is non-empty, `HelpTip.svelte` draws an info icon at the value editor's upper right; hover
or keyboard focus shows the tip (`role="tooltip"`, linked with `aria-describedby`). Hidden
properties keep their values. All text goes through Paraglide, prefixed `wpi_prop_*` for new keys.

### D6. Selection

The panel shows exactly one editor or nothing:

- one element node selected → its element editor (Properties tab) and Node tab;
- *Page* → page editor; *Panes* → the selected pane, or the root pane when none is selected;
- *Panel* → the selected leaf pane's panel; nothing when the selected pane is not a leaf;
- otherwise (no selection, several nodes, flow-only nodes) → `PropertiesPanel`'s existing
  Flow-node view for flow nodes, and an empty panel with no text for everything else.

Choosing the page, a pane or a panel also clears Svelte Flow's node selection; otherwise the next
change to the nodes (a pane split, say) reports the old node as selected and the panel jumps back
to it.

### D7. One undo step per focus

`wpi-editor.svelte` keeps an edit session: `PropertyEditor` calls `onEditStart` on `focusin` of a
property and `onEditEnd` when focus leaves it. While a session is open, the first change takes
the undo snapshot and later changes in the same session do not. Any other snapshot (canvas
action, undo, redo) closes the session's recording so the next property change records again.
A change made without focus (e.g. a checkbox clicked in Safari, which does not focus it) records
its own step. The logic is a pure helper (`canvas/edit-session.ts`) with tests.

### D8. Immediate effect and two-way sync

Editors read the live state (`$derived` from `nodes`, `loadedPage`), so page resizing, pane
resizing and pane renames show at once. A valid value is applied on every change, not on blur.
`PagePreview` already computes the selected element's rectangle for its tool bar; it now also
reports the selected element's and the selected pane's position (x, y in page pixels from the
page's top-left, rounded) through `onSelectionGeometry`, and the editors show it read-only in a
*Position* group. The page shows no position.

### D9. Reusable modules

The renderer and the simple editors (text, multiline, number, switch, value-picker, string-list,
read-only, JSON, sort-order) form one catalogued module, *Property Editor*. Localized text, ordered
multi-picker, color picker, length, data source, column list and validation editors are
catalogued separately (`KnowledgeStore/doc-repo/modules/` and `kb.reusable_modules`).

## Risks / Trade-offs

- [Behaviour lost in the rewrite: 1,270 lines of special cases] → per-module definition tests
  compare groups, visibility and choices with today's lists; browser check of every editor.
- [Pending changes' spec text contradicts the new rules] → noted in the proposal; this spec wins
  and the pending ones are reconciled on archive.
- [Invalid text no longer applied (text element used to apply it and show an error)] → the
  editor keeps the typed text and the reason on screen until the author fixes or leaves it.
- [Positions depend on DOM measurement] → measured after each render tick, same trigger as the
  existing tool bar placement; shown only when known.
- [Focus-based sessions merge two pulldown choices made without leaving the pulldown] → this is
  the rule the user chose (one step per focus).

## Migration Plan

Editor-only; no stored data changes. Roll back by reverting the commit.

## Open Questions

None.
