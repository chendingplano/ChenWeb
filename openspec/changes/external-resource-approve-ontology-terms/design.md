## Context

`ApproveResource` (`server/api/terminologyresourcehandler/handler.go:135-198`) currently does two things
on approve: flip the local draft manifest's `license_review_status` to `approved`
(`terminology.ApproveDraft`), then run `terminology.Runner{DB}.Import(ctx, manifestPath)`
(`server/api/ontology/terminology/importer.go:339-385`), which writes only to the keyword lexicon's
immutable staging tables (`kb.keyword_sources`, `kb.keyword_source_artifacts`,
`kb.keyword_catalog_entries/labels/relations`, `kb.keyword_ucum_codes`). It never touches `kb.ontology_*`.

For QUDT, the whole combined graph (units + quantity kinds + dimension vectors) is downloaded as one file,
`qudt-all.ttl`. The adapter that feeds the keyword-lexicon import (`QUDTAdapter.Convert`,
`server/api/ontology/terminology/qudt.go:248-302`) parses the full graph via the shared
`terminology.ParseQUDTGraph` but then filters to `QuantityKind` resources only — units and dimension
vectors are parsed and silently dropped.

Separately, `server/cmd/qudt-import/main.go` already knows how to turn the same kind of QUDT content into
governed `kb.ontology_terms`/`kb.ontology_term_labels`/`kb.ontology_mappings` rows for the `quantity`
module (`term_kind` = `quantity_kind`/`unit`/`dimension`, already in the DB CHECK constraint — added for
this exact purpose). It has just never been run against the live database, which is the gap this change
closes. `cmd/qudt-import` lives in `package main`, so its logic isn't importable as-is; this change needs
to lift the reusable parts into an importable package.

Per the ADR (`2026072901`, §15.2), only `core` is currently a released+active 4a module; `quantity` has
seed/import code but zero live rows.

## Goals / Non-Goals

