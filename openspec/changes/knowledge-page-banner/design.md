> **Post-implementation update:** the banner (Goals bullets 1-2, Decisions 3-4, Migration Plan step 2-3) was built and then reverted at the user's request — visually it "does not look good" on this page. Decisions 1-2 (the `knowledge-menus`→`knowledge-content` rename) shipped and stand; the `Knowledge` struct/`SiteKnowledge` type/`[knowledge]` config section that Decision 3 introduced were removed along with the banner rather than left as dead code. Left below unedited as the record of what was considered and why, for anyone revisiting a Knowledge-page banner in the future.

## Context

`/semos/workspace`'s masthead is config-driven end to end: `SiteConfig.workspace` (`Workspace` Go struct / `SiteWorkspace` TS interface — `kicker`, `banner_title`, `banner_subtitle`, `banner_image`) supplies English/default copy from `config/site/site-default.toml`'s `[workspace]` section, and `config/workspace-content/labels-<lang>.toml` supplies per-locale overrides keyed by content id (`ws-kicker`, `ws-banner-title`, `ws-banner-subtitle`), fetched via `getWorkspaceContentConfig()` → `GET /api/v1/workspace/content-config`.

`/home3/knowledge` has a parallel but incomplete mechanism: `config/knowledge-menus/labels-<lang>.toml` + a `[knowledge-menus]` section in `config.toml`/`config.local.toml`, loaded by `kbhandler.LoadKnowledgeMenuLabels` / `appconfig.GetKnowledgeMenusConfig` and served by `GET /api/v1/kb/menu-config?lang=`, consumed by `getKbMenuConfig()` in `kbService.ts`. This only covers Wiki sidebar menu item ids (`kb-search`, `kb-doc-wiki`, ...) — there's no `[knowledge]` site-config section, and the directory/section/Go-symbol naming (`knowledge-menus`) diverges from the `workspace-content` convention used everywhere else.

Both mechanisms are structurally the same shape: a `map[string]bool` visibility map + a `map[string]string` label map, keyed by arbitrary content ids, fail-open on missing/malformed files. Nothing about them is menu-specific — they're generic per-page content-override plumbing that happens to only be used for menu items today.

## Goals / Non-Goals

**Goals:**
- `/home3/knowledge` gets a masthead banner visually matching `/semos/workspace`'s (image, gradient, watermark, kicker+dateline rule, title, subtitle), with its own copy.
- Banner copy/image are configurable the same way workspace's are: a `[knowledge]` site-config section for defaults, per-locale overrides via labels file.
- `config/knowledge-menus/` and all related Go/config naming is renamed to `config/knowledge-content/`, matching the `workspace-content` convention exactly (directory, `config.toml` section name, Go struct field + accessor, loader file/functions, env var, test file).
- The rename is purely mechanical (same generic `map[string]bool`/`map[string]string` shapes) — no new response fields, no new endpoint.

**Non-Goals:**
- No announcements/bulletin strip on the Knowledge page (that stays specific to `/semos/workspace`, backed by `kb.site_announcements`).
- No fix for the pre-existing dead `[descriptions]` table in `knowledge-menus/labels-zh-cn.toml` (parsed by nothing today — `rawKnowledgeMenuLabels` only has a `Labels` field). Out of scope; noted so it isn't mistaken for a regression.
- No change to `/api/v1/kb/menu-config`'s URL path or response shape (`{status, menus, labels}`) — only what's *inside* `menus`/`labels` grows (new banner ids), and the identifiers it's loaded from/by are renamed.
- No admin UI for editing banner config — same as workspace today (TOML file edits only).

## Decisions

**1. Reuse the existing generic menu-config mechanism for the banner, rather than building a workspace-content-style `visibility+labels+descriptions` triple.**
`kb_menu_handler.go`'s `menus`/`labels` maps are already keyed by arbitrary string ids with fail-open defaults — adding `kb-kicker`, `kb-banner-title`, `kb-banner-subtitle` as new ids alongside `kb-search` etc. costs nothing beyond a couple of TOML entries and frontend read-throughs. Alternative considered: mirror workspace's separate `getWorkspaceContentConfig`/descriptions-triple exactly — rejected as unnecessary duplication; the banner doesn't need per-id *descriptions*, only labels, so the existing shape already covers it.

