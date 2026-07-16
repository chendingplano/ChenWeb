## Context

The `/home3/knowledge` sidebar (`web/src/routes/home3/knowledge/+page.svelte`) hardcodes a `menuItems: KbMenuItem[]` array, two levels deep: top-level items (`id`, `label`, `description`, `icon`, optional `children`) and children (`id`, `label`, `description`). All ids come from a single flat string-literal union `KbSectionId` — ids are unique across the whole tree, not scoped per-parent. One extra source, `knowledge-sections.js`, spreads a handful of placeholder "under construction" ids into the Wiki children array before rendering.

Config loading already has a proven pattern for an arbitrary, admin-editable TOML section: `[doc-reviews]` in `config.local.toml` is unmarshalled by viper into `AppConfigDef.DocReviews map[string][]string` (`server/cmd/config/config.go`), which correctly merges `config.local.toml` over `config.toml`. A separate, unrelated mechanism (`server/api/kbhandler/kb_config_handler.go`, `GET /api/v1/kb/config`) parses `config.toml` directly with go-toml/v2 and does **not** merge `config.local.toml` — it is not suitable here since the user explicitly wants `config.local.toml` to be the config source.

## Goals / Non-Goals

**Goals:**
- Let an operator enable/disable any top-level menu section or any individual child item via `[knowledge-menus]` in `config.local.toml`, with no code change.
- Disabling a parent hides its whole subtree; disabling a child hides just that child.
- Preserve today's behavior (full menu shown) when the section is absent.

**Non-Goals:**
- Defining brand-new menu items (label/route/icon) purely from config. Config only toggles visibility of ids that already exist in the Svelte menu definition.
- Per-tenant or per-user menu config (this is a single deployment-wide config file, same scope as `[doc-reviews]`).
- Reordering menu items via config.

## Decisions

**1. Flat `map[string]bool` keyed by existing menu ids, not nested TOML tables.**
Confirmed with the user (see options considered below). Ids (`kb-doc-wiki`, `kb-llm-wiki-v3`, ...) are already globally unique across the tree, so a flat table fully expresses "turn off this node" without re-encoding parent/child relationships in TOML — those relationships already live in `+page.svelte` and would drift if duplicated. This also maps directly onto `map[string]bool` in Go, mirroring the `[doc-reviews]` `map[string][]string` precedent.
*Alternative considered*: nested tables (`[knowledge-menus.kb-doc-wiki]` with per-child keys). Rejected — more visually hierarchical, but requires a messier Go type (`map[string]interface{}` or per-parent structs) and a second copy of the tree shape that must stay in sync with the Svelte definition.

**2. Default-enabled, opt-out semantics; ids absent from the map are treated as `true`.**
Matches the `[doc-reviews]` precedent ("remove this whole section to fall back to the built-in defaults") and means shipping this feature is a no-op for every deployment until someone edits `config.local.toml`.

**3. No server-side id whitelist/validation.**
`[doc-reviews]` validates tier item names against `validAspectNames()` because that list is Go-owned (`ListAspects()`). Menu ids are Svelte-owned; there's no equivalent Go-side source of truth, and building one would mean updating two files (the Svelte menu and a Go id list) every time a menu item is added/renamed, which contradicts the "don't duplicate the hierarchy" reasoning in Decision 1. The resolved map is passed through as-is; an unrecognized id in config is simply inert on the frontend (no matching item to hide).

**4. New dedicated endpoint `GET /api/v1/kb/menu-config`, not an addition to `GET /api/v1/kb/config`.**
`kb/config`'s handler (`kb_config_handler.go`) reads `config.toml` directly and deliberately does not go through `AppConfig`/viper, so it never sees `config.local.toml` overrides. Bolting a viper-backed field onto that handler would mean two different config-loading mechanisms inside one response. A small new handler in the same `kbhandler` package, backed by `appconfig.GetKnowledgeMenusConfig()`, keeps the viper-loaded value in its own viper-consistent path (same pattern as `docreviews.HandleListTiers` calling `appconfig.GetDocReviewsConfig()`).

**5. Visibility resolution (parent cascade + empty-parent collapse) happens client-side in `+page.svelte`.**
The server returns the raw resolved `map[string]bool`; it has no notion of the menu tree shape. `+page.svelte` is the only place that already owns `menuItems`, so it computes a derived, filtered list:
- A top-level item is dropped if `config[item.id] === false`.
- Otherwise, if it has `children`, each child is dropped if `config[child.id] === false`.
- If a parent item originally had a non-empty `children` array and every child was filtered out, the parent itself is dropped too (avoids an expandable node with nothing inside).
This is a pure derivation (`$derived`) over the existing hardcoded `menuItems`, so no restructuring of the current data/rendering split is needed.

## Risks / Trade-offs

- **Parent/child id collision** (`kb-import` and `kb-chunks` each reuse their own id for their single/first child) → Not a real ambiguity: toggling that id hides both the parent and that specific child simultaneously, which is the only sensible outcome since they represent the same concept today. No mitigation needed; noted here so it isn't mistaken for a bug during implementation.
- **Stale/unknown ids in config after a menu refactor** (item renamed/removed in Svelte but config still references old id) → silently inert, no error surfaced to the operator. Mitigation: none for v1 (consistent with `[doc-reviews]`'s "unknown names are ignored" behavior); could add a startup log of ignored/unknown ids later if this becomes a real pain point.
- **Extra network round-trip before the sidebar can render its final state** → Mitigation: render the full hardcoded menu first paint, then reconcile once the config fetch resolves (brief flash for the rare disabled item, no loading spinner needed) — same UX pattern already used elsewhere in this page for async data.

## Migration Plan

No data migration. Deploying with no `[knowledge-menus]` section is behaviorally identical to today. Rollback is deleting the section (or reverting the deploy) — the frontend already defaults to "show everything" when the fetch returns an empty/absent map or fails.
