## Context

Four extractors already have a gold benchmark, all built the same way:

1. **Gold skill** in `.agents/skills/<x>-benchmark`.
   - It reads the whole line file of one `kb.inputs` record, inventories candidates and labels
     them under a frozen rules file (`rules/<version>.md`, SHA-256 in `rules/versions.json`).
   - It audits the result, then validates and saves one immutable run with
     `scripts/benchmark_io.py`.
   - A run is a row in `testbed.<x>_benchmark_runs`, plus its accepted rows and its
     excluded/duplicate candidates.
2. **Scorer skill** in `.agents/skills/score-<x>`.
   - `score_io.py export` writes the `input.json` audit snapshot.
   - For provisions, `project` also writes a compact `matching-input.json`, and an agent or LLM
     returns compact `matches.json`.
   - `score_io.py score` checks the matches and computes `round(100 × soft F1)`.
3. **ChenWeb pages** under System Admin › Benchmark (development page only):
   - a gold viewer: the production artifact view with `source="testbed"`;
   - a scoring page that runs the scorer in the background (`executeScore` in
     `metric_score_runner.go`, parameterised by `scoreKind`) and stores each run in a
     `<x>_score_runs` table.

`extract_products` writes `kb.products`. One row is one (product, relation_type) pair:
- `product_rel_id` (`<record>_prd_<n>`), `product_name`, `canonical_name`, `product_type`;
- `relation_type`, `relation_summary`, `evidence_quote`;
- `evidence_lines`: string spans `"N"`, `"N-M"` or a table row `"N#rK"` / `"N#hK"`;
- `obligation_level`, `requirement_text`, `conditions`, `exceptions`;
- `parameters` (`{name,value,unit}`), `related_products` (`{product_name,relationship}`),
  `responsible_actor`;
- `confidence`, the `_en` translations, `category_paths`, `product_name_id`.

The vocabularies come from `prompt-enrich-product-mention-v4.md` and
`prompt-extract-products-merged-v2.md`:
- `product_type`: 10 values;
- `relation_type`: 21 values, of which only four are inter-product (`component_of`,
  `contains_product`, `compatible_with`, `replacement_or_alternative`);
- `obligation_level`: 7 values.

Eleven records have product rows. Record 416 has 138 rows over 130 product names (2026-11-07).
Most of them are `scope` (79).

## Goals / Non-Goals

**Goals:**
- A gold skill and rules 1.0.0 that decide, for every line of a document:
  - which noun phrases are products;
  - which relation types each product has;
  - the graded enrichment fields.

  The rules must be domain-neutral (any document kind, language or domain).
- A reproducible 1–100 score for `kb.products` against one gold run, with a per-field breakdown
  and a separate product-detection measure.
- Gold Products and Benchmark pages that work like their Provisions counterparts.
- A saved first gold run for record 416, and a first score of its existing `kb.products` rows.

**Non-Goals:**
- Changing `extract_products`, its prompts or `kb.products`.
- Grading `relation_summary`, `confidence`, translations (`_en`), `category_paths` or
  `product_name_id` resolution. They are free text or derived data, with no gold-checkable meaning.
- Scoring Pass-1-only runs (rows without `relation_type`). The scorer warns about them but grades
  them like any other row.
- A "Review Products" page.

## Decisions

### D1. Gold unit: one (product, relation_type) row, grouped by product (user ruling)

A gold row is one product-relation pair, exactly the unit `kb.products` stores. Every row also
carries `product_key` (`<record>_gp_<n>`). All rows of one product share the key, and with it the
same `product_name`, `canonical_name` and `product_type`.

- One product appears once per document. Its relation rows cite every line that supports that
  relation, wherever it is in the document. This mirrors Deterministic Step B, which merges a
  product's rows of one relation type.
- A relation type is included only when the evidence directly and clearly supports it (Pass 2's
  own Relation Rule 3). Marginal or inferred relations are not gold rows, so an extractor gains
  nothing by padding.
- When a product is plainly within the document's subject matter but has no specific supported
  relation, it gets exactly one `scope` row (extractor Relation Rule 4). A gold product never has
  zero rows.

*Alternatives:*
- One row per product with a relation set. The user rejected it: it hides per-relation
  enrichment (requirement text, parameters).
- Products only. Rejected: relation and enrichment quality are where the merged-vs-two-pass A/B
  found the real differences.

