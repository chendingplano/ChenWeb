## Context

Devdoc `KnowledgeStore/doc-repo/devdocs/202610/2026102601-devdoc-web-page-interpreter.md`
is the design source; its §8 defines this pilot and its §3–§7 define the concepts used here
(WPI app, layout template, pane, panel, page, element, layouter, action, input doc). This
document fixes the technical choices and the exact descriptor format the pilot accepts.

Current state: no descriptor-driven UI exists. Relevant existing pieces:

- Backend handlers live in `server/api/<package>/`, registered in `server/api/routes.go` on
  the `/api/v1` group, which already runs `authmiddleware.AuthMiddleware`. Handlers use the
  shared pool `ApiTypes.ProjectDBHandle` and `loggerutil.CreateDefaultLogger(loc)`;
  `pageconfighandler` shows the admin check (`user.Admin || user.IsOwner || "admin" role`).
- `lib/pq` is a direct dependency (`pq.QuoteIdentifier` is already used);
  `server/api/datasync/schema_introspect.go` shows `information_schema` lookups.
- Frontend is SvelteKit with `adapter-static`, Svelte 5, Tailwind 4, bits-ui based
  `components/ui` (button, input, select, dialog, resizable, …), Paraglide i18n enforced by
  `bun run check`. Frontend unit tests are `*.test.ts` run with `bun test`.
- Unauthenticated HTML requests to frontend paths are redirected to `/login` by the
  middleware in `routes.go`, so a new route needs no extra auth code.

## Goals / Non-Goals

**Goals:**

- The `customer-admin` app of devdoc §4 runs from descriptors stored in
  `kb.wpi_descriptors` and meets the four "done when" checks of devdoc §8.
- The interpreter has no page-specific code: adding a page means adding descriptors.
- Validation catches broken descriptors before an app is shown, and nothing fails
  silently.

**Non-Goals:**

- Timed refresh, panel age-out, editable lists, lists fed through the input doc, page
  state, conditions, sequences, expressions, appearance attributes, element classes.
- i18n of text inside descriptors; a ChenWeb menu entry for WPI apps.
- A descriptor editor UI; LLM-generated descriptors; staging → production descriptor copy.
- Conflict detection on save (last save wins); per-action permissions.
- Moving code to `shared/` (only when a second project needs it).

## Decisions

### D1. Code layout

- Backend: one package `server/api/wpi` — `types.go` (descriptor structs),
  `validate.go`, `refs.go` (`$args` substitution), `catalog.go` (table/column existence),
  `sqlbuild.go` (pure SQL + argument building), `store.go` (descriptor queries),
  `handler.go` (HTTP).
- Frontend: `web/src/lib/wpi/` — pure TypeScript logic (`refs.ts`, `dirty.ts`,
  `panels.ts`, `layout.ts`, `api.ts`, `types.ts`) kept separate from Svelte components
  (`WpiApp.svelte`, `templates/`, `Pane.svelte`, `Panel.svelte`, `layouters/`,
  `elements/`, `ErrorBox.svelte`, `UnsavedDialog.svelte`) so the logic is unit-testable
  with `bun test`. Route: `web/src/routes/wpi/[appId]/+page.svelte` with `ssr = false`.

### D2. One bundle request per app

`GET /api/v1/wpi/apps/:appId` returns the app descriptor and every page, element and action
descriptor reachable from it, after validation. The frontend makes no further descriptor
requests.

- *Why:* one round trip; validation happens in one place over the whole graph, so
  reference and pane errors are caught before anything is drawn.
- *Alternative:* fetch each page when first opened. Better for large apps, unnecessary for
  the pilot; the bundle can be split later without changing descriptors.

Reachability: the app's pane pages; each page's element references; each element's trigger
actions, list sources and menu items; `open_page` targets and their `input` load actions;
each page's `save_action`. Traversal tracks visited IDs, so cycles terminate.

### D3. Validation lives in Go only

