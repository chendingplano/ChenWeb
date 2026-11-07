## ADDED Requirements

### Requirement: Reproducible provision score
The `score-extract-provisions` skill SHALL score a record's `kb.provisions` rows against one gold
run (`testbed.provision_benchmark_runs`) on a scale of 1 to 100.

The work is split in three steps:
1. `export` writes the complete `input.json` audit snapshot (gold, predictions, ledger,
   suggestions and warnings) and the compact `matching-input.json` model view. `project`
   SHALL regenerate the model view from a finalized snapshot without querying the database.
2. The matcher writes compact v2 `matches.json`; the script expands it to
   `matches-expanded.json`: `pairs`, `missed`, `false_positives`, `overrides`.
3. `score` computes credit per pair from weighted field checks: `provision_type`, `text`,
   `lines`, `subject`, `location_type`, `is_explicit`. Default weights come from `weights.json`.
   `score` writes `score.json` and `report.md`. The score is `round(100 × soft F1)`, clamped to
   1–100.

#### Scenario: Perfect extraction
- **WHEN** every gold row is paired with a prediction whose checked fields all agree, and there are no false positives
- **THEN** the score is 100

#### Scenario: No gold
- **WHEN** `export` runs for a record without a gold run
- **THEN** it exits 3 with a `NO_GOLD` message and writes nothing

#### Scenario: No predictions
- **WHEN** the record has a gold run but no `kb.provisions` rows
- **THEN** `export` exits 4 with `NO_PREDICTIONS`

### Requirement: Compact model contract with complete audit evidence
The model view SHALL contain only version `v=2`, source-comparability `lc`, source-availability
`sa`, and arrays `g`, `p`, `c`. Gold and prediction rows SHALL be `[source_line_spans, statement,
subject]`; ledger rows SHALL be `[source_line_spans, text, rule_id, reason]`. A row's local ID
SHALL be its one-based array position. All rows SHALL be retained, in audit-snapshot order.
The matching request SHALL retain the complete available source document and output language,
but SHALL NOT send translations, names, confidence, deterministic field grades, suggestions,
run metadata or provenance. The complete evidence SHALL remain in the saved audit snapshot.

The response SHALL contain exactly `v=2`, `p`, `m`, `f`, `o`, with all arrays explicit:
- `p`: `[gold,prediction]`, optionally followed by an exceptional explanation.
- `m`: `[gold,paired_prediction]` for a merge, or `[gold,0,reason]` for a genuine miss.
- `f`: `[prediction,cause,target]`, optionally followed by an exceptional explanation.
  Causes SHALL be `x` (ledger index), `s` (paired gold index), `d` (other prediction index),
  `n` (not considered by gold), or `i` (misreads source). `n`/`i` SHALL have null targets and
  nonempty explanations; `n` SHALL address whether gold missed a real provision.
- `o`: `[paired_gold,boolean,reason]`, subject-only overrides with a nonempty reason.

The decoder SHALL restore full IDs, ledger rules and routine explanations locally, preserving
exceptional explanations. Unknown keys/versions/causes, noninteger or out-of-range indices,
wrong target types, duplicate overrides, self/cyclic duplicate references and invalid row
lengths SHALL fail. Merge targets SHALL be paired and share a line when `lc=true`.
No missing decision SHALL be inferred. The existing formula and field checks SHALL remain unchanged.

#### Scenario: Captured decisions are compacted
- **WHEN** compact decisions are expanded against the same full audit snapshot
- **THEN** their score, aggregate/per-type metrics, per-kind recall and field accuracy equal those of the original decisions

#### Scenario: Changed or unavailable source
- **WHEN** the source is changed or unreadable
- **THEN** `lc` is false and line checks are disabled; `sa` distinguishes available changed source from unavailable source

#### Scenario: Old scoring snapshot replay
- **WHEN** legacy verbose matches are scored without `--require-compact`
- **THEN** they remain readable; new UI scoring requires the compact contract

### Requirement: Valid matches only
`score` SHALL refuse `matches.json` unless all of the following hold:
- Every gold row appears exactly once in `pairs` or `missed`.
- Every prediction appears exactly once in `pairs` or `false_positives`.
- Paired rows share a source line, unless line numbers are not comparable.
- Every false positive has a cause from `gold_excluded duplicate split not_in_gold invalid`.
  `gold_excluded` names a ledger candidate, `duplicate` names another prediction, and `split`
  names a paired gold row.
