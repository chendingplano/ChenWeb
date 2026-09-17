## Context

`ListVideos` (`server/api/videohandler/handler.go:389`) currently runs a fixed
`SELECT ... FROM kb.videos ORDER BY created_at DESC, id DESC` with no query
params, and `videoService.ts`'s `listVideos()` calls `GET /api/v1/videos` with
no arguments and returns a bare `VideoMeta[]`. The table component
(`video-management-view.svelte`) just renders whatever comes back.

The codebase already has an established pattern for server-side sort+filter
on an admin list: `ListSemanticAssertions` →
`server/api/ontology/assertions/assertions_store.go` `ListAdmin`, which builds
a parameterized `WHERE` clause, maps `SortBy` through an allowlist
(`sortColumns map[string]string`) to the real column name (never
interpolating the client-supplied sort key directly into SQL), and appends
`ASC`/`DESC` based on `SortDir`.

## Goals / Non-Goals

**Goals:**
- Sort the entire `kb.videos` table by name, upload time, or size, ascending
  or descending, via one `sort_by`/`sort_dir` param pair.
- Filter the entire table by a name substring (`ILIKE`) and/or an upload-time
  range (`time_from`/`time_to`), combinable with sort and with each other.
- Reuse the assertions-store allowlist/parameterized-query pattern so the sort
  column can never be attacker-controlled SQL.

**Non-Goals:**
- Pagination / `LIMIT`+`OFFSET` — not requested; the table already renders
  its full (currently small) result set, and adding `{results, total, page,
  page_size}` would be a breaking response-shape change to
  `videoService.ts` for no requested benefit. Can be added later as its own
  change if the video count grows.
- Filtering by source/category/status — out of scope; only Name and Time
  filters were requested.
- Client-side-only filtering (rejected as an approach) — the request is
  explicit that sort/filter apply to "the entire table", i.e. the full
  database result set, not just whatever page of rows happens to be loaded.

## Decisions

1. **Query params on the existing endpoint, not a new one.**
   `GET /api/v1/videos` gains optional `sort_by`, `sort_dir`, `name`,
   `time_from`, `time_to` params. Omitting all of them reproduces today's
   behavior exactly (`ORDER BY created_at DESC, id DESC`), so this is
   backward compatible.

2. **Sort allowlist map**, mirroring `assertions_store.go`:
   ```go
   sortColumns := map[string]string{
       "name":       "COALESCE(name, filename)",
       "created_at": "created_at",
       "size_bytes": "size_bytes",
   }
   ```
   `sort_by` not in the map (or empty) falls back to `created_at`; `sort_dir`
   anything other than case-insensitive `asc` becomes `DESC`. A stable
   secondary key (`, id <dir>`) is kept to match the existing tie-break
   behavior.

3. **Name filter** is `ILIKE '%<name>%'` against `COALESCE(name, filename)`
   (the same expression the table already displays), parameterized —
   consistent with the assertions store's `logical_identity_key ILIKE $n`.

4. **Time filter** is inclusive `created_at >= $from` / `created_at <= $to`,
   each added to the `WHERE` clause only when present. Dates come from the
   frontend as ISO date strings (`YYYY-MM-DD`); the backend parses/validates
   them and rejects malformed input with 400, rather than silently ignoring
   it.

5. **Frontend controls row**: a new row above the table in
   `video-management-view.svelte` with a `<select>` for Sort (6 fixed
   options mapping to `sort_by`+`sort_dir` pairs), a text `<input>` for Filter
   by Name, and two `<input type="date">` fields for Filter by Time, plus an
   Apply action — mirroring the filter-panel + "Apply Filters" affordance in
   `semantic-assertions-view.svelte` rather than the assertions view's
   separate clickable-column-header sort (a single dropdown was what was
   requested, not per-column click-to-sort).

6. **Response shape unchanged** (`VideoMeta[]`) — see Non-Goals. Only
   `listVideos()`'s signature grows an optional options argument
   (`{sortBy?, sortDir?, name?, timeFrom?, timeTo?}`) that gets serialized
   into `URLSearchParams`.

## Risks / Trade-offs

- **No pagination** → if the video table grows very large, "sort/filter the
  entire table" still returns every matching row in one response. Acceptable
  now given current row counts (tens of videos); revisit if this becomes a
  problem.
- **Date-only time filter** (no time-of-day) → a video uploaded at 23:59 on
  the end date is included by using `< time_to + 1 day` (or an inclusive
  `<=` against a timestamp normalized to end-of-day) rather than a naive
  string compare that could exclude same-day uploads after midnight.
  Mitigation: backend normalizes `time_to` to the end of that calendar day
  before comparing.
- **Invalid date input** → mitigated by explicit 400 validation instead of
  silently dropping the filter (which could surprise the admin with an
  unfiltered result set).

## Migration Plan

No data migration. Deploy is a normal code change: backend handler update,
frontend component + service update, shipped together (the frontend now
depends on the new query params being accepted, though the backend tolerates
their absence for backward compatibility during rollout).
