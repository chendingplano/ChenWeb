## Context

Two independent "language" mechanisms exist in ChenWeb today, and neither reaches `/home3/knowledge`:

1. **Paraglide-js** (`@inlang/paraglide-js`) is the real site-wide UI locale: `getLocale()`/`setLocale()` from `$lib/paraglide/runtime`, cookie/localStorage-backed, `locales = ["en", "zh-cn"]`, `baseLocale = "zh-cn"`. It drives `/semos` chrome (`SiteHeader.svelte`, `semos/+page.svelte`, `semos/workspace/+page.svelte`) via compile-time message catalogs (`web/messages/{locale}.json` → generated `web/src/lib/paraglide/messages/*.js`). `setLocale()` "owns its own reload strategy" (existing code comment) — switching locale triggers a reload, after which `getLocale()` reflects the new value.
2. A page-local `?lang=` URL query param, used only for Wiki *article content* language (`artifactWikiLang` in `+page.svelte`, and `metricWikiService.ts`), backed by an LLM translation pipeline (`TRANSLATION_MODEL_NAME`). Unrelated to UI chrome.

`/home3/knowledge` (where the Wiki sidebar menu lives) uses neither Paraglide nor a persisted locale today — labels are hardcoded English strings in `+page.svelte`'s `menuItems` array.

Separately, `config/site/site-default.toml` and `config/site/site-default-zh-cn.toml` establish a precedent: **customer-facing content is configured via one TOML file per language**, selected today by a single static path (`[config].config_filename` in `config.local.toml`) rather than dynamically per request — genuine per-request language switching for that content doesn't exist yet either, and the user has explicitly deferred fixing that broader problem.

The user's stated direction: Paraglide's locale (as surfaced by the `/semos` language toggle) should become the *one* site-wide language control, and "menus are content" — they should follow the same general shape (configurable + multi-lingual, one file per language) as other configurable content, without this change having to solve full per-user language persistence.

## Goals / Non-Goals

**Goals:**
- Let an operator supply a translated (or simply renamed) label for any Wiki sidebar menu item, per language, via a config file — no frontend code change.
- Make the Wiki sidebar follow the same site-wide language signal as `/semos` (Paraglide's `getLocale()`), the first time `/home3/knowledge` adopts Paraglide.
- Preserve today's behavior exactly when no label files are configured (hardcoded English labels for everyone).

**Non-Goals:**
- Persisting a user's chosen language across visits/sessions — deferred by the user as a separate, site-wide problem.
- Translating menu item *descriptions* (the secondary line) — only primary labels, per the request. The same mechanism could extend to descriptions later.
- Changing how `/semos` site-config files are selected, or unifying the `?lang=` wiki-content-language mechanism with Paraglide's locale — out of scope; this change only adds a new, independent consumer of Paraglide's locale.
- Server-side validation of `lang` values against Paraglide's `locales` list — an unrecognized value simply resolves to "no label file found," which is a safe no-op (consistent with `[knowledge-menus]`'s existing "unknown ids are inert" precedent).

## Decisions

