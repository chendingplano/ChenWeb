## Why

The Review Metrics page (change `llm-review-metrics`) always writes its report in
English, while most users work in Chinese and the documents are Chinese. Reviews are
cached per record, so a user switching the UI to 中文 still sees the English report,
and the only way to get a Chinese one is to pay for a full re-review. There is also no
way to take a report off the page to share it.

## What Changes

- Each review is stored with a language (`kb.metric_reviews.lang`, `en` | `zh-cn`).
  The page passes the current UI locale; a new review writes its prose in that language.
- Loading a review looks only at the current language. When none exists but a finished
  review exists in another language, the page asks whether to translate it; on
  confirmation the server translates the report's prose with the LLM (much cheaper than a
  re-review) and stores the result as a new review row in the current language.
- An **Export** pulldown next to **Review** offers *Export Markdown* (downloads a `.md`)
  and *Export PDF* (opens the browser print dialog on a print-styled copy of the report,
  where the user chooses "Save as PDF"). Only the displayed (current-language) review is
  exported.
- New prompt `prompt-review-metric-extraction-v2.md` (output language taken from the
  input) and new prompt `prompt-translate-metric-review-v1.md`, named by env var
  `REVIEW_METRICS_TRANSLATE_PROMPT`.
- Page labels move into paraglide messages so the page chrome follows the locale too.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `metric-extraction-review`: reviews become per-language; translation of an existing
  review; export to Markdown/PDF. (The base spec is still under the unarchived change
  `llm-review-metrics`, so the delta is written as ADDED requirements.)

## Impact

- DB: goose migration adding `lang` and `translated_from_id` to `kb.metric_reviews`
  (existing rows become `en`).
- API: `GET /api/v1/kb/metric-reviews/:record_id?lang=` now filters by language and
  returns `other_langs`; `POST` body gains `lang`; new
  `POST /api/v1/kb/metric-reviews/:record_id/translate` `{lang}`.
- Server: `server/api/kbhandler/metric_review_handler.go` (+ tests), route registration.
- Web: `metric-review-view.svelte`, `metric-review-client.ts` (+ tests), `messages/*.json`.
- Config: `REVIEW_METRICS_PROMPT` → v2 and new `REVIEW_METRICS_TRANSLATE_PROMPT` in
  `mise.local.toml` and production env.
- Docs: devdoc `2026092907-devdoc-review-metrics-page.md`.
