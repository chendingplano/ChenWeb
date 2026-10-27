## Context

The WPI pilot (`openspec/changes/wpi-pilot`, devdoc 2026102603) runs the `customer-admin`
demo app from descriptors in `kb.wpi_descriptors`. Elements and actions are descriptors of
their own, each naming a hard-coded kind (`element_type`, `action_type`); a page refers to
stored elements by ID and may override any of their attributes, or writes elements inline;
triggers refer to stored actions by ID or write them inline.

Devdoc 2026102601 §3.5, §3.6 and §5 (revised 2026-10-27 with the user) replace this with
classes and instances. Two gaps were settled with the user on 2026-10-27:

- A class's code defines which instance attributes exist. The class row gives a default, or
  `null` for attributes every instance must set; attributes the row leaves out are optional
  with no default.
- Action class IDs are kebab-case (`open-page`, `close-page`, `save-page`), because
  descriptor IDs must match `^[a-z0-9][a-z0-9-]{0,127}$` (also a CHECK constraint).

## Goals / Non-Goals

**Goals:**

- Element and action classes are stored rows whose IDs name the code that implements them,
  in the Go validator and in the frontend; nothing else names a kind.
- Instances live only in pages and are checked against their class.
- The `customer-admin` app runs in the new format and passes the pilot's four "done when"
  checks (devdoc §8) again.

**Non-Goals:**

- Classes for pages; extended attributes; named docs besides `$input_doc` (devdoc §9).
- Multi-language text other than `label` and `description` (menu items, list columns,
  select options and text elements keep plain strings).
- New element or action classes beyond `save-page`; a descriptor editor.
- Keeping format `0.1` working.

## Decisions

### D1. A class ID is the key of its code

Backend: `elementAttrs` (validate.go) is keyed by element class ID and a new `actionAttrs`
is keyed by action class ID; each lists the instance attributes the code defines, split into
`required` (the code cannot work without a value) and `optional`. `elementTypes` and
`actionTypes` in types.go become `elementClasses` and `actionClasses`, derived from these
maps. Frontend: the registry `elementClasses` in `elements/index.ts` maps class ID →
component; the runtime dispatches action instances by class ID.

A class row whose ID has no code, or an instance naming such a class, is a validation error.

*Alternative:* keep `element_type` so several classes can share code. Rejected by the user:
it is how the number of classes starts to grow.

### D2. Class rows

A class row holds the class attributes `descriptor_id`, `schema_version` (both required) and
`description` (optional). Every other key is an instance attribute the class's code defines;
its value is the default, or `null` for "every instance must set it". The validator checks:

- each key is a class attribute or an attribute the code defines;
- each attribute the code requires appears in the row (default or `null`), so every
  instance can be complete;
- each non-null default passes the attribute's value checks;
- attributes that hold action instances (`on_click`, `on_row_click`, `source`, `items` for
  elements; `input` for `open-page`) are `null` or left out — never a default (D5).

### D3. Instances, checked in two stages

An element instance is `{ "element": "<class>", ...instance attributes }`; an action
instance is `{ "action": "<class>", "args"?: {...}, ...instance attributes }`. An instance
may not set class attributes.

The **merged instance** is the class row's non-null defaults overlaid with the instance's
own attributes. Go (`mergeInstance`) and TypeScript (`entries.ts`) implement the same rule.

- **When a page is written** (`PUT`, no other descriptors needed): `element`/`action` names
  a class with code, keys are attributes the code defines, present values pass their value
  checks, nested action instances are checked recursively.