The Go validator runs on `PUT` (structure and capabilities of the one descriptor) and on
the bundle request (the whole reachable graph: structure, references, panes, tables and
columns, bindings, the consistency warning). The frontend trusts a validated bundle but
still renders an `ErrorBox` for anything it cannot draw.

- *Why:* one implementation of the rules; the backend must validate anyway because it runs
  `load`/`save`.
- *Alternative:* a shared JSON Schema for Go and TypeScript. More machinery than the pilot
  needs; worth revisiting when LLMs generate descriptors.
- `PUT` does not check references, so descriptors can be written in any order; the bundle
  request reports dangling references.

### D4. Actions are invoked by reference; `$args` is resolved on the server

`POST /api/v1/wpi/actions/run` takes either `{"action": "<id>"}` (a stored action
descriptor) or `{"page": "<id>", "pointer": "<JSON Pointer>"}` (an inline action inside a
stored page, RFC 6901 pointer to the trigger object), plus `args` (values only) and, for
`save`, `input_doc`.

- The browser resolves `$record.*` and `$input_doc.*` inside a trigger's `args` before
  calling; action descriptors may only reference `$args.*`, which the server substitutes.
  So the browser chooses values, never tables, columns or SQL.
- *Alternative:* send the action descriptor from the browser. Rejected: any user could then
  reach any table with crafted JSON.

### D5. SQL building

- `table` is `name` or `schema.name` (default schema `public`); table and every column are
  checked against `information_schema.columns` on each run and quoted with
  `pq.QuoteIdentifier`. No caching in the pilot (traffic is tiny).
- Values are always `$n` parameters. Request JSON is decoded with `UseNumber`, and numbers
  are passed as strings so PostgreSQL converts them to the column's type; strings,
  booleans and nulls pass through. Supported column types in the pilot: text-like,
  numeric, boolean, date/time; others fail with a clear error.
- Scanned values: `[]byte` → string, `time.Time` → RFC 3339 string, others as-is.
- `load` list results default to 200 rows, capped at 1000.
- SQL text is built by pure functions in `sqlbuild.go`, unit-tested without a database.

### D6. Save semantics

- `key` names the key column; the input doc attribute of the same name holds its value.
- `mode`: `insert` → `INSERT … RETURNING <key>`; `update` → `UPDATE … WHERE <key> = $n`
  (zero rows → 404 "record not found"); `upsert` → insert when the key attribute is
  missing or empty, update otherwise. No version check: last save wins.
- Server-side rules per field: `required` (non-empty after trimming strings), `max_length`,
  `pattern` (Go RE2, anchored as written), `format: "email"`. Failures return
  `422 {"field_errors": {"<attribute>": ["required" | "max_length" | "pattern" | "format"]}}`.
  Codes, not sentences, so the browser shows translated messages (`wpi_err_*` keys).
- On success: `{"key_attribute": "<key>", "key_value": …}`; the browser writes the key into
  the input doc and takes a new clean snapshot.

### D7. Descriptor storage

One table for all kinds: `kb.wpi_descriptors(id, kind, descriptor_id, version,
schema_version, body JSONB, status, created_by, updated_by, created_at, updated_at)`,
`UNIQUE (kind, descriptor_id, version)`, `CHECK` constraints on `kind`
(`app|page|element|action`) and `status` (`draft|active|retired`), index on
`(kind, descriptor_id, version DESC) WHERE status = 'active'`. Versions are append-only;
`PUT` inserts `max(version)+1` as `active`; the latest active version wins
(`DISTINCT ON (kind, descriptor_id) … ORDER BY version DESC`).

- *Alternative:* one table per kind. No benefit while bodies are JSONB and queries are
  identical.

### D8. Frontend runtime and panel bookkeeping

- `panels.ts` (pure) holds per-pane panel lists, the visible panel, menu bindings
  (`menu instance → item_id → panel instance ID`) and the replace decision. The sharing key
  of a panel is `page ID + canonical JSON of its resolved input`; menu items whose
  `open_page` resolves to the same key bind to the same panel (devdoc §10 rule 1).
