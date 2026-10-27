## Why

Devdoc 2026102601 §3.2 (2026-10-27) settled how WPI pages work: pages have no classes and are
reused by copying, so authors need to find pages by name and description; pages may carry
static data; and data may come from named SQL statements and stored files, because some
customers' firewalls block traffic that carries SQL text. None of this is built: pages are
`page` rows of `kb.wpi_descriptors`, have no names or descriptions, and get input only as `{}`
or a `load` instance.

## What Changes

- Pages move to their own table, `kb.frontend_pages` (same columns as `kb.wpi_descriptors`
  minus `kind`). A migration moves the existing page rows, and `kb.wpi_descriptors` keeps
  apps and element and action classes. The admin write endpoint keeps its URL
  (`PUT /api/v1/wpi/descriptors/page/:id`) and writes pages to the new table.
- New page attributes: `page_name` and `description` (text in one or several languages),
  `page_alignment`, `width`, `height`, `min_width`, `max_width` (the page inside its pane),
  and a page-wide default `binding_mode` for its elements (instance, then page, then class).
- A page's or `open-page`'s `input` may also be static data: any object without an `action`
  attribute is the input doc itself.
- New action class `query`: runs a SQL statement stored in the new table `kb.stored_stmts`,
  named by `sql_stmt_id`, with `params` bound as query parameters, in a read-only
  transaction. Allowed as an `input` and as a list's `source`.
- New action class `load-file`: returns a JSON doc stored in the new table
  `kb.stored_files`, named by `file_id`. Allowed as an `input`.
- Both tables have `access_controls`: the user roles that may use the row; null, empty or
  `all` means everyone. Neither has a write API, so SQL text never travels over HTTP.
- The bundle check reports `query` and `load-file` instances that name unregistered rows.
- New endpoints for authors: `GET /api/v1/wpi/pages?q=` searches pages by ID, name and
  description; `GET /api/v1/wpi/pages/:pageId` returns a page's latest descriptor (to copy
  it).
- The demo app gains a customer directory (a `query` list), an About page (`load-file`,
  centred, read-only) and a header with static data, and names and describes its pages.

## Capabilities

### New Capabilities

- `wpi-pages`: pages without classes: their attributes, storage in `kb.frontend_pages`,
  search and read endpoints, size and alignment in the pane, page-wide `binding_mode`, and
  static input.
- `wpi-stored-sources`: `kb.stored_stmts` and `kb.stored_files`, the `query` and
  `load-file` action classes, and their access controls.

### Modified Capabilities

- `wpi-descriptor-store`: pages are no longer stored in `kb.wpi_descriptors`; the bundle
  also checks that named statements and files are registered.
- `wpi-classes`: the classes each action position accepts (`input` takes static data,
  `load`, `query`, `load-file`; `source` takes `load` or `query`); `page_name` and a page's
  `description` are text attributes.
- `wpi-data-actions`: the run endpoint runs `query` and `load-file` instances as well as
  `load` and `save`.

## Impact

- Backend `server/api/wpi`: store (table per kind), validator (page attributes, positions,
  new classes), bundle (registry check), actions (query, load-file, access), handler (page
  search and read), routes in `server/api/routes.go`.
- Frontend `web/src/lib/wpi`: types, page input (static docs), page size and alignment,
  page-wide `binding_mode` in element merging.
- Migrations: create `kb.frontend_pages` and move pages; create `kb.stored_stmts` and
  `kb.stored_files`; seed the `query` and `load-file` classes and the demo additions.
- Docs: devdoc 2026102601 (§3.2 status, §5.1, §6, §9), devdoc 2026102603, reusable modules
  20261026-01 and 20261026-02.
