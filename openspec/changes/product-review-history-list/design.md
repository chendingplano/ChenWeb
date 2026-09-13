## Context

The intake page (`product-review-intake-view.svelte`) already has a fully-working "resume an
existing profile" path, but it's only reachable by accident: a user has to retype the *exact*
normalized product name to trigger the backend's `FindProfileByName` duplicate check, which then
takes over the whole screen with a hero ("View results" / "Re-run") instead of leaving the form
usable. There is no way to browse what's already been reviewed, and no endpoint that lists
profiles at all — `GET /kb/product-reviews` (`ListProductReviews` → `RunStore.ListRequests`)
lists review *requests*, not profiles, and a `Request` only carries `profile_id` (int), not the
profile's name/description/keywords/notes needed to render a card or refill the form.

## Goals / Non-Goals

**Goals:**
- Let a user browse past reviews as cards without knowing/retyping the product name.
- Selecting a card refills the form (name, description, keywords, notes) and relabels the submit
  button "Re-Run", while keeping the form itself visible and editable — not the existing
  full-screen duplicate hero.
- Submitting from a selected card reuses the exact resume branches `handleRerun()` already
  implements (rerun the latest request if one exists, else resume-build a never-run draft
  profile) — no new backend rerun semantics.
- One new read endpoint, sized for this app's actual data volume (a knowledge-base team's product
  catalog — dozens to low hundreds of profiles, not a public-scale table).

**Non-Goals:**
- No search/filter/pagination UI on the card list in this change — a single bounded "most
  recently updated N" list is enough for the current data volume. (Backend still takes a `limit`
  param so this isn't a dead end — see Decision 1.)
- No change to the existing duplicate-name hero flow (`duplicateProfile` state) — it still fires
  exactly as it does today when a user types a name that already exists. That flow and the new
  card-select flow are independent; nothing here removes it.
- No change to the review pipeline, run-viewer page, or run persistence.
- No fuzzy matching or dedup logic beyond what `product-review-intake` already has.

## Decisions

**1. New endpoint `GET /kb/product-profiles?limit=` returns profile summaries, newest-updated
first, each carrying its latest request/run inline.**
`Store` gains `ListProfiles(ctx, tenantID string, limit int) ([]ProfileSummary, error)`:
```sql
SELECT p.id, p.tenant_id, p.name, p.product_description, p.keywords, p.notes, p.status,
       p.updated_at, r.id AS latest_request_id, ru.id AS latest_run_id, ru.status AS latest_run_status,
       ru.finished_at AS latest_run_finished_at
FROM kb.product_profiles p
LEFT JOIN LATERAL (
  SELECT id FROM kb.product_review_requests WHERE profile_id = p.id ORDER BY created_at DESC LIMIT 1
) r ON true
LEFT JOIN LATERAL (
  SELECT id, status, finished_at FROM kb.product_review_runs
  WHERE request_id = r.id ORDER BY run_number DESC LIMIT 1
) ru ON true
WHERE p.tenant_id = $1
ORDER BY p.updated_at DESC
LIMIT $2
```
One query, no N+1 profile-then-run fetches from the frontend. Default `limit` 50 (the intake
page only needs "recent," not "all"); handler clamps to a sane max (e.g. 200) if a caller passes
something larger.
*Alternative considered:* reuse `RunStore.ListRequests(0)` (all requests) and have the frontend
resolve each to its profile. Rejected — that's one request row per *run attempt*, not per
product, so the same product re-run three times would show three cards instead of one, and it
still requires an extra `getProfile` call per row for name/description/keywords.
*Alternative considered:* full-text/keyword search or filter params on this endpoint now.
Rejected — no UI need for it yet (Non-Goals); the `limit` param and a plain `ORDER BY updated_at`
leave room to add `?q=`/cursor pagination later without breaking this shape.

