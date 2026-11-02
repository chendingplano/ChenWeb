# WPI Class tab

The WPI Editor replaces its Node tab with a Class tab. It reads the latest stored
class, independently of an instance's frozen class copy. A selected element uses
its element class, an action uses its action class, a pane uses the page layouter,
and a panel uses its element layouter. Pages and flow-only nodes have no class.

Attribute names are disclosure buttons in a recursive tree. Their inline forms
edit name, description, class/instance scope, default value, value editor and its
configuration. Descriptor IDs and schema versions retain their identity rules.
Defaults retain their JSON types and optional/required distinctions. Editor choices
include the documented editors, including those whose runtime implementation is
still pending; the latter are identified in the UI.

Optional `attribute_definitions` metadata maps JSON pointers into the logical
attribute tree to `{description, scope, value_editor: {id, options}}`. It is editor
metadata, excluded from runtime defaults. Top-level element attributes marked class are
fixed through `instance_modifiable: false`; existing class identity attributes
stay class attributes. Existing descriptors without metadata keep their behavior.
Nested element defaults use logical paths, e.g. `/source/table`, rather than exposing
the `default_value` storage wrapper. Renaming moves the subtree's metadata.

Save explicitly updates the loaded record, retaining version and draft/active
status, using authenticated, admin-only write endpoints and existing validation.
Unsaved class drafts are retained by kind and ID while changing selection, canvas
tabs, Properties/Class tabs, sidebar views or preview mode. Browser unload and
route navigation warn before discarding drafts. Explicit Discard changes asks
for confirmation.
Class saves refresh the available class lists, without rewriting existing copies.
The node toolbar's Edit class opens the same Class tab. An action-class selector
assigns a class to action boxes, including classes not supported by click triggers.

The Class/Instance flag records authoring scope for nested fields and non-element
classes. It does not impose new runtime constraints on action or layouter copies.
For top-level element attributes it also uses the existing fixed-default mechanism.
The UI explains this distinction beside the scope control.

## Implementation plan

- [x] Add backend coverage for metadata validation and generic class read/update;
      implement the endpoints without changing the existing element endpoints.
- [x] Test selection mapping, recursive attribute edits, metadata preservation,
      scope/default conversion and editor configuration; implement the model.
- [x] Build the tree, inline forms and configuration controls; integrate tabs and
      selection handling, using the existing panel styles and Paraglide messages.
- [x] Update the two WPI devdocs and the behavior contract, run Go/Bun tests,
      `bun run check`, review the changes and commit only this task through jj.

## Validation

- `go test ./server/api/wpi` and `go vet ./server/api/wpi` pass.
- `go test ./server/api -run '^$'` compiles the API routes.
- Targeted WPI/canvas/value-editor Bun suite: 196 tests pass.
- `bun run check`: no Svelte errors or warnings; English/Chinese checks pass.
- Both `web/tests/wpi-class-editor/test_editor.py` and `test_full_editor.py` pass
  against mocked HTTP APIs, including actual editor action assignment and preview
  draft retention. No browser errors or panel overflow.
- WPI frontend/backend module records and documents agree on versions and files.
