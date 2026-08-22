## Context

`kb.ontology_terms` is a single table shared by 8 term kinds (`class`,
`property`, `individual`, `concept`, `metric_definition`, `quantity_kind`,
`unit`, `dimension`). Only one producer — the ADR 2026081201 auto-promotion
path (`alignment.go:EnsureAcceptedOrCreate`) — has ever populated
`value_type`, `range_type`, `permitted_unit_term_ids`; every other producer
(seed content, QUDT import, candidate promotion) leaves them NULL. The
append-only revision architecture from ADR 2026081701
(`kb.ontology_term_headers` / `kb.ontology_term_revisions` +
`kb.sync_ontology_term_revision_after_insert()` trigger +
`kb.ontology_terms_current` view) mirrors every column of
`kb.ontology_terms`, including these three, so a schema change here touches
both tables, the trigger, and the view.

A grep sweep (see proposal Impact) confirms `terms_store.go` is the only Go
code that reads or writes these three columns — no other handler, wiki
generator, comparison store, or frontend view consumes them — so removing
them is contained to that file plus the two call sites that populate/read
`Term.ValueType`/`RangeType`/`PermittedUnitTermIDs` (`alignment.go`
writes them; nothing reads them back off a `terms.Term` value today).

The bug's data-loss findings (definition sourcing, unit string) are
independent of the schema question and are fixed in the same change only
because both live in the same synthesis call site
(`extract-metrics.go:resolveAll`) and the same `TermSynthesisInput` type.

## Goals / Non-Goals

**Goals:**
- Stop losing the unit string and the descriptive-sentence definition on
  every newly auto-promoted `metric_definition` term.
- Give kind-specific term data (today: metric synthesis fields) a schema
  home that doesn't spend three columns other 7 kinds never use.
- Keep the append-only revision mirror and `kb.ontology_terms_current`
  consistent with the base table, as every prior migration in this area
  has.

**Non-Goals:**
- Backfilling or repairing existing auto-promoted rows — the bug doc is
  explicit that affected data is already cleaned, and the schema cut is
  clean (no old rows survive to backfill).
- Migrating the QUDT importer's `symbol`/`deprecated` payload into
  `properties` — the new column is defined generally enough for that to
  happen later, but doing it now would touch `qudt.go`,
  `terminology/qudt_terms.go`, and `terminologyresourcehandler/qudt_ontology.go`
  for no behavior change today. Out of scope.
- Refreshing an existing auto-promoted term when a richer later occurrence
  of the same concept appears (`EnsureAcceptedOrCreate`'s early return).
  Explicitly deferred by the bug doc as a separate decision.

## Decisions

**D1: `properties JSONB`, not per-kind tables or more flat columns.**
A single JSONB column keyed by convention per `term_kind` matches the
existing precedent (QUDT's staged `map[string]any{"deprecated", "symbol"}`
payload) without a schema migration every time a new kind needs a new
property. Alternative considered: per-kind child tables
(`kb.ontology_metric_term_properties`, etc.) — rejected as premature; only
one kind has structured properties today, and a join-per-read for a
single-producer feature isn't justified yet.

**D2: Drop the three old columns outright, not deprecate-in-place.**
The bug doc leaves "keep for one cycle vs. backfill-and-drop" as an open
call; this design picks drop-now because (a) the user has already cleared
the only data that ever populated them, so there is nothing to backfill,
and (b) `terms_store.go`'s own comment already documents them as
single-producer dead weight for every other kind — leaving them in the
schema as permanently-NULL columns for 7/8 kinds is the thing being fixed,
not a acceptable intermediate state. This is called out as **BREAKING** in
the proposal since it changes the `kb.ontology_terms` / `_current` /
`terms.Term` JSON shape.

**D3: `properties` keys for `metric_definition`: `value_type`,
`range_type`, `permitted_unit_term_ids`, `raw_unit`.**
`raw_unit` is new — the untrimmed `metric_unit` string from the triggering
`kb.metrics` row, written unconditionally whenever non-empty, independent
of whether `MatchUnitLabel` resolves it. `permitted_unit_term_ids` is
still written only when resolution succeeds. This directly fixes finding 3:
a resolver miss (unreleased unit, transient DB issue, an unmatched symbol
form) no longer destroys the source string, because it was never only
carried in the field that resolution populates.

**D4: `definition` sourcing: `metric_desc`, falling back to
`formula_or_definition`.**
Per the bug doc's assessment: `metric_desc` is already the descriptive
sentence every other `definition` writer produces; `formula_or_definition`
is legitimately empty for bare-threshold sources per the extraction
prompt. Fallback order (not concatenation) keeps a single coherent
sentence rather than a stitched one — concatenation was considered and
rejected because the two fields can both be non-empty with overlapping
content (see the live example row, where `metric_desc` already restates
the metric name).

**D5: `terms.Term.Properties map[string]any`, marshaled with
`encoding/json` on write and unmarshaled on scan.**
Matches how `permitted_unit_term_ids` was already handled
(`nullableStringArray`/`scanStringArray` helpers) — same pattern, generalized
to arbitrary JSON. `nil`/empty map persists as SQL NULL, matching current
NULL-for-unpopulated-kinds behavior.

## Risks / Trade-offs

- [Dropping columns is irreversible in the down-migration once new rows
  exist with `properties` populated] → Mitigation: down-migration
  reconstructs `value_type`/`range_type`/`permitted_unit_term_ids` from
  `properties` best-effort (JSONB extraction), acceptable lossy rollback
  since this environment tolerates destructive migration operations
  (per workspace CLAUDE.md: "this is a staging server").
- [`properties` is untyped `map[string]any` in Go, weaker than the old
  typed columns] → Mitigation: only one producer writes it today
  (`alignment.go`), and the key convention is documented in the struct's
  doc comment, same as the old columns' single-producer comment was.
- [Trigger/view drift if a future column is added to `kb.ontology_terms`
  without updating both mirrors] → Pre-existing risk from ADR 2026081701,
  not introduced by this change; not addressed here.

## Migration Plan

Single forward migration:
1. `ALTER TABLE kb.ontology_terms ADD COLUMN properties JSONB;`
2. `ALTER TABLE kb.ontology_terms DROP COLUMN value_type, DROP COLUMN range_type, DROP COLUMN permitted_unit_term_ids;`
3. Same two steps on `kb.ontology_term_revisions`.
4. `CREATE OR REPLACE FUNCTION kb.sync_ontology_term_revision_after_insert()` — replace the three columns with `properties` in both the INSERT column list and the `NEW.*` values.
5. `CREATE OR REPLACE VIEW kb.ontology_terms_current` — same column swap.

Down migration reverses in opposite order, re-adding the three columns and
best-effort extracting them from `properties` where present.

`mise dev`/`air` auto-applies migrations on the live dev server (per
project memory) — no manual migration run needed.

## Open Questions

None — the bug doc's one deferred decision (refresh-on-later-occurrence)
is out of scope per the proposal.
