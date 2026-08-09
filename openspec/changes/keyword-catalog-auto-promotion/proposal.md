## Why

Approving an external terminology resource (QUDT, UCUM, SIRP, Wikidata, IEC) only ever populates the keyword lexicon's immutable staging tables (`kb.keyword_catalog_entries` and friends). Nothing today turns that staged data into usable `kb.keyword_concepts` rows — the mechanism the codebase was designed around for this (`PromotionStore.ApplyReviewedPromotion`, a hand-authored reviewed-promotion file) has zero production callers; it's dead code.

The operating principle for this system should be: promotion happens autonomously, without requiring a human in the loop, but every autonomously-created row is clearly flagged as such, and a human can optionally review/merge it further — never *must*. Mandatory human review is not a safer default here; it's a bottleneck and it doesn't eliminate error (humans misjudge too). `kb.keyword_concepts` is already explicitly designed as "ungoverned" for exactly this reason (no draft/review status exists in its schema) — the same principle already applies to the keyword-reconciliation subsystem, handled separately. This change applies it to the external-resource staging path.

Access to the approve action is already gated by System Admin page access control — the operator who can click Approve is already trusted with the consequences of that action. Nothing about promotion should re-litigate that trust with a second, source-data-quality-based gate; it should default to happening, with an admin able to opt a specific resource out if they want more caution there.

## What Changes

- **Auto-promote staged catalog entries into flagged provisional concepts.** After an Approve writes to the keyword-lexicon staging tables, a new step walks the newly-registered `(source, release)`'s `kb.keyword_catalog_entries`/`kb.keyword_catalog_labels` and creates a provisional `kb.keyword_concepts` row per entry (converging on an existing one if the same normalized label+scope already resolved to a concept — the same idempotent, content-hashed-ID mechanism the existing online auto-create path already uses), tagged `gloss_source = 'auto:import:<source>'` and surface `provenance = 'auto:import:<source>'`.
- **Enabled by default for every resource.** No source-trust gating on the default — the System Admin page's own access control is the authorization. An admin can explicitly disable auto-promotion for a specific resource if they want.
- **Runs in the background right after Approve**, without blocking the HTTP response, since a full catalog can be hundreds to thousands of entries.
- **Out of scope, deliberately:** anything about the keyword-lexicon reconciler (`Reconciler.Run`/`ReconcileAmbiguous`), its scheduling, or its merge-candidate eligibility. That subsystem's "human optional" behavior has already been addressed separately. Auto-promoted concepts here are ordinary provisional concepts, reviewable and mergeable through the existing manual merge endpoint at any time; whether/when they're picked up by automatic reconciliation is that subsystem's concern, not this change's.

## Capabilities

### New Capabilities
- `keyword-catalog-auto-promotion`: staged external-resource catalog entries are automatically promoted into flagged, provisional keyword concepts, enabled by default and admin-disable-able per resource, without requiring a human review step, while remaining fully human-reviewable/mergeable afterward through existing mechanisms.

### Modified Capabilities
(none — no existing spec covers this behavior)

## Impact

- **Backend code**: a new auto-promotion function in `server/api/ontology/keywords/`; a new promotion-policy store; a background-trigger call site added to `ApproveResource` (a `go func()` after the existing keyword-lexicon import step, following patterns already used elsewhere in this codebase, e.g. `api/llmreporthandler/jobs.go`); a new small admin CRUD surface for the per-resource override.
- **Database**: one new migration for the mutable promotion-policy table (`kb.keyword_sources` itself is immutable by trigger and cannot carry this flag). No changes to any existing staging table, and no changes to `reconcile.go` or the scheduler.
- **Frontend**: a new per-resource auto-promotion toggle on the External Terminology Resources page. No changes to the Review External Resources approve/disapprove flow itself.
- **Downstream consumers**: none beyond `kb.keyword_concepts` gaining new provisional rows tagged with a new `gloss_source` prefix (`auto:import:*`) that didn't previously occur in the data.
