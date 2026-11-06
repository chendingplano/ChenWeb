## Context

Three extractors already have a gold benchmark, all built the same way:

1. A gold skill in `.agents/skills/<x>-benchmark`. It reads the whole line file of one
   `kb.inputs` record, inventories candidates, labels them under a frozen rules file
   (`rules/<version>.md`, SHA-256 in `rules/versions.json`), audits the result, then validates
   and saves one immutable run with `scripts/benchmark_io.py`. A run is a row in
   `testbed.<x>_benchmark_runs` plus its accepted rows and its excluded/duplicate candidates.
2. A scorer skill in `.agents/skills/score-<x>`. `score_io.py export` writes `input.json` (gold,
   predictions, ledger, suggestions). An agent or LLM writes `matches.json`. `score_io.py score`
   checks the matches and computes `round(100 × soft F1)`.
3. ChenWeb pages under System Admin › Benchmark (development page only):
   - a gold viewer, which is the production artifact view with `source="testbed"`;
   - a scoring page that runs the scorer in the background (`executeScore` in
     `metric_score_runner.go`, parameterised by `scoreKind`) and stores each run in
     `kb.<x>_score_runs`.

`extract_provisions` writes `kb.provisions` rows with these fields: `prov_id`
(`<record>_prv_<n>`), `provision_type` (`mandatory|recommended|optional`), `provision`,
`source_text`, `source_line_spans` (`"N"` / `"N-M"`), `provision_subject`, `location_type`,
`is_explicit`, `prov_name`, `model_name`, `prompt_name`, `extract_id`. Only record 638 has rows
today. Record 416 (168 lines), which the user chose for the first gold run, has none. Its July
`.provisions` artifact predates the September line-file regeneration.

## Goals / Non-Goals

**Goals:**
- A gold skill and rules 1.0.0 that decide, for every line of a document, which statements are
  provisions and how to label them. Rules must be domain-neutral (any document kind, language
  or domain).
- A reproducible 1–100 score for `kb.provisions` against one gold run, with a per-field
  breakdown.
- Gold Provisions and Benchmark pages that work like their Metrics counterparts.
- A saved first gold run for record 416.

**Non-Goals:**
- Changing `extract_provisions`, its prompt, or `kb.provisions`.
- Benchmarking provision objects (`objects[]`), keywords, category paths, descriptions or
  context. They are free text or derived data, with no gold-checkable meaning yet.
- A "Review Provisions" page (Metrics has Review Metrics; it is not part of the benchmark).
- Fixing the References Benchmark page. `freezeScoreTools` now requires `weights.json`, which
  `score-extract-references` lacks, so new reference score runs would fail with
  `SCORER_UNAVAILABLE`. This is reported separately.

## Decisions

### D1. Gold unit: one normative statement (user ruling)

A provision is one normative statement. Normally that is a sentence, or a list item or table cell
that is not a full sentence.

- Consecutive sentences are separate provisions.
- A sentence is split only where its provision type changes (应 → 可) or its explicit grammatical
  subject changes. A clause without its own subject stays with the clause before it.
- A lead-in plus its list items ("应符合下列要求：a)…") gives one provision per item, with
  `location_type = heading_context`. The lead-in gives each item its modal verb.

*Alternatives:*
- One row per atomic modal clause. The user rejected it: too many rows, and it penalises
  extractors for reasonable merges.
- One row per numbered clause. Too coarse: it hides type and subject changes.

### D2. Labels: type, kind, explicitness

**Kind and type.** `provision_kind` is a gold-only refinement. It maps to `provision_type`, the
field production stores:

| `provision_kind` | `provision_type` |
|---|---|
| `requirement` (应/必须/shall/must) | `mandatory` |
| `prohibition` (不应/不得/禁止/shall not) | `mandatory` |
| `instruction` (imperative action, including test-method steps — user ruling) | `mandatory` |
| `recommendation` (宜/不宜/鼓励/提倡/should) | `recommended` |
| `permission` (可/可以/不必/may/need not) | `optional` |

`validate` enforces the mapping. The kind lets the report give recall per kind. Production
cannot be graded on the kind itself.

**Explicitness.** `is_explicit` is true when the statement carries a normative marker (a modal
verb, a recommending verb, or the imperative mood). It is false when a declarative sentence in a
normative clause prescribes an action, a responsible party or a state without one. Example:
"可回收物运输至资源回收处理单位。" That case is rule **A5**: mandatory, implicit.

**Excluded material.** These are ledger candidates under **X** rules, not provisions:
statements about the document itself; scope; the normative-reference boilerplate and list
entries; definitions and descriptive text; pure pointers ("见表1"); objectives and principles
that prescribe nothing checkable; statements of possibility or fact (能/可能/can); and table value
rows that another provision invokes ("应达到表2的要求"). The value rows belong to the metrics
benchmark; the invoking sentence is the provision. Rule IDs are bold in the rules file, as in
the other skills, so `validate` can parse them.

