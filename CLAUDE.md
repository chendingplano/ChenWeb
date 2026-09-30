This file provides guidance to code in ChenWeb.

# 1. Coding Guidelines

## 1.1 Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 1.2 Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 1.3 Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 1.4 Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

# 2. Prompts
* All prompts should be saved in `prompts`. File names must be prefixed with `prompt-` and endded with `-v<number>.md` (for markdown) or `_v<number>.txt` for other types. Example: `prompt-for-my-test-v1.md. Example: `prompt-for-my-test-v1.md`
* NEVER hard-code prompts in code

# 3. Internationalization (English + Chinese)
Every page must work in English and Chinese. Decision and rationale: ADR
`KnowledgeStore/doc-repo/adrs/202609/2026093001-adr-paraglide-standard-i18n.md`.

* **All user-visible text goes through Paraglide.** In `.svelte` files write `{m.<key>()}` (import
  `{ m } from '$lib/paraglide/messages.js'`), never literal text — this includes `placeholder`,
  `title`, `aria-label`, `alt`, button labels, menu labels, empty/error/status messages, and strings
  built in `<script>` that end up on screen.
* **Add each key to both `web/messages/en.json` and `web/messages/zh-cn.json` in the same change**,
  with a real Chinese translation (not a copy of the English). Prefix keys by page/feature (e.g.
  `mrv_*` for Review Metrics, `nav_*` for the side menu). Use parameters (`{count}`) instead of
  concatenating translated fragments.
* **Do not add `kb.page_config` rows to translate text.** `kb.page_config` is only for
  operator-controlled visibility, role-based access, and runtime label overrides.
* **New menu item** in `nav-rail.svelte`: `label: m.nav_<id>()` (id with `-` → `_`) plus both keys.
* **Verify with `bun run check`** (from `web/`). It fails when the two message files differ or when a
  `.svelte` file has more hard-coded text than `web/i18n-baseline.json` allows (new files: zero).
  Hard-coded text = markup text, `placeholder`/`title`/`aria-label`/`alt`/`label` text, and text-like
  string literals inside markup expressions (`{busy ? 'Saving…' : 'Save'}`). It cannot see strings
  in `<script>`; those are still your responsibility. `bun scripts/check-i18n.ts --list <file>` shows
  what it found. After converting a page, run `bun scripts/check-i18n.ts --update` to lower its
  baseline — never to allow new hard-coded text.
* **Converting an existing page** — use `web/scripts/i18n-extract.ts` (see its header):
  1. `bun scripts/i18n-extract.ts --pending p.json <files>` converts markup (sentences with values
     become one message with named params; `{n === 1 ? '' : 's'}` becomes `{plural}`).
  2. `--script-candidates <files> > c.json`, delete entries that are not displayed text (ids, log
     lines, sample data, values sent to the API), then `--script-apply c.json p.json`.
  3. Write the Chinese (or English, for Chinese source text) for every key in `p.json` and apply
     with `--apply`. Keep params identical; `{plural}` may be dropped in Chinese.
  4. Rename a local variable called `m` first (the tool skips such files). Check for English
     grammar passed as a param (`' has' : 's have'`) and fix by hand.
  5. `bun run check`, then `--update` the baseline.
* **Text generated on the server** (LLM output, reports) is not covered by Paraglide: pass
  `getLocale()` to the API and produce/store it per language (see Review Metrics, `lang`).
* Locale in code: `getLocale()` from `$lib/paraglide/runtime` (`en` | `zh-cn`); a language switch
  reloads the page.
