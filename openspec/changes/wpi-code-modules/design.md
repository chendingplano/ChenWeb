## Context

WPI element classes are rows in `kb.wpi_descriptors` (format `0.3`). Today a class's ID is
also the key of its code, in two registries: `elementClasses` (frontend,
`web/src/lib/wpi/elements/index.ts`) and `elementAttrs` (backend, `server/api/wpi/validate.go`,
which lists what each piece of code accepts). Validation runs in two stages. Stage 1,
`ValidateDescriptor`, uses no database and checks a page's instances against the code of
their class, found by class ID. Stage 2, `pageInstances`, runs only in `BuildBundle` and
checks instances against the stored class rows.

Commit `kstv` (2026-10-28) made *Edit class* (`PUT /classes/element/:id`) structural only, so
an active class can name attributes its code does not know. The bundle still checks used
classes against code, so the user's `button` version 3, which adds `label_hori_align`,
makes `customer-admin` fail with "unknown attribute".

KnowledgeStore devdoc 2026102601 §3.5.1–§3.5.2 (decided 2026-10-28) describes the target. A
class names its *code module* in `code_module_id`. Each module is documented in
`KnowledgeStore/doc-repo/code_modules/` and has a row in a new table, `kb.code_modules`,
which the server can check (the server cannot see KnowledgeStore). Classes may fix
attributes with `"instance_modifiable": false`, and attributes a module does not support
are stored with a warning.

## Goals / Non-Goals

**Goals:**
- Classes name their code module; several classes can share one.
- Classes can fix attributes for all their instances.
- Attributes a module does not support are stored, have no effect and are reported as
  warnings, never as errors.
- Existing class rows, including the user's edits, move to format `0.4` unchanged otherwise.

**Non-Goals:**
- Any new code module, or new attributes in the existing six.
- An API that writes `kb.code_modules`: rows come from migrations or `psql`.
- Checking that a module's KnowledgeStore document exists: `module_doc` only names it.
- Action classes: their class ID still names their code. `kb.code_modules` allows action,
  page and layouter rows, but none are seeded and nothing checks them yet.
- Listing which pages break when a class is edited (devdoc open point).

## Decisions

### D1 `code_module_id` is a class attribute; element class format `0.4`

`classAttrs` gains `code_module_id`. `ElementSchemaVersion` becomes `"0.4"`: an element
class row without it, or in `0.3`, is refused, as `0.2` rows were refused when `0.3` came in.
In an active row (`ValidateDescriptor`, the bundle, *Edit class* of an active class),
`code_module_id` is required and the module must have code in this interpreter. A draft
(create, *Edit class* of a draft) may leave it out, and the module it names may have no
code yet. Either way a named module must be an active element module in `kb.code_modules`
(D9). An action class row that sets it is refused.

*Alternative:* accept `0.3` rows and read a missing `code_module_id` as the class ID. That
keeps two formats alive in every reader; the migration (D7) makes it unnecessary.

### D2 Registries keyed by code module

The backend's `elementAttrs` becomes `codeModules` (same contents, keyed by module ID); the
module-specific value checks (`element`, `elementComplete`) take a module ID. The frontend
`elementClasses` becomes `codeModules`. `resolveEntry` adds the class row's
`code_module_id` to `ResolvedElement` as `module`, and `Element.svelte` draws
`codeModules[element.module]`. An unknown module shows an error box naming the module.

### D3 Unsupported attributes are warnings

An element class row may name any well-formed attribute. `classWarnings(id, body)` returns
one warning for each instance attribute that the row's registered module does not define.
Size attributes are defined by every module. The warnings come back as `warnings` from the
three class writes:

- `PUT /descriptors/element/:id`, 200;
- `POST /classes/element`, 201, only when the draft names a registered module;
- `PUT /classes/element/:id`, 200.

They also appear in the bundle's `warnings` for the classes an app uses. `Issue` gets no
severity field: warnings are a separate list, as bundle warnings already are.

