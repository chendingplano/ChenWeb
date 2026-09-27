## 1. Backend: query params on `GET /api/v1/videos`

- [x] 1.1 In `server/api/videohandler/handler.go`, extend `ListVideos` to read
      repeated `sort_by`/`sort_dir` pairs plus `name`, `time_from`, and
      `time_to` query params.
- [x] 1.2 Add the sort allowlist map (`name` → `COALESCE(name, filename)`,
      `created_at` → `created_at`, `size_bytes` → `size_bytes`), applying
      valid fields in query order and defaulting to `created_at DESC` when no
      valid fields are supplied, with `id DESC` as a stable tie-break — matching the pattern in
      `server/api/ontology/assertions/assertions_store.go`.
- [x] 1.3 Build a parameterized `WHERE` clause: append
      `COALESCE(name, filename) ILIKE $n` when `name` is non-empty, and
      `created_at >= $n` / `created_at <= $n` (end date normalized to end of
      day) when `time_from`/`time_to` are present.
- [x] 1.4 Validate `time_from`/`time_to` as `YYYY-MM-DD`; return
      `400 (CWB_VID_0xx)` on malformed dates or `time_from > time_to`.
- [x] 1.5 Confirm the no-params request still returns identical results/order
      to current behavior (regression check).

## 2. Frontend: service layer

- [x] 2.1 In `web/src/lib/services/videoService.ts`, add ordered sort
      descriptors to `listVideos()` options, serialized as repeated
      `sort_by`/`sort_dir` query params; retain the legacy single-sort options.
- [x] 2.2 Keep the return type `VideoMeta[]` (no response-shape change).

## 3. Frontend: controls UI

- [x] 3.1 In `web/src/lib/components/home3/video-management-view.svelte`, add
      Sort ASC, Sort DESC, and No Sort menus to the Name, Size, and Uploaded
      headings, plus a Filter by Name text `<input>` and
      Filter by Time start/end `<input type="date">` fields, styled
      consistently with the page's existing dark-theme controls (e.g. the
      "Upload video" button).
- [x] 3.2 Wire state for sort/name/time-range and call the updated
      `listVideos()` with the current selections whenever they change (or on
      an explicit Apply action, per design.md decision 5), re-rendering the
      table from the response.
- [x] 3.3 Show a validation message (not a silent no-op) if the backend
      rejects the time range with a 400.

## 4. Verification

- [x] 4.1 `cd server && go build ./...` and `go vet ./...`.
- [ ] 4.2 Manually verify in `mise dev`: default load order unchanged; each
      of the 6 sort options; name filter narrows correctly
      (case-insensitive); time range filter (start only, end only, both,
      invalid range); combined sort + both filters together.
- [ ] 4.3 Update `openspec/specs/` by archiving this change once merged
      (`ChenWeb:openspec-archive-change`), promoting
      `video-list-management` into the permanent spec set.
