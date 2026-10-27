## Context

WPI (devdoc 2026102601, OpenSpec changes `wpi-pilot` and `wpi-classes-and-instances`, both
archived) stores apps, pages and element and action classes in `kb.wpi_descriptors`, one row
per version. Pages are written by admins through `PUT /api/v1/wpi/descriptors/:kind/:id`,
checked by the validator in `server/api/wpi/validate.go`, and served to the browser in the
app bundle. Action instances that touch data (`load`, `save`) run on the server through
`POST /api/v1/wpi/actions/run`, named by page and JSON Pointer; the browser never sends a
table name or an action body.

Devdoc §3.2 (2026-10-27) decided: pages have no classes and are reused by copying; pages are
stored in `kb.frontend_pages`; pages get names, descriptions, size, alignment and a
page-wide `binding_mode`; `input` may be static data, a `load`, a `query` (named SQL
statement) or a `load-file` (stored file). The user then chose the registries:
`kb.stored_stmts (stmt_name, sql_stmt, description, creator, access_controls)` and "the same
for files". The WPI editor (the user's work in `web/src/lib/components/shared-ui/canvas`)
will need to find and copy pages.

## Goals / Non-Goals

**Goals:**

- Pages live in `kb.frontend_pages`, with the existing page rows moved there, and every
  existing endpoint keeps working.
- The page attributes, input forms and action classes of devdoc §3.2, validated as strictly
  as the rest of the format.
- Named SQL statements and stored files that never travel over HTTP as text, with role-based
  access, run read-only.
- Endpoints an editor needs to search pages and copy one.
- The demo app exercises every new feature, so a browser check covers them.

**Non-Goals:**

- Write APIs for `kb.stored_stmts` and `kb.stored_files` (rows come from migrations or
  `psql`).
- Any editor UI; showing `page_name` in the interpreter.
- Versioning of statements and files; lineage between copied pages.
- Moving apps or classes out of `kb.wpi_descriptors`.

## Decisions

**D1. `kb.frontend_pages` mirrors `kb.wpi_descriptors` without `kind`.** Columns `id`,
`descriptor_id`, `version`, `schema_version`, `body`, `status`, `created_by`, `updated_by`,
`created_at`, `updated_at`; `UNIQUE (descriptor_id, version)`; the same checks and a partial
index for the latest active version. The store picks the table by kind (`page` →
`kb.frontend_pages`, others → `kb.wpi_descriptors`), so `Loader`, `PutDescriptor` and the
bundle are unchanged above the store. The migration copies every page row (all versions,
with their audit columns), deletes them from `kb.wpi_descriptors` and narrows its `kind`
check to `app`, `element`, `action`, so a page cannot be written to the old table by mistake.
*Alternative:* a new endpoint for pages — rejected; the editor and admins already use the
descriptor endpoint and the kind in its URL is enough.

**D2. Format stays `0.2`.** Every new attribute is optional and every new input form is new,
so every stored `0.2` descriptor stays valid. There is one interpreter, deployed with its
descriptors, so a reader that does not know the new attributes cannot meet them.
*Alternative:* `0.3` — rejected; it would rewrite every row for no reader's benefit.

**D3. Page attributes.** Optional `page_name` and `description` are text attributes (string,
or texts by language including `en`); `page_alignment` is `left`, `center` or `right`;
`width`, `height`, `min_width`, `max_width` are `auto` or a length (`px`, `rem`, `em`, `%`) —
not `elastic`, which only a layouter can compute; `binding_mode` is `read-only` or
`read-write`. The frontend wraps the layouter in a block whose style comes from a pure
function `pageStyle(page)`: the sizes as given, and `margin-left/right: auto` for `center`,
`margin-left: auto` for `right`. With no width the page fills the pane, so alignment only
matters together with `width` or `max_width`.

**D4. Page-wide `binding_mode` is applied before the class merge.** For an element whose
class code defines `binding_mode` (`input`, `select`) and that does not set it, the page's
value is added to the instance's own attributes; then the instance is merged with its class
row as before. That gives the order instance, page, class, and a class row whose
`binding_mode` is `null` is satisfied by the page. Backend (`pageInstances`, which also feeds
the unbound-save warning) and frontend (`resolveEntry`) do the same.

**D5. Input forms.** At an `input` position (a page's `input`, an `open-page`'s `input`), an
object with an `action` attribute is an action instance and any other object, `{}`
included, is static data. The validator skips static data (any JSON object is a valid doc);
the run endpoint refuses it (it is not an action). In the browser, `InputSpec` gains
`doc`: a panel opened with static data starts with a deep copy as its doc and as its clean
copy, and is ready at once; refresh never reloads it (it has no `ref`). Allowed classes:
`input` — `load`, `query`, `load-file`; list `source` — `load`, `query`. `load-file` is not
a list source because a stored file is one doc.

