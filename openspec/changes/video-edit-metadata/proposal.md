## Why

The Videos manager (SYSTEM ADMIN → Resources → Videos) lets an admin upload and delete a video, but a mistake in the metadata entered at upload time (name, description, category, keywords, status, cover image, etc.) can only be fixed by deleting the video and re-uploading the file. There is no update path today — the API exposes only `POST /videos` (create), `GET /videos` (list), `GET /videos/:id/stream|download`, and `DELETE /videos/:id`.

## What Changes

- Add an **Edit** action to each row's Actions column (alongside View / Download / Delete) in `video-management-view.svelte`.
- Clicking Edit opens the existing upload/edit dialog in **edit mode**: fields are pre-filled from the selected video's current metadata (name, description, source, url, keywords, category, subcategory, container, status, notes, video_type, cover image). The video file input is shown but **optional** in this mode — a chosen file replaces the stored video; leaving it empty keeps the current file.
- Add `PATCH /api/v1/videos/:id` to `server/api/videohandler/handler.go`: updates the metadata columns (same validation rules as upload — `source` ∈ {Recording, Web}, `url` required when Web, `status` ∈ {draft, published, archived}), and, when a `file` part is present, saves it (same type/size validation as upload), updates `filename`/`stored_path`/`size_bytes`/`content_type`, and removes the old on-disk file after the DB update succeeds. `uploaded_by` is untouched either way.
- Add `updateVideo()` to `web/src/lib/services/videoService.ts` calling the new endpoint.
- Route registered in `server/api/routes.go` next to the other `/videos` routes.

## Capabilities

### New Capabilities
- `video-metadata-editing`: an authenticated `PATCH /api/v1/videos/:id` endpoint that updates a video's metadata fields and, optionally, replaces its stored file, plus an Edit affordance in the video manager UI that opens the upload dialog pre-filled in an edit mode to submit those changes.

### Modified Capabilities
(none — no existing `video-management` spec file to amend; the upload dialog component is reused, not spec-changed)

## Impact

- **Backend**: `server/api/videohandler/handler.go` (new `UpdateVideo` handler), `server/api/routes.go` (new route).
- **Frontend**: `web/src/lib/services/videoService.ts` (new `updateVideo()`), `web/src/lib/components/home3/video-management-view.svelte` (Edit button, dialog edit-mode, pre-fill, submit branch).
- **No DB migration** — no new columns, only updating existing ones.
- **No change** to upload, list, stream, download, or delete behavior.
- Replacing a file writes the new file to disk before the DB update commits, and only removes the old file after the DB update succeeds — a failed update never leaves the video pointing at a missing file.
