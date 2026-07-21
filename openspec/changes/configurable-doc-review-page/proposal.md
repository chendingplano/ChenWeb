## Why

The Document Review wizard (`document-review-view.svelte`, reached via
`/development` → Applications → 文档审核) hard-codes all of its UI chrome —
title, subtitle, the four step-indicator labels, per-step headings, navigation
buttons, form field labels, and the P1–P6 aspect-group labels — in English in
the `.svelte` source. Operators cannot rename or translate any of it, and the
page renders English even when the Paraglide locale is `zh-cn`. This is the
same gap already solved for `/home3/knowledge` and `/semos/workspace`; the
KnowledgeStore spec §11 recipe exists precisely to onboard more pages onto the
DB-backed page-config capability.

## What Changes

- Onboard the Document Review page onto the existing DB-backed page-config
  capability (`kb.page_def` / `kb.page_config`, resolver
  `GET /api/v1/page-config/:pageKey`) following the §11 recipe. **No new Go
  handler, route, or table** — the resolution/admin backend is page-agnostic.
- Add one `kb.page_def` row (`page_key = doc-review`) and one `kb.page_config`
  row per configurable entry per language (`en`, `zh-cn`) via a goose migration
  in `project_migrations/`, seeded idempotently so English rendering is
  unchanged and `zh-cn` gains translated labels.
- Wire `document-review-view.svelte` to fetch `getPageConfig('doc-review',
  getLocale())` into a nullable `$state`, fail open to the hardcoded defaults,
  and drive every page-owned static string through `isVisible` / `labelFor`
  (overlay model, §4.1 / §11.2).
- Surface a `console.warn` diagnostic for unknown/stale `entry_key`s (§4.4).
- **Out of scope:** the tiers and aspects (Must/Should/External review levels
  and their chips) stay sourced from `doc-review.local.toml` via
  `listTiers()` / `listAspects()` — only page-owned static text is migrated.

## Capabilities

### New Capabilities
- `configurable-doc-review-page`: Makes the Document Review wizard's page-owned
  static text (headings, step labels, buttons, field labels, P1–P6 group
  labels) visibility-toggleable and per-language translatable through the
  DB-backed page-config resolver, keyed by stable `page_key + entry_key`, with
  fail-open fallback to the hardcoded defaults.

### Modified Capabilities
<!-- None. The page-config backend (resolver + admin API + tables) already
     exists and is page-agnostic; this change only adds data + a frontend
     consumer, so no existing capability's requirements change. -->

## Impact

- **Frontend:** `web/src/lib/components/home3/document-review-view.svelte`
  (add page-config fetch + `isVisible`/`labelFor` resolvers over all static
  strings; add `getLocale` import). Reuses `getPageConfig` from
  `web/src/lib/services/pageConfigService.ts` (no change).
- **Database:** new goose migration under `project_migrations/` seeding
  `kb.page_def` (1 row) and `kb.page_config` (entries × {en, zh-cn}). Applied
  automatically by the running `mise dev` / air on rebuild.
- **Admin:** the page appears automatically in `/semos/admin/page-config` once
  its `kb.page_def` row exists — no admin wiring.
- **No backend Go changes; no new route; no nav-rail entry** (the page is not a
  standalone route — it is an app view inside the `/development` dashboard).