**D6. `query`.** Instance attributes: `sql_stmt_id` (required, an ID), `result` (required,
`record` or `list`), `params` (optional array of values; `$args.<name>` references allowed,
as in `load`'s `where`). The class row seeds both required attributes as `null`. To run it,
the server:

1. reads the `kb.stored_stmts` row named by `sql_stmt_id` (400 if missing);
2. checks `access_controls` against the user (403);
3. substitutes `$args` in `params` and converts each value as `load` does (`paramValue`);
4. in a read-only transaction with `SET LOCAL statement_timeout = '10s'`, runs
   `SELECT to_jsonb(t)::text FROM (<sql_stmt>) t LIMIT 1000`, where `<sql_stmt>` is the stored
   text with trailing semicolons and whitespace removed and `params` bind `$1…$n`;
5. returns `{"result": rows}` for `list`, or the first row for `record` (404 when none),
   exactly as `load` does.

Wrapping in `to_jsonb` lets PostgreSQL turn every column type into JSON, so the stored
statement's column names (or aliases) become the attributes — no `fields` mapping is needed.
Extended-protocol execution with parameters rejects several statements in one string, and
the read-only transaction rejects writes. *Alternative:* named placeholders (`:id`) —
rejected; finding them needs a SQL parser (`::` casts, literals).

**D7. `load-file`.** Instance attribute `file_id` (required). The server reads the
`kb.stored_files` row (400 if missing), checks access (403) and returns
`{"result": file_content}`. `file_content` is JSONB and must be an object, since it becomes
an input doc.

**D8. Registry tables.** `kb.stored_stmts (stmt_name PK, sql_stmt TEXT, description TEXT,
creator TEXT, access_controls TEXT[], created_at, updated_at)` and
`kb.stored_files (file_name PK, file_content JSONB, description TEXT, creator TEXT,
access_controls TEXT[], created_at, updated_at)`. Names follow the descriptor ID pattern, so
the validator can check `sql_stmt_id` and `file_id` the same way. `access_controls`: `NULL`,
`{}` or a list containing `all` lets every logged-in user use the row; otherwise the user
needs one of the listed roles. A user's roles are `UserInfo.Roles`, plus `admin` when the
user is an admin by the existing rule (admin flag, owner or `admin` role). No write
endpoint exists, so the SQL text and the files' contents never cross the network as
descriptor or request text.

**D9. Bundle check for registered rows.** `BuildBundle` takes one more dependency,
`Registered func(ctx, kind, names) (map[string]bool, error)` with kind `statement` or
`file`, and reports every `query` or `load-file` instance whose row does not exist — like
the table check, so a page fails before it runs. Access is not checked at bundle time; it
depends on the user who runs the action.

**D10. Page search and read.** `GET /api/v1/wpi/pages?q=&limit=` returns, for the latest
active version of each page, `descriptor_id`, `version`, `page_name`, `description` and
`updated_at`, filtered by a case-insensitive substring of the ID, the name or the
description (in any language), ordered by ID, `limit` default 50, at most 200.
`GET /api/v1/wpi/pages/:pageId` returns `{"version": n, "page": <body>}` (404 if none). Both
need a logged-in user, like the bundle, which already sends pages to any logged-in user.
Copying is a read followed by an admin `PUT` under the new ID; no copy endpoint.

**D11. Demo.** A seed migration adds the `query` and `load-file` classes, statements
`wpi-demo-customer-directory` (list, everyone) and `wpi-demo-customer-by-id` (record,
`$1`), file `wpi-demo-about`, and page versions (in `kb.frontend_pages`): every demo page
gets `page_name` and `description`; `app-header` shows its title from static data;
`customer-nav` gets *Directory* and *About* items; new page `customer-directory` lists
customers through `query` and opens one in the right pane through a nested `query` with
`params: ["$args.id"]`; new page `customer-admin-about` loads `wpi-demo-about` through
`load-file`, is `read-only` page-wide, centred with `max_width`. Seeds use
`ON CONFLICT DO NOTHING`, because the live dev server once applied a migration twice.

## Risks / Trade-offs

- [A stored statement can read any table the database user can] → Statements come only from
  migrations or `psql` by people who can already run SQL; `access_controls` limits who may run
  each; the transaction is read-only with a timeout.
- [A stored statement that is not a plain query (e.g. `SET`, `DO`) breaks the wrapper] → It
  fails with a database error, reported as "Unable to run action" and logged.
- [Static data with a top-level `action` attribute is read as an action instance] → It fails
  validation visibly ("action class … has no code"); devdoc §3.2.2 states the rule.
- [Old code reading pages from `kb.wpi_descriptors` after the migration] → Code and
  migration ship together; in dev, the migration is applied with `--migrate-only` right after
  the code compiles, and the server restarts on the next Go edit.
- [Page search scans every page] → Pages are few (hundreds); a text index can come later.

## Migration Plan

1. `20261027000003_create_kb_frontend_pages.sql`: create the table, copy page rows, delete
   them from `kb.wpi_descriptors`, replace its `kind` check. Down: restore the check, copy
   rows back, drop the table.
2. `20261027000004_create_kb_stored_stmts_and_files.sql`: both tables. Down: drop them.
3. `20261027000005_seed_wpi_pages_demo.sql`: classes, registry rows, demo page versions.
   Down: delete what it inserted (by `created_by`/`creator`).

Each Up is dry-run inside `BEGIN … ROLLBACK` against `miner` before it is applied.

## Open Questions

None blocking. Whether pages should show `page_name` anywhere (a panel title, the browser
tab) is left for the editor work.
