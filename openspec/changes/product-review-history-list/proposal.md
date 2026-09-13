## Why

The Product Review intake page (`product-review-intake` change) only has a blank form: every
review is either brand-new or discovered by accident, by retyping a product name and hitting the
duplicate-detection path. A user who wants to re-run last week's "Ventilator" review, or just
see what's already been reviewed, has no way to browse past reviews from this page — they'd have
to remember the exact product name, character for character, and retype it.

## What Changes

- New backend endpoint, `GET /kb/product-profiles`, listing product profiles (most recently
  updated first) with enough data per profile — name, description, keywords, notes, status, and
  the profile's latest request/run summary — to render a card and to drive a re-run with no
  further round trips.
- New "Past Reviews" card list on the intake page, below the form, one card per profile: product
  name, description snippet, keyword chips, and the latest run's status + relative time (or "never
  run" if the profile has no request yet).
- Clicking a card populates the name/description/keywords/notes fields in the form above (the
  form stays visible and editable — this is distinct from the existing full-screen duplicate-name
  hero) and relabels the submit button "Re-Run". Submitting re-runs that profile, reusing the
  existing resume logic (`rerunReview` if a request/run already exists, otherwise
  `startProductReviewIntake` with `resume_profile_id` if the profile was created but never run) —
  the same two branches `handleRerun()` already implements for the duplicate-name path.
- Editing the "Product name" field away from the selected card's name drops the selection and
  reverts the button to "Start", so a "Re-Run" can never fire against a profile the visible name
  no longer matches.

## Capabilities

### New Capabilities
- `product-review-history-list`: listing past product-review profiles as cards on the intake
  page and resuming a review by selecting one — the list endpoint, the card UI, the
  select-to-prefill behavior, and the Start/Re-Run button relabeling.

### Modified Capabilities
(none — `product-review-intake` hasn't been promoted to `openspec/specs/` yet since that change
isn't archived. This change reuses its `handleRerun` resume logic as-is and adds new,
additive UI/state around it; it doesn't change that capability's existing requirements.)

## Impact

- **Backend** (`server/api/product-reviews/`): new `Store.ListProfiles` (or equivalent) query
  joining each profile to its latest request/run; new `ListProductProfiles` handler; new route
  registered in `server/api/routes.go` next to the other `/kb/product-profiles` routes.
- **Frontend** (`web/src/`): new `listProfiles()` call + response type in
  `productMetricReviewService.ts`; new card-list markup, selection state, and button-label logic
  in `product-review-intake-view.svelte`. No changes to the existing duplicate-name hero flow.
- **No changes** to the run-viewer page, retrieval pipeline, or run persistence.
