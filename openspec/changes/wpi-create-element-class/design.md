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

**D1. A draft is a format-`0.2` class row.** The body is the same shape as an active class
row: `descriptor_id`, `schema_version`, `description`, then one key per instance attribute,
whose value is its default, or `null` meaning every instance must set it. When the class is
implemented, its code and the row must agree, as for any class. *Alternative:* a separate
"class spec" format with types and per-attribute descriptions. Rejected because it would be
a second format to validate and convert, and the user asked only for names and values.

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
  `schema_version` is `0.2`;
- `description` is **required** and is a text attribute (string, or texts by language with
  `en`). A draft has no code, so its description is the only statement of what to build;
- every other key is an attribute name matching `^[A-Za-z_][A-Za-z0-9_]*$`, and is not
  `element`, the key with which an instance names its class;
- attributes that hold actions (`on_click`, `on_row_click`, `source`, `items`, `input`)
  have no default (`null` only);
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
`{descriptor_id, version, status, description}`, ordered by ID. Retired classes are left
out. Any logged-in user may read it, as with page search.

**D7. The form.** `CreateElementDialog.svelte` is a modal over the editor, styled like the
editor's other dark panels. The descriptor is one list of attributes, in row order. It
starts with the class attributes (`descriptor_id` empty, `schema_version` `"0.2"`,
`description` `{"en": "", "zh-cn": ""}`), which are tagged "class" in a column of their
own and cannot be removed or renamed.
Above the list, an attribute row (name, default value, "Every instance must set it") and
**Add Attribute** append instance attributes, which have a remove button. Double-clicking a
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

No schema change. Deploy the backend and frontend together; rollback is reverting both. A
draft created before a rollback stays in the table and is ignored, because only active
rows are read.
