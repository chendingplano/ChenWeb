## Context

`video-management-view.svelte` already has a full upload dialog (file picker, name/description/source/url, keywords/category/subcategory/container/status/notes/video_type, cover-image picker/auto-generate) wired to `POST /api/v1/videos`. The backend (`server/api/videohandler/handler.go`) has no path to update an existing row — `kb.videos` rows are immutable once created except via `DELETE`.

## Goals / Non-Goals

**Goals:**
- Let an admin fix metadata on an existing video, and optionally swap its underlying file, without deleting and re-creating the row (which would lose its id/history).
- Reuse the existing dialog markup/state instead of building a second form.

**Non-Goals:**
- Editing `uploaded_by` or `created_at` — those stay server-owned regardless of whether the file is replaced.

## Decisions

- **One dialog, two modes.** Add an `editingId: number | null` state var. `openDialog()` (upload) sets it `null`; a new `openEditDialog(video)` sets it to `video.id` and pre-fills every field from `video`. The template already binds all fields to the same `let` vars, so no duplication — the file-input row's label and the submit handler branch on `editingId === null`. Chosen over a separate edit dialog component to avoid maintaining two copies of ~150 lines of form markup for what is otherwise identical.
- **PATCH, not PUT.** `PATCH /api/v1/videos/:id` takes the same form fields as upload plus an *optional* `file` part, and only touches columns implied by what's present — PUT's full-replacement semantics would suggest the file is mandatory, which it isn't here.
- **File replacement is opt-in per request, detected server-side by presence of the `file` part** (`c.FormFile("file")`), not a separate flag — mirrors how the dialog already either has a chosen `File` or doesn't. When present: same type/size validation as `UploadVideo`, save under a fresh unique name, run one `UPDATE` that sets both the metadata columns and `filename`/`stored_path`/`size_bytes`/`content_type`, and only remove the old on-disk file *after* that `UPDATE` commits (best-effort; a failure to remove the old file is logged, not returned as an error, since the row already correctly points at the new one). When absent: the same `UPDATE` runs with the row's existing filename/stored_path/size_bytes/content_type values (fetched up front), so there's one code path and one SQL statement either way.
- **Validation mirrors `UploadVideo`.** `source` ∈ {Recording, Web}, `url` required when Web, `status` ∈ {draft, published, archived}, plus (only when a file is present) the same content-type/extension allowlist and `maxVideoBytes()` check.
- **`updateVideo()` uses `XMLHttpRequest` + `FormData`, like `uploadVideo()`, not `fetch`.** A replacement file needs multipart encoding and benefits from the same upload-progress events `uploadVideo` already reports; using one request shape for both keeps the two functions symmetric instead of forking metadata-only PATCH onto `fetch`/urlencoded and then having to redo it for file support.

## Risks / Trade-offs

- [Risk] Reusing the dialog's `submitUpload` name/shape could get confusing with two code paths. → Mitigation: rename the submit handler to `submitDialog()` and branch inside it (`editingId === null ? upload : update`), rather than growing two near-duplicate functions.
- [Risk] A crash between saving the new file and committing the `UPDATE` would leak an orphaned file on disk. → Mitigation: accepted — same failure mode `UploadVideo` already has (it removes the just-saved file on a failed `INSERT`, but a hard crash mid-request still leaks); no new class of risk, and cleanup of orphaned files is out of scope for both.
- [Risk] The `UPDATE` could fail after the old file was already deleted, losing the video. → Mitigation: ordering is fixed as save-new → update-row → delete-old, never delete-old before the row update commits.
- [Risk] Pre-filling `coverImage` needs an `ImageMeta` shape but the row only carries `image_id`/`image_url`, not the other `ImageMeta` fields (`filename`, `size_bytes`, `content_type`, `origin`, `created_at`). → Mitigation: build a placeholder object with `id`/`content_url` from the video row and empty/zero values for the rest — `video-management-view.svelte` only ever reads `.id` (on submit) and `.content_url` (for the `<img>` preview) off `coverImage`.
- [Risk] `/api/v1/videos/:id/stream` and `/download` are id-keyed URLs that never change, so a browser's HTTP cache can keep serving a previously-fetched video after its file is replaced (confirmed in manual testing — no `Cache-Control` header meant heuristic caching treated the old response as still fresh). → Mitigation: `Cache-Control: no-store` on both responses in `handler.go`, forcing a fresh fetch on every view/download.

## Migration Plan

No data migration. Backend route addition is additive (new PATCH handler + route registration); frontend change is additive (new button + dialog branch). Deploy backend and frontend together as usual since the button calls the new endpoint immediately.
