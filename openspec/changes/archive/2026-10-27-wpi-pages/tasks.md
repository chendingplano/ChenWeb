## 1. Storage

- [x] 1.1 Store: pick `kb.frontend_pages` for kind `page` in `latest` and `insertVersion`; tests with sqlmock
- [x] 1.2 Store: page search (`q`, `limit`) and latest-page read; registry lookups for statements and files (existence, and one row with access controls)
- [x] 1.3 Migration `20261027000003_create_kb_frontend_pages.sql` (create, move rows, narrow the `kind` check; Down reverses)
- [x] 1.4 Migration `20261027000004_create_kb_stored_stmts_and_files.sql`

## 2. Validation

- [x] 2.1 Page attributes: `page_name`, `description`, `page_alignment`, page sizes (no `elastic`), `binding_mode`
- [x] 2.2 Input positions take static data; allowed classes per position (`input`: load/query/load-file; `source`: load/query)
- [x] 2.3 `query` and `load-file` in `actionAttrs` with value checks (`sql_stmt_id`, `result`, `params` with `$args`; `file_id`)
- [x] 2.4 Page-wide `binding_mode` filled into element instances before the class merge (`pageInstances`)
- [x] 2.5 Bundle: `Registered` dependency and errors for unregistered statements and files
- [x] 2.6 Validator and bundle tests for 2.1–2.5

## 3. Running query and load-file

- [x] 3.1 `resolveAction` accepts `query` and `load-file`; static input refused
- [x] 3.2 Access check from `access_controls` and the user's roles (403)
- [x] 3.3 `query`: read-only transaction, timeout, `to_jsonb` wrapper, params, record/list results
- [x] 3.4 `load-file`: return the stored content
- [x] 3.5 Action tests (sqlmock): query list/record/not found/params/missing statement/forbidden; load-file ok/missing/forbidden

## 4. Page endpoints

- [x] 4.1 `GET /api/v1/wpi/pages` and `GET /api/v1/wpi/pages/:pageId` handlers and routes
- [x] 4.2 Handler tests (401, search, read, 404)

## 5. Frontend

- [x] 5.1 Types: page attributes, static input, `InputSpec.doc`
- [x] 5.2 Runtime: static input for pages and `open-page` (copy as doc and clean copy, ready at once, no refresh reload)
- [x] 5.3 `resolveEntry` applies the page's `binding_mode` before the class merge
- [x] 5.4 `pageStyle(page)` for sizes and alignment, used by `Panel.svelte`
- [x] 5.5 Tests (`bun test src/lib/wpi`) and `bun run check`

## 6. Demo and verification

- [x] 6.1 Seed migration `20261027000005_seed_wpi_pages_demo.sql`: `query` and `load-file` classes, stored statements and file, demo page versions; `seed_test.go` validates its bodies
- [x] 6.2 Dry-run the migrations in `BEGIN … ROLLBACK`, then apply them; `go test ./server/api/wpi/`, `go build ./...`
- [x] 6.3 Live check: bundle 200, query list, nested query by id, load-file, search and read endpoints
- [x] 6.4 Browser check with the E2E harness: header from static data, directory list and row click, About page centred and read-only, existing checks still pass

## 7. Docs

- [x] 7.1 Devdoc 2026102601: §3.2 built, §5.1 classes no longer planned, §6, §9, status line; devdoc 2026102603
- [x] 7.2 Reusable modules 20261026-01 and 20261026-02 (doc and row): version, files, description
