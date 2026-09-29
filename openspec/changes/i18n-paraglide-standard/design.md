## Context

See ADR 2026093001 (KnowledgeStore `doc-repo/adrs/202609/`), which holds the full rationale.

## Goals / Non-Goals

**Goals:** a written rule; automatic failure for new untranslated pages; menu labels maintained in
one place. **Non-Goals:** converting existing pages; server-generated text; removing `kb.page_config`.

## Decisions

- **Ratchet, not a blanket lint.** 5,403 existing strings in 235 files would fail a strict check at
  once. Counts per file in `i18n-baseline.json`; a file may only go down. New files start at 0.
- **Scan the Svelte AST, not regex.** `svelte/compiler` `parse` gives text nodes and attributes
  without false hits from JS expressions; `<code>`/`<pre>` skipped (identifiers, raw values).
  Strings in `<script>` are not scanned — too noisy (ids, CSS, API values); the rule covers them.
- **Menu defaults from the existing DB text.** `nav_*` values are copied from the `kb.page_config`
  labels (en override where one existed, e.g. *Semantic Assertion Evidence*), so the menu looks the
  same; then the duplicated DB labels are removed by exact-match migration so they cannot shadow
  later edits.

## Risks / Trade-offs

- Production may hold labels that differ from dev; those are kept (exact match only) and keep
  overriding — visible in the admin page, removable by hand.
- The ratchet counts, it does not diff: converting one string and adding another in the same file
  passes. Accepted; review covers it.
