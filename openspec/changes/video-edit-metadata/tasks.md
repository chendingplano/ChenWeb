## 1. Backend: update endpoint

- [x] 1.1 Add `UpdateVideo` handler in `server/api/videohandler/handler.go`: parse `:id`, load the existing row (404 if missing), read the same form fields as `UploadVideo` (name, description, source, url, image_id, keywords, category, subcategory, container, status, notes, video_type) with the same validation (source ∈ {Recording, Web}, url required when Web, status ∈ {draft, published, archived}), `UPDATE kb.videos SET ... WHERE id = $n RETURNING ...` the same column set as `ListVideos`/`UploadVideo`, set `ImageURL` via `imageURLFor`, return the updated `videoMeta` as JSON.
- [x] 1.2 Register `apiGroup.PATCH("/videos/:id", videohandler.UpdateVideo)` in `server/api/routes.go` next to the existing `/videos/:id` routes.
- [x] 1.3 `cd server && go build ./...` and `go vet ./...` to confirm it compiles.

## 2. Frontend: service

- [x] 2.1 Add `updateVideo(id: number, fields: VideoUploadFields, fetchFn = fetch): Promise<VideoMeta>` to `web/src/lib/services/videoService.ts` — PATCH `/api/v1/videos/${id}` with the fields as a URL-encoded or form body (mirroring the field set already sent by `uploadVideo`, minus the file), same `credentials: 'same-origin'`, throw with the response's `error` message on failure (mirror `deleteVideo`'s error handling).

## 3. Frontend: edit mode in the manager view

- [x] 3.1 In `video-management-view.svelte`, add `editingId = $state<number | null>(null)` and rename `openDialog()`'s body into a shared reset, with `openDialog()` (upload) setting `editingId = null` and a new `openEditDialog(video: VideoMeta)` setting `editingId = video.id` and pre-filling `name`, `description`, `source`, `url`, `keywords`, `category`, `subcategory`, `container`, `status`, `notes`, `videoType` from `video`, plus `coverImage` from `video.image_id`/`video.image_url` when present (placeholder `ImageMeta`-shaped object per design.md).
- [x] 3.2 Add an **Edit** button to the Actions cell (between View and Download, matching existing button styling) calling `openEditDialog(video)`.
- [x] 3.3 Rename `submitUpload()` to `submitDialog()`; keep upload validation/behavior for `editingId === null`; add an edit branch (skip the "choose a video file" check, call `updateVideo(editingId, fields)`, set the success `info` message, close dialog, `refresh()`).
- [x] 3.4 In the dialog template: hide/skip the "Video file" input row when `editingId !== null`; change the dialog title to "Edit video" and the submit button label to "Save" in edit mode; wire the submit button's `onclick` to `submitDialog`.
- [ ] 3.5 Manually verify in the browser: edit a video's Name/Status and confirm the table updates and the file/size/uploaded fields are unchanged; cancel an edit and confirm nothing changes; upload still works unaffected.

## 4. Wrap-up

