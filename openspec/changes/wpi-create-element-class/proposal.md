## Why

The WPI editor's tool bar offers **Create Nodes → Create Element**, but it only drops a
blank node on the canvas. Authors need it to define a new element class: a kind of element
with its own instance attributes. Devdoc 2026102601 §3.5.1 adds a class only together with
its code, and the backend refuses any class ID without code. So, as things stand, a class
cannot be described before its code exists, and nobody can say what the code should
implement.

## What Changes

- **Create Element opens a form** that defines a new element class descriptor as a list of
  attributes. The list starts with the class attributes (`descriptor_id`, `schema_version`,
  `description`). Instance attributes are entered with a name and either a default value or
  "every instance must set it" (`null`), then added with **Add Attribute**, and can be
  removed. Double-clicking an attribute edits its value. **Create Element** stores the
  descriptor.
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
- **Element class rows move to format `0.3`.** Each instance attribute is an object
  `{"help_tip": {"en": …, "zh-cn": …}, "default_value": …}`: `default_value` `null` means
  every instance must set it, and leaving it out means optional with no default. This holds
  for drafts and active classes; apps, pages and action classes stay `0.2`. A migration
  stores the six seeded element classes as version 2 in the new format, with help tips, and
  both interpreters read `default_value` when they merge an instance with its class.
- **New class attribute `implement_notes`**: the prompt for implementing the element, which
  also helps authors understand it. The form lists it from the start and edits it in a
  multi-line editor; the help tips are entered in English and Chinese next to each new
  attribute.
- **The palette's ELEMENTS folder lists the stored element classes.** A draft is shown as
  not yet implemented. Dragging a class onto the canvas adds an element node labelled with
  that class, which is what Create Element used to do.
- **The node tool bar works** (added 2026-10-27). The no-op Configure button is gone;
  Duplicate copies the node and Delete removes it with its edges, both undoable, on the WPI
  editor and the Flow page. An element node also gets **Edit class**, which opens the Create
  Element form on that class's latest row; saving stores the class's next version with the
  same status, through the new `GET`/`PUT /api/v1/wpi/classes/element/:classId`.

## Capabilities

### New Capabilities

- `wpi-element-class-authoring`: creating element classes as drafts from the WPI editor:
  the create and list endpoints, the rules a draft must meet, the Create Element form, and
  the ELEMENTS folder of the palette.

### Modified Capabilities

- `wpi-classes`: a class row with no code may be stored as a `draft` through the create
  endpoint. An `active` class row, and every instance, still needs code. Element class rows
  are format `0.3`, with each instance attribute an object holding a help tip and a
  default, and every class may have `implement_notes`; `help_tip` and `implement_notes` are
  text attributes.
- `wpi-descriptor-store`: the validator checks each descriptor in its kind's format (`0.3`
  for element class rows, `0.2` otherwise), and an element class draft only for structure.

## Impact

- Backend `server/api/wpi`: draft validation (`validate.go`), draft insert and class list
  (`store.go`), two handlers (`handler.go`), routes in `server/api/routes.go`.
- Frontend: `web/src/lib/wpi/api.ts` (two calls), a new form component under
  `web/src/lib/components/shared-ui/canvas/`, `wpi-editor.svelte` (Create Element opens the
  form; ELEMENTS folder from the class list), and new message keys in `web/messages/*.json`.
- Migration `20261027000006_wpi_element_classes_0_3.sql` (version 2 of the six element
  classes in format `0.3`); no table change, since `kb.wpi_descriptors.status` already
  allows `draft`.
- Interpreter: `classDefaults`/`mergeInstance` in `server/api/wpi/validate.go`,
  `mergeElement` in `web/src/lib/wpi/entries.ts`; seed tests read the new migration.
- Node tool bar (2026-10-27): `canvas/nodes/BaseNode.svelte` and the node components that
  passed no-op tool bar callbacks, `canvas/node-actions.ts` (+ tests), `canvas-01.svelte`;
  `GetElementClass`/`UpdateElementClass` and two store methods for the class edit.
- Docs: devdoc 2026102601 §3.5.1, §6 and §9 (classes may be drafted before their code),
  devdoc 2026102603, and the WPI reusable-module entries for the new endpoints.
