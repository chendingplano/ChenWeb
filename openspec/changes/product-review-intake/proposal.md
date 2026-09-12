## Why

Product Metric Reviewer's review-viewing page (from the committed `product-metric-reviewer`
change) has no way for a user to start a review themselves: creating a profile, building it,
marking it ready, and starting a run are four separate backend calls with no UI in front of
them, and a knowledge-base operator has to chain them by hand (documented as a known boundary
in the user manual). The app also isn't reachable from the Applications menu at all — both its
existing pages are direct-URL-only. Without a self-service start flow and nav entry, the app
can't be used end-to-end by anyone but an operator with API access.

## What Changes

- New "Product Review" start page (form: product name, short description, keywords, notes, and
  a Start button) that lets a user kick off a review without operator involvement.
- New backend duplicate-detection: before creating a new profile, look up whether a profile for
  the same product name already exists. If one does, the start flow surfaces it to the user as a
  choice — view the existing results, or re-run — instead of silently creating a second profile
  for the same product.
- New backend orchestration that chains create-profile → build → ready → create-review into a
  single request, so one "Start" click produces a run the frontend can navigate straight to
  (reusing the existing run-viewer page at `routes/home3/product-metric-review`).
- Extend the product profile with `keywords` and `notes` fields captured from the form. `notes`
  is carried into the review run (the run-create request already accepts `notes`); `keywords` is
  stored as metadata for future search/display and is not sent to the LLM proposer, which
  continues to consume only product name + description.
- Register "Product Review" under the Applications menu in `nav-rail.svelte` /
  `content-panel.svelte`, covering both the new start page and the existing run-viewer page —
  neither is registered there today.

## Capabilities

### New Capabilities
- `product-review-intake`: the self-service start flow end to end — the intake form, the
  duplicate-product check and its view-existing-vs-re-run choice, and the orchestrated
  create-and-run request that returns a navigable run.
- `product-review-nav-entry`: Applications-menu registration for the Product Review app (both
  the intake page and the existing run-viewer page).

### Modified Capabilities
(none — the profile/run capabilities from `product-metric-reviewer` haven't been promoted to
`openspec/specs/` yet since that change isn't archived; the `keywords`/`notes` field additions
to the profile are covered as part of `product-review-intake` above rather than as a delta
against an unregistered spec.)

## Impact

- **Backend** (`server/api/product-reviews/`): new `Store` lookup for an existing profile by
  normalized product name; a new handler/route that orchestrates create→build→ready→create-review
  as one call; a migration adding `keywords`/`notes` columns to `kb.product_profiles`.
- **Frontend** (`web/src/`): new `routes/home3/product-review/+page.svelte` +
  `lib/components/home3/product-review-intake-view.svelte`; extends
  `productMetricReviewService.ts` with the new start/lookup calls; edits to `nav-rail.svelte`
  and `content-panel.svelte` for the Applications entry.
- **No changes** to the existing run-viewer page, retrieval pipeline, or run persistence — those
  already work and are reused as-is.
