## Context

`server/api/product-reviews/` (package `productreviews`) already implements the full review
pipeline as four separate, synchronous, single-purpose endpoints:

1. `POST /kb/product-profiles` — `Store.CreateProfile` inserts a draft `kb.product_profiles` row
   + root node (`store.go:77-117`).
2. `POST /kb/product-profiles/:id/build` — `Builder.Build` runs the LLM proposer, grounding, and
   graph-expansion synchronously and returns when done (`handler.go:142-165`).
3. `POST /kb/product-profiles/:id/ready` — `Store.SetProfileStatus` flips `draft` → `ready`
   (`handler.go:245-`, `store.go:195-`).
4. `POST /kb/product-reviews` — `RunController.StartReview` requires `Status == ProfileReady`
   (else `ErrDraftProfile`/`CWB_KB_PMR_030`), then runs `executeRun` synchronously: retrieval,
   `Runs.SaveScopedDocs`, `Runs.SaveResults`, `Runs.CompleteRun` (`run_controller.go:34-156`).

Nothing chains these, nothing checks whether a profile for the given product name already
exists, and neither this page nor its sibling `ontology-metric-analysis` is reachable from the
Applications nav — both are direct-URL-only (`nav-rail.svelte`, `content-panel.svelte`). This
change adds the missing self-service entry point on top of the existing, already-working
pipeline; it does not touch the pipeline's internals.

## Goals / Non-Goals

**Goals:**
- One user action ("Start") produces a completed run, reusing steps 1–4 above unchanged.
- Re-submitting the same product name surfaces the existing profile/run instead of silently
  creating a duplicate.
- The app becomes reachable from Applications → Product Review.
- Capture `keywords` and `notes` from the form and persist them on the profile.

**Non-Goals:**
- No change to the proposer, grounder, expander, retrieval, tiering, or report logic.
- No background job queue / async status polling. The pipeline is already synchronous end to
  end (an LLM call plus SQL work); the new orchestration endpoint stays synchronous too, matching
  the existing style. A future change can make it async if run times become a UX problem.
- No fuzzy/semantic duplicate matching. Exact, normalized name match only.
- No change to the existing run-viewer page, its filters, evidence drawer, or report tab.

## Decisions

**1. Duplicate check = normalized exact name match, scoped to tenant.**
`Store` gains `FindProfileByName(ctx, tenantID, name string) (*Profile, error)`, matching on
`lower(trim(name)) = lower(trim($1))`. This is a new SQL query in `store.go`, not a new table.
*Alternative considered:* embedding/semantic similarity search (e.g. against
`kb.product_profile_nodes.embedding`) to catch near-duplicates like "Ventilator" vs "ICU
Ventilator Model X". Rejected for v1 — the user manual already documents "one product family per
profile" as an intentional constraint, so exact-name reuse (a user re-submitting the same
product) is the case that matters; fuzzy matching is a bigger, separate design problem
(threshold tuning, false positives) better deferred.

**2. One new orchestration endpoint, `POST /kb/product-reviews/intake`, calling existing pieces
in-process.** The handler: (a) calls `FindProfileByName`; if found, skips straight to the
duplicate response ((3) below) without creating anything; (b) otherwise calls
`Store.CreateProfile` (now taking `Keywords`/`Notes` too), `Builder.Build`, `Store.SetProfileStatus(ready)`,
then `RunController.StartReview`, in sequence, in the same request/goroutine.
*Alternative considered:* have the frontend call the four existing endpoints in sequence itself.
Rejected — it triples the network round trips for no benefit, and duplicates
sequencing/error-handling logic (what to do if build succeeds but ready fails, etc.) in the
client instead of once on the server. A dedicated endpoint also gives one clean place to run the
duplicate check before anything is created.
*Alternative considered:* extend `CreateProductProfile` itself to optionally cascade through
build/ready/run via a query flag. Rejected — it would overload one endpoint with two very
different contracts (an idempotent-ish "just create a draft" call vs. a heavyweight "do
everything" call) and complicate its existing callers/tests.

**3. Duplicate response shape carries enough for the frontend to offer both choices without
another round trip.** When `FindProfileByName` hits, the handler also calls
`Runs.ListRequests(profileID)` and, for the most recent request, `Runs.ListRuns(requestID)` to
find the latest run (if any). Response:
```json
{
  "status": true,
  "duplicate": true,
  "profile": { "id": ..., "name": ..., "status": "ready" },
  "latest_request_id": 123,
  "latest_run": { "id": 456, "status": "completed" }
}
```
- If `latest_run` is present and `status == "completed"`: frontend offers "View results" (navigate
  to `/home3/product-metric-review?run=456`) and "Re-run" (`POST
  /kb/product-reviews/:id/rerun` with `latest_request_id` — the existing endpoint, unchanged).
- If there is no run yet, or the profile is still `draft` (an earlier intake attempt failed
  mid-pipeline): frontend offers only "Re-run", which calls the intake endpoint with an optional
  `resume_profile_id` set to that profile's id. When present, the handler skips the
  `FindProfileByName` lookup entirely and drives build → ready → start-review against that
  specific profile id instead of creating a new one. (Without this, "Re-run" on a draft profile
  would call the same name-based lookup, find the same draft profile, and loop back to the
  duplicate response instead of ever progressing it.)

**4. `keywords`/`notes` are plain stored metadata, not proposer input.** New nullable columns
`kb.product_profiles.keywords` (`text[]` or a comma-joined `text` — see Open Questions) and
`.notes` (`text`). The `Proposer` continues to read only `product_name` + `product_description`
(+ optional `seed_excerpts`) — unchanged (`propose.go:129-146`). `notes` from the form is also
passed through as the review request's existing `notes` field (`CreateProductReview` already
accepts one, `handler.go:283-312`), so it shows up on the run/request the same way a manually
triggered review's notes would. `keywords` has no consumer yet beyond storage/display; it's
captured now because the form asks for it, per the request.
*Alternative considered:* feed keywords into the LLM proposer prompt as extra hints. Rejected —
out of scope per the proposal, and would require prompt changes to
`prompt-product-structure-v1.md`, which the request didn't ask for.

