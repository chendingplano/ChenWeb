# product-benchmark-scoring Specification

## Purpose
Scoring `kb.products` rows of one document against a gold run: the `score-extract-products`
skill and the Benchmark page match rows through the compact matching contract, compute a 1–100 soft-F1 row
score with per-field credit and product-detection precision/recall, and store each run in
`testbed.product_score_runs`.
## Requirements
### Requirement: Reproducible product score
The `score-extract-products` skill SHALL score a record's `kb.products` rows against one gold run
(`testbed.product_benchmark_runs`) on a scale of 1 to 100.

The work is split in three steps:
1. `export` writes the complete `input.json` audit snapshot: gold, predictions, ledger,
   suggestions and warnings. `project` SHALL write the compact `matching-input.json` from a
   finalized snapshot without querying the database.
2. The matcher writes compact `matches.json`. The script expands it to `matches-expanded.json`:
   `pairs`, `missed`, `false_positives`, `overrides`.
3. `score` computes credit per pair from the weighted field checks that apply to it:
   `relation_type`, `requirement_text`, `product_type`, `name`, `lines`, `obligation_level`,
   `parameters`, `conditions`, `related_products`, `responsible_actor`. Default weights come from
   `weights.json`.

`score` SHALL write `score.json` and `report.md`. The score is `round(100 × soft F1)` over relation
rows, clamped to 1–100.

#### Scenario: Perfect extraction
- **WHEN** every gold row is paired with a prediction whose applicable checks all agree, and there are no false positives
- **THEN** the score is 100

#### Scenario: Wrong relation type on the right product
- **WHEN** a prediction is paired with a gold row and differs only in `relation_type`
- **THEN** the pair earns the weighted share of every other applicable check, not zero

#### Scenario: No gold
- **WHEN** `export` runs for a record without a gold run
- **THEN** it exits 3 with a `NO_GOLD` message and writes nothing

#### Scenario: No predictions
- **WHEN** the record has a gold run but no active `kb.products` rows
- **THEN** `export` exits 4 with `NO_PREDICTIONS`

### Requirement: Product detection measure
`score` SHALL also report unweighted product-detection precision, recall and F1:
- **Recall:** a gold product is detected when at least one of its rows is paired, or is the
  target of an `extra_relation` false positive.
- **Precision:** a prediction group (rows with the same normalized `canonical_name`) is correct
  when at least one of its rows is paired or is an `extra_relation` false positive.

The report SHALL list missed gold products separately from missed relations of detected products.

#### Scenario: Product found with the wrong relation only
- **WHEN** gold product P has one `scope` row and the only prediction for P is a `regulated_object` row classified `extra_relation` with P's row as target, and P's gold row is missed
- **THEN** P counts as detected for product recall, and the gold row counts as a missed relation

### Requirement: Compact model contract with complete audit evidence
The model view SHALL contain only `v=2`, `lc`, `sa` and the arrays `g`, `p`, `c`:
- gold rows: `[evidence_lines, product_name, canonical_name, relation_type, requirement_text,
  product]`, where `product` is the gold product ordinal;
- prediction rows: `[evidence_lines, product_name, canonical_name, relation_type,
  requirement_text]`;
- ledger rows: `[spans, text, rule_id, reason]`.

A row's local ID SHALL be its one-based array position.

The response SHALL contain exactly `v=2`, `p`, `m`, `f`, `o`:
- `f` causes SHALL be `x` (ledger index), `r` (a gold row of the same product), `d` (another
  prediction), `n` or `i`. `n` and `i` SHALL have null targets and non-empty explanations.
- `o` rows SHALL be `[paired_gold, field, true, reason]`, with `field` in `name`,
  `related_products` or `responsible_actor`.

The decoder SHALL refuse:
- unknown keys, versions, causes or override fields;
- out-of-range indices and wrong target types;
- duplicate overrides;
- self or cyclic duplicate references;
- invalid row lengths.

#### Scenario: Override of a non-overridable field
- **WHEN** `o` contains `[3,"relation_type",true,"close enough"]`
- **THEN** `score` exits 1 and no score is written