### DR1 — Paraglide's `getLocale()` is the language signal, not `?lang=`
Confirmed with the user: one site-wide language control, driven by the same mechanism as the `/semos` language toggle. `+page.svelte` imports `getLocale` from `$lib/paraglide/runtime` for the first time and uses it (not the page's existing `?lang=` param, which stays scoped to Wiki article *content* language and is untouched by this change) to decide which label file to request.

### DR2 — New dedicated directory, not `config/site/`
Label files live at `config/knowledge-menus/labels-<lang>.toml`, not inside `config/site/`. `config/site/*` is tenant-scoped, customer-facing marketing content loaded through a completely different mechanism (`sitehandler.go`, `site_tenants` table, `SiteConfig` struct). `/home3/knowledge` is an internal admin/KB tool whose menu visibility already lives in `config.local.toml` under `[knowledge-menus]`, a different, operator-facing config domain. Reusing `config/site/` would entangle two unrelated systems (tenant marketing content vs. global operator menu config) for a shallow reason (both happen to be "per-language files"). A new directory keeps the *pattern* (file-per-language) without inheriting tenant-scoping machinery that doesn't apply here.

### DR3 — Direct file read per request, not viper/`AppConfigDef`
Unlike `[knowledge-menus]` visibility (a `config.local.toml`-overridable operator toggle, merged via viper — see `configurable-knowledge-menus` ADR), label files are *whole content files* selected by language, analogous to how `config/site/site-default-<lang>.toml` is a complete file read directly (via `go-toml/v2`, `LoadSiteConfig`) rather than merged/overlaid. There is no "config.local.toml overrides labels-en.toml" concept — the file for a language *is* the config for that language. `kb_menu_handler.go` reads and parses the file for the requested `lang` directly on each request (small file, no meaningful perf concern; matches `kb_config_handler.go`'s existing direct-read-per-request pattern for `GET /api/v1/kb/config`).

### DR4 — Two-tier fallback: requested-language file → hardcoded default. No `[languages].default` tier.
Considered adding a middle fallback tier using `[languages].default` from `config.local.toml` (currently `"zh"`). Rejected: that value doesn't match Paraglide's actual locale codes (`"en"` / `"zh-cn"`) — `"zh"` vs `"zh-cn"` is a pre-existing inconsistency in the codebase, unrelated to this change and out of scope to fix. Adding a fallback tier keyed on a value that can never actually match the requested locale would be dead code. The fallback is simply:
1. `labels-<requested-lang>.toml` has an entry for this id → use it.
2. Otherwise → the existing hardcoded label already in `+page.svelte`'s `menuItems` (today's English text), which continues to serve as the universal baseline exactly as it does today.

### DR5 — Response shape: extend the existing endpoint, don't add a new one
`GET /api/v1/kb/menu-config?lang=<code>` gains a `labels: Record<string,string>` field alongside the existing `menus: Record<string,bool>` field, rather than a second endpoint. The frontend already fetches this endpoint once on mount; splitting labels into a separate call would double a request that serves the same page-load moment for no benefit. `lang` is optional; omitted or unrecognized values simply produce an empty `labels` map (all items fall back to hardcoded defaults) — the same fail-open behavior as an empty `[knowledge-menus]` map today.

### DR6 — Label resolution happens server-side, not client-side
The handler returns only the *resolved* overrides for the requested language (not all languages' data). The frontend does not need Paraglide message-catalog-style bundling of every language's labels — it asks for the one it currently needs, matching how the existing `?lang=`-driven Wiki content endpoints work (request the language you want, get back content for it).

### Alternative Decisions
- Bundle labels into Paraglide's own compile-time message catalog (`web/messages/*.json`) — rejected. Paraglide messages are baked into `web/src/lib/paraglide/messages/*.js` at build time; that makes them developer-editable but not *operator*-configurable at runtime without a rebuild+redeploy, which fails the "operator can configure without a code change" requirement that motivated `[knowledge-menus]` in the first place.
- Nest labels inside `[knowledge-menus]` in `config.local.toml` as `map[string]map[string]string` (id → lang → label) — rejected in favor of DR2/DR3: it would mix an operator-toggle-style merged config (visibility) with a content-file-style per-language artifact (labels) in one file, and `config.local.toml` is explicitly documented as "typically not committed" (gitignored), which is wrong for translated label *content* that should be reviewable/versioned like `config/site/*.toml` already is.
- Reuse the page's existing `?lang=` query param instead of Paraglide — rejected per the user's explicit direction (DR1): that param is already doing a different job (Wiki article content language) and conflating it with UI chrome language was called out as one of the two live-but-unsynced mechanisms the user wants unified around Paraglide, not further entangled.

### Database Migrations
None.

### Data Formats
- New file format, `config/knowledge-menus/labels-<lang>.toml`:
  ```toml
  [labels]
  kb-metrics = "指标"
  kb-doc-wiki = "知识百科"
  ```
  Optional per language; ids not listed use the hardcoded default label.
- `GET /api/v1/kb/menu-config?lang=<code>` response, extended:
  ```json
  {
    "status": true,
    "menus": { "kb-metrics": false },
    "labels": { "kb-metrics": "指标", "kb-doc-wiki": "知识百科" }
  }
  ```
  `lang` query param is optional; omitted/unrecognized → `labels: {}`.

### Environment Variables
None added.

## Risks / Trade-offs

- **Locale-code drift** (`[languages].default = "zh"` vs. Paraglide's `"zh-cn"`) already exists in the codebase; this change deliberately routes around it (DR4) rather than fixing it, since that's a pre-existing, broader inconsistency outside this change's scope. Flagged here so it isn't mistaken for a bug introduced by this change.
- **File-per-request read** for labels (DR3) adds a small filesystem read to every `GET /api/v1/kb/menu-config` call. Acceptable: the file is small (a handful of KB entries), read once per page load, same cost class as the existing `kb_config_handler.go` direct-read pattern.
- **Descriptions remain untranslated** — an operator who wants a fully localized sidebar still sees the English description line under a translated label. Accepted as a stated non-goal; the same file/endpoint shape could add a `descriptions` field later without a breaking change.
- **First-ever Paraglide usage on `/home3/knowledge`** — this page previously had zero Paraglide integration. Adding `getLocale()` is a small, additive import; it does not require wiring the page into Paraglide's URL-prefix routing strategy (which isn't active anyway — `runtime.js` strategy is `["globalVariable", "baseLocale"]`, not `"url"`).

## Migration Plan

No data migration. Deploying with no `config/knowledge-menus/labels-*.toml` files is behaviorally identical to today (hardcoded English labels, regardless of locale). Rollback is deleting the files or reverting the deploy.