**5. Nav entry follows the existing `window.open` pattern used for other home3 apps, not the
embedded `content-panel.svelte` pattern.** Add `{ id: 'apps-product-review', label: 'Product
Review' }` to the `applications.children` array in `nav-rail.svelte` (~line 139-151), and a
`child?.id === 'apps-product-review'` branch in `selectItem` (~line 464-484) that does
`window.open('/home3/product-review?dark=...', '_blank', 'noopener')` — the same treatment
`kb-metrics`, `kb-chunks`, etc. already get. No `content-panel.svelte` change.
*Alternative considered:* embed the intake form as an in-dashboard panel via
`content-panel.svelte`. Rejected — the existing run-viewer page it hands off to is already a
standalone routed page (it needs its own URL for `?run=<id>` deep links), so a standalone intake
page keeps both halves of the app consistent and reuses the simpler, already-proven nav pattern.

**6. Intake page is a new route, the viewer route is untouched.** New
`routes/home3/product-review/+page.svelte` + `lib/components/home3/product-review-intake-view.svelte`
(form + duplicate-choice UI). On successful (non-duplicate) intake, navigate to
`/home3/product-metric-review?run=<id>&dark=...`. On duplicate-with-run, same navigation (View
results) or call rerun then navigate. `product-metric-review-view.svelte` is not modified.

## Risks / Trade-offs

- **[Risk] The orchestration endpoint's request can take tens of seconds (LLM proposer call +
  grounding + retrieval) with the caller blocked the whole time.** → Mitigation: this is no worse
  than today's `BuildProductProfile` call, which already blocks on the same LLM step; the intake
  view shows a loading state and the Echo server's existing request timeout (if any) applies
  unchanged. No new timeout handling introduced.
- **[Risk] Partial failure mid-orchestration** (e.g. profile created, build fails) **leaves a
  `draft` profile with no run.** → Mitigation: this already happens today if an operator's manual
  build step fails; the existing "failure leaves the request re-runnable" behavior
  (`run_controller.go`) is unchanged. The duplicate-check path (Decision 3) surfaces such a
  half-finished profile on the next attempt with the same name rather than losing track of it.
- **[Risk] Exact-name matching means "Ventilator" and "ventilator " (trailing space) collide but
  "Ventilator" and "Ventilator Model X" don't**, so a user who slightly varies the name gets a
  fresh duplicate profile instead of a nudge. → Mitigation: accepted for v1 per Decision 1;
  revisit if this proves to be a real problem in practice.

## Open Questions

- **`keywords` storage shape**: Postgres `text[]` (queryable, matches the "for search" framing)
  vs. a single `text` column with comma/space-joined values (simpler, matches how `notes` is
  already a plain `text` field elsewhere in this package). Leaning `text[]` since the proposal
  explicitly says "for search," but no search UI consumes it yet in this change — confirm during
  implementation which is less friction given the existing migration/model conventions in
  `server/api/product-reviews/`.
- **Resolved:** re-running a `draft` (never-completed) profile resumes the same profile row via
  `resume_profile_id` (see Decision 3) rather than deleting and recreating it — deleting would
  need a new `Store.DeleteProfile` method that nothing else needs, whereas resuming reuses the
  same build → ready → start-review sequence the fresh-profile path already calls.
