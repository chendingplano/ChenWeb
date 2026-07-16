## 1. Backend label files & loader

- [x] 1.1 Create `config/knowledge-menus/` directory with two starter files: `labels-en.toml` and `labels-zh-cn.toml`, each with a `[labels]` table. Leave them with a small illustrative entry or empty `[labels]` table plus a comment explaining the format (mirroring the `[knowledge-menus]` comment style in `config.local.toml`).
- [x] 1.2 Add a loader in `server/api/kbhandler/` (e.g. `kb_menu_labels.go`) that, given a `lang` string, resolves `config/knowledge-menus/labels-<lang>.toml` relative to the repo root (reuse/adapt the walk-up-from-cwd path resolution already in `kb_config_handler.go`'s `resolveKbConfigFilePath`), parses its `[labels]` table with `go-toml/v2` (matching `LoadKbFrontendConfig`'s direct-parse style — no viper/`AppConfigDef` involvement, per design.md DR3), and returns `map[string]string`. A missing file returns an empty map, not an error.
- [x] 1.3 Add `server/cmd/config/knowledge_menu_labels_loader_test.go` or a kbhandler-side test (whichever matches where 1.2 lands) covering: missing file → empty map; file with a `[labels]` table → correct map; malformed TOML → error surfaced (not silently swallowed) so a bad file is visible in logs.

## 2. Backend API

- [x] 2.1 Update `server/api/kbhandler/kb_menu_handler.go`'s `GetKbMenuConfig` to read the `lang` query param (`c.QueryParam("lang")`), call the loader from 1.2, and add a `labels` field (`map[string]string`, defaulting to `map[string]string{}` when `lang` is empty/unrecognized or the file is missing) to `kbMenuConfigResponse`.
- [x] 2.2 Update `server/api/kbhandler/kb_menu_handler_test.go`: add cases for `lang` omitted (labels `{}`), `lang` matching a configured file (labels populated), and `lang` with no matching file (labels `{}`, no error, `menus` still correct).

## 3. Frontend

- [x] 3.1 Update `web/src/lib/services/kbService.ts`: `getKbMenuConfig(lang?: string)` appends `?lang=` when provided; extend the response type (`KbMenuConfig` usage / a new field) to include `labels: Record<string, string>`, and have the function return `{ menus, labels }` (or two accessors — match whatever keeps `+page.svelte`'s call site simplest).
- [x] 3.2 In `web/src/routes/home3/knowledge/+page.svelte`, import `getLocale` from `$lib/paraglide/runtime` (first use of Paraglide on this page). In the existing `onMount` fetch, call `getKbMenuConfig(getLocale())` and store the returned labels in a new `$state` map (e.g. `menuLabels`), fail-open (empty map on error, same as `menuConfig` today).
- [x] 3.3 Extend the `visibleMenuItems` `$derived` filter (added in `configurable-knowledge-menus`) so that, after the existing visibility filtering, each surviving top-level item and child has its `label` replaced with `menuLabels[item.id] ?? item.label` (i.e. configured override, else existing hardcoded default). Do not touch `description`.
- [x] 3.4 Manually verify in the browser: no label files → sidebar unchanged (hardcoded English labels) regardless of locale; add a `zh-cn` override for one item, switch the site language via the existing `/semos` Paraglide locale control, navigate to `/home3/knowledge` → that item's label reflects the override; switch back to `en` (no `labels-en.toml` override configured for that id) → label reverts to the hardcoded default.

## 4. Wrap-up

- [x] 4.1 Run `cd server && go build ./... && go test ./api/kbhandler/...` (and `./cmd/config/...` if 1.2/1.3 land there instead).
- [x] 4.2 Run frontend typecheck (`bun run check`) for the touched files; confirm no new errors/warnings.
- [x] 4.3 Update the `configurable-knowledge-menus` ADR (`KnowledgeStore/doc-repo/adrs/202607/2026071601-adr-configurable-knowledge-menus.md`) or add a new ADR documenting: the `[knowledge-menus]` visibility mechanism is unchanged; labels are now separately configurable per language via `config/knowledge-menus/labels-<lang>.toml`; how the two mechanisms (visibility vs. labels) relate and why they're stored differently (design.md DR2/DR3). Note this as the doc of record for the "how to configure menu labels" question.
