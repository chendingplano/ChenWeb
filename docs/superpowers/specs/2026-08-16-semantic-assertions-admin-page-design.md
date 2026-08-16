# Semantic Assertions Admin Page Design

## Goal

Add a full-management admin page for `kb.semantic_assertions` under
Development → System Admin → Doc Process Pipeline → Semantic Assertions,
following the existing Semantic Decision Candidates page.

## Design

Add a dedicated paginated API for assertions that supports filtered and sorted
listing, row detail retrieval, revision creation, lifecycle transitions, and
defer/retry actions. The API will use `AssertionStore` and preserve the
revisioned model: edits create a new revision rather than mutating an existing
assertion payload.

The frontend will consist of a typed client and a sibling Svelte admin view.
It will reuse the candidate page's visual language and interaction patterns:
filters, pagination, sortable columns, refresh, a create/revision form, row
details, lifecycle controls, defer/retry controls, and clear success/error
feedback. The primary fields shown will identify the logical assertion,
subject, predicate, object, status, confidence, revision, and modification
time. Details will expose the full assertion record, including structured
literal and qualifier JSON where present.

The navigation will add `Semantic Assertions` as a sibling of `Semantic
Decision Candidates` under `Doc Process Pipeline`. No generic table framework,
deletion operation, or unrelated refactor is included; governed assertion
history must remain recoverable through revisions.

## Verification

- Add focused backend tests for list filtering/pagination and admin handlers.
- Run the relevant Go tests and frontend type/build checks provided by ChenWeb.
- Confirm the navigation item resolves to the new view and the API paths match
  the typed client.
