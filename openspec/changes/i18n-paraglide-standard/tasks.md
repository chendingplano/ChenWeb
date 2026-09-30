## 1. Decision

- [x] 1.1 ADR 2026093001; pointer in spec 2026072001 §11; `ChenWeb/CLAUDE.md` §3.

## 2. Enforcement

- [x] 2.1 `web/scripts/check-i18n.ts` (parity + AST ratchet) with unit tests.
- [x] 2.2 `web/i18n-baseline.json` generated (235 files / 5,403 items); `bun run check` runs it.
- [x] 2.3 Verified: fails for a new file with hard-coded text and for a key missing in zh-cn; passes on the tree.

## 3. Menu labels

- [x] 3.1 `nav-rail.svelte`: labels (`nav_<id>`), group headings, rail title, user menu via `m.*()`.
- [x] 3.2 `nav_*` messages: en/zh from existing `kb.page_config` labels; 21 new Chinese labels.
- [x] 3.3 Migration stripping the 98 duplicated labels (dry-run in a rolled-back transaction).
- [ ] 3.4 Browser check of the menu in both languages after the next server start (migration applies then).

## 4. Conversion

- [x] 4.1 Converter `web/scripts/i18n-extract.ts` (markup, markup-expression literals, reviewed
      `<script>` strings) with unit tests; check extended to markup-expression literals.
- [x] 4.2 Batch 1 (2026-09-30): the 50 smallest `home3` components — 0 hard-coded items left;
      ~740 messages with Chinese. Verified: svelte-check, i18n check, 46/50 components
      server-rendered in en and zh-cn with a throwaway harness (other 4: Vite-only glob, snippet
      prop, workspace package, `$app` inside a store — harness limits, not conversion errors).
- [ ] 4.3 Remaining batches (baseline after batch 1 + stricter rule: 195 files / 5,664 items).
