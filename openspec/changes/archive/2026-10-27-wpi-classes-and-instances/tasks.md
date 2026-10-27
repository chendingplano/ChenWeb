## 1. Backend: classes and validation

- [x] 1.1 `types.go`: `SchemaVersion` `"0.2"`; `elementClasses` / `actionClasses` (from the attribute maps) replace `elementTypes` / `actionTypes`; `Action.Class` (`json:"action"`) replaces `ActionType`; bundle doc comment says classes.
- [x] 1.2 `validate.go`: `actionAttrs` keyed by action class (`load`, `save`, `open-page`, `close-page`, `save-page`); element and action checks take the class ID and a `complete` flag (design D3); class-row validation for `kind` element and action (design D2: class attributes, code-defined keys, code-required present, defaults valid, no defaults for action-holding attributes); text attributes `label` / `description` (design D9).
- [x] 1.3 `validate.go` page checks: element instances `{element, ...}` (class has code, no class attributes, code-defined keys, values valid), action instances by position (design D5), page `input` and `save_action` as instances; remove stored-element references, inline element descriptors, actions by ID.
- [x] 1.4 `mergeInstance` and the bundle-stage instance check: every `null` attribute of the class row is set, merged instance passes the `complete` checks.
- [x] 1.5 Table-driven tests in `validate_test.go`: one case per class-row rule, per instance rule, per position rule, text attributes, format `0.1` rejected.

## 2. Backend: bundle and actions

- [x] 2.1 `store.go`: `latest` with `ids == nil` returns every active descriptor of the kind. sqlmock test.
- [x] 2.2 `bundle.go`: load all classes first, pages in rounds; merge instances as pages arrive; references (missing class rows, pages opened by `open-page`), instance checks, panes, tables and the unbound-save warning on merged instances; bundle carries only used classes and validates only those (design D4). Tests: valid app, missing class row, unset `null` attribute, broken unused class, unknown pane, missing column, cycle.
- [x] 2.3 `actions.go`: `ref` must be `{kind: "page", id, pointer}`; load the action class, check and merge the instance, run (design D6). Tests for every scenario in `specs/wpi-data-actions/spec.md`.
- [x] 2.4 `go build ./...`, `go vet ./server/api/wpi/...` and `go test ./server/api/wpi/...` pass.

## 3. Data migration

- [x] 3.1 Check the database holds no WPI descriptors besides the `seed:wpi-pilot` rows.
- [x] 3.2 `project_migrations/20261027000002_seed_wpi_classes_and_instances.sql` (design D11): eleven classes, `customer-admin` app and pages version 2 in format `0.2` with `save-page` on the Save button and `zh-cn` labels on the form fields; retire the pilot's stored elements and actions; Down reverses. Dry-run the Up section in `BEGIN … ROLLBACK`.
- [x] 3.3 `seed_test.go` validates every body in the new migration as a bundle (classes + pages), and no longer the pilot's.
- [x] 3.4 Apply the migration (`go run . --migrate-only` from `server/cmd/doc-service`) and confirm `GET /api/v1/wpi/apps/customer-admin` builds without errors (bundle check against the live database).

## 4. Frontend

- [x] 4.1 `types.ts`: element/action class and instance types, merged element shape, `ActionRef` page-only, runtime `InputSpec` `{ref?, args?}`, bundle of classes.
- [x] 4.2 `entries.ts`: resolve an element instance by merging with its class (same rule as Go); the resolved element knows its page and pointer; helper to merge an action instance with its class; `entries.test.ts` updated.
- [x] 4.3 `elements/index.ts` registry `elementClasses` keyed by class ID; `Element.svelte` error box names the class; triggers build `{kind: "page", id, pointer}`; menu bindings keyed by the instance pointer (design D10).
- [x] 4.4 Text helper: localized `label` (string or per-language object, `en` fallback) used by input, select and button.
- [x] 4.5 `runtime.svelte.ts`: dispatch by action class (`open-page`, `close-page`, `save`, `save-page`); `open-page` input refs and argument passing (design D7); list `source` and page `input` / `save_action` by pointer.
- [x] 4.6 `bun test src/lib/wpi` and `bun run check` pass.

## 5. Verification

- [x] 5.1 Update `.cache/wpi-e2e-server/wpi_e2e.py` only if selectors changed; rerun the browser checks (devdoc §8 checks 1–4 and the leave warning) against the vite dev server; clean up the test rows.
- [x] 5.2 Check a Chinese label shows in `zh-cn` and English in `en`.

## 6. Docs and commit

- [x] 6.1 Devdoc 2026102601: kebab-case action classes, format `0.2`, attribute rule (code defines, row sets defaults / `null`, omitted = optional), no defaults for action-holding attributes, nested arguments, §4 rewritten to the new demo descriptors, status line.
- [x] 6.2 Devdoc 2026102603 and module docs 20261026-01 / 20261026-02 (plus their `kb.reusable_modules` rows): describe the class model as built; bump versions.
- [x] 6.3 Commit ChenWeb (explicit paths) and KnowledgeStore separately with `jj`; `jj log` shows linear history.
