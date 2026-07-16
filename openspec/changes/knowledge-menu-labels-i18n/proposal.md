## Why

The Wiki sidebar menu on `/home3/knowledge` (see `configurable-knowledge-menus`, already implemented) lets operators toggle item *visibility* via `[knowledge-menus]` in `config.local.toml`, but every label ("Metrics", "Wiki", "Document Processing", ...) is a hardcoded English string in `+page.svelte`. There is no way to (a) show a translated label when the site is viewed in a different language, or (b) let an operator rename a label without a frontend code change. The user wants both, treating menu labels as "content" that should be configurable and multi-lingual the same way other site content already is.

## What Changes

- Add a per-language, file-based label override mechanism: `config/knowledge-menus/labels-<lang>.toml`, one optional file per language, each a flat `id -> label` map — mirroring the existing "separate file per language" convention already used for `config/site/site-default-<lang>.toml`, applied to this different (operator/admin-tool) config domain.
- `GET /api/v1/kb/menu-config` gains an optional `?lang=` query param and a new `labels` field in its response (alongside the existing `menus` visibility field). Ids without a configured override for the requested language are simply absent from `labels`.
- `/home3/knowledge` adopts Paraglide's `getLocale()` (`$lib/paraglide/runtime`) for the first time — the same site-wide language control already used on `/semos` — as the single source of truth for "what language is the user viewing in," and passes it as `?lang=` when fetching menu config.
- The sidebar renders `menuLabels[item.id] ?? item.label` (configured override, else the existing hardcoded default) for both top-level items and children — visibility filtering (`[knowledge-menus]`) is unaffected and stays exactly as implemented.
- **Non-goal / not included**: syncing or persisting a user's chosen language across visits (explicitly deferred by the user — a separate, larger site-wide problem), translating menu item *descriptions* (only labels, per the request), and changing how `/semos` site-config language files are selected today.

## Capabilities

### New Capabilities
- `knowledge-menu-labels`: Per-language, config-file-driven label overrides for Wiki sidebar menu items, resolved against the site-wide Paraglide UI locale and exposed through the existing `GET /api/v1/kb/menu-config` endpoint (which gains an optional `lang` query parameter and a `labels` field; its existing `menus` visibility behavior and response shape are otherwise unchanged).

### Modified Capabilities
(none — `knowledge-menu-config`, from the prior `configurable-knowledge-menus` change, was never archived to `openspec/specs/`, so there is no baseline to file a delta against. Its endpoint's extension is described as part of the new `knowledge-menu-labels` capability above instead.)

## Impact

- `server/cmd/config/config.go`: no visibility-related changes; a small new loader for per-language label files (see design.md for whether this goes through `AppConfigDef`/viper or a direct file read).
- `server/api/kbhandler/kb_menu_handler.go`: reads `?lang=`, loads/resolves labels, adds `labels` to the response.
- `web/src/lib/services/kbService.ts`: `getKbMenuConfig` takes an optional `lang` argument; response type gains `labels`.
- `web/src/routes/home3/knowledge/+page.svelte`: imports `getLocale` from `$lib/paraglide/runtime` for the first time on this page; `visibleMenuItems` derivation applies label overrides.
- New config files: `config/knowledge-menus/labels-en.toml`, `config/knowledge-menus/labels-zh-cn.toml` (optional; absence means all labels stay at their hardcoded default for that language).
- No database changes. No breaking changes: a deployment with no label files behaves exactly as today (hardcoded English labels for everyone, regardless of locale).
