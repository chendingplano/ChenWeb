## Why

An element class's ID is also the name of the code that draws it, so every class needs code
of its own. Variants of one kind of element, such as `simple-button`, `fancy-button` and
`animated-button`, would need three copies of the button code, or one class whose descriptor
grows large and complicated. KnowledgeStore devdoc 2026102601 §3.5.1–§3.5.2 (decided
2026-10-28) separates the two: a class names its code, its *code module*, in
`code_module_id`, and several classes can share one module. Classes also need to fix
attributes for all their instances, and authors need to add attributes before the code
supports them. Today such an attribute breaks every app that uses the class: the user's
`button` version 3 adds `label_hori_align`, so the bundle check refuses `customer-admin`.

## What Changes

- **BREAKING** Element class rows move to format `0.4`. A new class attribute,
  `code_module_id`, names the class's code module. It is required in an active class and
  must name a code module that has code in this interpreter. A draft may leave it out or
  name a module whose code is not written yet.
- A new table, `kb.code_modules`, lists the code modules: ID, name, description, type
  (`element`, `action`, `page`, `layouter`), the file name of its document, notes and
  status (`active`, `suspended`). A `code_module_id` must name an active element module in
  it, in drafts and active classes alike, so the server checks what was otherwise only
  documented. `GET /api/v1/wpi/code-modules` lists it for the editor. It is seeded with
  the six element modules; rows come from migrations or `psql`.
- The frontend registry and the backend validator are keyed by code module, not by class ID.
- An attribute object may set `"instance_modifiable": false`. The attribute is then a class
  attribute: its `default_value`, which may not be `null`, is the value of every instance,
  and an instance that sets it fails validation. Attributes that hold actions cannot be fixed.
- A class row may name attributes its code module does not support. They are stored but
  have no effect, and the class writes and the app bundle report each one as a warning
  instead of refusing the class.
- An element instance may set only the attributes its class names, plus the size
  attributes, and none that the class fixes. Writing a page now checks its instances
  against the stored class rows, as the bundle does.
- A migration stores every element class's latest draft or active row again in format
  `0.4`, keeping the author's edits. Active rows get `code_module_id` equal to their class ID.
- In the WPI editor, the Create Element and Edit class form lists `code_module_id` with the
  active element modules of `kb.code_modules`, can fix an attribute, and shows the warnings
  a save returns. The Properties panel shows fixed attributes read-only. After a draft is
  created, the editor says pages can use it once an admin makes it active (it said "once
  its code is implemented", which is wrong for a class of an existing module).

## Capabilities

### New Capabilities
- `wpi-code-modules`: code modules, `kb.code_modules` and `code_module_id`, fixed
  attributes, warnings for attributes a module does not support, and the migration to
  format `0.4`.

### Modified Capabilities
- `wpi-classes`: element classes name their code module instead of being named after their
  code; class rows may fix attributes and name unsupported ones; instances set only what
  their class names.
- `wpi-descriptor-store`: element class rows are format `0.4`; page writes check instances
  against class rows; capabilities are checked by code module.
- `wpi-page-rendering`: elements are drawn by the code module their class names.
- `wpi-element-class-authoring` (from the unarchived change `wpi-create-element-class`):
  drafts and edits take `code_module_id` and fixed attributes, writes return warnings, and
  the form and Properties panel show them.

## Impact

- Backend `server/api/wpi`: `validate.go` (registry, class row, instance checks),
  `bundle.go` (class loading, warnings, `kb.code_modules` check), `handler.go` (page write
  checks instances, class writes check the module and return warnings, `ListCodeModules`),
  `store.go` (`kb.code_modules` queries), `types.go` (`ElementSchemaVersion` `0.4`), tests;
  route `GET /api/v1/wpi/code-modules` in `server/api/routes.go`.
- Frontend `web/src/lib/wpi`: `elements/index.ts` (`codeModules`), `Element.svelte`,
  `entries.ts`, `types.ts`, `api.ts`; editor `CreateElementDialog.svelte`,
  `element-class-form.ts`, `PropertiesPanel.svelte`; messages in English and Chinese.
- Migrations `project_migrations/20261028000001_wpi_element_classes_0_4.sql` and
  `20261028000002_create_kb_code_modules.sql`.
- Archive order: `wpi-create-element-class` must be archived before this change, since both
  modify `wpi-classes` and this one modifies its `wpi-element-class-authoring`.
- Docs: devdoc 2026102601 (status: built), reusable modules 20261026-01 and -02, the code
  module documents in `KnowledgeStore/doc-repo/code_modules/`.