**2. Do the full `knowledge-menus` → `knowledge-content` rename (directory, `config.toml` section, Go symbols, env var, test file), not just the frontend labels directory.**
`workspace-content` uses the identical name for both the `config.toml` visibility section (`mapstructure:"workspace-content"`) and the labels directory. `knowledge-menus` is the only place in the codebase where these two concepts have different names from their workspace counterpart. A frontend-only rename would leave Go code, `config.local.toml`, and code comments referring to `knowledge-menus` while the directory is called `knowledge-content` — worse than not renaming, since the two names would then refer to the same thing inconsistently. Alternative considered: leave the name as `knowledge-menus` and just add banner ids to it — rejected because the user explicitly flagged the inconsistency and the full rename is a clean, mechanical, same-day fix now that the actual blast radius (5 Go files across `kbhandler`, one test file, one env var, one config.toml section, one directory) is known.
- Renamed: `config/knowledge-menus/` → `config/knowledge-content/`; `[knowledge-menus]` → `[knowledge-content]` in `config.toml`/`config.local.toml`; `AppConfig.KnowledgeMenus` → `AppConfig.KnowledgeContent` (`mapstructure:"knowledge-content"`); `GetKnowledgeMenusConfig()` → `GetKnowledgeContentConfig()`; `kb_menu_labels.go` → `kb_content_labels.go` (`LoadKnowledgeMenuLabels` → `LoadKnowledgeContentLabels`, `resolveKnowledgeMenuLabelsPath` → `resolveKnowledgeContentLabelsPath`); env var `KNOWLEDGE_MENU_LABELS_DIR` → `KNOWLEDGE_CONTENT_LABELS_DIR`; test file `knowledge_menus_config_test.go` → `knowledge_content_config_test.go`.
- NOT renamed: the HTTP endpoint path `/api/v1/kb/menu-config` and the handler function `GetKbMenuConfig`/`getKbMenuConfig()` — these describe the *endpoint's* purpose (serving the Wiki menu's config, which now also happens to carry banner ids), not the config-file naming convention being fixed. Renaming the URL is unrelated churn.

**3. Add a `Knowledge` Go struct / `SiteKnowledge` TS interface to `SiteConfig`, mirroring `Workspace`/`SiteWorkspace` minus `Apps`.**
`kicker`, `banner_title`, `banner_subtitle`, `banner_image` only — the Knowledge page has no app-tile grid. Populated from a new `[knowledge]` section in `config/site/site-default.toml`.

**4. Masthead markup: duplicate, don't extract a shared component, for this change.**
The workspace masthead's ~90 lines are entangled with workspace-only concerns (config-diagnostics block for unknown content ids, `tenantError` display, the `reveal` scroll-in action, announcements section immediately following it). Extracting a clean shared `<Masthead>` component would need to generalize all of that first. Given the Knowledge banner is simpler (no announcements, no tenant error), duplicating the masthead structure (image/gradient/watermark/kicker/dateline/title/subtitle) directly into `+page.svelte` is less risky than a premature abstraction. Flagged as a candidate for extraction later if a third page needs the same pattern.

## Risks / Trade-offs

- [Renaming `[knowledge-menus]` → `[knowledge-content]` in `config.toml`/`config.local.toml` is a breaking change for any deployed config still using the old section name] → Acceptable: this is a staging environment (per workspace CLAUDE.md, destructive/breaking config changes are fine here), and the loader fails open (missing/renamed section silently resolves to "no overrides", not an error) — worst case is toggles silently reset to default-enabled until `config.local.toml` is updated, which this change does in the same commit.
- [Reusing the generic `menus`/`labels` maps for banner ids means there's no type-level guarantee the frontend requests ids that exist server-side, same as today for menu ids] → Accepted pre-existing pattern; the existing "unrecognized id" diagnostic block (already in `+page.svelte`) surfaces mistakes visibly rather than silently.
- [Duplicating masthead markup instead of extracting a component risks visual drift between the two banners over time] → Accepted for now per Decision 4; both are small, self-contained sections and a future third use is what would justify extraction.

## Migration Plan

1. Rename Go files/symbols/env var and `config/knowledge-menus/` → `config/knowledge-content/` (including updating `config.toml`/`config.local.toml` section names) in one commit — mechanical, no behavior change.
2. Add `[knowledge]` to `site-default.toml`, `Knowledge`/`SiteKnowledge` types, new banner ids to `knowledge-content/labels-<lang>.toml`.
3. Add the masthead section to `+page.svelte`.
4. Manually verify both `/semos/workspace` and `/home3/knowledge` render correctly, in both locales and both light/dark themes, before considering this done — no automated visual regression coverage exists for either page today.

No rollback complexity: this is a UI + config-naming change with no data migration, no schema change, and no external API surface.
