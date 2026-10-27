## Why

The WPI editor's tool bar offers **Create Nodes → Create Element**, but it only drops a
blank node on the canvas. Authors need it to define a new element class: a kind of element
with its own instance attributes. Devdoc 2026102601 §3.5.1 adds a class only together with
its code, and the backend refuses any class ID without code. So, as things stand, a class
cannot be described before its code exists, and nobody can say what the code should
implement.

## What Changes

- **Create Element opens a form** that defines a new element class descriptor: the class
  ID, a description in English and optionally Chinese, and its instance attributes. Each
  attribute is entered with a name and either a default value or "every instance must set
  it" (`null`), then added with **Add Attribute**. A list shows the attributes added so
  far, each of which can be removed. **Create Element** stores the descriptor.
- **The new class is stored as a draft**: version 1 of `kind = 'element'` in
  `kb.wpi_descriptors`, with `status = 'draft'`. If any row of that class ID already
  exists, whatever its version or status, nothing is stored and the error is reported:
  "element class `x` already exists".
- **A draft class is defined before its code.** The create endpoint checks a draft's
  structure (ID, description, attribute names and values) but not that its code exists.
  The interpreter uses only `active` rows, so a draft is never run. After creation, a
  developer implements the class: registers its Svelte component in `elementClasses` and
  its attributes in Go `elementAttrs`. The class is then stored as `active` through the
  existing `PUT /api/v1/wpi/descriptors/element/:id`, which still requires code.
- **New endpoints**: `POST /api/v1/wpi/classes/element` creates a draft (admins only), and
  `GET /api/v1/wpi/classes/element` lists the latest draft or active version of every
  element class (logged-in users).
- **The palette's ELEMENTS folder lists the stored element classes.** A draft is shown as
  not yet implemented. Dragging a class onto the canvas adds an element node labelled with
  that class, which is what Create Element used to do.

## Capabilities

### New Capabilities

- `wpi-element-class-authoring`: creating element classes as drafts from the WPI editor:
  the create and list endpoints, the rules a draft must meet, the Create Element form, and
  the ELEMENTS folder of the palette.

### Modified Capabilities

- `wpi-classes`: a class row with no code may be stored as a `draft` through the create
  endpoint. An `active` class row, and every instance, still needs code.

## Impact

- Backend `server/api/wpi`: draft validation (`validate.go`), draft insert and class list
  (`store.go`), two handlers (`handler.go`), routes in `server/api/routes.go`.
- Frontend: `web/src/lib/wpi/api.ts` (two calls), a new form component under
  `web/src/lib/components/shared-ui/canvas/`, `wpi-editor.svelte` (Create Element opens the
  form; ELEMENTS folder from the class list), and new message keys in `web/messages/*.json`.
- No migration: `kb.wpi_descriptors.status` already allows `draft`.
- Docs: devdoc 2026102601 §3.5.1, §6 and §9 (classes may be drafted before their code),
  devdoc 2026102603, and the WPI reusable-module entries for the new endpoints.