- Overrides touch only `subject` and carry a reason.

#### Scenario: Gold row counted twice
- **WHEN** a gold ID appears in both `pairs` and `missed`
- **THEN** `score` exits 1 with "gold rows accounted for more than once"

#### Scenario: Split cause without a paired gold row
- **WHEN** a false positive has cause `split` and names a gold row that is in `missed`
- **THEN** `score` exits 1

### Requirement: Score runs from the Benchmark page
The system SHALL expose `/api/v1/kb/provision-scores` to admin/root users: `GET /models`,
`GET /gold-runs?record_id`, `GET` (history), `POST` (start), `GET /:id`,
`GET /:id/artifacts/:kind`.

A started run SHALL execute in the background with the shared score runner and the prompt
`prompt-score-extract-provisions-v2.md`. The runner SHALL freeze the scorer, `weights.json`, the
gold helper and the rules into the run directory. Each run SHALL be stored in
`testbed.provision_score_runs` with its input, matches, score and report snapshots.
The runner SHALL save the exact compact projection, complete request text, raw compact matches
and expanded matches in its evidence directory. Successful API matches snapshots SHALL retain
the verbose schema. Invalid responses SHALL NOT be persisted as successful matches or silently
normalized. Failed compact validation SHALL leave the API matches snapshot null and retain raw
attempts in the evidence directory. The runner MAY request one complete repair with the compact
previous decisions and authoritative validation error; a second invalid response SHALL fail.

A record SHALL have at most one running score at a time. Failures SHALL be stored with a stable
error code: `NO_GOLD`, `NO_PREDICTIONS`, `INVALID_MATCHES`, `LLM_FAILED`, etc.

#### Scenario: Start and finish
- **WHEN** an admin posts `{record_id: 416, model, lang}` and record 416 has gold and predictions
- **THEN** the API answers 202 with a running run, and the run later becomes `done` with a score snapshot

#### Scenario: Second start while running
- **WHEN** a score for record 416 is running and another start for 416 arrives
- **THEN** the API returns the running run instead of creating a new one

### Requirement: Benchmark and Gold Provisions pages
On the development page, System Admin › Benchmark SHALL contain a **Provisions** folder with two
entries.

**Gold Provisions** (`sysadmin-llm-provisions-gold`) SHALL:
- list documents that have gold provision runs, filtered by gold rules version and model;
- show one run's gold provisions and dropped candidates, read-only, next to the source PDF with
  the selected provision's lines highlighted;
- highlight the selected list record in the same blue background and white text as Gold Metrics;
- show a floating info box beside the selected provision's PDF highlight, using the Gold Metrics
  card style and displaying the record ID, confidence (when present), provision ID, name,
  description/source text, provision type, location type and source span count.

**Benchmark** (`sysadmin-llm-provisions-benchmark`) SHALL:
- start a score for a chosen document, scoring model and gold run;
- list score history;
- open a report in `BenchmarkReportViewer` with:
  - score cards;
  - field accuracy;
  - pairs with failed checks shown as gold → extracted;
  - missed gold rows;
  - false positives with their cause.
- highlight the selected entry's source lines in the PDF.

All visible text SHALL go through Paraglide in English and Chinese, and both pages SHALL allow
text selection.

#### Scenario: Gold Provisions is read-only
- **WHEN** an admin opens Gold Provisions and selects a gold run of record 416
- **THEN** the gold provisions are listed and no extract, edit, save or delete control is shown

#### Scenario: Selected gold provision info box
- **WHEN** an admin selects a gold provision with source line coordinates in the PDF view
- **THEN** its list card uses the Gold Metrics selection colors and a floating info box shows
  that provision's summary next to its highlighted source lines
- **AND** selecting another provision updates both the highlighted list card and the info box

#### Scenario: Highlight from the report
- **WHEN** a user selects a missed gold provision in an opened report
- **THEN** the PDF pane highlights that provision's source lines

#### Scenario: Chinese UI
- **WHEN** the locale is `zh-cn`
- **THEN** every label on both pages is Chinese, and `bun run check` passes
