## Context

`kb.metric_reviews` holds one row per review run; the page shows a record's newest row.
The prompt forces English prose. The UI locale comes from paraglide (`getLocale()`,
`en` | `zh-cn`); switching locale reloads the page, so the locale is stable for a page
session.

## Goals / Non-Goals

**Goals:** report prose in the UI language; reuse an existing review in another
language by translation instead of re-review; export the displayed review.

**Non-Goals:** translating document quotes, metric names from the snapshot, stored
values or enum codes; a history view; server-side PDF rendering; languages beyond the
two paraglide locales.

## Decisions

**D1 — Language is a column, and every lookup is per language.** Add
`lang TEXT NOT NULL DEFAULT 'en'` and `translated_from_id BIGINT NULL` (the source
review of a translation). The index becomes `(input_record_id, lang, created_at DESC)`.
"Latest review" and the D3 cache rule of the base change (running → reuse, done → reuse
unless force, failed → rerun) apply within one language. Existing rows were all written
in English, so the default `en` is correct for them. Unsupported `lang` → 400.

**D2 — Output language is an input field, not a code-appended instruction.** The v2
prompt says "write all prose in the language named by `output_language` in the DOCUMENT
header"; the server adds `output_language: <code> (<name>)` to the header. Enum values,
metric IDs, field names and document quotes stay verbatim. v1 is kept unchanged so older
reviews' `prompt_name` still identifies the text that produced them.

**D3 — Translation translates only prose, keyed by path.** The server collects the
translatable strings (`summary`, `missed_metrics[i].name|reason`,
`non_metrics[i].reason`, `attribute_issues[i].stored|suggested|reason`, `recommendations[i]`)
into `{"strings": {"<path>": "<text>"}}`, sends them with the translate prompt, and
writes the returned strings back by path. Everything else (tally, IDs, severities,
categories, field names, line numbers, values/units, snapshot) is copied from the source.
`stored`/`suggested` are translated only when they are more than one word, because the
model often writes prose there (first live run: "Comparator included in value…"); a
single token is a literal (code, number, unit) and is never sent — the first live run
had translated `qualitative` to 定性. The prompt also tells the model to keep codes. A missing or
empty returned key keeps the source text, so a partial answer never loses content.
Alternative rejected: asking the LLM to re-emit the whole report JSON — larger output
and it could alter IDs or counts that the server guarantees.

**D4 — Translation is its own endpoint and run.** `POST .../:record_id/translate {lang}`
picks the newest *done* review of the record in any other language as the source,
inserts a `running` row (`lang` = target, `translated_from_id` = source,
`model_name`/`metrics_count` from the run) and translates in the background, reusing
the review's polling. If a review in the target language is already running or done it
is returned instead (no double translation). Uses the same model
(`REVIEW_METRICS_MODEL_NAME`); prompt from `REVIEW_METRICS_TRANSLATE_PROMPT`; call
reason `review_metrics_translate`.

**D5 — GET tells the page what else exists.** `GET ...?lang=X` returns
`{review, other_langs}` where `other_langs` lists languages ≠ X that have a done review.
`other_langs` is filled when `review` is null **or failed** — a failed translation (first
seen live: the env var was missing until `mise dev` restarted) must stay retryable
without a full re-review. The page shows a translate prompt in both cases when
`other_langs` is non-empty;
the user confirms with a **Translate** button (or can run a fresh Review instead).

**D6 — Export is client-side.** The report is already on the page, so no endpoint.
Markdown is built by a pure function in `metric-review-client.ts` (unit-tested) and
downloaded as `review-<record>-<lang>.md`. PDF: render that Markdown to HTML with the
existing `marked` dependency into a new window with print CSS and call `print()`; the
browser's "Save as PDF" handles CJK fonts, which a JS PDF library would need embedded
fonts for. Section headings and labels in the export follow the review's language.

## Risks / Trade-offs

- Browser print is one extra click versus a direct download → accepted; it keeps CJK
  output correct with no new dependency. Pop-up blockers: the window is opened from the
  click handler.
- A translation inherits the source review's errors; the page shows "translated from
  review #N" so it is clear it is not an independent review.
- Production must set the new env vars, or reviews/translations fail with an error naming
  the variable (same behaviour as today).