Attributes the module *requires* stay errors in `ValidateDescriptor` and the bundle: the
code cannot draw the class without them. *Edit class* keeps `kstv`'s rule and stores an
active class that lacks them; its bundle then fails, as it does today.

### D4 Fixed attributes

An attribute object may hold `instance_modifiable` (boolean; absent means `true`). When it
is `false`:

- `default_value` must be present and not `null`;
- the attribute must not hold actions (`actionHolding`);
- an instance that sets the attribute fails stage 2 with "attribute %q is fixed by element
  class %q; an instance cannot set it";
- merging puts the fixed values last, in Go and TS, so they win even if a check were missed.

A page-wide `binding_mode` fills in only when the class names `binding_mode` and does not
fix it (D6). Otherwise it would make every instance of a class that fixes `binding_mode`
fail.

### D5 Instances are checked against their class row, at page write too

The attributes an instance may set are those its class row names, plus the size attributes
the layouter applies to every element. The class row, not the code, is the authority,
because a class may name unsupported attributes (D3) or leave out ones the code accepts.
Finding the module of an instance needs its class row, so the element-instance checks move
from stage 1 to stage 2:

- Stage 1 (`ValidateDescriptor`, no database) checks only that `element` is a well-formed ID,
  that no class attribute is set, and the shared attributes: sizes, `label`,
  `data_binding`, `binding_mode`.
- Stage 2 (`againstClass` and `pageInstances`) checks:
  - that the class exists and is active;
  - which attributes the instance may set, and that it sets no fixed one;
  - the values that need the module (`element(module, own)`);
  - completeness (`elementComplete(module, merged)`).

`PutDescriptor` for a page loads the active element and action classes, one query per kind
(the bundle loads them the same way, through a shared helper). It runs stage 2, so a page
that names an unknown or draft class is still refused when written.

`actionPositions` no longer asks the code whether an element holds an action: `on_click`,
`source`, `on_row_click` and menu-style `items` are action positions on any element, as
their names are already reserved by `actionHolding`. The action endpoint, page load and
validation all use it without class rows.

*Alternative:* keep stage 1 strict by mapping class to module in a static table. That would
duplicate the database, and the mapping changes whenever an admin creates a class.

### D6 Page `binding_mode` follows the class row

`withPageDefaults` (Go) and `resolveEntry` (TS) fill in the page's `binding_mode` when the
instance does not set it, the class row names `binding_mode`, and the class does not fix
it. Today Go asks the code and TS asks the row; both now ask the row.

### D7 Migration transforms rows instead of reseeding them

