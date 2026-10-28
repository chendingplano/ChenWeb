## Context

WPI element classes are rows of `kind = 'element'` in `kb.wpi_descriptors`. Each row's
`descriptor_id` names the code that draws the class: a Svelte component in
`web/src/lib/wpi/elements/index.ts` (`elementClasses`) and the attributes it accepts in
`server/api/wpi/validate.go` (`elementAttrs`). The only write path,
`PUT /api/v1/wpi/descriptors/:kind/:id`, stores every version as `active`, and `classRow`
refuses an ID without code. The six classes with code (`text`, `input`, `select`, `button`,
`list`, `menu`) are all stored. The table's `status` column already allows `draft`, and
every read of classes (`latest`, the bundle) filters on `active`.

The WPI editor (`web/src/lib/components/shared-ui/wpi-editor.svelte`) is a Svelte Flow
canvas. Its tool bar's **Create Nodes → Create Element** creates a blank `wpi-element`
node. Its palette has an empty ELEMENTS folder.

The user decided (2026-10-27) that Create Element defines a new element class, which is
added to the ELEMENTS folder. Because the class has no code yet, it is stored as a draft.
The workflow: create the descriptor; if the ID is not new, report the error; otherwise
implement the class afterwards.

## Goals / Non-Goals

**Goals:**

- An admin can define a new element class (ID, description, instance attributes with
  defaults or `null`) in the editor and store it as a draft.
- An ID that already exists is refused with a clear error.
- Drafts are listed in the palette, marked as not yet implemented, and never run.
- Implementing a draft needs no new mechanism: code, then the existing PUT.

**Non-Goals:**

- Editing or deleting a draft, or creating a new version of an existing class, from the
  editor.
- Creating action classes (Create Action) or pages (Create Page).
- Attributes that are optional and have no default. A class row cannot express them; the
  class's code adds them when it is implemented.
- Instance attributes on canvas nodes, or building a page from nodes.
- Generating the class's code.

## Decisions

**D1. A draft is an ordinary element class row.** The body has the same shape as an active
element class row (format `0.3`, D9): the class attributes, then one key per instance
attribute. When the class is implemented, its code and the row must agree, as for any
class. *Alternative:* a separate "class spec" format for drafts. Rejected because it would
be a second format to validate and convert when the class becomes active.

**D2. A separate create endpoint: `POST /api/v1/wpi/classes/element`.** The PUT means "the
next version, active" and needs code. Create means "a new ID, draft, version 1" and fails
if the ID exists. Keeping them apart leaves the PUT's contract unchanged. *Alternative:* a
`?status=draft` flag on the PUT. Rejected because it would allow a draft version on top
of an active class's history, and the "not new" check does not fit an endpoint that adds
versions.

**D3. "Not new" means any row of that ID, whatever its version or status.** The insert is
`INSERT … SELECT … WHERE NOT EXISTS (SELECT 1 … WHERE kind = 'element' AND descriptor_id =
$id)`. No row returned, or a unique violation from a concurrent create, means the response
is 409 `element class "<id>" already exists`. A retired ID is not reused, so its history
keeps one meaning.

**D4. What the create endpoint checks in a draft.** These are the structural rules the
validator can apply without code:

- the body's `descriptor_id` matches the ID pattern (the URL holds no ID), and
  `schema_version` is `0.3`;
- `description` is **required** and is a text attribute (string, or texts by language with
  `en`); `implement_notes`, if present, is a text attribute too. A draft has no code, so
  these are what say what to build;
- every other key is an attribute name matching `^[A-Za-z_][A-Za-z0-9_]*$`, and is not
  `element`, the key with which an instance names its class;
- each attribute is an object with only an optional `help_tip` (text) and an optional
  `default_value` (D9);
- attributes that hold actions (`on_click`, `on_row_click`, `source`, `items`, `input`)
  have no default: `default_value` is `null` or left out;
- defaults of attributes whose meaning is shared by all classes are checked: sizes
  (`width`, `height`, `min_width`, `max_width`), `label` (text), `data_binding` (a
  `$input_doc` path) and `binding_mode`.

Attribute names the interpreter does not know are accepted: defining them is the point of
a draft. The function is `ValidateDraftClass(body)` in `validate.go`, next to
`classRow`, and reuses the validator's helpers.

**D5. Implementing a draft uses the existing path.** A developer registers the component in
`elementClasses` and the attributes in `elementAttrs`, then PUTs the class row. The PUT
validates it against the code and stores the next version as `active`. A PUT for an ID that
has only drafts and no code is still refused ("has no code"), so a draft can never become
active by accident. Until then, a page that names the class fails validation, as for any
class without code.