### D2. Producthood (user ruling plus the spec's gate)

The rules restate the extractor's producthood gate as gold decisions.

**Accept (A rules):**
- physical products, product classes, equipment, devices, components, accessories, software
  products, consumables;
- materials, substances and regulated waste streams;
- packaging, when it is specifically regulated or specified.
- **Waste categories and waste streams are products.** Examples: 可回收物, 易腐垃圾, 有害垃圾,
  废玻璃, 餐厨垃圾. Their type is `product_class`, or `material` for a material stream. This was
  the user's ruling of 2026-11-08, and it follows the spec's v2 rebalance, which counts losing
  them as a recall failure.

**Exclude (X rules):**
- **Places, facilities and works.** The test: is it supplied as a movable article, or built at a
  site? Facilities, plants, stations, points, centres, works and on-site systems are excluded,
  with the spec's naming cues (站, 厂, 场, 基地, 中心, 园, 区, 房, 池, 工程, 设施, locational 点).
- **Citation-only mentions.** A noun phrase whose only support is a normative-reference entry
  or a citation title is excluded. A product that also appears in body text is accepted, and
  its rows cite the body lines only.
- Organizations, people, locations, abstract concepts and technologies, activities and services,
  legal acts and document sections.
- The document itself, and other documents.

**Naming:**
- `product_name` is the surface exactly as written. `validate` checks that it occurs in a cited
  line.
- A coordination with an elided shared head (收集、运输设备) gives one product per conjunct. The
  expanded name (收集设备) is allowed only with `ext_info.name_expanded = true` and its own rule
  ID. `validate` then checks each part instead of the whole name.
- `canonical_name` is the normalized base name (singular, no quantity, no document-local
  qualifier). Different surfaces of one product share one `product_key`.

Every decision is a ledger candidate with a bold rule ID (`A*`, `X*`, `D1`), as in the other
skills.

### D3. Relation and enrichment labels

- **Vocabularies.** They are the stored long names: `product_type` (no `unknown`),
  `relation_type`, `obligation_level`, and `related_products[].relationship`. `validate` checks
  them.
- **Relation order.** The rules give a decision order for the overlapping relation types:
  - a specific requirement relation (`performance_requirement` … `storage_requirement`) is used
    when the requirement's subject matter fits one;
  - `requirement_target` is used for a requirement on the product that fits none;
  - `regulated_object` is used for legal or compliance control;
  - `scope` is used for subject-matter inclusion with no requirement.

  This keeps different annotators consistent. It does not make relation choice a pairing gate
  (D5).
- **`related_products`** is non-empty exactly when the row's relation is one of the four
  inter-product types. `validate` checks both directions.
- **`requirement_text`** is exact source text: the clause that imposes the requirement.
  Non-contiguous parts are joined with ` … `, and `validate` checks each part against the cited
  lines. It is empty for a purely descriptive row (`obligation_level = descriptive`, no
  requirement). The user chose to grade it by similarity, which deliberately rewards copying the
  source.
- **`conditions` and `exceptions`** are short source-faithful phrases.
- **`parameters`** are `{name, value, unit}`. `value` keeps the source's comparator and number
  (`≥30`, `5.5~8.5`), and `unit` is null when there is none.
- **`responsible_actor`** is the party the source names as obliged, or null.

### D4. Gold storage mirrors production columns

`testbed.products` keeps the `kb.products` names for every checked field:
- `product_rel_id`, `product_name`, `product_name_en`, `canonical_name`, `canonical_name_en`;
- `product_type`, `relation_type`, `relation_summary`, `evidence_quote`, `evidence_lines`;
- `obligation_level`, `requirement_text`, `conditions`, `exceptions`, `parameters`,
  `related_products`, `responsible_actor`, `confidence`.

It adds gold-only `product_key`, `section`, `rule_id` and `ext_info`, the run key, and the
source path and hash.

- Gold IDs are `<record>_gprd_<n>`.
- Spans use the production form (`"N"`, `"N-M"`, `"N#rK"`, `"N#hK"`). Gold and prediction spans
  can then be compared with no conversion.
- `evidence_quote` is an exact substring of a cited line, or of a cited table row's text.
- `product_name_en` and `canonical_name_en` are required for non-English documents; the other
  `_en` fields are optional.

