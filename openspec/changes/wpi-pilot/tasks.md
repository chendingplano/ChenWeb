## 1. Database

- [ ] 1.1 Migration `project_migrations/<ts>_create_kb_wpi_descriptors.sql`: table, `CHECK` on `kind`/`status`, `UNIQUE (kind, descriptor_id, version)`, partial index for active versions, table/column comments, `Down` drop (design D7). Write the file complete in one step — the live dev server applies it on save.
- [ ] 1.2 Migration `<ts>_create_kb_wpi_demo_customers.sql`: demo table (`id BIGSERIAL PRIMARY KEY, user_name, phone_number, email, created_at, updated_at`) with three sample rows; `Down` drop.
- [ ] 1.3 Verify both migrations applied: `SELECT * FROM project_db_migration ORDER BY id DESC LIMIT 3` and `\d kb.wpi_descriptors`.

## 2. Backend: descriptor model and validation

- [ ] 2.1 `server/api/wpi/types.go`: kinds, descriptor shapes for schema_version `0.1`, pane IDs per layout template, supported element/action/layouter/template names (design "Descriptor format").
- [ ] 2.2 `refs.go`: parse whole-string references (`$input_doc`, `$record`, `$args`), path-segment rules, forbidden segments, allowed roots per location; `$args` substitution with a missing-argument error. Unit tests.
- [ ] 2.3 `validate.go` structure + capability checks for one descriptor (used by `PUT`): required/unknown attributes, enums, ID pattern, `schema_version`, element entries (reference with overrides vs inline), triggers (by ID vs inline). Table-driven unit tests, one case per rule.
- [ ] 2.4 `validate.go` graph checks for a bundle: reachability walk with cycle guard (design D2), references, panes against the template, binding roots, the unbound-save-attribute warning. Unit tests including a cycle and each error kind.
- [ ] 2.5 `catalog.go`: table/column existence via `information_schema.columns` for `name` / `schema.name` (default `public`); used by bundle validation and action runs. sqlmock tests.

## 3. Backend: storage and endpoints

- [ ] 3.1 `store.go`: latest-active lookup (`DISTINCT ON … ORDER BY version DESC`), load many by IDs, insert next version with actor. sqlmock tests for latest-active, draft ignored, version increment.
- [ ] 3.2 `GET /api/v1/wpi/apps/:appId` handler: 404 unknown app, 422 with errors, 200 bundle with warnings. sqlmock tests for each.
- [ ] 3.3 `PUT /api/v1/wpi/descriptors/:kind/:descriptorId` handler: admin check (same rule as `pageconfighandler`), URL/body ID match, structure validation, insert. Tests: 200, 400 invalid, 400 ID mismatch, 403 non-admin.
- [ ] 3.4 Register the three routes on the `/api/v1` group in `server/api/routes.go`; `go build ./...` and `go vet ./server/api/wpi/...` pass.

## 4. Backend: load and save actions

- [ ] 4.1 `sqlbuild.go`: pure builders for load (record/list, where, order_by, limit default 200 / cap 1000) and save (insert … RETURNING key, update … WHERE key) with quoted identifiers and `$n` parameters; numbers via `json.Number` strings. Unit tests on generated SQL and args, including an argument containing SQL.
- [ ] 4.2 Save-rule checker: `required`, `max_length`, `pattern`, `format: email` → `field_errors` codes. Unit tests including several failures at once.
- [ ] 4.3 `POST /api/v1/wpi/actions/run`: resolve `{action}` or `{page, pointer}` from stored descriptors only, reject non-load/save actions, substitute `$args`, catalog check, run, scan values (`[]byte`→string, time→RFC 3339), upsert mode choice, 404 for missing record/row, key write-back response, per-run log line (design D14). sqlmock tests covering every scenario in `specs/wpi-data-actions/spec.md`.
- [ ] 4.4 `go test ./server/api/wpi/...` passes.

## 5. Demo descriptors

- [ ] 5.1 Write the `customer-admin` descriptors (app; pages `app-header`, `customer-nav`, `customers`, `customer-form`; elements `customer-list`, `customer-user-name`, `customer-phone-number`, `customer-email`; actions `load-customers`, `load-customer`, `save-customer`, `open-customer`) against `kb.wpi_demo_customers`, following devdoc §4 with the pilot format.
- [ ] 5.2 Seed migration `<ts>_seed_wpi_customer_admin.sql` inserting them as version 1 `active`, `ON CONFLICT DO NOTHING`; `Down` deletes them. Write complete in one step.
- [ ] 5.3 With the dev server running, `GET /api/v1/wpi/apps/customer-admin` returns 200 with no errors and no warnings.

