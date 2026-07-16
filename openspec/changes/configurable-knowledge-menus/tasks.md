## 1. Backend config

- [x] 1.1 Add `KnowledgeMenus map[string]bool `mapstructure:"knowledge-menus"`` field to `AppConfigDef` in `server/cmd/config/config.go`, next to `DocReviews`, with a doc comment matching that field's style.
- [x] 1.2 Add `GetKnowledgeMenusConfig() map[string]bool` accessor in `server/cmd/config/config.go`, returning `AppConfig.KnowledgeMenus` (mirroring `GetDocReviewsConfig`).
- [x] 1.3 Add a `[knowledge-menus]` example/documentation block to `config.local.toml`, following the comment style already above `[doc-reviews]` (explain flat id->bool shape, default-enabled-when-absent, parent cascade, remove-section-to-restore-defaults). Leave it commented out or empty by default so no items are disabled out of the box.
- [x] 1.4 Add `server/cmd/config/knowledge_menus_config_test.go` covering: section absent → empty map; `config.local.toml` value overrides `config.toml` value (same pattern as `doc_reviews_config_test.go`).

## 2. Backend API endpoint

- [x] 2.1 Add `server/api/kbhandler/kb_menu_handler.go` with `GetKbMenuConfig(c echo.Context) error`, calling `appconfig.GetKnowledgeMenusConfig()` and returning `{"status": true, "menus": {...}}` (200 OK, empty map when unset).
- [x] 2.2 Register `apiGroup.GET("/kb/menu-config", kbhandler.GetKbMenuConfig)` in `server/api/routes.go`, next to the existing `/kb/config` registration.
- [x] 2.3 Add `server/api/kbhandler/kb_menu_handler_test.go` covering: no config → empty map response; configured overrides → matching response (follow the style of an existing kbhandler `*_test.go`, e.g. `default_store_handler_test.go`).

## 3. Frontend

- [x] 3.1 In `web/src/routes/home3/knowledge/+page.svelte`, fetch `GET /api/v1/kb/menu-config` on mount into a `$state` map (fail-open: on fetch error or before it resolves, treat as an empty map so the full menu renders — same as if the config section were absent).
- [x] 3.2 Add a derived (`$derived`) filtered menu computed from `menuItems` and the fetched map:
      - Drop a top-level item when `map[item.id] === false`.
      - Otherwise drop children whose id is `false` in the map.
      - Drop a top-level item that originally had non-empty `children` if all of its children were filtered out.
- [x] 3.3 Render the derived filtered list (`&lt;aside class="kb-menu"&gt;` section, lines ~373-598) instead of the raw `menuItems` array. Verify the collapsible-parent logic (`isCollapsibleParent`, open/closed state per id) still works against the filtered list.
- [x] 3.4 Manually verify in the browser: default config (no `[knowledge-menus]`) shows the unchanged menu; disabling a top-level id hides its subtree; disabling one child hides only that child; disabling every child of a section hides the section too.

## 4. Wrap-up

- [x] 4.1 Run `cd server && go build ./... && go test ./cmd/config/... ./api/kbhandler/...`.
- [x] 4.2 Run frontend typecheck/lint for the touched Svelte file per `ChenWeb/CLAUDE.md` / `tax`-style `mise` tasks available in this repo.
- [x] 4.3 Confirm the "What knowledge changed?" checklist from the workspace `CLAUDE.md`: note this proposal + spec as the doc of record for the new `[knowledge-menus]` capability; no other existing docs describe the Wiki sidebar menu today, so none go stale.
