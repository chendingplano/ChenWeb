## ADDED Requirements

### Requirement: Opt-in processor

The system SHALL provide a doc processor named `extract_metrics_simple` that extracts the metrics
of a record's chunks into the same tables as `extract_metrics` (`kb.metrics`,
`kb.metrics_dropped`, `kb.doc_proc_logs`, the `.metrics` artifact file). It SHALL run only when a
run requests it; it SHALL NOT be added to any default processor list or pipeline. A run whose
selected processors include both `extract_metrics` and `extract_metrics_simple` SHALL be refused
with an error before any processor runs, because both write the same record's rows. Both MAY be
configured (selectable) at the same time.

#### Scenario: Requested alone

- **WHEN** a run requests `extract_metrics_simple` for record 416 with `force_clear`
- **THEN** the record's `kb.metrics` rows SHALL be replaced by the rows it extracts, with
  `prompt_name` = the simple prompt's name
- **AND** its logs SHALL carry `doc_proc_name = 'extract_metrics_simple'`

#### Scenario: Both metric processors requested

- **WHEN** a run requests `extract_metrics` and `extract_metrics_simple` together
- **THEN** the run SHALL be refused with an error naming both processors, and neither SHALL run

#### Scenario: Not requested

- **WHEN** a run uses the default processors
- **THEN** `extract_metrics_simple` SHALL NOT run

### Requirement: One extraction call per chunk

For each chunk the processor SHALL make one LLM call that returns the chunk's final metric rows
and the items it set aside. It SHALL NOT make candidate-pass, enrich-batch, coverage-recovery or
multi-value-review calls. When that call fails and the chunk has at least two normal lines, the
processor SHALL retry once as two calls, each extracting from one half of the chunk's normal lines
with the other half marked as overlap context. When the retry also fails, the chunk SHALL count as
failed exactly as a failed candidate-pass chunk does in `extract_metrics`.

#### Scenario: Normal chunk

- **WHEN** a record has 17 chunks and every call succeeds
- **THEN** the processor SHALL make 17 extraction calls

#### Scenario: Failed call is split

- **WHEN** the call for a chunk with 30 normal lines fails
- **THEN** the processor SHALL call once for lines 1–15 (lines 16–30 marked overlap) and once for
  lines 16–30 (lines 1–15 marked overlap), and use both results

### Requirement: Cacheable request layout

Every extraction call SHALL send the instructions (the prompt text and its output schema) as the
system message and the chunk, serialized by `canonicalChunkInputText`, as the user message. The
system message SHALL be byte-identical for every chunk and every record that uses the same prompt
version.

#### Scenario: Two chunks of different records

- **WHEN** the processor calls the LLM for chunk 3 of record 416 and chunk 9 of record 800
- **THEN** both requests SHALL have the same system message

### Requirement: Warm-up call

Within one run, the processor SHALL send its first extraction call alone. Other calls SHALL wait
until that call has returned and either its cache-hit input tokens are at least half of its input
tokens, or 60 seconds have passed since it started. A failed first call SHALL release the others
under the same rule.

#### Scenario: Cold cache

- **WHEN** the first call returns after 25 seconds with no cache hits
- **THEN** the other calls SHALL start 60 seconds after the first call started

#### Scenario: Warm cache

- **WHEN** the first call returns with 9 000 of its 10 000 input tokens served from the cache
- **THEN** the other calls SHALL start immediately

### Requirement: Compact output contract

The model's response SHALL be a JSON object with `lang` (document language), `m` (rows) and `x`
(set-aside items). Each row SHALL use these keys and SHALL omit a key whose value is empty:

| Key | Stored field | Notes |
|---|---|---|
| `n` | `metric_name` | required |
| `ne` | `metric_name_en` | omitted for English documents |
| `s` | `subject` | required |
| `se` | `subject_en` | omitted for English documents |
| `v` | `metric_value` | |
| `u` | `unit` | |
| `r` | `value_range_type` | closed enum of `extract_metrics` |
| `k` | `value_class` | closed enum of `extract_metrics` |
| `c` | `condition` | `<kind>：<value>` parts joined by `；` |
| `ce` | `condition_en` | omitted for English documents |
| `l` | `source_line_spans` | required |
| `t` | `source_table_rows` | table rows only |
| `vs` | `value_structure` | non-scalar values only |
| `lo`, `hi` | `value_min`, `value_max` | only for a range not written `a~b`, `a-b` or `a±b` |
| `g` | `reasoning_tags` | |
| `d` | `formula_or_definition` | |
| `cat` | `metric_categories` | one or two category keys |

The model SHALL write the resources, methods and stages a value uses as labelled `condition`
parts (`试验装置：…`, `试剂：…`, `药物：50％葡萄糖`, `给药途径：…`, `治疗阶段：…`); the shared frame step
moves them into `frame`. Each `x` item SHALL have `l` (lines), `n` (name) and `r` (reason, from the
drop reasons of `extract_metrics`, plus `no_value` for a quantity whose value is left open). `x`
SHALL list only set-aside statements that contain a number or name a measurable quantity; a
statement with neither is left out, since setting it aside cannot lose a metric. The
response SHALL NOT contain `frame`, `desc`, `context`, `keywords`, `objects`,
`metric_categories_en`, `measurement_frequency` or other English fields. A row without `n` or `l`
SHALL be set aside with reason `invalid_row`.