- Showing a new panel in a pane: if the current panel is bound to a menu item it is
  hidden; otherwise it is closed first, through the unsaved-changes check (rule 2). If the
  user chooses "keep editing", the open is cancelled.
- `runtime.svelte.ts` wraps that logic in Svelte 5 `$state`; each panel's input doc is a
  deep `$state` object and its clean snapshot a canonical JSON string (`dirty.ts`).
- Hidden panels stay mounted and are hidden with CSS, so element state survives.
- Refresh on show: each panel has a `shown` counter incremented when it goes from hidden to
  visible; lists re-query when it changes, and a page whose input came from a load action
  reloads it unless the doc is dirty (rule 3).

### D9. Layout templates and the flow layouter

- Templates are Svelte components in a registry (`single-pane`, `top-left-middle-right-
  bottom`). The five-region template uses CSS grid rows (top / middle / bottom) and, in the
  middle row, the existing `resizable` pane group; regions the app does not mention are not
  rendered. Pane geometry attributes map to CSS sizes and the pane group's min/max.
- `flow-layouter` is a wrapping flex container. Width mapping: fixed → `flex: 0 0 <w>`;
  `elastic` → `flex: 1 1 <min_width or 0>` with `min-width`/`max-width`; `auto` →
  `flex: 0 0 auto`. The browser recomputes widths on pane resize.

### D10. Elements

A registry maps `element_type` to a Svelte component receiving the resolved descriptor and
its panel. Components wrap `components/ui` parts (`input`, `select`, `button`, the list as a
simple table, the menu as a nested list with folder toggles). An element whose type is not
in the registry renders an `ErrorBox`.

### D11. Unsaved changes and leaving

One `UnsavedDialog` in the app shell offers save / discard / keep editing. `closePanel`
returns a promise of whether the panel closed; "save" runs the page's `save_action` and
closes only on success. A `beforeunload` handler warns while any panel (visible or hidden)
is dirty.

### D12. Route, text and selection

- `/wpi/[appId]` is outside the home3 dashboard shell; its root sets `select-text`.
- The interpreter's own text (dialog, error boxes, field error messages, loading and
  empty states) uses Paraglide keys prefixed `wpi_`; text from descriptors is rendered as
  data and is not translated in the pilot.

### D13. Demo data

A migration creates `kb.wpi_demo_customers (id BIGSERIAL PRIMARY KEY, user_name, phone_number,
email, created_at, updated_at)` with a few rows; a seed migration inserts the
`customer-admin` descriptors (version 1, `active`, `ON CONFLICT DO NOTHING`). The demo
actions name `kb.wpi_demo_customers`.

### D14. Logging and testing

- Each Go file gets its own `CreateDefaultLogger("20261026-NNN")` location. Every action run
  logs action reference, user, table, mode, row count, duration and error.
- Go tests use `go-sqlmock` (as `reusablemodules` does). No test touches the `miner`
  database.
- Pure TypeScript is covered by `bun test`; the four §8 checks are run in a browser against
  the dev server.

## Descriptor format, schema_version "0.1"

Every descriptor has `descriptor_id` (`^[a-z0-9][a-z0-9-]{0,127}$`) and `schema_version`
`"0.1"`; unknown attributes are errors.

| Kind | Attributes |
| --- | --- |
| app | `layout_template`; `panes`: map of pane ID → `{width, height, min_width, max_width, min_height, max_height, resizable, scroll ("auto"\|"none"), page}` (all optional) |
| page | `layouter` (`flow-layouter`); `elements` (array); optional `input` (`{}` or `{action, args}`), `save_action`, `refresh: {on_show: bool}` |
| element (all) | `element_type`; optional `width`, `height`, `min_width`, `max_width` |
| `text` | `text` or `data_binding` |
| `input` | `label`, `data_binding`, `binding_mode`; optional `input_type` (`text`\|`number`\|`email`) |
| `select` | `label`, `data_binding`, `binding_mode`, `options: [{value, label}]` |
| `button` | `label`, `on_click` |
| `list` | `source: {action, args?}`, `presentation` (`rows`), `columns: [{field, label, width?}]`; optional `on_row_click` |
| `menu` | `items: [{item_id, label, on_click} \| {item_id, label, children}]`; optional `initial_item` |
| action `load` | `table`, `result` (`record`\|`list`), `fields: [{column, attribute}]`; optional `where: {column: value}`, `order_by: [{column, direction}]`, `limit` |
| action `save` | `table`, `mode` (`insert`\|`update`\|`upsert`), `key`, `fields: [{attribute, column, required?, rules?: {max_length?, pattern?, format?}}]` |
| action `open_page` | `page`, `pane`; optional `input` |
| action `close_page` | — |