**Goals:**
- Approving the `qudt` resource writes `quantity_kind`, `unit`, and `dimension` terms (full parity with
  `cmd/qudt-import`'s scope) into `kb.ontology_terms`, with labels into `kb.ontology_term_labels` and the
  QUDT source-IRI exact-mapping into `kb.ontology_mappings`.
- The write path matches the existing precedent (`cmd/qudt-import`, `cmd/ontology-seed`): direct store
  writes through `terms.TermStore` / `terms.LabelStore` / `terms.MappingStore` / `modules.ModuleStore`,
  not a per-term `kb.ontology_candidates` round trip.
- After writing terms, a new `quantity` module release is created and activated
  (`modules.ReleaseStore.CreateRelease` + `.Activate`), so the terms are immediately
  `status='included_in_release'` and visible to `associate_semantics.resolveUnitTerms`
  (`server/api/ontology/assertions/associate_semantics.go:330-335`), which only reads that status.
- Re-approving byte-identical QUDT content is a safe no-op on the ontology side, mirroring the existing
  `alreadyImportedIdentical` checksum short-circuit on the keyword-lexicon side.

**Non-Goals:**
- No new migration. Existing columns (`term_kind` enum, `kb.ontology_mappings.to_iri`) already fit this
  data at the same fidelity `cmd/qudt-import` already achieves (identity, kind, one label, one symbol,
  exact-IRI mapping). QUDT conversion factors, dimension-vector numeric components, and governed
  unit↔quantity-kind axioms stay out of scope — `cmd/qudt-import` doesn't capture those today either.
- No change to Disapprove/reject, and no change to any resource other than `qudt`.
- No change to the existing keyword-lexicon import behavior or its tables/immutability triggers.
- Not building an admin UI for reviewing/editing the parsed terms before they land — approve on the
  existing page is the only gate, same as it already is for the keyword-lexicon side.
- Not updating previously-imported term content when a re-approved QUDT release changes an existing term's
  label/definition — matches `cmd/qudt-import`'s current create-if-absent-only behavior; term *updates* are
  a separate future capability if ever needed.

## Decisions

**1. Extract `cmd/qudt-import`'s parse/classify logic into an importable package.**
Move `parseTerms`, `termKindFor`, and `pickLabel` (currently `server/cmd/qudt-import/main.go:90-152`) into
`server/api/ontology/terminology/` (alongside `ParseQUDTGraph`, which they already call), keeping
`cmd/qudt-import`'s `main.go` as a thin CLI wrapper over the same package. This avoids duplicating the
class-filter/term-ID/term-kind logic between the CLI and the HTTP handler — the single highest-risk source
of drift if the two paths were maintained separately (e.g. `qk_`/`unit_`/`dim_` ID-prefix conventions
silently diverging).
*Alternative considered:* duplicate the ~60 lines directly in the handler. Rejected — this is exactly the
kind of "two implementations of the same parsing rule" that causes term-ID mismatches down the line.

**2. Reuse the already-downloaded `qudt-all.ttl`, not three separate files.**
`cmd/qudt-import` takes three file-path flags (`--units`/`--quantity-kinds`/`--dimensions`) but
`ParseQUDTGraph` parses the whole graph regardless of which flag pointed at it — the class filter happens
after parsing. The handler will parse `qudt-all.ttl` (the file `terminology.Runner.Import` already read)
once and run all three class filters over the same parsed resource list, avoiding a second network fetch
or a require on three separate artifacts that don't exist in this pipeline.

**3. Sequencing and failure handling: two steps, but Step A is a real shared transaction, not just
idempotent retry.**
Revised after reading the actual store code: `terms.TermStore`, `terms.LabelStore`, and `terms.MappingStore`
all take a `DBX` interface already satisfied by both `*sql.DB` and `*sql.Tx` (see `terms_store.go`'s `DBX`
doc comment — "module release flow tags content rows atomically in one tx"), and
`terminology.Runner.Import`'s own transaction helper (`withImportTransaction`) already detects when it's
handed an existing `*sql.Tx` and reuses it instead of nesting. So **Step A** — the keyword-lexicon import
*and* the term/label/mapping batch writes — runs inside one transaction the handler opens itself
(`db.BeginTx`), passed as `terminology.Runner{DB: tx}` and `terms.TermStore{DB: tx}` /
`LabelStore{DB: tx}` / `MappingStore{DB: tx}`, committed once at the end. This gives literal atomicity for
the part that matters most (governed content correctness), not just idempotent-retry safety.
`modules.ModuleStore` and `modules.ReleaseStore`, by contrast, are concretely typed `DB *sql.DB` and each
open their *own* internal transaction (`s.DB.Begin()` inside `CreateRelease`/`Activate`) — they cannot join
an externally-supplied `*sql.Tx` without changing their field type and internal transaction ownership
everywhere they're used (the module compiler, other HTTP handlers). That's out of scope here (Simplicity
First / Surgical Changes) — so **Step B** (ensure module exists, create release, activate) stays its own,
separately-transactional unit, run only after Step A commits.
If Step B fails after Step A committed, the newly-written terms sit at `status='approved'`, un-released —
a safe, valid state (not corrupt), not different in kind from the normal window between `qudt-import`'s
plain run and its `--release` step. See Decision 4 for how a retry correctly resumes from exactly this
state.
*Alternative considered (previous revision of this doc):* treat both steps as independently idempotent and
retry-safe without a real shared transaction. Superseded — a real transaction for Step A is no harder to
write, given the stores already support it, and removes an entire class of "term written, label missing"
interleaving that idempotent retry alone would only paper over.

**4. Release versioning and gating: auto-increment version; gate Step B on live pending content, not on
this run's insert count.**
`cmd/qudt-import --release` hardcodes version `1.0.0`, which only works once. The handler instead queries
the current highest `kb.ontology_module_releases.version` for `module_id='quantity'` (or starts at `1.0.0`
if none exists) and bumps the patch component for the new release.
Whether to run Step B at all is decided by a fresh query — `SELECT EXISTS(... kb.ontology_terms WHERE
module_id='quantity' AND status='approved')` — **not** by how many terms Step A inserted on this particular
call. This matters for the retry case: if a prior approve's Step A committed new terms but Step B then
failed, those terms remain `status='approved'` and pending; a subsequent approve may find zero *new* terms
in Step A (they already exist) yet must still run Step B to release the ones stranded from before.
`ReleaseStore.CreateRelease`'s own validation (`buildSnapshot` only pulls rows still at `status='approved'`,
and errors with "no approved terms to release" if none exist) makes the existence check necessary, not
optional — calling `CreateRelease` unconditionally would turn a true no-op re-approve into an error.

