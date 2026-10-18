## Why

A metric is more than a number with a unit. To read "40 ml" you need to know what is dosed, for whom,
how it is given and at which step. Today `extract_metrics` has one place for all of that: the
free-text `condition`. So the model turns the specification of the thing used into a metric of
its own. In record 364 A.2.4, "静推50％葡萄糖40ml" produced two rows: the dose (`364_mtc_29`,
40 ml) and a "葡萄糖浓度 50 %" row (`364_mtc_30`). The second row only says which product the dose
uses. 364 has three more such rows (mtc_33, mtc_51, mtc_79).

At the same time, `kb.metrics.condition` has grown about 45 ad-hoc labels. Many are not
conditions:
- 试验装置 (167 rows), 器材类型, 溶媒 and 试验介质 are things used.
- 试验方法, 给药途径 and 给氧方式 are methods.
- 治疗阶段, 用药时机 and 操作 are steps of a procedure.

The ontology already models the measurement frame (`mea:condition`, `mea:procedure`,
`mea:measured_by`), but extraction never fills it.

## What Changes

- A metric row gets a structured **frame** with four facets:
  - **condition**: when, for whom, in what state. This is the existing `condition` /
    `condition_en`, now narrowed.
  - **uses**: resources, each with a role (agent, material, apparatus, reagent, medium, sample,
    device), a name and an optional spec (property, value, unit).
  - **method**: how the value is applied or measured, for example route, mode, technique or a
    cited test method.
  - **procedure**: where the value sits in a procedure: step reference, stage, preceding step and
    the existing `provision_id` link.
- **Resource-spec rule:** a number that only specifies a resource used by another value is part
  of that resource's spec, never a metric row. Examples are a drug strength (50％), a dilution
  (1:1000), a reagent concentration (0.1 mol/L) and an apparatus or material grade (5 等量块). It
  stays a metric only when the resource is itself the object the document specifies, as in a
  product standard for glucose injection.
- Such numbers are dropped and saved in `kb.metrics_dropped` with the new reason
  `resource_spec`, pointing to the host row.
- A **step without an amount** ("之后以10％葡萄糖静点") is no metric at all. It is a pure
  requirement (gold rules X15/X16) whose resource is in `uses`.
- `metric_name` no longer carries a resource's strength or grade: `葡萄糖静推剂量`, with the
  agent 葡萄糖注射液 {浓度: 50 %} in `uses`. The spec stays part of metric identity, so the 50 %
  and 25 % doses remain distinct rows.
- A deterministic **label map** routes the legacy condition labels to facets. A backfill moves
  existing qualifiers out of `condition` into `frame` without an LLM.
- **Prompts:** new candidates and enrich versions emit and use the frame. A code guard
  `dropResourceSpecRows` catches the rows that slip through.
- **Benchmark:**
  - Gold rules **9.0.0** add the resource-spec rule and the frame shape.
  - Scorer version **4** splits the condition weight across the facets. **BREAKING** for score
    comparison: v4 scores cannot be compared with v3 scores.
- **Ontology:** add the vocabulary terms `mea:resource`, `mea:uses_resource` and the resource
  role terms to the `measurement` module. Projecting frames into semantic assertions is
  deferred.
- **UI and API:** metric read APIs return `frame`. The metric detail panel shows Conditions,
  Uses, Method and Procedure, in English and Chinese.

## Capabilities

### New Capabilities
- `metric-frame`: storage, shape, identity and API exposure of the structured metric frame, and
  the legacy-label map and backfill.
- `metric-resource-spec`: the rule that a resource's specification is part of the frame and not a
  metric. This covers extractor behaviour, the `resource_spec` drop and the amount-less step case.
- `metric-frame-benchmark`: gold rules 9.0.0 frame annotation and scorer v4 facet scoring.

### Modified Capabilities
- `metric-provision-link`: `provision_id` becomes the procedure facet's step link. It may be set
  by `extract_metrics` when the step's provision is known (it is still optional).

The `metric-condition` capability is still in the unarchived change `extract-metrics-conditions`.
This change narrows it, so a condition no longer holds apparatus, method or stage, and
`metric-frame` restates the narrowed rule. Archive `extract-metrics-conditions` first.

## Impact

- **Database (goose):** `kb.metrics.frame jsonb`, `testbed.metrics.frame jsonb`, and the drop
  reason `resource_spec` in `kb.metrics_dropped`.
- **Code (`server/api/doc-processing/`):** `extract-metrics.go` (schema, normalisation, save and
  upsert, dedupe key), `metrics_merge.go` (identity), `metric_value_notation.go` (condition
  inheritance moves to frame facets; new `dropResourceSpecRows`), and a new
  `metric_frame.go` (label map, normaliser, backfill).
- **Prompts:** `prompts/prompt-extract-metric-candidates-v29.md` and
  `prompts/prompt-enrich-metrics-v28.md`, with new default pins in code and in
  `mise.local.toml`.
- **API and UI:** `server/api/kbhandler/metrics_handler.go`, `metric_wiki_compile.go`,
  `metrics_dropped.go`; `web/src/lib/services/kbService.ts`, `metric-detail-groups.ts`, the
  detail panel and `messages/*.json`.
- **`.agents` skills:**
  - `extract-metrics-benchmark` gets rules `9.0.0.md` and frame fields in the gold tooling.
  - `score-extract-metrics` gets scoring v4.
- **Gold data:** re-annotate 364 and 371 under 9.0.0. Validate on one non-clinical document as
  well, because SemOS must stay domain-neutral.
