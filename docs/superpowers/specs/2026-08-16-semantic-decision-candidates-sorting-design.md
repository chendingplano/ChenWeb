# Semantic Decision Candidates Sorting

## Goal

Make the `Identity`, `Kind`, `Method`, `Source`, `Confidence`, `Status`,
`Resolution`, and `Modified` columns in the Semantic Decision Candidates admin
table sortable across all filtered candidates, while preserving server-side
pagination.

## Design

The list API accepts `sort_by` and `sort_dir` query parameters. The store maps
the eight UI sort keys to a fixed SQL column expression and accepts only `asc`
or `desc`; unsupported values fall back to the existing `id DESC` ordering.
The generated `ORDER BY` places nulls last and appends `id` in the same
direction as a deterministic tie-breaker.

The Svelte client tracks the active sort key and direction, sends both values
with every list request, resets to page 1 when a header is activated, and
renders each sortable header as an accessible button with a direction marker.
The existing default order remains unchanged until a sortable header is
clicked.

## Data flow

Header click → client sort state → API query parameters → validated store
ordering → paginated response. Filters and pagination continue to use the
existing request and response shape.

## Validation

Add focused store coverage for the whitelist, direction handling, null ordering,
and stable tie-breaking. Run the relevant Go tests and the web type/build check.
