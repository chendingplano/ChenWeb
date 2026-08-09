## Why

Approving a QUDT resource on the "Review External Resources" page today only stages it into the keyword
lexicon's external-terminology tables (`kb.keyword_catalog_entries` and friends) — an immutable evidence
layer, not governed vocabulary. The `quantity` 4a ontology module (`kb.ontology_terms`) stays empty:
`cmd/qudt-import`, the only tool that can populate it, has never been run against the live database. The
ADR `2026072901` explicitly flags this as an open gap (§15.2): the module's claimed 4151-term catalog has
"never been run/verified against a live instance." The reviewer already does the licensing/provenance
review on this page; there is no reason approving should stop short of also producing the governed terms
that review was gating.

## What Changes

- `ApproveResource` gains a second write path, alongside its existing keyword-lexicon import: after the
  existing `terminology.Runner.Import` succeeds, parse the same downloaded `qudt-all.ttl` artifact for
  `quantity_kind`, `unit`, and `dimension` resources (reusing `terminology.ParseQUDTGraph`, not just the
  quantity-kind-only filter `QUDTAdapter.Convert` uses today) and write them into `kb.ontology_terms` /
  `kb.ontology_term_labels` / `kb.ontology_mappings`, following the same direct-store-write pattern
  `cmd/qudt-import` already uses (`terms.TermStore`, `terms.LabelStore`, `terms.MappingStore`).
- Both writes happen in a single approve request. If either the keyword-lexicon import or the ontology
  write fails, the whole approve fails and neither is left partially committed — no new inconsistent state
  should be observable to callers.
- After inserting/updating terms, Approve creates a new `quantity` module release (`modules.ReleaseStore.CreateRelease`)
  and activates it (`modules.ReleaseStore.Activate`), so the terms are immediately at
  `status='included_in_release'` and visible to the existing unit-resolution consumer
  (`associate_semantics.resolveUnitTerms`, which only reads `included_in_release` rows) without a separate
  manual release step.
- Re-approving byte-identical content stays idempotent on both sides: the existing
  `alreadyImportedIdentical` checksum short-circuit is extended to also skip the ontology write when the
  `quantity` module already has a release built from this exact source checksum.
- Disapprove/reject is unaffected — this change only extends the Approve path.
- Only the `qudt` resource is in scope. Other registered terminology resources are unaffected unless
  they're later found to need the same treatment.

## Capabilities

### New Capabilities
- `ontology-quantity-governance`: approving a reviewed QUDT resource promotes its quantity-kind, unit, and
  dimension terms into the governed `quantity` ontology module (`kb.ontology_terms` and related tables),
  including release and activation, so the module stops being seed-code-only.

### Modified Capabilities
(none — no existing spec covers this behavior; the current Approve behavior is only implicit in code, not
spec'd, so this is captured as a new capability rather than a delta)

## Impact

- **Backend code**: `server/api/terminologyresourcehandler/handler.go` (`ApproveResource`), a new helper
  package (or additions to `server/api/ontology/terminology/`) that reuses `ParseQUDTGraph` for all three
  QUDT classes and reuses `terms.TermStore` / `terms.LabelStore` / `terms.MappingStore` /
  `modules.ModuleStore` / `modules.ReleaseStore` (all under `server/api/ontology/`).
- **Database**: no new migrations — `kb.ontology_terms.term_kind` already includes `quantity_kind`/`unit`/
  `dimension`; `kb.ontology_mappings.to_iri` already models the QUDT source-IRI mapping. Writes land in
  `kb.ontology_terms`, `kb.ontology_term_labels`, `kb.ontology_mappings`, `kb.ontology_modules`,
  `kb.ontology_module_releases`, `kb.ontology_active_releases`.
- **Downstream consumers**: `associate_semantics.resolveUnitTerms` (unit resolution during Phase D) starts
  seeing real `quantity` module rows instead of zero; this is the intended effect, but it means Approve on
  this page now has a direct, immediate production side effect it didn't have before.
- **No frontend changes expected** — the existing Approve button/flow is unchanged from the operator's
  point of view; only its server-side effects grow.