`20261028000001_wpi_element_classes_0_4.sql` inserts, for every element class whose latest
draft or active row is format `0.3`, a new version: the same body with
`"schema_version": "0.4"`, plus `"code_module_id": <class ID>` when the row is active
(today's class ID is its code). The status is kept, the version is the class's
`MAX(version) + 1`, and `created_by` is `migration:wpi-code-modules`. `ON CONFLICT DO
NOTHING` makes a concurrent second run harmless, and the down migration deletes those rows.
The user's `button` version 3 thus becomes version 4 with `label_hori_align` kept, which is
now a warning (D3). `seed_test.go` applies the same transform to the seeded bodies, so the
demo app is still checked in the format the database will hold. The migration is dry-run
against the dev database inside `BEGIN … ROLLBACK` before it is applied.

*Alternative:* store the six classes again from literal JSON, like migration
`20261027000006`. That would overwrite the user's edits.

### D8 Editor

- `element-class-form.ts`:
  - the class attributes are `descriptor_id`, `schema_version` (`0.4`), `code_module_id`,
    `description` and `implement_notes`;
  - `code_module_id` is text, optional, and left out when empty;
  - an instance attribute's fields gain *fixed*, which `newAttribute`, `attributeFields` and
    `attributeFromFields` carry as `instance_modifiable: false`;
  - *fixed* excludes *required*, and a fixed attribute with no default is refused.
- `CreateElementDialog.svelte`:
  - while `code_module_id` is being edited, a hint lists the active element modules of
    `kb.code_modules` (`GET /api/v1/wpi/code-modules`, D9);
  - fixed attributes carry a "fixed" tag;
  - a save that returns warnings keeps the dialog open, shows "Saved" with the warnings, and
    closes through `onSaved` when the author clicks OK.
- `PropertiesPanel.svelte` shows a fixed attribute's value read-only, with "Fixed by the
  class". `ClassSummary` leaves out `code_module_id`, as it does every class attribute.
- All new text goes through Paraglide, in both languages. The message after creating a
  draft says pages can use it once an admin makes it active; it said "once its code is
  implemented", which is wrong for a class of a module that already has code.

### D9 `kb.code_modules`

The user's table (devdoc §3.5.2). Its columns are:

- `code_module_id`: primary key, ID pattern;
- `module_name`: unique, not empty;
- `description`;
- `module_type`: `element`, `action`, `page` or `layouter`;
- `module_doc`: the document's file name in `doc-repo/code_modules`;
- `notes`;
- `status`: `active` or `suspended`;
- `creator`, `created_at`, `updated_at`.

There is no write API, as for `kb.stored_stmts`. Migration
`20261028000002_create_kb_code_modules.sql` creates it and seeds the six element modules.
`TestSeedCodeModules` checks that the seeded element modules and the backend's
`codeModules` list the same IDs, each with `<id>.md`.

A `code_module_id` is checked through the bundle's existing `Registered` lookup, under a new
kind, `storedModule`, whose query finds active element modules. `moduleErrors` reports each
class row whose module is not one: "code module %q is not an active element module in
kb.code_modules". It runs:

- in the three class writes (`checkModule`), after the structural checks;
- in `BuildBundle`, for the element classes an app uses.

A suspended module therefore stops new classes from naming it and the apps that use its
classes from loading. Code (D1) and row are separate checks: a draft may name a module that
has a row but no code yet, which is how a new module starts. `GET /api/v1/wpi/code-modules`
(logged-in users) returns every row for the editor.

*Alternative:* list valid modules only in code (D2) and the documents. The server cannot see
KnowledgeStore, so "the document comes first" could not be checked; the user chose the table.

## Risks / Trade-offs

- [A class silently does nothing with an attribute] → Warnings at every class write and in
  the bundle; the code module documents list what each module supports.
- [Page writes now query the database] → Two indexed queries over a few dozen rows, the same
  as a bundle load.
- [Two unarchived changes modify `wpi-classes`] → Archive `wpi-create-element-class` first;
  this change's deltas are written against its text.
- [Frontend and backend registries can disagree, or with kb.code_modules] → They list the
  same six modules today; `TestSeedCodeModules` ties the table's seed to the backend
  registry, and each module document names every place a module is registered.
- [Suspending a module stops every app that uses one of its classes] → That is what
  suspending is for; the bundle error names the class and the module.

## Migration Plan

1. Dry-run both migrations' Up sections against the dev database in `BEGIN … ROLLBACK`.
   Check that the latest class rows are `0.4` with `code_module_id`, and that
   `kb.code_modules` holds six active element rows.
2. Deploy the code and the migrations together: the old code refuses `0.4` rows, and the
   new code refuses `0.3` rows and needs `kb.code_modules`.
3. Apply them with `doc-service --migrate-only`; load `/wpi/customer-admin` and check that
   it returns 200, with one warning for `button`'s `label_hori_align`.
4. Rollback: run the down migrations in reverse order. `20261028000002` drops the table;
   `20261028000001` deletes the `0.4` rows, so the `0.3` rows are latest again. Then deploy
   the previous code.

## Open Questions

None blocking. Later: listing the pages a class edit would break; rows, and checks, for
action, page and layouter modules.
