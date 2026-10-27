## 1. Backend

- [x] 1.1 Validator: `ValidateDraftClass(body)` — ID, schema version, required description, attribute names (pattern, not `element`), action-holding attributes `null`, shared-attribute value checks; tests
- [x] 1.2 Store: `insertDraftClass` (version 1, `draft`, only when no row of the ID exists; unique violation → conflict) and `elementClasses` (latest draft-or-active version per class); sqlmock tests
- [x] 1.3 Handlers `CreateElementClass` (admin, 201/400/403/409) and `ListElementClasses` (logged in), routes in `server/api/routes.go`; handler tests
- [x] 1.4 `go test ./api/wpi/...` and `go vet` pass

## 2. Frontend

- [x] 2.1 `web/src/lib/wpi/api.ts`: `listElementClasses`, `createElementClass` (WpiError carrying status and issues)
- [x] 2.2 `element-class-form.ts`: parse a default (JSON or string), check an attribute name, build the class body; unit tests
- [x] 2.3 `CreateElementDialog.svelte`: class ID, description en/zh-cn, attribute row with Add Attribute, attribute list with remove, Create Element / Cancel, errors shown
- [x] 2.4 `wpi-editor.svelte`: Create Element opens the form; load the class list on open and after a create; ELEMENTS folder entries (drafts marked); dropping a class adds a `wpi-element` node labelled with the class
- [x] 2.5 Message keys in `web/messages/en.json` and `zh-cn.json`; `bun run check` and the unit tests pass

## 3. Verification

- [x] 3.1 Against the dev server: create a draft through the form, see it in ELEMENTS, get the "already exists" error on a second create, drag a class onto the canvas
- [x] 3.2 Remove the verification draft row afterwards

## 4. Docs

- [x] 4.1 Devdoc 2026102601: §3.5.1 (a class may be drafted before its code; the create workflow), §6 (draft rows, the create and list endpoints), §9 (authoring)
- [x] 4.2 Devdoc 2026102603 and the WPI reusable-module entries: the new endpoints and the form

## 5. Form revision (class attributes in the list)

- [x] 5.1 `element-class-form.ts`: initial class attributes, value parsing per attribute, edit text, edit and row checks; unit tests
- [x] 5.2 `CreateElementDialog.svelte`: drop the class ID and description inputs; list starts with the class attributes (not removable); double-click (or Enter/F2) edits a value in place; stale create errors cleared on change
- [x] 5.3 Messages updated in both languages; `bun run check`, unit tests and the browser run pass; spec, design and devdocs updated

## 6. Form revision (aligned class column, resizing, renaming)

- [x] 6.1 Class tags in a column of their own; resize grip (bottom-right, centred dialog grows by twice the drag, 480×320 minimum)
- [x] 6.2 Double-click an instance attribute's name to rename it (`renameProblem`); class attribute names read-only; unit tests
- [x] 6.3 Messages in both languages; `bun run check`, unit tests and the browser run (alignment, rename, resize) pass; spec, design and devdocs updated

## 7. Form revision (entry layout)

- [x] 7.1 Default value is a multi-line editor using the rest of the name's line (Enter adds a line, Ctrl/Cmd+Enter adds the attribute); checkbox and Add Attribute on the next line
- [x] 7.2 `bun run check` and the browser run (layout, multi-line JSON default) pass

## 8. Element class format 0.3 and implement_notes

- [x] 8.1 Validator: per-kind schema versions (element 0.3); `implement_notes` class attribute; attribute objects (`help_tip`, `default_value`) for active and draft element rows; `classDefaults` for merges and required attributes; tests
- [x] 8.2 Migration `20261027000006_wpi_element_classes_0_3.sql` (version 2 of the six element classes, help tips en/zh-cn); seed tests read it; dry-run, then applied to dev
- [x] 8.3 Frontend interpreter: `mergeElement` reads `default_value`; tests
- [x] 8.4 Form: `implement_notes` in the list (multi-line editor), help tips entered and edited, empty default = no default; helpers and tests; messages in both languages
- [x] 8.5 `go test`, `bun run check`, unit tests, the pilot app's browser run and the editor's browser run pass

## 9. Element nodes: properties, no connection points, resizing

- [x] 9.1 `GET /api/v1/wpi/classes/element` returns each class's instance attributes (`ClassSummary.Attributes`); store and handler tests
- [x] 9.2 `WpiNode.svelte`: element nodes have no connection points; `wpi-editor.svelte`: a dropped or created node becomes the canvas selection (resize grips show at once)
- [x] 9.3 `PropertiesPanel.svelte`: element class and instance attributes (help tip, default or required as placeholder) edited into `data.attrs`; `element-attributes.ts` + tests; messages in both languages
- [x] 9.4 `go test`, `bun run check`, unit tests and the editor's browser run (no handles, grips, corner and edge resize, attribute edits) pass; spec, design, devdocs 2601/2603 and module docs (2.4.0) updated
