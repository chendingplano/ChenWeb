## Why

Every ChenWeb page is hand-written Svelte, so frontend code grows with the number of pages.
Devdoc `2026102601-devdoc-web-page-interpreter` (the WPI guiding document) proposes a Web
Page Interpreter: apps, pages, elements and actions described as JSON descriptors stored in
the database and drawn by one interpreter. This change builds the pilot defined in its §8 —
a menu-driven app whose list and form pages load and save records, built only from
descriptors — to prove the model before adding more element types, actions or state.

## What Changes

- New `kb` tables (goose migrations): `kb.wpi_descriptors` (versioned app, page, element and
  action descriptors) and `kb.wpi_demo_customers` (the pilot's demo table), plus seed rows
  for the demo `customer-admin` app.
- New Go package `server/api/wpi`:
  - descriptor validation (structure, references, pane IDs against the layout template,
    tables and columns against the database catalog, binding paths, capabilities);
  - `GET /api/v1/wpi/apps/:appId` returns an app's descriptor and every descriptor it
    reaches, validated, in one response;
  - `PUT /api/v1/wpi/descriptors/:kind/:descriptorId` (admins) validates a descriptor and
    stores it as a new version;
  - `POST /api/v1/wpi/actions/run` runs a stored `load` or `save` action against any table.
    The browser names a stored action and supplies argument values only; it never sends a
    table name, column name or descriptor body.
- New frontend interpreter in `web/src/lib/wpi/` and route `web/src/routes/wpi/[appId]`:
  - layout templates `single-pane` and `top-left-middle-right-bottom`;
  - panes holding several panels, menu-item → panel bindings (items that open the same page
    with the same input share a panel), hidden panels kept only while bound, refresh on
    show;
  - element types `text`, `input`, `select`, `button`, `list`, `menu`; the `flow-layouter`
    with fixed, elastic and auto widths;
  - `$input_doc` data binding (`read-only` / `read-write`), dirty tracking, an
    unsaved-changes prompt (save / discard / keep editing) and a browser leave warning;
  - actions `open_page` and `close_page` in the browser, `load` and `save` through the API.
- New Paraglide keys (`wpi_*`) for the interpreter's own text. Text inside descriptors is not
  translated in the pilot (devdoc §9), so WPI apps get no ChenWeb menu entry.
- Not in this change: timed refresh, panel age-out, editable lists, page state, conditions,
  i18n of descriptor text, a descriptor editor UI, embedding an app in an ordinary page,
  conflict detection on save (last save wins).

## Capabilities

### New Capabilities

- `wpi-descriptor-store`: versioned descriptor storage, the validation rules, the admin write
  endpoint and the app bundle endpoint.
- `wpi-data-actions`: server-side `load` and `save` execution — stored-action-only
  invocation, argument substitution, catalog checks, parameterized SQL, server-side
  validation with per-attribute errors, key write-back, last save wins.
- `wpi-app-shell`: opening an app by URL, layout templates and panes, panels and their
  visibility, menu bindings and panel sharing, closing and replacing panels, refresh on show.
- `wpi-page-rendering`: element types, the flow layouter, data binding, the input doc
  lifecycle with dirty tracking and the unsaved-changes prompt, error boxes.

### Modified Capabilities

None.

## Impact

- **Database:** two new `kb` tables and one seed migration in `project_migrations/`. The
  live `mise dev` server applies migrations on save, so migration files must be complete
  before they are written.
- **Backend:** new package `server/api/wpi`; four routes added to the authenticated
  `/api/v1` group in `server/api/routes.go`. No change to existing handlers.
- **Frontend:** new `web/src/lib/wpi/` and `web/src/routes/wpi/[appId]/`; new message keys in
  `web/messages/en.json` and `zh-cn.json`. Reuses existing `components/ui` parts (button,
  input, select, dialog, resizable). No change to existing pages.
- **Security:** `load`/`save` may touch any table, by decision (devdoc §5.2). The safeguard is
  that only descriptors stored in `kb.wpi_descriptors` run, only logged-in users can run
  them, and only admins can write descriptors.
- **Docs:** devdoc 2026102601 is the design source; a short post-build devdoc and a
  `KnowledgeStore` module entry follow once the pilot is verified.
