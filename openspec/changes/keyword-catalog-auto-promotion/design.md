## Context

Staging never becomes a concept. `terminology.Runner.Import` (invoked by `ApproveResource`) writes only to `kb.keyword_catalog_entries`/`kb.keyword_catalog_labels`/`kb.keyword_catalog_relations` — immutable evidence. The mechanism the design docs describe for turning that into `kb.keyword_concepts` content, `keywords.PromotionStore.ApplyReviewedPromotion` (`server/api/ontology/keywords/identity_import.go:176`), has no production caller anywhere — no CLI, no route, only test files.

The one thing in this codebase that already lives the target principle end-to-end is the online resolve path: when it hits a targeted miss, `KeywordFamily.autoCreateConcept` (`keywordfamily.go:663-711`) immediately mints a `status='provisional'` concept tagged `gloss_source='auto:d11'` and a matching `provenance='auto:resolver'` surface, using a content-hashed concept ID (`autoConceptID`, `keywordfamily.go:717-720`, `sha256(norm_key|scope)[:6]`) so repeated attempts at the same normalized name converge on one row instead of duplicating. No human approval gate exists anywhere in the keyword-concept lifecycle — the schema itself has no `pending_review`-style status (`concepts_store.go:21-23`'s doc comment calls concepts "ungoverned" explicitly).

This change extends the same pattern (auto-create, tag, leave cleanup to whatever already handles it) to the external-resource staging path. It deliberately does **not** touch the keyword-lexicon reconciler (`Reconciler.Run`/`ReconcileAmbiguous`) or its scheduling — that subsystem's "human optional" behavior is out of scope here, already handled separately.

## Goals / Non-Goals

**Goals:**
- Staged catalog entries from an approved resource become provisional, clearly-flagged `kb.keyword_concepts` rows without requiring a human to author anything.
- Enabled by default for every resource — the System Admin page's own access control is the authorization; no additional source-trust gate on the default.
- An admin can still explicitly disable auto-promotion for a specific resource.
- A human can still review, merge, or otherwise act on any auto-promoted concept through the existing `POST /kb/keyword-concepts/:concept_id/merge` endpoint at any time; nothing here removes that capability or requires it.

**Non-Goals:**
- Not building `PromotionStore.ApplyReviewedPromotion`'s consumer (the reviewed, evidence-attaching promotion file flow) or the "authoritative identity claim" machinery it feeds (`kb.keyword_surface_evidence`/`kb.keyword_external_ids`). Separate system.
- **Not touching `Reconciler.Run`/`ReconcileAmbiguous`, its eligibility query, or its scheduling.** That subsystem's autonomy has already been addressed in separate work. Whether or when auto-promoted concepts get automatically merged/cleaned up by that subsystem is its own concern.
- Not building a `kb.keyword_unresolved` backlog admin UI. Unrelated to this gap.
- Not changing anything about Disapprove, or about resources whose staging import itself fails.
- Not adding retry/backoff/queueing for the background promotion trigger. If it fails, it's logged; the next Approve of the same (or a re-downloaded) resource attempts it again, and it's idempotent by construction (Decision 2).

## Decisions

**1. New auto-promotion function, not a rebuild of `ApplyReviewedPromotion`.**
Add `PromoteCatalogEntries(ctx, db, source, release, scope string, by string) (PromotionCounts, error)` to `server/api/ontology/keywords/` (new file `catalog_promotion.go`). For each `kb.keyword_catalog_entries` row of the given `(source, release)`, it: picks a preferred label from the row's `kb.keyword_catalog_labels` (English-preferred-first, same fallback chain pattern as `pickQUDTLabel`/`pickLabel` elsewhere in this codebase), normalizes it (`semid.Normalizer{Version: <current>}.Normalize(label)`), and calls the same create-or-converge sequence `autoCreateConcept` uses — `ConceptStore.CreateConcept` with `GlossSource: "auto:import:" + source`, falling back to `GetConcept`/`FollowMerge` on a content-hash collision — then `SurfaceStore.CreateSurface` with `Provenance: "auto:import:" + source`.
*Alternative considered:* wire up `ApplyReviewedPromotion` instead. Rejected — it requires a human-authored `ReviewedPromotionFile` per entry and only *links* to an already-`active` concept; it does not create one. That's a fundamentally different (and still human-gated) operation from what's being asked for here.

**2. Idempotency via the existing content-hash convergence, not a new "already promoted" tracking table.**
Because the concept ID is `sha256(norm_key|scope)[:6]`, re-running `PromoteCatalogEntries` for the same `(source, release)` — e.g. on a replayed Approve — converges on the same concept rows instead of duplicating them, exactly like two independent online misses on the same name already do. No new schema is needed to track "has this catalog entry been promoted."
*Trade-off:* a full re-scan re-attempts every entry every time (an insert-then-fallback-to-lookup per entry), which is wasted work on a no-op replay. Accepted for now — same cost profile the existing auto-create path already accepts, and catalog sizes here (hundreds to a few thousand rows, per resource) keep it cheap.

**3. Enabled by default; a plain per-resource opt-out, not a source-trust-gated default.**
`kb.keyword_sources` has an unconditional immutability trigger (`keyword_sources_immutable`, `project_migrations/20260807000001...sql:258-260` — blocks UPDATE/DELETE outright, no column exceptions), so an "auto-promote enabled" flag cannot live there regardless of how the default is computed. A new, deliberately mutable table, `kb.keyword_source_promotion_policy` (`source TEXT PRIMARY KEY`, `auto_promote_enabled BOOLEAN NOT NULL DEFAULT TRUE`, `set_by`, `set_at`, timestamps), holds the override. No row means enabled (the column default). This is a plain boolean, not derived from `authority_role` or any other source-trust classification — the page's own System Admin access control is treated as the authorization for this action, matching how the operator is already trusted to approve the resource in the first place.
*Alternative considered (earlier revision of this doc):* default gated by `kb.keyword_sources.authority_role` (only `exact_identity_authority`/`conditional_identity_authority` sources auto-promote by default). Superseded — conflates a data-quality signal about the *source* with an authorization decision about the *operator*, which the page's access control already makes. An admin who wants to be more cautious about a specific low-authority source can still disable it explicitly; the system just doesn't assume that caution is needed by default.

**4. Background trigger, not synchronous, not per-catalog-entry.**
After `ApproveResource`'s existing keyword-lexicon import step succeeds (fresh or replayed) for a resource whose policy row (Decision 3) is enabled, the handler launches `go func() { PromoteCatalogEntries(context.Background(), db, source, release, scope, logger) }()`, capturing `db` and `logger` (via `rc.GetLogger()`) before the goroutine starts — the request's own context is cancelled once the response is written, so the goroutine must use its own. Errors are logged, not surfaced to the HTTP response; the operator's Approve action already returned successfully by the time this runs. This mirrors the existing `StartBackgroundDailyUsageReportGeneration`-style pattern (`api/llmreporthandler/jobs.go`) rather than inventing a new one.
*Alternative considered:* run it synchronously in the same request (like the QUDT ontology-term write in a separate, earlier change). Rejected specifically for this step — unlike that write, this one has no natural bound proportional to "how much new content," since a full catalog promotion pass touches every entry in the release; keeping Approve's response time independent of catalog size was judged more valuable here than the stronger observability a synchronous call gives.

## Risks / Trade-offs

- **[Risk] A resource with a low-quality label set could flood the concept table with junk provisionals**, and there's no source-trust default holding that back anymore. → **Mitigation:** the admin override (Decision 3) gives an operator an explicit kill switch per source without needing a code change or migration, and this is a pre-existing risk class (the online auto-create path already accepts it for the same reason: cleanup, whenever/however it happens, is a separate concern from creation).
- **[Trade-off] Auto-promoted concepts are not automatically merged/cleaned up by anything as part of this change** — they sit as provisional rows, flagged, until either a human merges them via the existing endpoint or a *separately maintained* reconciliation mechanism picks them up (explicitly out of scope here, per the Non-Goals). This is an intentional scope boundary, not an oversight — flagging it clearly so it isn't mistaken for "this change makes them auto-merge too."
- **[Risk] Background goroutine failures are invisible unless someone reads logs.** → **Mitigation:** accepted (matches Decision 4's non-goal on retry/queueing); the idempotent convergence (Decision 2) means the next Approve of the same resource self-heals a failed run.

## Migration Plan

- One new migration: `kb.keyword_source_promotion_policy` (Decision 3). No changes to any existing table or trigger, and no changes to `reconcile.go` or scheduler code.
- No backfill needed — the table starts empty, and an empty/missing row means "enabled," which is the correct default behavior for every source that predates this change.
- Rollback: revert the code; existing auto-promoted concepts (if any were created) remain in `kb.keyword_concepts` as ordinary provisional rows — nothing about them requires this feature to exist afterward.

## Open Questions

- Exact preferred-label selection rule when a catalog entry has labels in multiple languages and none marked `en` (currently: same fallback chain as `pickQUDTLabel`, first-preferred-then-first-available) — confirm this is the right default for non-QUDT sources (UCUM/SIRP/Wikidata) too, or whether it needs to be per-source.