- **When the bundle is built** (classes known): every attribute the class row sets to
  `null` is set by the instance, and the merged instance passes the complete checks —
  required attributes present and the cross-attribute rules (a `text` element has exactly
  one of `text` and `data_binding`; a menu's `initial_item` is a leaf). Pane, table and
  column checks run on merged action instances.

The validator's element and action checks take a `complete` flag: `false` checks only the
values present (page writes and class defaults), `true` also checks requiredness and
cross-attribute rules (merged instances).

### D4. Bundle loading

The bundle loads **all** active element classes and action classes, one query per kind
(`Loader` with `ids == nil`), then pages by ID in rounds as before. Classes are few by
design (devdoc §3.5.1), so loading all costs less than tracking which are needed, and every
instance can be merged as soon as its page arrives — including finding the page an
`open-page` instance opens. The response carries only the classes the app uses; only those
are validated, so a broken unused class row does not break every app.

### D5. Where action instances live

| Position | Allowed classes |
| --- | --- |
| button `on_click`, menu item `on_click`, list `on_row_click` | `save`, `open-page`, `close-page`, `save-page` |
| list `source` | `load` |
| page `input`, `open-page` `input` | `{}` or `load` |
| page `save_action` | `save` |

Because attributes holding action instances cannot have class defaults (D2), every action
instance is written in a page. `ActionRef` becomes `{kind: "page", id, pointer}` only; the
`kind: "action"` and `kind: "element"` forms are removed.

### D6. Running load and save on the server

`POST /api/v1/wpi/actions/run` loads the page named by `ref`, resolves `pointer` to an
object whose `action` is `load` or `save`, loads that action class, checks the instance
against it (both stages of D3, tables and columns via the catalog) and runs the merged
instance. The typed `Action` struct takes its class from `action` instead of `action_type`.
The browser still sends only the reference, argument values and (for `save`) the input doc.

### D7. Arguments

Unchanged from the pilot except for nesting: a trigger's `args` are resolved where it runs
(`$input_doc`, plus `$record` for a row click); a list `source`'s `args` may use
`$input_doc`; a page's own `input` has literal `args` only. A load instance nested as an
`open-page` `input` receives the `open-page`'s resolved arguments; if it has `args` of its
own, those are resolved with `$args` meaning the `open-page`'s arguments, and are sent
instead. On the server, `$args.<name>` in a load's `where` is replaced with the request's
argument as before.

### D8. `save-page`

A frontend action with no instance attributes: it saves the triggering panel with its
page's `save_action` (`{kind: "page", id: page, pointer: "/save_action"}`), exactly as the
unsaved-changes prompt does. A trigger may still be a `save` instance itself.

### D9. Text in several languages

`label` (instance attribute of `input`, `select`, `button`) and `description` (class
attribute) accept a non-empty string or an object whose keys are language codes
(`^[a-z]{2,3}(-[a-z0-9]{2,8})*$`) and whose values are non-empty strings; the object must
include `en`. The frontend shows the current Paraglide locale's text, else `en`.

### D10. Element instance identity

Inline elements lose their `descriptor_id`. The interpreter identifies an element instance by
its page and JSON Pointer (`/elements/<i>`); a menu's panel bindings are keyed
`<panel>/elements/<i>/<item_id>` instead of `<panel>/<descriptor_id>/<item_id>`.

### D11. Format `0.2` and the data migration

`SchemaVersion` becomes `"0.2"`; descriptors in `0.1` are rejected with the existing
"schema_version is not supported" error. A new data migration
(`project_migrations/20261027000002_seed_wpi_classes_and_instances.sql`, created by
`seed:wpi-classes`):

- inserts the eleven classes as version 1;
- inserts version 2 of the `customer-admin` app and its four pages in the new format (the
  app body only changes its `schema_version`);
- sets `status = 'retired'` on the pilot's stored elements and actions
  (`created_by = 'seed:wpi-pilot'`, kinds `element` and `action`), which are not classes.

Down deletes the `seed:wpi-classes` rows and re-activates the retired pilot rows. Rolling
back the code as well restores the pilot exactly, because the pilot's version 1 rows are
untouched.

*Alternative:* accept both formats during a transition. Rejected: there is one demo app, and
two formats double the validator and interpreter.

## Risks / Trade-offs

- [Pages get longer: nothing reusable by name, e.g. the customer list's row click is a full
  `open-page` with its `load`] → Accepted by the user (devdoc §5); reuse, if needed, belongs
  to composition (devdoc §9).
- [A class row change reaches every page at once, and a bad class row breaks every app that
  uses it] → Class rows are few and written by developers together with code; the bundle
  reports the class row as the descriptor at fault.
- [Loading all classes per bundle] → Bounded by design (about 100 classes at most); one
  indexed query per kind.
- [Format `0.1` descriptors written outside the seed stop loading] → None exist besides the
  demo app (checked in the database before migrating).

## Migration Plan

1. Merge code and migration together; the dev server applies the migration at its next
   restart (or `go run . --migrate-only` from `server/cmd/doc-service`).
2. Verify: `GET /api/v1/wpi/apps/customer-admin` returns 200, and the browser checks pass.
3. Rollback: goose down the migration, then revert the code.

## Open Questions

None; both gaps were settled on 2026-10-27.