#### Scenario: Extra-relation without a valid gold target
- **WHEN** a false positive has cause `r` and a null or out-of-range gold index
- **THEN** `score` exits 1 and no score is written

### Requirement: Valid matches only
`score` SHALL refuse `matches.json` unless all of the following hold:
- Every gold row appears exactly once in `pairs` or `missed`.
- Every prediction appears exactly once in `pairs` or `false_positives`.
- Paired rows share a source line, unless line numbers are not comparable.
- Every false positive has a cause from `gold_excluded extra_relation duplicate not_in_gold
  invalid`, with the target its cause requires.

#### Scenario: Gold row counted twice
- **WHEN** a gold ID appears in both `pairs` and `missed`
- **THEN** `score` exits 1 with "gold rows accounted for more than once"

#### Scenario: Pair without a shared line
- **WHEN** lines are comparable and a pair's evidence lines do not intersect
- **THEN** `score` exits 1

### Requirement: Score runs from the Benchmark page
The system SHALL expose `/api/v1/kb/product-scores` to admin/root users:
- `GET /models`;
- `GET /gold-runs?record_id`;
- `GET` (history);
- `POST` (start);
- `GET /:id`;
- `GET /:id/artifacts/:kind`.

A started run SHALL execute in the background with the shared score runner and the prompt
`prompt-score-extract-products-v1.md`.

The runner SHALL:
- freeze the scorer, `weights.json`, the gold helper and the rules into the run directory;
- store each run in `testbed.product_score_runs`, including its `llm_run_id`;
- use the compact contract exactly as for provisions: save the projection, the request, and the
  raw and expanded matches;
- allow at most one repair attempt;
- keep the model profile's reasoning setting.

A record SHALL have at most one running score at a time. A failure SHALL be stored with a stable
error code (`NO_GOLD`, `NO_PREDICTIONS`, `INVALID_MATCHES`, `LLM_FAILED`, etc.).

#### Scenario: Start and finish
- **WHEN** an admin posts `{record_id: 416, model, lang}` and record 416 has gold and predictions
- **THEN** the API answers 202 with a running run, and the run later becomes `done` with a score snapshot

#### Scenario: Second start while running
- **WHEN** a score for record 416 is running and another start for 416 arrives
- **THEN** the API returns the running run instead of creating a new one

#### Scenario: Provision scoring unchanged
- **WHEN** the runner's provision-specific branches are replaced by the `compact` flag
- **THEN** the existing provision score tests pass unchanged

### Requirement: Benchmark and Gold Products pages
On the development page, System Admin › Benchmark SHALL contain a **Products** folder with two
entries.

**Gold Products** (`sysadmin-llm-products-gold`) SHALL:
- list documents that have gold product runs, filtered by gold rules version and model;
- show one run's gold rows, read-only, next to the source PDF, with the selected row's evidence
  lines highlighted;
- offer a Gold/Dropped switch for the run's dropped candidates.

**Benchmark** (`sysadmin-llm-products-benchmark`) SHALL:
- start a score for a chosen document, scoring model and gold run;
- list score history;
- open a report in `BenchmarkReportViewer` with:
  - score cards, including product detection;
  - field accuracy;
  - pairs with failed checks shown as gold → extracted;
  - missed gold rows;
  - false positives with their cause.
- highlight the selected entry's lines in the PDF.

All visible text SHALL go through Paraglide in English and Chinese. Both pages SHALL allow text
selection. The knowledge-base Products view SHALL behave as before.

#### Scenario: Gold Products is read-only
- **WHEN** an admin opens Gold Products and selects a gold run of record 416
- **THEN** the gold rows are listed and no extract, edit, save or delete control is shown

#### Scenario: Highlight from the report
- **WHEN** a user selects a missed gold row in an opened report
- **THEN** the PDF pane highlights that row's evidence lines

#### Scenario: Chinese UI
- **WHEN** the locale is `zh-cn`
- **THEN** every label on both pages is Chinese, and `bun run check` passes

