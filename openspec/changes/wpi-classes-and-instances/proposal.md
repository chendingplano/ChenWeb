## Why

The WPI pilot (`wpi-pilot`) stores page-specific elements and actions as descriptors of their
own (`customer-email`, `save-customer`, …), each naming a hard-coded kind in `element_type`
or `action_type`. Every new page therefore adds new stored definitions that authors must
search through, and stored descriptors mix definitions with what are really single uses.
Devdoc `2026102601-devdoc-web-page-interpreter` §3.5, §3.6 and §5 (revised 2026-10-27)
replace this with a class/instance model: a small, stable set of element and action
*classes*, each mapping to the code that implements it, and *instances* written inside the
pages that use them. This change implements that model.

## What Changes

- **BREAKING** Descriptor format `schema_version` `"0.2"` replaces `"0.1"`; the interpreter
  accepts only `"0.2"`.
- **BREAKING** `kind = 'element'` rows are element classes: `text`, `input`, `select`,
  `button`, `list`, `menu`. `kind = 'action'` rows are action classes: `load`, `save`,
  `open-page`, `close-page`, `save-page`. A class ID is the name of the code that implements
  it; `element_type` and `action_type` are removed. A class row holds the class attributes
  (`descriptor_id`, `schema_version`, `description`) and, for instance attributes, a default
  value or `null` (every instance must set it); instance attributes it leaves out are
  optional and have no default. The class's code defines which instance attributes exist.
- **BREAKING** Element instances are written in a page's `elements` as
  `{ "element": "<class>", ...instance attributes }`; action instances are written where
  they are used as `{ "action": "<class>", ...instance attributes }` (triggers, a list's
  `source`, a page's `input` and `save_action`, an `open-page`'s `input`). Stored elements,
  stored actions, inline element descriptors and references to actions by ID are removed.
- New action class `save-page`: runs the triggering panel's `save_action`, so a Save button
  and the unsaved-changes prompt save the same way.
- `label` (instance) and `description` (class) take a plain string or one text per
  language, `{ "en": …, "zh-cn": … }`, falling back to `en`.
- `POST /api/v1/wpi/actions/run` names an action instance only by its place in a stored page
  (`{kind: "page", id, pointer}`); the server merges it with its class before running it.
- The app bundle loads all active classes (one query per kind) and returns the classes the
  app uses.
- New migration seeds the classes, rewrites the `customer-admin` demo app as version 2 in
  the new format, and retires the pilot's stored elements and actions.
- Devdoc 2026102601 updated to match (kebab-case action classes, format `0.2`, the
  attribute rules above, §4 in the new format); devdoc 2026102603 and the WPI reusable
  module docs updated to describe what is built.

## Capabilities

### New Capabilities

- `wpi-classes`: element and action classes and their instances — class rows, class versus
  instance attributes, defaults and `null`, merging an instance with its class, where
  instances may appear, text attributes in several languages.

### Modified Capabilities

- `wpi-descriptor-store`: validation rules (capabilities are classes with code, instance
  checks, format `0.2`) and the app bundle (classes instead of stored elements and actions).
- `wpi-data-actions`: only action instances inside stored pages run; arguments of nested
  instances; `save-page`.
- `wpi-page-rendering`: elements are instances of the six element classes; labels in
  several languages.

`wpi-pilot` is not archived yet, so its specs under `openspec/changes/wpi-pilot/specs/` are
the base these deltas modify; archive `wpi-pilot` before archiving this change.

## Impact

- Backend `server/api/wpi`: `types.go`, `validate.go`, `bundle.go`, `actions.go`,
  `store.go` and their tests; `seed_test.go` validates the new seed.
- Frontend `web/src/lib/wpi`: `types.ts`, `entries.ts`, `runtime.svelte.ts`, the element
  registry and components, their tests; a text helper for multi-language labels.
- Database: new goose migration in `project_migrations/` (data only, no schema change).
  Existing apps in format `0.1` stop loading until rewritten; the demo app is the only one.
- Docs in KnowledgeStore: devdocs 2026102601 and 2026102603, modules 20261026-01 and
  20261026-02.
