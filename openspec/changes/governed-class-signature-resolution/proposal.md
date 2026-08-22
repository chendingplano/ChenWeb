## Why

Metric class identity today is decided by exactly one of three signals, in order: a prior
`metric_definition_term_id`, keyword-concept alignment, or a hash of the raw `metric_name` string
alone. None of the three consult `subject`, `object_name`, `value_class`, or unit/quantity — the
signals that distinguish "ambient temperature" from "device-surface temperature" when both are
extracted under the identical name string. Two genuinely different governed metrics silently
collapse into one class on the primary path, not just in an edge case (ADR `2026081701` DR7, never
built; ADR `2026082203` §2.2). Separately, `kb.ontology_terms.properties` is an arbitrary bag of
whatever an operator exposed, `unit`/`unit_term_id` is the only field anywhere with resolve-against-
a-governed-catalog-but-keep-the-raw-value treatment, and a live-data review (bug `2026082101`
findings 5-8) surfaced that nothing in the codebase actually depends on the `object_literal` vs
`qualifiers` split it inherited.

## What Changes

- Add `kb.governed_property_value_map`: one generic resolve-or-propose table (dimension, raw_value)
  → term_id, generalizing `kb.metric_value_range_type_map`'s existing propose/approve shape (ADR
  `2026081401`) across every identity-bearing occurrence field instead of growing more one-off
  tables. A resolver mirrors `ValueRangeTypeMapper.Lookup` exactly: cache-backed lookup,
  auto-insert `status='proposed'` on a miss, `occurrence_count`/`first_seen`/`last_seen` bookkeeping,
  never discards the raw value regardless of resolution outcome. No `term_kind` is minted or decided
  by this change (OD2) — approving a `proposed` row to a `term_id` stays a curator/admin-handler
  action, exactly like today's value-range-type triage; the table imposes no opinion on which
  governed vocabulary an approved value belongs to.
- **BREAKING**: `[ontology_term_property_map]` moves from a flat `"artifact_type:field:property"`
  string list to a TOML table array (`[[ontology_term_property_map.<type>]]`) carrying two new
  per-entry attributes: `identity` (does this field participate in class-identity matching) and
  `dimension` (which `kb.governed_property_value_map` dimension resolves it). Every deployment's
  `config.local.toml`/`config.toml` needs its `[ontology_term_property_map]` section rewritten, not
  just `metric`'s entries. `[semantic_assertion_property_map]` (instance-level fields) keeps its
  existing flat-string format — DR4 does not touch it.
- `resolveOrCreateMetricClass` gains a new first resolution step: resolve every `identity = true`
  configured dimension for the occurrence, and reuse an existing class only when every dimension
  resolved on **both** sides agrees exactly (strict match — an unresolved/absent dimension on
  either side is a don't-care, never a mismatch). No match falls through to today's unchanged
  three-step order (`MetricDefinitionTermID` → `ConceptID` → name-hash), so behavior is identical to
  today until `kb.governed_property_value_map` accumulates approved rows.
- `kb.ontology_terms.properties` is reframed (no schema change) as the class-identity signature:
  every configured `identity = true` field is written in the shape `{"raw": ..., "term_id": ...}`
  (`term_id: null` = attempted, unresolved; key absent = not configured for this artifact type),
  distinguishing "unknown" from "not applicable." Non-identity fields keep today's plain-value
  shape.
- Document (no code change) that `object_literal` and `qualifiers` already function as one property
  bag in practice — nothing branches on the distinction beyond the writer that populates them and
  the CHECK constraint separating literal-valued from reference-valued assertions, which is
  unrelated and stays exactly as-is. The two columns are **not** merged or renamed in this change
  (OD5 deferred: lower migration risk until DR1's "no functional dependency" claim is re-verified
  against a live corpus).

**Explicitly out of scope** (confirmed with the user before drafting this proposal):
- **DR6** (`search_document` drawing on signature fields) is deferred in full. ADR `2026082102`
  (hybrid search over `kb.ontology_terms`), which DR6 extends, has not been implemented — no
  `kb.ontology_term_search` table, no `ReindexOntologyTermSearch`, no openspec change for it exists
  in this repo yet. There is nothing for DR6 to attach to.
- **OD1** (per-dimension normalization strength) uses the ADR's own stated floor —
  `normalizeValueRangeTypeRaw`'s existing lowercase/trim/dash-to-underscore rule — for every
  dimension. Stronger, dimension-specific normalization is explicitly deferred to a future change
  once real `kb.governed_property_value_map` `proposed` volume exists to inform it (the ADR's own
  stated precondition).
- **OD4** (exact config-array schema) is resolved by this change's own DR4 migration — see Impact.

## Capabilities

### New Capabilities
- `governed-property-value-resolution`: the generic per-dimension resolve-or-propose mechanism
  (`kb.governed_property_value_map`) and its resolver, generalizing the existing
  `kb.metric_value_range_type_map` pattern.
- `class-identity-signature`: `kb.ontology_terms.properties` as a deterministic, per-artifact-type
  configured class-identity signature (DR2/DR4), including the property-bag unification invariant
  (DR1).

### Modified Capabilities
- `class-resolution-decisions`: adds a signature-based resolution step, tried before the existing
  `MetricDefinitionTermID`/`ConceptID`/name-hash fallback order, using strict agreement on every
  dimension resolved on both sides.

## Impact

- **New table + migration**: `kb.governed_property_value_map` (`project_migrations/`, goose format,
  mirroring `20260814000002_create_kb_metric_value_range_type_map.sql`).
- **New/changed Go code**: `server/api/ontology/assertions/` (new governed-property resolver
  alongside `metric_normalizer.go`'s `ValueRangeTypeMapper`; `metric_lossless_writer.go`'s
  `resolveOrCreateMetricClass`), `server/cmd/config/config.go` (`OntologyTermPropertyMapConfig`
  restructured to a table-array shape; `GetOntologyTermPropertyMap` return type changes), config
  consumers in `metric_normalizer.go`/`class_synthesis.go`/`alignment.go` (`termProperties`/
  `BuildMappedProperties` call sites now need identity/dimension awareness, not just flat
  field→property copying).
- **Breaking config change**: `config.toml`/`config.local.toml`'s `[ontology_term_property_map]`.
  Found during exploration: the currently-checked-out working tree already has an *uncommitted*,
  in-progress change (`fix-metric-property-scope-conflation`, bug `2026082101` findings 5-8) whose
  own `tasks.md` claims `config.local.toml`'s `[ontology_term_property_map]` was emptied and a new
  `[semantic_assertion_property_map]` section added with 12 entries — but the actual file on disk
  (gitignored, per-environment) still has the old 12-entry flat list under
  `[ontology_term_property_map]` and no `[semantic_assertion_property_map]` section at all. This
  change's DR4 migration rewrites `[ontology_term_property_map]` wholesale regardless, so it
  produces the correct end state either way; flagged here rather than silently assumed.
- **No schema change** to `kb.ontology_terms.properties` (stays `JSONB`) or `kb.semantic_assertions`
  (`object_literal`/`qualifiers` stay two columns, per OD5 deferral).
- **Out of scope**: `kb.ontology_term_search`/`search_document` (ADR `2026082102`, not yet built).
