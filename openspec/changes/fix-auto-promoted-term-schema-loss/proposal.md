## Why

The metric_definition auto-promotion path (ADR 2026081201 DR1/DR3) loses
recoverable data on every term it creates: `definition` is bound to a
field (`formula_or_definition`) the extraction prompt documents as
legitimately empty for bare-threshold sources, while the field that
actually holds descriptive prose for those same rows (`metric_desc`) is
never read; metric-only synthesis fields (`value_type`, `range_type`,
`permitted_unit_term_ids`) are flat columns on a table shared by 8 term
kinds, with no general home for kind-specific properties (the QUDT
importer has the identical problem with its orphaned `symbol`/`deprecated`
payload); and the raw `metric_unit` string is discarded whenever unit
resolution doesn't populate `permitted_unit_term_ids`, with no fallback
field to recover it later. Full root-cause detail:
`KnowledgeStore/doc-repo/bugs/202608/2026082101-bug-auto-promoted-ontology-terms-schema-and-data-loss.md`.

## What Changes

- Auto-promotion synthesis populates `kb.ontology_terms.definition` from
  `kb.metrics.metric_desc`, falling back to `formula_or_definition` when
  `metric_desc` is empty, instead of reading `formula_or_definition` alone.
- Add `kb.ontology_terms.properties JSONB` (and the matching column on
  `kb.ontology_term_revisions`, the sync trigger, and the
  `kb.ontology_terms_current` view). **BREAKING**: drop the
  `value_type`, `range_type`, `permitted_unit_term_ids` columns from both
  tables — the bug was filed after the affected data was already cleaned,
  so this is a clean schema cut with no backfill step. New writers key
  `properties` by term_kind convention; for `metric_definition` the keys
  are `value_type`, `range_type`, `permitted_unit_term_ids`, and (new)
  `raw_unit` — the untranslated `metric_unit` string, always stored
  alongside (or instead of, when resolution misses) `permitted_unit_term_ids`
  so a resolver miss never loses the source data.
- `terms.Term` gains a `Properties map[string]any` field replacing
  `ValueType`/`RangeType`/`PermittedUnitTermIDs`; all current producers of
  those three columns (auto-promotion in `alignment.go`) move to the new
  field. Other producers (QUDT import, candidate promotion, seed content)
  are unaffected since they never populated the old columns.
- Any reader of the removed columns (comparison/wiki/analysis code paths
  that select `value_type`/`range_type`/`permitted_unit_term_ids` from
  `kb.ontology_terms`/`kb.ontology_terms_current`) is updated to read the
  same data out of `properties`.

Not included (explicitly deferred by the bug doc as a separate decision):
refreshing an existing auto-promoted term's fields when a later, richer
metric occurrence for the same concept appears. `EnsureAcceptedOrCreate`
keeps its current early-return-on-existing-alignment behavior.

## Capabilities

### New Capabilities
- `ontology-term-kind-properties`: a `properties JSONB` bag on
  `kb.ontology_terms` for kind-specific structured data, replacing the
  metric-only flat columns and giving QUDT's `unit`/`quantity_kind`
  `symbol`/`deprecated` payload a home it can adopt later (adoption itself
  is out of scope here — only the column and the metric_definition usage
  are added now).

### Modified Capabilities
- `governed-term-auto-promotion` (spec draft currently in
  `openspec/changes/auto-promoted-governed-terms/specs/`, not yet
  archived): the "Auto-created term content is derived only from
  already-extracted structured fields" requirement's definition-sourcing
  and unit-handling scenarios change as described above.

## Impact

- Migration: new migration file adding/dropping columns on
  `kb.ontology_terms` and `kb.ontology_term_revisions`, updating
  `kb.sync_ontology_term_revision_after_insert()` and
  `kb.ontology_terms_current`.
- `server/api/ontology/terms/terms_store.go`: `Term` struct, `scanTerm`,
  `termColumns`/`termRevisionColumns`, `CreateTerm`, `CreateTermVersion`,
  `insertTermChunk`.
- `server/api/ontology/keywords/alignment.go`: `TermSynthesisInput`,
  `EnsureAcceptedOrCreate`.
- `server/api/doc-processing/extract-metrics.go`: `resolveAll` synthesis
  construction (definition sourcing, always-populate raw unit).
- Any other reader of the three dropped columns (grep sweep required
  during implementation — `metric_ontology_analysis_handler.go` and
  similar are candidates to verify).