**5. Batch the inserts rather than one row at a time.**
The QUDT catalog is ~4151 terms; a naive per-row `INSERT` loop across `TermStore`/`LabelStore`/
`MappingStore` (roughly 3 statements/term ≈ 12,000+ round trips) risks a slow or timed-out HTTP request.
The handler will batch inserts (multi-row `INSERT ... VALUES (...), (...), ...` or `COPY`-style batching)
within `TermStore`/`LabelStore`/`MappingStore`, adding a batch-insert method to each store rather than
calling the existing single-row `Create*` in a loop 4000+ times.

## Risks / Trade-offs

- **[Risk] Approve becomes a long-running HTTP request** (thousands of terms to insert/label/map, plus a
  release+activate step) → **Mitigation:** batched inserts (Decision 5); if batching still isn't fast
  enough in practice, revisit as a follow-up (background job + polling), but start with the synchronous
  path since it matches how Approve already behaves today (the keyword-lexicon import is already a
  non-trivial synchronous write).
- **[Risk] Auto-activating a release on every approve means Approve now has an immediate, un-gated
  production effect on unit resolution** (`associate_semantics`) where before it had none → **Mitigation:**
  this is the deliberately chosen trade-off (see proposal.md); make sure the Approve response/UI
  communicates that terms were released and activated, and rely on the existing
  `kb.ontology_active_releases` audit trail (activated_at/by, deactivated_at/by) plus
  `modules.ReleaseStore`'s rollback capability as the recovery lever if a bad release goes live.
- **[Risk] Step A (keyword-lexicon import + term/label/mapping writes) fails partway** →
  **Mitigation:** Decision 3's shared transaction means this can't leave a partial state at all — the
  whole thing commits or the whole thing rolls back. The remaining, narrower risk is Step B (release +
  activate) failing *after* Step A committed, leaving terms at `status='approved'` but un-released; Decision
  4's existence-based gating (not a per-run insert count) ensures a retry still finds and releases them.
- **[Trade-off] Term *content* updates are not handled** — if a future QUDT release relabels an existing
  term, the existing row is left as-is (create-if-absent only, matching `cmd/qudt-import` today). Flagged
  as a known limitation, not silently masked.

## Migration Plan

- No database migration required (Non-Goals).
- Code ships as a normal deploy; under the workspace's `mise dev` (air hot-reload) this applies immediately
  in the dev DB.
- Rollback of the *code change* is a normal revert — it only stops future Approves from writing ontology
  terms; it does not retroactively remove terms/releases already written.
- Rollback of a *bad release* (if an activated `quantity` release turns out to have bad data) goes through
  the existing `kb.ontology_active_releases` deactivate/rollback mechanism on `modules.ReleaseStore`, not
  through this feature.

## Open Questions

- Exact patch-version bump scheme for `quantity` releases (Decision 4) — confirm `1.0.0 → 1.0.1 → 1.0.2...`
  per successful non-empty write is acceptable, versus e.g. stamping the QUDT release string (`3.5.0`) into
  the version somehow.
- Batching approach for `TermStore`/`LabelStore`/`MappingStore` (Decision 5) — multi-row `INSERT` vs.
  `COPY`; left to tasks/implementation to pick based on what the existing store code already looks like.
- Whether the Approve response payload should report back how many terms/labels/mappings were written and
  the new release version, for the admin page to surface to the operator.