**D6. `GET /api/v1/wpi/classes/element` lists classes for the palette.** It returns the
latest version whose status is `draft` or `active` of each element class, as
`{descriptor_id, version, status, description, attributes}`, ordered by ID; `attributes`
holds the row's instance attributes (everything but the class attributes), each
`{help_tip, default_value}`, for the editor's Properties panel. Retired classes are left
out. Any logged-in user may read it, as with page search.

**D7. The form.** `CreateElementDialog.svelte` is a modal over the editor, styled like the
editor's other dark panels. The descriptor is one list of attributes, in row order. It
starts with the class attributes (`descriptor_id` empty, `schema_version` `"0.3"`,
`description` `{"en": "", "zh-cn": ""}`, `implement_notes` empty), which are tagged
"class" in a column of their own and cannot be removed or renamed and have no help tip.
The list's columns are the name, the class tag, the value (an instance attribute's
`default_value`: "no default" when it has none) and the help tip.
Above the list, the attribute entry appends instance attributes, which have a remove
button. Its first line holds the name and a multi-line default value that takes the
remaining width (Enter starts a new line, so Ctrl/Cmd+Enter adds the attribute, as Enter
does in the name). The next line holds "Every instance must set it" and **Add Attribute**. Double-clicking a
value edits it in place, and double-clicking an instance attribute's name renames it in
place, keeping its position and value (Enter or blur keeps the edit, Escape drops it
without closing the form; Enter or F2 on the focused value or name does the same from the
keyboard). A grip in the bottom-right corner resizes the form. The form is centred, so a
drag changes its size by twice the pointer's movement, which keeps the corner under the
pointer; the size is limited to at least 480×320 and at most the window. **Create Element**
sends the descriptor. The user decided (2026-10-27) that the class attributes are created
automatically and that the list is their only editor, replacing separate class ID and
description inputs.

- A typed value is parsed as JSON when it is valid JSON (`200`, `true`,
  `{"en": "Name"}`), otherwise it is taken as a string (`200px`, `read-write`).
  `descriptor_id` and `schema_version` are always strings. The list shows the stored JSON,
  so `"123"` versus `123` is visible, and an edit starts from text that reads back as the
  same value.
- Empty texts in the description object are left out, so an unfilled Chinese text is not
  sent.
- Client-side checks give quick feedback on the class ID, an English description, the
  attribute name pattern, duplicate names, reserved names (`descriptor_id`,
  `schema_version`, `description`, `element`), a missing default and an instance attribute
  edited to empty. The server's rules are the ones that count, and its issues are shown in
  the form. A 409 is shown as "already exists". A change to the list clears the last
  create's error.
- The pure parts (the initial list, parsing a value, checking a name, building and checking
  the body) are in `element-class-form.ts`, with unit tests.

**D9. Element class rows are format `0.3`: each instance attribute is an object.** The
user decided (2026-10-27) that an attribute is an object
`{"help_tip": <text>, "default_value": <value>}` and that this holds for every element
class row, drafts and active ones alike. `help_tip` is text for authors (a string, or texts
by language with `en`); `default_value` is the default, `null` means every instance must
set it, and leaving it out means the attribute is optional with no default, which the flat
format could not write down. Both keys are optional, and no other key is allowed.

- `schema_version` is per kind: element class rows are `0.3`; apps, pages and action
  classes stay `0.2` (`SchemaVersion`, `ElementSchemaVersion`, `schemaVersionOf` in
  `types.go`). Only element class rows changed shape, so only they changed version, and an
  interpreter that knows only `0.2` rejects them instead of misreading them.
  *Alternative:* bump every descriptor to `0.3`. Rejected because every app and page would
  need a new version for no change of shape.
- `classDefaults(kind, row)` in `validate.go` reads a row's defaults and its required
  attributes for both formats; `mergeInstance` and `againstClass` use it. The frontend has
  `mergeElement` (format `0.3`) next to `mergeInstance` (action rows) in `entries.ts`.
- Migration `20261027000006_wpi_element_classes_0_3.sql` stores version 2 of the six seeded
  element classes in the new format, with the same defaults and a help tip in English and
  Chinese for every attribute. It also lists the list's `on_row_click` and the menu's
  `initial_item`, which are optional with no default. `seed_test.go` reads the seed
  migrations in order, a later row replacing an earlier one.
- Action classes keep the flat format for now.

