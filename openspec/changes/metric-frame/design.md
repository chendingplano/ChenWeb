## Context

`extract_metrics` writes one row per (subject, property, condition, value) into `kb.metrics`.
Since change `extract-metrics-conditions` (2026-10-16), `condition` / `condition_en` hold every
qualifier as `<label>：<value>` joined by `；`. That field now carries four kinds of information.
Some of them are not conditions:

| Facet | Labels found in `kb.metrics.condition` (2026-10-18, 2,530 rows, 596 with a condition) |
|---|---|
| condition | 适用人群, 适用情形, 适用条件, 触发, 病情分层, 临床表现, 心率, 收缩压, 呼吸频率, 体温, 意识, 性别, 年龄, 症状, 伴随表现, 心律失常类型, 模型, 器材类型 (a variant of the specified object), 检验项目, 电阻位置, 加载部位, 加载状态, 受力形式, 测量部位, 流量点, 静载荷力, 电源类型, 形状, 检验水平, 抽样方案, 称量, 剂量, 用药情形 |
| uses | 试验装置, 试验介质, 溶媒, 药物 (candidates v29 hint), 药物浓度 (spec) |
| method | 试验方法, 给药途径, 给药方式, 给氧方式, 操作, 评估依据 |
| procedure | 治疗阶段, 用药时机 |

Without a slot for "the thing used", the model turns a resource's specification into a row.
Record 364 has four such rows: mtc_30 葡萄糖浓度 50 %, mtc_33 10％葡萄糖静点浓度, mtc_51 葡萄糖注射液浓度 5 %
and mtc_79 肾上腺素浓度 1:1000. Gold rules 8.0.0 §2.6 already exclude these rows, but only for
treatment protocols, and the extractor keeps making them.

Related mechanisms already in place:
- Rule 6.5.0, "numbers that qualify another value": an apparatus grade such as 5 等量块 goes
  into the qualifier, not a row.
- `inheritClauseConditions` and `inheritServedQuantityConditions`.
- `dropConditionPropertyRepeats`.
- `kb.metrics_dropped` with `drop_stage`.
- `kb.metrics.provision_id`, which is never filled.
- The ontology terms `mea:procedure`, `mea:condition` and `mea:measured_by`.

## Goals / Non-Goals

**Goals:**
- Model a metric's frame explicitly as condition, uses, method and procedure, so that a resource
  specification is never a metric row.
- Keep `condition` working for current readers: scorer, UI, merge identity, gold tooling.
- Migrate existing rows deterministically. No LLM re-run is needed to adopt the new shape.
- Keep the design domain-neutral, so that one model covers drugs, test apparatus, reagents and
  materials.

**Non-Goals:**
- Projecting frames into `kb.semantic_assertions` or class contracts. Only the vocabulary terms
  are added now; the projection comes in a follow-up change.
- Resolving resources to ontology objects, for example linking 葡萄糖注射液 to an object node.
- Extracting procedures as their own entity. The procedure facet only points at a step and a
  stage.
- Backfilling `provision_id` for existing rows.

## Decisions

### D1. One `frame jsonb` column; `condition` stays the condition facet

Shape (both languages in one object, matching the `condition` / `condition_en` pairing):

```json
{
  "uses": [
    {"role": "agent", "name": "葡萄糖注射液", "name_en": "glucose injection",
     "spec": [{"property": "浓度", "property_en": "concentration", "value": "50", "unit": "%"}]}
  ],
  "method":    [{"label": "给药途径", "value": "静脉推注", "label_en": "Route", "value_en": "IV push"}],
  "procedure": {"step": "A.2.4", "stage": "", "stage_en": "", "after": "", "after_en": ""}
}
```

- `role` is one of `agent | material | apparatus | reagent | medium | sample | device`. This is
  a closed list, mapped to ontology role terms (D7).
- `spec[].value` is a string, so ratios (`1:1000`) and grades (`5 等`) fit.
- `condition` / `condition_en` keep the existing `<label>：<value>` text, narrowed to the
  condition facet.

**Alternatives considered:**
- Putting all four facets in `frame` and dropping `condition`. Rejected: scorer v2/v3, the gold
  tooling, the wiki infobox and merge identity all read `condition`, and the condition change is
  only two days old.
- Four separate columns. Rejected: `uses` is a list of nested objects anyway, and one column
  means one migration and one API field.
