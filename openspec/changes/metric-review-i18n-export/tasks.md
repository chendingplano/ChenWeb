## 1. Database

- [x] 1.1 Goose migration: add `lang TEXT NOT NULL DEFAULT 'en'`, `translated_from_id BIGINT`
      to `kb.metric_reviews`; replace the record index with `(input_record_id, lang, created_at DESC)`.

## 2. Prompts and config

- [x] 2.1 `prompts/prompt-review-metric-extraction-v2.md`: v1 with output language taken from `output_language`.
- [x] 2.2 `prompts/prompt-translate-metric-review-v1.md`: translate `{"strings": {...}}` by key.
- [x] 2.3 `mise.local.toml`: `REVIEW_METRICS_PROMPT` → v2, add `REVIEW_METRICS_TRANSLATE_PROMPT`.

## 3. Server

- [x] 3.1 Language normalisation (`en`, `zh-cn`), `lang` on GET (query) / POST (body); per-language
      latest lookup; `other_langs` in the GET response; `output_language` in the LLM input header.
- [x] 3.2 `POST /kb/metric-reviews/:record_id/translate`: source selection, reuse rules, background run.
- [x] 3.3 Translation core: collect prose strings by path, apply returned strings (fallback to source).
- [x] 3.4 Register the route; unit tests for lang normalisation, string collection/apply, input header.

## 4. Web

- [x] 4.1 Client: `lang` on get/start, `translateMetricReview`, `other_langs`, `translated_from_id`;
      `buildReviewMarkdown(record, review, labels)` with unit tests.
- [x] 4.2 View: pass `getLocale()`; translate prompt + Translate button; "translated from #N" note.
- [x] 4.3 Export pulldown (Export Markdown / Export PDF) next to Review.
- [x] 4.4 Page labels in `messages/en.json` / `messages/zh-cn.json`.

## 5. Verification and docs

- [x] 5.1 `go build`, `go test ./server/api/kbhandler/...`, `bun run check`, client tests.
- [x] 5.2 Live check on record 416: `zh-cn` lookup shows translate prompt; translate; export both formats.
      Verified 2026-09-29 with a temporary harness (since deleted) against the dev DB + real
      model (gpt-6-luna): translating review #4 (en) to zh-cn took ~13 s, tally/IDs/codes
      unchanged; the first run translated the code `qualitative` → 定性, fixed by skipping
      single-token stored/suggested values. A fresh zh-cn review with the v2 prompt (~50 s)
      came back in Chinese with English codes intact. Migration applied to `miner` (existing
      4 rows → `en`). NOT verified: the page in a browser (translate prompt, Export menu,
      print dialog) — needs a logged-in session and a `mise dev` restart to load the new env
      vars.
- [x] 5.3 Update devdoc `2026092907-devdoc-review-metrics-page.md`; commit via `jj` (only this change's files).
