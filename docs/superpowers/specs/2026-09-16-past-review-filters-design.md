# Past Reviews Sorting and Filtering

## Context

The Product Review intake page already lists tenant-scoped `kb.product_profiles` as Past reviews
cards. The list is currently ordered by recent update and has no controls. Users need to sort the
entire returned table, select one or more profile keywords with OR semantics, and search product
names with a case-insensitive substring match.

## Goals

- Add the six requested sort modes:
  - By time ASC / DESC
  - By product name ASC / DESC
  - By metrics ASC / DESC
- Add a multi-select keyword filter whose options are all distinct keywords in the tenant's
  `kb.product_profiles` rows.
- Add a product-name filter using SQL `ILIKE` substring matching.
- Preserve current card selection, re-run, and View results behavior.
- Keep filtering and sorting server-side so ordering applies before the bounded result set is
  returned.

## Non-goals

- No pagination redesign.
- No changes to review execution or profile persistence.
- No AND semantics between selected keywords; selected keywords always match as OR.
- No fuzzy search beyond `ILIKE '%query%'`.

## Design

### Backend API

Extend `GET /api/v1/kb/product-profiles` with optional query parameters:

- `sort`: one of `time_asc`, `time_desc`, `name_asc`, `name_desc`, `metrics_asc`,
  `metrics_desc`; invalid or absent values use `time_desc`.
- `keywords`: repeated query parameters or a comma-separated list of selected keywords. The
  handler normalizes whitespace, removes duplicates, and ignores empty values.
- `name`: a free-text product-name filter.
- `limit`: retain the existing default/max behavior.

The response remains `{ status: true, profiles: [...] }` and adds a tenant-scoped
`keywords: string[]` vocabulary containing all distinct non-empty keyword strings from
`kb.product_profiles`. This avoids a separate vocabulary request.

The profile query uses a whitelist-selected `ORDER BY`, `ILIKE` for the name filter, and an
`EXISTS` predicate over `jsonb_array_elements_text(p.keywords)` for keyword OR matching. Metrics
use the existing latest-run metric count. Null metric counts sort consistently after real counts
in ascending mode and before real counts in descending mode. Every order includes `id` as a stable
tie-breaker. Filtering and ordering occur before `LIMIT`.

The keyword vocabulary query is tenant-scoped, deduplicated, and alphabetically ordered. Query
values remain SQL parameters; only the sort expression is selected from a fixed server-side map.

### Frontend

Add a compact single-row toolbar immediately under the Past reviews heading:

1. A native sort pulldown with the six user-facing labels.
2. A keyword multi-select control that displays selected values as removable chips and exposes the
   vocabulary returned by the API.
3. A `Filter by name` text input.

Sort and keyword changes reload immediately. Name changes reload after a short debounce. Loading
state is scoped to the list, and stale responses are ignored so a slower earlier request cannot
overwrite a newer filter result.

The UI sends the current query state through `listProfiles`. It keeps the current no-profiles empty
state and adds a separate no-matches state when filters produce zero cards. Existing cards remain
selectable and preserve their current keyboard behavior and nested View results action.

### Error handling

Invalid sort values fall back to the default. Empty filters are omitted. A failed refresh preserves
the last successfully loaded cards and shows the existing page-level error treatment only if the
current component conventions support it; initial list-load failure remains non-blocking for the
intake form.

## Testing

- Backend unit tests verify sort whitelist mapping/fallback, all six orderings, keyword OR SQL,
  name `ILIKE`, tenant scoping, vocabulary extraction, parameter normalization, and filter-before-
  limit query shape.
- Frontend service tests verify serialization of sort, repeated keywords, name, and limit.
- Component tests verify immediate sort/keyword refresh, debounced name refresh, chip removal,
  empty filtered results, and that card select/re-run behavior remains intact.
- Run existing ChenWeb Go and web checks, plus focused product-review tests.

## Documentation impact

The next Product Metric Reviewer manual revision should document the toolbar, OR keyword behavior,
name substring matching, and all sort choices. The prior manual revisions remain unchanged.