- A child table `kb.metric_resources`. This would be better for cross-document queries ("every
  metric using 葡萄糖注射液"), but it is premature before the ontology projection exists. A GIN
  index on `frame` covers the near-term queries.

### D2. The resource-spec rule

A number that only specifies a resource used by another value of the same clause becomes
`uses[].spec` of that value's row. It is never a row of its own. This generalises rule 6.5.0
(apparatus grade) and §2.6 (drug formulation) into one rule that applies in every document kind.

The test, as written in the prompt and the gold rules: *does the clause set this number as
something to provide, apply or meet on its subject, or does it only say which product, reagent,
apparatus or material is used?*

There is one exception. When the resource *is* the object the document specifies (a product
standard for glucose injection, a reagent specification, a calibration standard for gauge
blocks), the spec is a metric (A1), as it is today.

**Reagent concentration in a test method** ("用 0.1 mol/L 盐酸溶液提取") is a resource spec. Amounts,
volumes, ratios, times and temperatures of the method stay A6 test settings. This follows VIM,
where a reagent's identity and grade are part of the measurement procedure, not a measured
quantity.

### D3. Amount-less steps leave the metrics

"之后以10％葡萄糖静点" has a resource spec but no amount, rate or duration. After D2 it has no value,
so it fails the metric gate. The candidates prompt routes it to the pure-requirement path (X16,
ADR 2026100603 DR3), and the provision keeps the step. The enrich guard drops it with the reason
`resource_spec`.

### D4. `metric_name` drops the strength

The new name is `葡萄糖静推剂量`, not `50％葡萄糖静推剂量`. The agent's name stays in the name when it is
the only way to tell rows apart (肾上腺素肌肉注射剂量), but its strength or grade moves to `uses`.

Identity must not lose the distinction. The in-run dedupe key, `metricsIdentical`,
`metricStaticMatch` and merge-resolve compare a normalised `uses` signature: the sorted list of
`(role, name, spec)`. So the 50 % adult dose and the 25 % child dose stay two rows.

**Alternative considered:** keeping the strength in the name. Easier to read, but cross-document
matching would then split "葡萄糖静推剂量" by every formulation.

### D5. Deterministic label map and backfill

`metric_frame.go` holds one table, `conditionLabelFacet`, that maps each legacy label (Chinese
and English) to a facet and, for `uses`, a role. `splitLegacyCondition(condition, condition_en)`
returns the narrowed condition and a frame. It is used twice:
1. at save time, as a safety net for model output that still puts a method or apparatus in
   `condition`;
2. by a one-shot backfill command over `kb.metrics` and `testbed.metrics`.

An unknown label stays in `condition`. It is never guessed into another facet. `药物浓度：X` becomes
a spec on the row's single agent; when the row has no agent, it stays in `condition` and is
logged.

The existing inheritance steps also move to the facets they serve:
- `inheritClauseConditions` copies `试验装置/试验方法/试验介质` qualifiers into
  `uses` / `method`.
- `inheritServedQuantityConditions` copies the whole frame of the served quantity.

The behaviour stays the same; only the target slot changes.

### D6. Code guard `dropResourceSpecRows`

This runs after enrich and before final dedupe, beside `dropConditionPropertyRepeats`. It drops a
row when all of the following hold:
- its unit is `%`, a ratio, `mol/L` or `g/L` (or a grade word);
- another row of the same `source_line_spans` has a `uses` entry with a matching spec;
- the matching spec is an identifying one (浓度, 稀释度, 等级, 型号, …), never an accuracy or error the
  model wrote as a spec (the 2026-10-18 live run did that for 标准测力计 准确度 0.1 %).

A product standard's own property has no host row, so it is kept. The dropped row is written to
`kb.metrics_dropped` with `drop_stage = enrich` and `drop_reason = resource_spec`; its data names
the host by `candidate_id` and `metric_name` (`resource_spec_host`), because metric ids are
assigned later. The guard is
deliberately narrow, because the prompt does the main work.

### D7. Ontology vocabulary only

The change adds governed terms to the `measurement` module:
- `mea:resource` (class);
- `mea:uses_resource` (property);
- `mea:resource_role`, with the seven roles as terms;
- `mea:has_spec`.

Method maps to the existing `mea:procedure` / `mea:measured_by`, and the condition to
`mea:condition`. These are seeded through the normal term seeding path. Nothing reads them in
this change.

### D8. Benchmark: rules 9.0.0 and scoring v4

- **Rules 9.0.0** add D2–D4 and the frame shape to gold rows (`frame` in `testbed.metrics`).
  They retire the §2.6 formulation sentence in favour of the general rule.
- **Scoring v4:**

  | Field | Weight |
  |---|---|
  | value | .25 |
  | range | .15 |
  | kind | .15 |
  | unit | .15 |
  | lines | .10 |
  | condition | .10 |
  | uses | .05 |
  | method | .05 |

  `uses` is a soft match on (role, name, spec value); `method` compares the normalised value.
- Scores under v4 cannot be compared with v3, as was already the case for v1→v2.

## Risks / Trade-offs

- **The model mislabels facets** (a method in `condition`, a population in `method`) →
  `splitLegacyCondition` at save time fixes the known labels. The scorer's facet split shows
  any drift.
- **A real metric is dropped as a resource spec** (a product standard's concentration
  requirement) → the prompt has an explicit exception, and the guard requires a host row in the
  same line span that uses the resource. A quality case covers a product-standard clause.
- **Name change splits merge history.** Existing rows named `50％葡萄糖静推剂量` will not merge with
  new `葡萄糖静推剂量` rows → the backfill does not rename rows (it cannot tell the agent's name from
  the property's); records are re-extracted with `force_clear` instead.
- **Only 364 and 371 are validated** → task 8 requires one non-clinical, non-test-equipment
  document before the prompts become defaults.
- **More output tokens per row** → `frame` is omitted when it is empty, and the enrich schema
  keeps every facet optional.

## Migration Plan

1. Goose migration: add `frame jsonb` to `kb.metrics` and `testbed.metrics` (nullable, with a GIN
   index on `kb.metrics.frame`).
2. Deploy the code with the label map at save time. Old prompts still work, because their
   `condition` output is split at save time.
3. Run the backfill command; it reports counts per label.
4. Pin the new prompts in code and in `mise.local.toml`, then restart the doc-processor.
5. Re-annotate the gold, then rescore with v4.

**Rollback:** `frame` is additive. Revert the prompt pins. The backfill writes the before-image of
every changed row to a `--backup` file, and `--revert <file>` restores it.

## Open Questions

- Should a `method` value that cites a standard ("按 GB/T 5009.11") also set
  `ref_mention_id`? Probably yes, but that belongs with the extract_references work.
- Should a person who acts in a step (a doctor, a trained operator) be a `uses` role? It is left
  out for now and kept in `condition`, as 7.x does.