- A page's element entry is either `{"element": "<id>", …overrides}` (any element attribute
  except `element_type` and `descriptor_id`) or an inline element with a page-unique
  `descriptor_id`.
- A trigger (`on_click`, `on_row_click`, menu `on_click`) is `{"action": "<id>", "args"?}` or
  an inline action `{"action_type": …, …, "args"?}`.
- References are whole-string values: `$input_doc.<path>`, `$record.<field>`,
  `$args.<name>`. Path segments match `^[A-Za-z_][A-Za-z0-9_]*$`; `__proto__`,
  `constructor` and `prototype` are rejected. `data_binding` allows only `$input_doc`;
  trigger `args` allow `$input_doc` and, in a list's `on_row_click`, `$record`; action
  bodies allow only `$args`.
- Pane IDs: `single-pane` → `main`; `top-left-middle-right-bottom` → `top`, `left`,
  `middle`, `right`, `bottom`.

## API

| Method and path | Who | Request | Response |
| --- | --- | --- | --- |
| `GET /api/v1/wpi/apps/:appId` | logged-in | — | 200 `{app, pages, elements, actions, warnings}` (maps by ID); 404 unknown app; 422 `{errors: [{kind, descriptor_id, path, message}], warnings}` |
| `PUT /api/v1/wpi/descriptors/:kind/:descriptorId` | admin | descriptor body | 200 `{kind, descriptor_id, version}`; 400 validation errors; 403 non-admin |
| `POST /api/v1/wpi/actions/run` | logged-in | `{action}` or `{page, pointer}`, `args`, `input_doc` | `load`: 200 `{result}`; `save`: 200 `{key_attribute, key_value}` or 422 `{field_errors}`; 400 bad reference or arguments; 404 record not found |

## Risks / Trade-offs

- [Any logged-in user can run any stored action against any table] → by decision (devdoc
  §5.2). Only admins write descriptors, every run is logged, and WPI apps are not linked
  from ChenWeb menus. Revisit with permission capabilities before wider use.
- [Catalog lookups on every action run add latency] → negligible at pilot traffic; add a
  short-lived cache if measurements show a need.
- [Text-parameter coercion surprises for unusual column types] → pilot supports a stated
  set of types and fails clearly on others.
- [Strict "unknown attribute" validation blocks forward-compatible descriptors] → the
  `schema_version` gate is the intended way to evolve the format.
- [Hidden panels stay mounted and accumulate memory] → acceptable without age-out in the
  pilot; age-out is the next step (devdoc §9).
- [Untranslated descriptor text breaks the "every page in two languages" rule] → WPI apps
  get no menu entry until descriptor i18n is decided (devdoc §9).
- [Go RE2 `pattern` semantics differ from JavaScript regex] → patterns are checked on the
  server only in the pilot.

## Migration Plan

- Three migrations in `project_migrations/`: create `kb.wpi_descriptors`; create and fill
  `kb.wpi_demo_customers`; seed the `customer-admin` descriptors. The live `mise dev`
  server applies them on save, so each file is written complete in one step.
- Rollback: the `Down` sections drop the seed rows and both tables. Nothing else depends on
  them.
- Production: the migrations ship with the binary like any other; no menu entry exposes
  the demo app.

## Open Questions

None blocking. Noted for later: restricting which actions a user may run (for example only
those reachable from an app they opened), and a shared schema when LLMs start writing
descriptors.