**`location_type`.** Uses the extractor's vocabulary (`sentence paragraph bullet table_row
table_cell heading_context mixed`):
- Decided from the cited unit and the parser's line types.
- `bullet` means a whole `list-item` line, including numbered clauses the parser types as
  list items.
- `sentence` means part of a paragraph or list item.
- Table lines require `table_row` or `table_cell`, and `validate` checks this.

### D3. Gold storage mirrors production columns

`testbed.provisions` keeps the production names for the checked fields: `prov_id`,
`provision_type`, `provision`, `provision_en`, `prov_name`, `prov_name_en`, `provision_subject`,
`provision_subject_en`, `location_type`, `is_explicit`, `source_line_spans`. It adds the gold
fields `section`, `provision_kind`, `source_table_rows`, `evidence`, `confidence`, `ext_info`,
the run key, and the source path and hash.

- Spans use the production form (`"N"` / `"N-M"`), not the references skill's `N:M`. The same
  spans can then be compared with no conversion.
- Gold IDs are `<record>_gprv_<n>`.
- `provision` is exact source text. Non-contiguous parts are joined with ` … `, and `validate`
  checks each part against the cited lines.
- `_en` names and subjects are required for non-English documents. `provision_en` is optional.

The run and dropped tables copy `testbed.reference_benchmark_runs` and
`testbed.ref_mentions_dropped`, without `document_identifier`.

*Alternative:* reuse `testbed.metrics`-style columns. Rejected: a provision is not a metric, and
a separate table keeps each gold set's checks local.

### D4. Scoring

A pair means both rows state the same normative statement: they share a source line and impose
the same obligation on the same subject. Pairs are one-to-one.

- **Merges.** When the extractor merges several gold statements into one row, the closest gold
  row is paired and the others are `missed`.
- **Splits.** When it splits one gold statement into several rows, one row is paired and the
  others are false positives with the new cause `split`, which names the gold row (`gold`).
- **Other false-positive causes:** `gold_excluded` (with the ledger `candidate_id`/`rule_id`),
  `duplicate`, `not_in_gold`, `invalid`.

Field checks on a pair. Default weights are in `weights.json`; the runner freezes that file,
which also makes it a required file. Weights are relative and normalised to 1.

| Check | Weight | How |
|---|---:|---|
| `provision_type` | 30 | equal |
| `text` | 20 | `difflib` ratio of NFKC, whitespace-free `provision` texts (production falls back to `source_text`); partial credit |
| `lines` | 20 | Jaccard of line sets; partial credit; left out (rest renormalised) when line numbers are not comparable |
| `subject` | 15 | normalised equality; overridable (paraphrase) |
| `location_type` | 10 | equal |
| `is_explicit` | 5 | equal |

- Score: `round(100 × soft F1)`, clamped to 1–100.
- Report groups: by `provision_type` (both sides), and recall by gold `provision_kind`.
- Overrides are allowed only for `subject`.
- Exit codes: 3 `NO_GOLD`, 4 `NO_PREDICTIONS`.

*Alternative:* the metrics scorer v3 model, where missing and false-positive rows carry their own
penalty weights. Rejected for 1.0: the references model is simpler to explain and enough for a
first gold set. `weights.json` keeps the weights adjustable.

### D5. ChenWeb integration reuses the existing benchmark machinery

**Backend.**
- `goldRunTables["provisions"] = "testbed.provision_benchmark_runs"` drives the gold kb-inputs
  list and `/kb/provisions/gold-options`.
- `ListProvisions` branches to `listGoldProvisions` for `source=testbed`, as `ListReferences`
  does.
- `provision_score_handler.go` copies `reference_score_handler.go` with
  `provisionScoreKind{"PROVISION_BENCHMARK_SCORER_PATH", "score-extract-provisions",
  "extract-provisions-benchmark", "provision-score-runs", …}`.
- `executeScore` retries a failed match once with validator feedback when `matchBasis` is
  `"metric"`. Provision matching uses the same repair loop, so the condition is widened to
  include `"provision"`.
- The scorer's validation messages are added to the runner's allowlist of prefixes. Only
  recognised messages reach the UI.

**Frontend.**
- **Gold Provisions** is `ProvisionMgmtView` with `source="testbed"`. A `GoldRunFilter('provisions')`
  drives the record browser, and a Gold/Dropped switch selects the list. Extract, save, edit,
  delete and raw-line editing are hidden in testbed mode.
- **Benchmark** is a new `provision-score-view.svelte`, a pared-down copy of
  `metric-score-view.svelte` on `BenchmarkReportViewer`:
  - pick a document (with-gold filter), a model and a gold run, then run;
  - history, then the report with score cards, field accuracy, pairs, missed and false
    positives;
  - selecting an entry highlights its source lines in the PDF.
  - The pairs list shows the gold-versus-extracted comparison inline. This avoids widening the
    context-shelf contract for one more kind.

### D6. Predictions for record 416

Record 416 has no `kb.provisions` rows. Scoring it needs one `extract_provisions` run through the
doc-processor. Starting the doc-processor drains queued NATS events, so the user decides when to
run it. Until then, the Benchmark page answers 416 with `NO_PREDICTIONS`, which is the designed
behaviour.

## Risks / Trade-offs

- **[Sentence-level gold is harsh on extractors that emit one row per numbered clause]** →
  `split` and `missed` notes make the pattern visible. If it proves too strict, a rules 2.0.0
  can change the unit without touching run 1.0.0.
- **[`text` similarity rewards copying the source]** → That is intended: production `provision`
  is supposed to be the original text. Its weight (20) is below `provision_type`.
- **[A single gold annotator (the invoking model)]** → The completeness and correctness reviews
  plus the audits in the rules file are required before saving. Runs are immutable, and a
  re-annotation is a new run.
- **[`kb.provisions.location_type` values outside the vocabulary (`list-item` on record 638)]** →
  These fail the check. That is a real extractor defect and is reported.

## Migration Plan

1. Two goose project migrations (testbed tables; `kb.provision_score_runs` + page_config). The
   live `mise dev` server applies them, or `doc-service --migrate-only` does. Down migrations
   drop what they create.
2. Skills need no deployment beyond the `.agents` repo. Pages appear on the development page
   only.
3. Rollback: down migrations; revert the nav entries.

## Open Questions

- When to run `extract_provisions` on record 416 (D6).
- Whether later rules versions should add a gold `condition` field. The prompt asks for
  conditions inside the text; 1.0 keeps them in `provision`.