- [x] 4.1 `cd web && npm run check` (or the project's equivalent svelte-check/typecheck command) to confirm no type errors.

## 5. Backend: optional file replacement on update

- [x] 5.1 In `UpdateVideo`, before validating fields, fetch the existing row's `filename`, `stored_path`, `size_bytes`, `content_type` (separate query from `lookupVideo` — don't change that function's signature, it's used by Stream/Download/Delete). 404 if missing.
- [x] 5.2 After the existing source/url/status validation, check for an optional uploaded file via `c.FormFile("file")`. When present: validate with `isAllowedVideo` and `maxVideoBytes()` (same checks as `UploadVideo`), `os.MkdirAll(videoDir())`, save under a fresh unique path (`fmt.Sprintf("%d_%s", time.Now().UnixNano(), sanitizeFilename(header.Filename))`), and remember the new filename/stored_path/size/content_type plus the old `stored_path` to remove afterward. When absent: reuse the existing filename/stored_path/size_bytes/content_type fetched in 5.1 unchanged.
- [x] 5.3 Extend the `UPDATE kb.videos SET ...` statement to also set `filename`, `stored_path`, `size_bytes`, `content_type` (from 5.2's resolved values) alongside the metadata columns, `RETURNING` the same column set as before.
- [x] 5.4 On `UPDATE` failure when a new file was saved in 5.2: remove the newly-saved file (rollback) before returning the error. On `UPDATE` success when a file was replaced: remove the old file from disk (best-effort — log a failure, don't fail the response since the row is already correctly updated).
- [x] 5.5 `cd server && go build ./... && go vet ./...`.

## 6. Frontend: file replacement in edit mode

- [x] 6.1 Change `updateVideo()` in `videoService.ts` to accept an optional `file: File | null` and `onProgress` callback, and send the request as `XMLHttpRequest` + `FormData` (mirroring `uploadVideo`) with method `PATCH`, instead of `fetch` + `URLSearchParams`. Include `file` in the form only when provided.
- [x] 6.2 In `video-management-view.svelte`, remove the `{#if editingId === null}` guard hiding the "Video file" input — show it in both modes, with the label reading "Replace video file (optional)" when `editingId !== null`. Leave `onDialogFileChosen` as-is (it already only auto-fills `name`/`videoType` when those are empty, which is a no-op in edit mode since they're pre-filled).
- [x] 6.3 In `submitDialog()`'s edit branch, call `updateVideo(editingId, fields, file, (f) => (uploadProgress = f))` instead of the no-file call. Do not add a "file required" check for edit mode.
- [x] 6.4 Update the progress-bar and submit-button-label conditions to also show progress when editing with a chosen file (`uploading && (editingId === null || file)`; Save button shows `Saving… NN%` when `file` is set, plain `Saving…` otherwise).
- [x] 6.5 `cd web && npm run check` to confirm no type errors.

## 7. Verify

- [ ] 7.1 Manually verify in the browser (needs an authenticated session — not run by the agent): edit a video and replace its file — confirm the table's Size column updates and View/Download serve the new file; edit metadata only (no file chosen) — confirm the file/size are unchanged; cancel — confirm nothing changes.

## 8. Frontend polish: file field as name/value + Pick File button

- [x] 8.1 Replace the native `<input type="file">` row's label+control (which rendered as two stacked clickable-looking elements — the label and the browser's own "Choose File" button — and read as confusing) with a name/value pair matching the rest of the form: label "Video File Name" (required-asterisk only in upload mode), a readonly text box showing `file.name` once chosen or `currentFilename` (the video's current stored filename, set in `openEditDialog`/cleared in `openDialog`) otherwise, and a "Pick File" button beside it that calls `dialogFileInput?.click()`. The real `<input type="file">` stays mounted but visually hidden (`display:none`) and is still the thing `onDialogFileChosen` listens on.
- [x] 8.2 `cd web && npm run check` to confirm no type errors.

## 9. Fix: stale cached video after file replacement

- [x] 9.1 Bug found via manual testing: after replacing a video's file through Edit → Save, `kb.videos.stored_path` updates correctly, but clicking View still plays the old video. Root cause: `StreamVideo`/`DownloadVideo` serve `/api/v1/videos/:id/(stream|download)` with a `Last-Modified` header (from `http.ServeContent`) but no `Cache-Control`, so the browser's HTTP cache heuristically treats a previously-fetched response for that URL as still fresh and never re-requests it — the id-based URL never changes across a file replacement. Fix: set `Cache-Control: no-store` on both responses in `server/api/videohandler/handler.go` so every view/download always hits the server and gets the current file.
- [x] 9.2 `cd server && go build ./... && go vet ./...` — confirmed air picked up the rebuild cleanly (`.cache/server.exe` timestamp advanced, `.cache/build-errors.log` empty).
