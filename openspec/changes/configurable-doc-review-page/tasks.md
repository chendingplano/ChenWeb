## 1. Database seed (goose migration)

- [x] 1.1 Add `project_migrations/<timestamp>_seed_page_config_doc_review.sql`
      with a `-- +goose Up` inserting one `kb.page_def` row
      (`page_key='doc-review'`, `route='/development'`, title/description).
- [x] 1.2 In the same migration, insert the `en` rows for all 36 entry_keys
      (content `{}`) and the `zh-cn` rows (translated labels per design D4),
      idempotent via `ON CONFLICT (page_key, entry_key, language) DO NOTHING`.
- [x] 1.3 Add the `UPDATE kb.page_config SET access_role = '[...]'` for
      `page_key='doc-review'` where `access_role IS NULL` (current
      `[system].access_roles` set), plus a `-- +goose Down` deleting all
      `page_key='doc-review'` rows from `kb.page_config` and `kb.page_def`.
- [x] 1.4 Applied the migration to the `miner` dev DB (air only rebuilds on Go
      writes, not new `.sql`), recorded `20260721000002` in
      `project_db_migration`; verified 72 rows (36 `en` + 36 `zh-cn`), correct
      `zh-cn` labels, `access_role`, and the `doc-review` `page_def` row.

## 2. Frontend wiring (document-review-view.svelte)

- [x] 2.1 Import `getPageConfig, type PageConfig` from
      `$lib/services/pageConfigService` and `getLocale` from
      `$lib/paraglide/runtime`.
- [x] 2.2 Add `let pageConfig = $state<PageConfig | null>(null)` and, in the
      existing `onMount`, `getPageConfig('doc-review', getLocale()).then(cfg =>
      pageConfig = cfg).catch(() => {})` (fail open).
- [x] 2.3 Add `isVisible(id)` and `labelFor(id, fallback)` helpers over
      `pageConfig` (overlay model, §11.2), plus a known-id set and a
      `$derived` + `$effect` that `console.warn`s unknown resolver ids (§4.4).
- [x] 2.4 Wrap every scoped static string (all 36 entries in design D4) with
      `labelFor('<entry_key>', '<hardcoded default>')`; wrap `groupLabels`
      lookups in `groupAspectNames` with `labelFor('dr-group-p{n}', default)`.
- [x] 2.5 Keep tiers/aspects, dynamic count text, "Depth {n}", and intro
      paragraphs untouched (out of scope).

## 3. Verification

- [x] 3.1 `svelte-check` passes: 0 errors (23 pre-existing warnings in
      unrelated `home5/home6`, none in `document-review-view.svelte`).
- [x] 3.1a Resolver endpoint routes and is auth-gated
      (`GET /api/v1/page-config/doc-review` → HTTP 401 unauthenticated); it is
      the same page-agnostic code path already serving `home3-knowledge`.
- [~] 3.2 Locale `en`: all `en` content is `{}` → hardcoded defaults, so
      rendering is unchanged by construction; confirm in the authenticated
      browser (data + fail-open verified).
- [~] 3.3 Locale `zh-cn`: seeded Chinese labels verified in the DB; confirm the
      rendered page in the authenticated browser (untranslated → EN fallback).
- [~] 3.4 In `/semos/admin/page-config`, the `doc-review` page appears once its
      `page_def` row exists (verified present); confirm disabling
      `dr-subtitle` / a Step-4 template field hides it on next load.
- [x] 3.5 All 36 seeded `entry_key`s are in the component's `knownPageConfigIds`
      set by construction, so no unknown-id `console.warn` fires for seeded data.

_Note: `[~]` items are verified at the data/build/routing layer; the final
in-browser confirmation needs an authenticated session (the requester's browser
is already logged in per the screenshot). No dev auth bypass exists to automate
this here._
