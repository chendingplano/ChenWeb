## Why

The Wiki sidebar menu on `/home3/knowledge` (Knowledge Stores, Injestion, Wiki, Document Processing, and their sub-items) is hardcoded in the Svelte page. Different deployments/tenants want to show a reduced or customized subset of this menu (e.g. hide in-progress features, disable a whole section) without a code change and redeploy. Today that requires editing `+page.svelte` directly.

## What Changes

- Add a `[knowledge-menus]` section to `config.toml` / `config.local.toml`: a flat table mapping a menu item's existing id (e.g. `kb-doc-wiki`, `kb-llm-wiki-v3`) to `true`/`false`. Items omitted from the table default to enabled, matching the `[doc-reviews]` precedent (remove the whole section to fall back to current behavior).
- Disabling a parent item's id hides its entire subtree (its children are never shown, regardless of their own flags). Disabling a child id hides only that item, leaving its parent and siblings visible. If disabling children leaves a parent with zero visible children, the parent is also hidden.
- Load `[knowledge-menus]` through the existing viper-based `AppConfigDef` (`server/cmd/config/config.go`), the same path already used for `[doc-reviews]`, so `config.local.toml` overrides are honored.
- Add a new read endpoint, `GET /api/v1/kb/menu-config`, returning the resolved `map[string]bool`.
- Update the `/home3/knowledge` Svelte page to fetch this map on load and filter its existing `menuItems` (and their `children`) against it before rendering the sidebar.
- No validation of ids against a server-side whitelist: the id list is owned by the frontend menu definition, and duplicating it in Go would need to stay in sync on every menu change. Unknown ids in config are simply inert (frontend won't have a matching item).

## Capabilities

### New Capabilities
- `knowledge-menu-config`: Config-driven enable/disable of Wiki sidebar menu items (top-level sections and their children), read from `[knowledge-menus]` in `config.toml`/`config.local.toml` and served to the frontend via a dedicated endpoint.

### Modified Capabilities
(none — no existing spec covers the Wiki sidebar menu today)

## Impact

- `server/cmd/config/config.go`: new `KnowledgeMenus map[string]bool` field on `AppConfigDef`, new `GetKnowledgeMenusConfig()` accessor.
- `server/api/kbhandler/`: new handler file exposing `GET /api/v1/kb/menu-config`.
- `server/api/routes.go`: register the new route.
- `web/src/routes/home3/knowledge/+page.svelte`: fetch the menu config and filter `menuItems`/`children` before render.
- `web/src/lib/services/kbService.ts` (or similar): new fetch function for the menu-config endpoint.
- `config.local.toml`: documentation comment block for the new `[knowledge-menus]` section (following the `[doc-reviews]` comment style already in the file).
- No database changes. No breaking changes — absent config preserves today's full menu.