The run and dropped tables copy `testbed.provision_benchmark_runs` and
`testbed.provisions_dropped`. A candidate is one product mention (noun phrase and its spans). An
accepted candidate links to its `product_key`.

*Alternative:* store one gold row per product with a nested relations array. Rejected because of
D1, and because a flat table lets the viewer reuse the production Products view unchanged.

### D5. Scoring

**Pairing.** A pair means both rows state the same relation of the same product:
- they name the same thing (surface differences allowed);
- with comparable lines, they share a source line;
- they describe the same fact.

A different relation type does not prevent a pair. It is graded by the `relation_type` check, so
choosing `scope` instead of `regulated_object` costs that check, not the whole row. Pairs are
one-to-one. Among several rows of one product, the matcher pairs by shared evidence and the same
or nearest relation.

**Unpaired rows.**
- A gold row that is not paired is `missed`. A merge (one prediction covering several gold rows,
  for example a coordinated name) pairs the closest gold row and lists the others as merged
  misses, as in the provisions contract.
- False-positive causes:

| Cause | Code | Target |
|---|---|---|
| `gold_excluded` | `x` | ledger candidate (facility, citation-only, organization …) |
| `extra_relation` (new) | `r` | a gold row of the same product: product right, relation not in gold |
| `duplicate` | `d` | another prediction |
| `not_in_gold` | `n` | null; explanation says whether gold missed a real product or relation |
| `invalid` | `i` | null; misreads the source |

**Field checks on a pair.** Default weights are in `weights.json`. They are relative and
normalised to 1 over the checks that apply to the pair.

| Check | Weight | How | Applies |
|---|---:|---|---|
| `relation_type` | 20 | equal | always |
| `requirement_text` | 15 | `difflib` ratio of NFKC, whitespace-free texts; partial credit | gold text non-empty |
| `product_type` | 10 | equal | always |
| `name` | 10 | normalized equality of either side's `product_name`/`canonical_name`; overridable | always |
| `lines` | 10 | Jaccard of line sets (table rows reduced to their line); partial credit | lines comparable |
| `obligation_level` | 10 | equal | always |
| `parameters` | 10 | F1 over items matched on normalized value + unit (names are paraphrased, so ignored) | either side non-empty |
| `conditions` | 5 | F1 over `conditions` and `exceptions` items; an item matches only one from the same list, at `difflib` ≥ 0.6 | either side non-empty |
| `related_products` | 5 | F1 over normalized names; overridable | gold relation inter-product, or either side non-empty |
| `responsible_actor` | 5 | normalized equality; overridable | either side non-empty |

- **Score:** `round(100 × soft F1)` over relation rows, clamped to 1–100.
- **Product detection.** It is computed from the same matches, unweighted:
  - recall = gold products with at least one paired row, or the target of an `r` false positive;
  - precision = prediction groups (by normalized `canonical_name`) of which at least one row is
    paired or `r`.

  This measures the Pass 1 producthood gate on its own, which is the number the spec's prompt A/Bs
  track.
- **Report groups:** precision, recall and F1 per `relation_type` (both sides) and per
  `product_type`. Missed gold products are listed separately from missed relations.
- **Overrides** are allowed only for `name`, `related_products` and `responsible_actor`, with a
  reason.
- **Exit codes:** 3 `NO_GOLD`, 4 `NO_PREDICTIONS`.

**Compact contract.** The contract is the provisions v2 shape, with product rows:
- model view `{v:2, lc, sa, g, p, c}`;
- gold rows `[evidence_lines, product_name, canonical_name, relation_type, requirement_text,
  product]`, where `product` is the one-based gold product ordinal, so the matcher sees the
  grouping;
- prediction rows `[evidence_lines, product_name, canonical_name, relation_type,
  requirement_text]`;
- ledger rows `[spans, text, rule_id, reason]`.

The response is `{v:2, p, m, f, o}`:
- `f` causes are `x r d n i`;
- `o` rows are `[paired_gold, field, true, reason]`.

The decoder restores full IDs and routine explanations locally, and refuses anything outside the
contract.

*Alternative:* the metrics scorer v3 model, where missing and false-positive rows carry their own
penalty weights. Rejected for 1.0, as for provisions: it is harder to explain, and
`weights.json` keeps the weights adjustable.

### D6. ChenWeb integration reuses the benchmark machinery

