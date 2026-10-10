## Why

Documents point at other documents, standards, laws, datasets, software and single values all the
time. Today SemOS throws that information away: `extract_metrics` sets aside a clause like "shall
meet the requirements of GB 7959" (`delegated_requirement`), and nothing records what was cited or
links it to the cited document. Users, in any domain and any language, cannot see how their
documents are connected or what limit actually applies behind such a clause.

Devdoc `KnowledgeStore/doc-repo/devdocs/202610/2026101002-devdoc-extract-references-processor.md`
fixes the design. This change builds it, with the metric family as the first consumer.

## What Changes

- New Phase B doc processor **`extract_references`**, a lower layer that no other processor
  feeds. It detects every reference in a document and labels it by:
  - target type and governed subtype (`others` fallback);
  - mechanism;
  - relationships (asserted);
  - target scope, locator and content;
  - form (`listed` / `in_text`), normativity (taken from formal lists only), and whether the
    reference sits inside a requirement.
- **Resolution levels:**
  - R1 resource catalog, separating identity from location, shared across customers for public
    identifiers;
  - R2 holding, which respects document access;
  - R3 location, minimal in this change.
- **A resolver contract** (`Takes` / `Resolve` / `Apply` / `Withdraw`) that artifact families
  implement:
  - step 5: `extract_references` calls the registered resolvers;
  - step 6: each artifact extractor calls its own resolver after writing its artifacts;
  - the rule "whoever comes second resolves", with write-then-look and idempotent upserts.
- **The metric resolver**, the first family:
  - it takes `criterion` / `criteria_set` references;
  - it matches them against the target document's metrics;
  - it writes links, and writes resolved rows into the referencing document's `kb.metrics` (ids
    `<rec>_rfm_<n>`, `ref_*` columns).
- **`extract_metrics` changes:**
  - its delete and merge never touch resolved rows;
  - it runs step 6 after saving.
- **Readers of `kb.metrics`:** every query site gets an explicit default for resolved rows. They
  are excluded from computation (reviews, Phase D, class contracts, benchmark, ontology) and shown
  in metric lists.
- **New tables** (goose migrations): `kb.ref_mentions`, `kb.ref_resources`,
  `kb.ref_resource_versions`, `kb.ref_resource_identifiers`, `kb.reference_artifact_links`. New
  `kb.metrics` columns: `ref_mention_id`, `ref_input_record_id`, `ref_metric_id`,
  `ref_metric_key`.
- **Not in this change** (separate follow-ups):
  - reference search partition and document-graph page;
  - review pages for candidates;
  - contradictions admin page;
  - generic R3 location;
  - resolvers for provisions, entities and inventory items;
  - reconciling references with entities.

## Capabilities

### New Capabilities
- `document-references`: detection of references, the reference model and its labels,
  normativity, the resource catalog, R1/R2 resolution, access rules, processor status and
  re-runs.
- `reference-artifact-resolution`: the layering contract between `extract_references` and artifact
  extractors, steps 5 and 6, write-then-look ordering, the link table, and re-run behaviour.
- `metric-reference-resolution`: the metric resolver, matching and thresholds, resolved rows in
  `kb.metrics`, how `extract_metrics` protects them, and the default for each `kb.metrics` reader.

### Modified Capabilities
<!-- none: no existing spec in openspec/specs/ covers extract_metrics persistence -->

## Impact

- **Code:**
  - `server/api/doc-processing/`: new `extract-references*.go`, `reference_*.go`;
    `extract-metrics.go` (delete/merge guard, step 6); `processor_plan.go`, `runtime.go`,
    `control.go` (registration);
  - the ~30 files that query `kb.metrics` (reader defaults);
  - `server/cmd/doc-processor` (registration);
  - the web dashboard processor lists.
- **Prompt:** `prompts/prompt-extract-references-v1.md`.
- **Config:** `config.toml` `required_processors`; env `EXTRACT_REFERENCES_MODEL_NAME`,
  `EXTRACT_REFERENCES_PROMPT`, `REFERENCE_METRIC_MATCH_MIN_SCORE`.
- **DB:** 5 new tables and 4 new `kb.metrics` columns.
- **Benchmark tooling** (`.agents` `score_io.py`, `benchmark_io.py`): exclude resolved rows.
- **Docs:** capsule `extract-references-spec.md` / `-impl.md`, capsule pipeline table, devdoc
  2026101002 status.
