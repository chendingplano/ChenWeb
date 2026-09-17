## Why

The Resources > Videos admin table (`video-management-view.svelte`) always lists
videos in a fixed `created_at DESC` order with no way to search or reorder them.
As the training-video library grows, admins need to find a specific video by name
or by upload date, and reorder the table by name, upload time, or file size.

## What Changes

- Add a controls row above the video table with:
  - A **Sort** dropdown: By Name ASC/DESC, By Time ASC/DESC, By Size ASC/DESC.
  - A **Filter by Name** text input (substring/`ILIKE` match against video name).
  - A **Filter by Time** range control (start date / end date, filtering on
    upload time).
- Sort and filter apply to the entire `kb.videos` table server-side (not just
  the currently-loaded rows), by adding `sort_by`, `sort_dir`, `name`,
  `time_from`, `time_to` query params to `GET /api/v1/videos`.
- `ListVideos` (`server/api/videohandler/handler.go`) builds a parameterized
  `WHERE`/`ORDER BY` from an allowlisted column map, mirroring the pattern
  already used by `ListSemanticAssertions` /
  `server/api/ontology/assertions/assertions_store.go`.
- `videoService.ts`'s `listVideos()` gains optional sort/filter parameters and
  forwards them as query params. The response stays a bare array — no
  pagination is introduced, since the request only asks for sort + filter over
  the whole table.

## Capabilities

### New Capabilities
- `video-list-management`: sorting and filtering behavior for the Resources >
  Videos admin list (sort keys, filter semantics, query contract between
  frontend and `GET /api/v1/videos`).

### Modified Capabilities
(none — no existing spec covers the Videos admin list)

## Impact

- Frontend: `web/src/lib/components/home3/video-management-view.svelte`,
  `web/src/lib/services/videoService.ts`.
- Backend: `server/api/videohandler/handler.go` (`ListVideos`), no route
  signature change (`GET /api/v1/videos` in `server/api/routes.go` stays the
  same, just gains query params).
- Database: read-only query change against `kb.videos`; no schema/migration
  needed (uses existing `name`, `size_bytes`, `created_at` columns).
