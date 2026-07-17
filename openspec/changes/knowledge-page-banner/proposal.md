## Why

`/home3/knowledge` and `/semos/workspace` are both entry points into the same product, but they currently look unrelated: `/home3/knowledge` renders none of the top navigation or branding that every `/semos` page shares. A user moving between "Workspace" and "Knowledge Base" in the nav should recognize they're in the same site.

**Update after implementation:** a masthead banner (matching `/semos/workspace`'s) was built and shipped below the `SiteHeader`, then removed at the user's request after visual review — "does not look good." The header-sharing and `knowledge-menus`→`knowledge-content` rename are kept; the banner is not. See Capabilities and tasks.md's "Rollback" section for what was reverted and why the reverted pieces were fully removed rather than left dormant.

## What Changes

- `/home3/knowledge` now renders the shared `SiteHeader` (top nav: logo, 首页/工作台/知识库/关于我们, language switcher, dark-mode toggle, login/logout) above its content, via a route-scoped `+layout.ts`/`+layout.svelte` — matching every `/semos` page. **(Implemented.)**
- The Knowledge left-menu's own logo/language-toggle/Workspace-link row — added by `kb-menu-branding-header` — is removed, since the shared `SiteHeader` now covers site identity, language switching, and cross-navigation. **(Implemented; see Modified Capabilities.)**
- `config/knowledge-menus/` is renamed to `config/knowledge-content/` (matching the `workspace-content/` naming convention) across the frontend, the Go backend (struct field, accessor, loader file/functions, env var, test files), and `config.toml`/`config.local.toml`. **(Implemented.)**
- ~~A masthead banner below the `SiteHeader`, matching `/semos/workspace`'s visual pattern~~ — **built, then reverted** per user feedback after seeing it rendered. No banner ships as part of this change.

## Capabilities

### New Capabilities
- `knowledge-page-header`: `/home3/knowledge` renders the shared `SiteHeader` above its content, matching `/semos` pages. *(Implemented.)*

### Modified Capabilities
- `kb-menu-branding-header`: The branding row (Company Logo, Language Control, Workspace button) at the top of the Knowledge left menu is **removed** in both expanded and collapsed states. Its purpose — site identity, language switching, and a way back to the Workspace — is now served by the shared `SiteHeader` introduced by `knowledge-page-header`.

### Dropped Capabilities
- `knowledge-page-banner`: proposed, implemented, then reverted before archiving — never became established behavior. Its spec file was deleted rather than kept as a REMOVED delta, since it never shipped as lasting truth (see tasks.md's Rollback section for the revert steps).

## Impact

- `ChenWeb/web/src/routes/home3/knowledge/+layout.ts`, `+layout.svelte` — new files (render `SiteHeader`, fetch `SiteConfig`).
- `ChenWeb/web/src/routes/home3/knowledge/+page.svelte` — old branding row removed; no masthead added (reverted).
- `ChenWeb/web/src/routes/home3/knowledge/+page.ts` — deleted (was redundant once the layout fetches `SiteConfig`).
- `ChenWeb/config/knowledge-menus/` → renamed `ChenWeb/config/knowledge-content/` (`labels-en.toml`, `labels-zh-cn.toml`).
- `server/cmd/config/config.go`, `server/api/kbhandler/kb_menu_labels.go` (→ `kb_content_labels.go`), `server/api/kbhandler/kb_menu_handler.go`, associated test files, `config.toml`/`config.local.toml` — `knowledge-menus` → `knowledge-content` rename.
- No changes to `shared/`, other projects. `server/api/sitehandler/sitehandler.go`'s `SiteConfig`/`Knowledge` struct and `config/site/site-default*.toml`'s `[knowledge]` section were added for the banner and then removed along with it.