**2. Selecting a card sets new, separate frontend state — it does not reuse `duplicateProfile`.**
New state: `selectedProfile: ProfileSummary | null`. Selecting a card sets it and copies
`name`/`description`/`keywordsInput`/`notes` into the existing form-bound `$state` variables (the
same variables the plain form already uses) — no new input markup, no hero takeover.
`duplicateProfile` (the existing typed-a-duplicate-name path) is untouched and can still fire
independently (e.g. a user selects a card, then changes the name to a *third*, already-existing
product name — normal `start()` runs and the existing duplicate check still applies).
*Alternative considered:* drive card-select through the existing `duplicateProfile` hero UI
(setting `duplicateProfile` on click). Rejected — the proposal explicitly asks for the plain form
to stay visible and editable, not the hero screen; conflating the two would also make "hero
takeover" fire on every card click, which reads as a much heavier interaction than "click a card,
tweak a field, resubmit."

**3. Submit branches on `selectedProfile`, reusing the two existing resume calls.**
`start()` becomes: if `selectedProfile` is set, do what `handleRerun()` already does today (call
`rerunReview(selectedProfile.latest_request_id)` if both a request and run exist, else
`startProductReviewIntake({ name, resume_profile_id: selectedProfile.id, notes })`); otherwise run
the existing plain-create path unchanged. This is a refactor of `handleRerun`'s body into a
shared helper called from both the duplicate-hero button and the new Re-Run button, not new
resume logic.

**4. Editing the "Product name" field away from the selected card's name clears the selection.**
A small `$effect`/`oninput` check: if `selectedProfile` is set and the live `name` no longer
equals `selectedProfile.name` (trimmed), clear `selectedProfile` and revert the button to "Start."
This guarantees "Re-Run" can never fire against a profile the visible name has drifted from.
*Alternative considered:* let the name diverge and still call rerun against the originally
selected profile id. Rejected — silently re-running a differently-named profile because of a
stale selection is a correctness footgun the proposal specifically calls out to avoid.
*Alternative considered:* disable the name field entirely once a card is selected (force
"Start Over" to edit it). Rejected — heavier-handed than needed; description/keywords/notes are
still meant to be editable pre-resume, and clearing on name-edit is enough to keep the label
honest.

**5. Card list renders unconditionally below the form (not gated behind the duplicate hero).**
It fetches `listProfiles()` once when the (non-embedded and embedded) view mounts, shows a compact
empty state ("No reviews yet") when the list is empty, and otherwise renders each summary as a
card: name, a one-line description clamp, keyword chips (from `keywords`), and a small status
line ("Completed 2h ago" / "Never run" / "Running…" / "Failed") derived from
`latest_run_status`/`latest_run_finished_at`. Clicking anywhere on a card (except none — no nested
interactive elements planned) selects it per Decision 2.

## Risks / Trade-offs

- **[Risk] The list can grow stale relative to a review just started in the same session** (a
  fresh "Start" won't appear in the list until the page/list is refetched). → Mitigation: refetch
  `listProfiles()` after a successful `start()`/re-run navigation would only matter if the user
  stayed on the page, but the existing flow always navigates away on success (`goToRun`), so
  there's no stale-list state visible to a user in practice.
- **[Risk] `LEFT JOIN LATERAL` per profile could get slow at large N.** → Mitigation: bounded by
  `limit` (default 50, clamped max) and this table's realistic size (a curated product catalog,
  not user-generated content at scale); revisit with an index or materialized summary if this
  page's data volume changes materially.
- **[Risk] Selecting a card, then editing description/keywords/notes, then submitting "Re-Run"
  silently discards those edits** because the resume path (`resume_profile_id`) only ever reuses
  the stored profile's existing description/keywords — it doesn't accept edited values (see
  `product-review-intake`'s Decision 4: the resume path resumes the profile row as-is). →
  Mitigation: out of scope for this change (changing what the resume endpoint accepts is a
  `product-review-intake`-capability change, not this one); the description/keywords fields
  remain visibly editable because they're the same shared fields the plain-create path uses, but
  this change does not promise those edits are persisted when re-running. `notes` is the one
  field the existing resume call does forward, so notes edits do take effect.

## Open Questions

- **Resolved:** keep this endpoint additive-only (no filters/pagination) since Non-Goals excludes
  search/filter UI in this change; `limit` alone is enough to keep the query bounded.