#### Scenario: Row expansion

- **WHEN** the model returns `{"n":"总砷","ne":"Total arsenic","s":"肥料","se":"Fertilizer","v":"15","u":"mg/kg","r":"upper_bound","k":"requirement","l":["12"],"cat":["heavy_metal_limit"]}`
- **THEN** the processor SHALL build a row with `metric_name` 总砷, `metric_value` 15, `unit`
  mg/kg, `value_range_type` upper_bound, `value_class` requirement, empty `condition`,
  `source_line_spans` ["12"] and `metric_categories` ["heavy_metal_limit"]

#### Scenario: Row without lines

- **WHEN** a row has `n` but no `l`
- **THEN** it SHALL be saved to `kb.metrics_dropped` with stage `enrich` and reason `invalid_row`

### Requirement: Fields built in code

The processor SHALL fill the stored fields the model does not return:

- `candidate_id`: `<chunk seq>_<n>`, numbering the chunk's rows and then its set-aside items
  from 1
- `context` and `threshold_or_target`: the text of the row's source lines in the chunk, joined
  by a space and cut to 300 characters (table rows then get the table context that
  `extract_metrics` already builds in code)
- `table_name_or_section`: the nearest heading or table caption line at or before the row's first
  line in the chunk, else empty
- `location_type`: `table` when the first line is a table line, else `text`
- `value_data_type`: `number` when `metric_value` is a number, interval or tolerance, else
  `string`
- `value_class_en`: equal to `value_class`
- `unit_en`: the unit itself when it has no CJK characters; otherwise its English form from a
  fixed table of common units, or the unit itself when it is not in the table
- `keywords` / `keywords_en`: the distinct non-empty values of name and subject (English: their
  English forms)
- `confidence` 0.8 and `is_explicit_metric` true
- `desc`, `desc_en`, `context_en`, `measurement_frequency`: empty
- `objects`: not set, so the existing synthesis from `subject` applies

#### Scenario: Table row

- **WHEN** a row cites line 201, a table line whose caption is line 199 "表 A.1 常用液化气体特性"
- **THEN** `location_type` SHALL be `table` and `table_name_or_section` SHALL be that caption

#### Scenario: Chinese unit

- **WHEN** a row's unit is `次`
- **THEN** `unit_en` SHALL be `times`

### Requirement: Rows on overlap lines

A row or set-aside item whose source lines are all overlap lines of its chunk SHALL be discarded
before the shared steps, as `extract_metrics` discards overlap-only candidates; the count SHALL be
logged.

#### Scenario: Overlap-only row

- **WHEN** a chunk's response has a row citing only line 88, which is an overlap line of that chunk
- **THEN** the row SHALL NOT be stored or set aside by this chunk

### Requirement: Set-aside items

Each `x` item SHALL be saved to `kb.metrics_dropped` with stage `enrich` and the item's reason. Its
`row_data` SHALL hold `candidate_id`, `metric_name`, `source_line_spans` and, as
`threshold_or_target`, the text of its source lines.

#### Scenario: Definition set aside

- **WHEN** the model returns `{"l":["30"],"n":"量热计压力","r":"term_definition"}`
- **THEN** `kb.metrics_dropped` SHALL get a row with stage `enrich`, reason `term_definition`,
  `metric_name` 量热计压力 and the text of line 30

### Requirement: Shared post-extraction steps

After the extraction calls, the processor SHALL apply the same steps, in the same order, as
`extract_metrics` applies after its enrich calls: drop-reason tags, metric frames, clause and
served-quantity condition inheritance, condition repeats, final dedup, value-range
canonicalization, value notation, resource-spec rows, pure requirements, rows without a value and
the open-value decision. It SHALL then save, merge (when `force_clear` is false), link provisions,
persist objects, write the `.metrics` file, harvest metric definitions, resolve incoming
references and index in Phase C exactly as `extract_metrics` does. Rows that keep a combined
multi-value value SHALL stay `unresolved` with tag `value_review_unresolved`, without a review call.

#### Scenario: Value without a value

- **WHEN** the model returns a row with no `v`
- **THEN** the row SHALL be saved to `kb.metrics_dropped` with stage `no_value`

#### Scenario: Merge mode

- **WHEN** the run has `force_clear` false and the record already has metrics
- **THEN** the new rows SHALL be merged with the existing rows by the `extract_metrics` merge
  procedure, including merge-resolve calls for conflicting groups

### Requirement: Prompt and model configuration

The processor SHALL load its prompt from env `EXTRACT_METRICS_SIMPLE_PROMPT`, default
`prompt-extract-metric-simple-v1.md`, and its model from `EXTRACT_METRICS_SIMPLE_MODEL_NAME`,
falling back to `ENRICH_METRICS_MODEL_NAME` and then `EXTRACT_METRICS_MODEL_NAME` (the model of
`extract_metrics`' enrich pass, so the comparison uses the same model). Thinking SHALL be disabled. A missing prompt or
model SHALL fail or skip the processor the same way as `extract_metrics`.

#### Scenario: Default prompt

- **WHEN** `EXTRACT_METRICS_SIMPLE_PROMPT` is not set
- **THEN** the processor SHALL use `prompts/prompt-extract-metric-simple-v1.md` and store that
  name in `kb.metrics.prompt_name`