**Backend.**
- `goldRunTables["products"] = "testbed.product_benchmark_runs"`. This drives the gold kb-inputs
  list and `GET /kb/products/gold-options`.
- `ListProducts` branches to `listGoldProducts` for `source=testbed`, as `ListProvisions` does.
  It returns `results` (gold rows in the `productRecord` shape, plus `product_key`) and `dropped`.
- `product_score_handler.go` copies `provision_score_handler.go` with
  `productScoreKind{"PRODUCT_BENCHMARK_SCORER_PATH", "score-extract-products",
  "extract-products-benchmark", "product-score-runs", …, "product"}`.
- **Runner.** `executeScore` tests `matchBasis == "provision"` in eight places to mean "compact
  contract":
  - source availability;
  - `project`;
  - saving the request;
  - keeping the profile's reasoning;
  - the repair loop;
  - expanded matches.

  This becomes a `compact bool` field on `scoreKind`, set for provisions and products. Product
  matching keeps the profile's reasoning for the same reason as provisions: without it,
  deepseek-flash loses the exhaustive index accounting. The runner's table map gains
  `"product-score-runs": "testbed.product_score_runs"`.
- The scorer's validation messages are added to the runner's allowlist of prefixes.

**Frontend.**
- **Gold Products** is `ProductsView` with `source="testbed"`. `KbExtractionView` gains optional,
  default-off props:
  - record-browser `listParams` and `listPausedMessage`;
  - a filter snippet;
  - the selected record passed to `loadItems`.

  `ProductsView` uses them for `GoldRunFilter('products')` and a Gold/Dropped switch. Dropped
  candidates are shown as items whose type is their rule ID. Existing callers of
  `KbExtractionView` are unchanged.
- **Benchmark** is a new `product-score-view.svelte`, a pared-down copy of
  `provision-score-view.svelte` on `BenchmarkReportViewer`, plus a product-detection score card.
  Selecting an entry highlights its lines in the PDF.

### D7. Predictions for record 416 already exist

Record 416 has 138 `kb.products` rows from 2026-11-07, so it can be scored as soon as the gold run
is saved. No doc-processor run is needed. If those rows come from more than one extraction, the
scorer warns, as the other scorers do.

## Risks / Trade-offs

- **[Relation types overlap (`scope` / `regulated_object` / `requirement_target`)]** → The
  relation type is a weighted check, not a pairing gate, and the rules fix a decision order. The
  per-relation report shows where the extractor and the gold disagree.
- **[Large gold sets.]** Record 416 will have about 130–160 rows, and one product can have
  several relation rows.
  → The inventory is line by line, with completeness and correctness audits in the rules file.
  A run is immutable; a re-annotation is a new run.
- **[`requirement_text` similarity penalises Pass 2's paraphrases]** → This is the user's ruling.
  The weight (15) is below `relation_type`, and a gold row with no requirement text leaves the
  check out.
- **[Name normalization differs from Go's `semid.Normalizer`]** → The scorer normalizes with
  NFKC, case fold, and whitespace and punctuation removal. The matcher can override `name` with a
  reason for real equivalents.
- **[Gold and scoring by the same model]** → The runner already warns when they match.
- **[A single gold annotator (the invoking model)]** → Required reviews and audits before saving.
  User rulings are recorded in the rules file.
- **[Found while designing: `executeScore`'s table map still names `kb.provision_score_runs`]** →
  The table moved to `testbed` on 2026-11-07, and `testbed.provision_score_runs` has no
  `llm_run_id` column (migration `20261108000001` altered the `kb` name). So provision score runs
  never persist their `llm_run_id`; it only logs a warning. This change does not fix it; it is
  reported separately. The new product table is created in `testbed` with `llm_run_id` from the
  start.

## Migration Plan

1. Two goose project migrations:
   - testbed gold tables;
   - `testbed.product_score_runs` (with `llm_run_id`) plus the page_config rows.

   The live `mise dev` server applies them, or `doc-service --migrate-only` does. Down migrations
   drop what they create.
2. The skills need no deployment beyond the `.agents` repo. The pages appear on the development
   page only.
3. Rollback: run the down migrations and revert the nav entries.

## Open Questions

- Whether a later rules version should grade `exceptions` separately from `conditions`, or grade
  `relation_summary`.
- Whether record 753 (a test-method standard with a different product profile) should be the
  second gold document.