**D10. `implement_notes` is a class attribute.** The user asked for an attribute holding the
prompt for implementing the element, which also helps authors understand it, one per
element. Instances cannot set it, so it is a class attribute next to `descriptor_id`,
`schema_version` and `description` (`classAttrs` in Go, `CLASS_ATTRS` in TypeScript), for
action classes too. It is optional, a text attribute, and is never merged into instances.
The form lists it from the start, edits it in a multi-line editor (Enter starts a line,
Ctrl/Cmd+Enter keeps the text), and leaves it out of the row while it is empty.

**D11. Editing a class stores its next version with the same status.** The user asked
(2026-10-27) for an edit button on a node that opens the node's class descriptor in a form
like Create Element. Only element nodes have a class in the editor (pages have none, and
action nodes do not name one yet), so only they get the button. `GET
/api/v1/wpi/classes/element/:classId` returns the class's latest draft or active row whole
(`{version, status, class}`); `PUT` on the same path takes the edited row and stores version
N+1 with the status of the latest row, decided on the server: a draft stays a draft and is
checked by the draft rules (D4). Active class edits use the same structural rules, with
an optional description. The class-edit endpoint accepts added, removed and renamed
attributes independently of the interpreter's current attribute list, so authors can
revise a class before updating its implementation. It still checks attribute names,
attribute objects, help tips, action-default restrictions and shared defaults. The
separate descriptor PUT and bundle validation retain their checks against code; saving
an edited class does not implement new attribute behavior. The class ID cannot change
(a 400 if the body's `descriptor_id` differs; the form shows it read-only). Admins only; last save wins, as for every descriptor.
*Alternatives:* save every edit as a draft version on top of an active class (the editor
and the running interpreter would then disagree about the class); edit drafts only
(implemented classes could not get help tips or new defaults from the editor). Both
rejected.
The form is `CreateElementDialog.svelte` given a `classId`: it loads the row
(`attributesFromRow` puts the class attributes first, in the create form's order, and gives
a description by language both language slots), fixes `descriptor_id`, says whether the
class is a draft or implemented, and its button is **Save**. `buildClassRow` now leaves out
an empty description, since an active class may have none; the form asks for one only for
drafts.

**D12. The node tool bar's actions come from the canvas.** `BaseNode`, shared by the WPI
editor and the Flow page, had three buttons whose callbacks every node component set to
no-ops. The user asked to remove the first (Configure) and make the other two work.
Duplicate and Delete change the canvas's node list and must be undoable, which only the
canvas that holds the nodes can do, so each canvas provides them through Svelte context
(`setNodeActions` in `node-actions.ts`), and `BaseNode` reads them; the node components no
longer pass tool bar callbacks. Duplicate copies the node's data and size one grid cell
down-right (further while another node sits there) and selects the copy; edges are not
copied. Delete removes the node and the edges that touch it. A tool bar click stops
propagation, or Svelte Flow's node click would select the clicked node again, and the bar
is `nodrag`. `WpiNode` passes `onEdit` (the Edit class button) for element nodes only.

**D8. The ELEMENTS folder shows the class list.** The editor loads the list when it opens,
and again after a create. Each class becomes a palette entry in the `Elements` category
with ID `wpi-element:<class>`, which is unique, as the palette's keyed list requires. A
draft's label carries a "draft" marker. Dropping an entry creates an ordinary `wpi-element`
node labelled with the class ID, with `element: <class>` in its data. So dragging a class
does what Create Element did before, now with a class.

## Risks / Trade-offs

- [Drafts pile up with no code] → They are visibly marked in the palette. A row is retired
  with `psql` (there is no delete endpoint), the same as other descriptor housekeeping.
- [A draft's attributes and its later code disagree] → The PUT validates the active row
  against the code, so the mismatch is reported when the class is implemented, not at run
  time.
- [JSON-or-string defaults surprise an author] → The list shows the stored JSON before the
  class is created.
- [A class ID that names existing code but has no row] → The create still stores a draft,
  and the admin can PUT it as active straight away. No ID is in this state today.

## Migration Plan

No table change. Migration `20261027000006` adds version 2 of the six element classes in
format `0.3`; the backend that reads them must be deployed with it, because an older
backend rejects `0.3` rows and a newer one rejects `0.2` rows. Deploy the backend, the
frontend and the migration together. Rollback: revert the code and run the migration's
Down, which deletes the version-2 rows, so version 1 (format `0.2`) is used again. A draft
created before a rollback stays in the table and is ignored, because only active rows are
read.