## 6. Frontend: pure logic (bun test)

- [ ] 6.1 `web/src/lib/wpi/types.ts` mirroring the Go descriptor shapes and the bundle/API responses; `api.ts` for the three endpoints.
- [ ] 6.2 `refs.ts`: get/set by `$input_doc` path (creating missing objects, rejecting forbidden segments), resolve trigger `args` with `$record` / `$input_doc`, resolve `$args` in `open_page` inputs. Tests.
- [ ] 6.3 `dirty.ts`: canonical JSON snapshot and compare (key order independent). Tests incl. edit-then-undo.
- [ ] 6.4 `panels.ts`: per-pane panel lists, visible panel, sharing key (page + canonical resolved input), menu bindings, replace decision (hide if bound, close otherwise). Tests for every scenario in `specs/wpi-app-shell/spec.md` that does not need a browser.
- [ ] 6.5 `layout.ts`: width/size → CSS mapping for fixed / elastic / auto (design D9). Tests.

## 7. Frontend: components

- [ ] 7.1 `runtime.svelte.ts`: app runtime with `$state` panes/panels/input docs, `openPage`, `closePanel` (returns whether closed), show counter for refresh, dirty tracking, save with key write-back and field errors.
- [ ] 7.2 Route `web/src/routes/wpi/[appId]/+page.svelte` (+ `+page.ts` with `ssr = false`): load bundle, show validation errors or `WpiApp`; root `select-text`; `beforeunload` while any panel is dirty.
- [ ] 7.3 Layout templates `single-pane` and `top-left-middle-right-bottom` (CSS grid rows; `resizable` pane group for the middle row; omitted regions not rendered); `Pane.svelte` keeping hidden panels mounted.
- [ ] 7.4 `Panel.svelte` + `layouters/FlowLayouter.svelte`; element registry with `ErrorBox` fallback.
- [ ] 7.5 Elements `text`, `input`, `select`, `button` with binding modes and field-error display (cleared on edit).
- [ ] 7.6 `list` element: query via `source`, loading / empty / error states, rows, `on_row_click` with `$record`, re-query on show counter change.
- [ ] 7.7 `menu` element: tree with folders, item click → `open_page` with binding, `initial_item` on start.
- [ ] 7.8 `UnsavedDialog.svelte` (save / discard / keep editing) wired to `closePanel`; `close_page` action.
- [ ] 7.9 Refresh on show for pages with `refresh.on_show` (lists re-query; load-backed input doc reloads unless dirty).

## 8. Frontend: text and checks

- [ ] 8.1 Add `wpi_*` keys to `web/messages/en.json` and `zh-cn.json` (real Chinese) for dialog, error boxes, field error codes, loading and empty states.
- [ ] 8.2 `bun test src/lib/wpi` and `bun run check` (from `web/`) pass with no new hard-coded text.

## 9. Verification against devdoc §8

- [ ] 9.1 In a browser on the dev server, `/wpi/customer-admin` opens with header, left menu and customer list in the middle.
- [ ] 9.2 Switching *Customers* ↔ *New customer* keeps a half-filled form and re-queries the list each time it is shown.
- [ ] 9.3 Inserting a customer works; a missing/invalid field shows a translated error next to the right input.
- [ ] 9.4 Clicking a customer opens it in the right pane; saving updates the row; clicking another customer with unsaved edits prompts first; "keep editing" cancels.
- [ ] 9.5 Switch the locale to Chinese and confirm the interpreter's own text changes while descriptor labels stay as written.

## 10. Documentation and wrap-up

- [ ] 10.1 Update devdoc 2026102601 header status to "pilot built" and note any decisions changed during implementation.
- [ ] 10.2 Write the post-build devdoc (create-devdoc skill, OpenSpec template) and catalogue the interpreter as a reusable module (create-reusable-module skill).
- [ ] 10.3 Commit with `jj` using explicit paths: migrations + backend, frontend, and docs as separate commits; confirm a linear `jj log`.
