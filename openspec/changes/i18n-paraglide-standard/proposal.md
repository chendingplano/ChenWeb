## Why

Every ChenWeb page must work in English and Chinese, now and for future pages. Translations were
being added as `kb.page_config` rows, which is manual, unchecked and needs dev→prod data sync; most
pages have their text hard-coded. ADR 2026093001 makes Paraglide the standard; this change puts the
rule in place and stops new hard-coded text.

## What Changes

- `web/scripts/check-i18n.ts`, run by `bun run check`: message parity between `en.json` and
  `zh-cn.json` (hard failure) and a per-file ratchet on hard-coded `.svelte` text
  (`web/i18n-baseline.json`).
- The `/development` NavRail menu (`nav-rail.svelte`) takes every label, group heading, rail title
  and user-menu text from Paraglide (`nav_*` keys); 21 items get a Chinese label for the first time
  (incl. *Review Metrics* → 指标审查).
- Migration removes `label` from the 98 `kb.page_config` menu rows whose label equals the new
  default (rows kept for visibility/access; operator renames untouched).
- `ChenWeb/CLAUDE.md` §3 rule; ADR 2026093001; pointer in spec 2026072001 §11.

## Capabilities

### New Capabilities
- `frontend-i18n`: interface text through Paraglide in both languages, enforced by `bun run check`.

### Modified Capabilities
<!-- page-config visibility/access behaviour is unchanged -->

## Impact

- Web: `nav-rail.svelte`, `messages/*.json`, `scripts/check-i18n*.ts`, `i18n-baseline.json`,
  `package.json`.
- DB: data-only migration `20260930000001_strip_nav_label_overrides_now_in_paraglide.sql`.
- Out of scope: converting the 235 files in the baseline (follow-up changes).
