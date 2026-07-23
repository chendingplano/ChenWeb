## 1. Database

- [x] 1.1 Add goose migration `project_migrations/20260722000006_add_kb_videos_classification.sql` with `ADD COLUMN IF NOT EXISTS` for `keywords TEXT`, `category TEXT`, `subcategory TEXT`, `container TEXT`, `status VARCHAR(16) NOT NULL DEFAULT 'draft'`, `notes TEXT`, `video_type VARCHAR(32)` (Up) and matching `DROP COLUMN IF EXISTS` (Down); comment the `status` value set.

## 2. Backend (server/api/videohandler/handler.go)

- [x] 2.1 Extend the `videoMeta` struct with `Keywords`, `Category`, `Subcategory`, `Container`, `Status`, `Notes`, `VideoType` JSON fields.
- [x] 2.2 In `UploadVideo`, read the new form values; validate `status ∈ {draft, published, archived}` (default `draft`, reject others with a 400); derive `video_type` from the filename extension when the form value is empty.
- [x] 2.3 Extend the `INSERT ... RETURNING` and its `Scan` to persist and return the seven fields (COALESCE text to `''`, `status` to `'draft'`).
- [x] 2.4 Extend the `ListVideos` `SELECT` and row `Scan` to include the seven fields with the same COALESCE defaults.

## 3. Frontend service (web/src/lib/services/videoService.ts)

- [x] 3.1 Add `keywords`, `category`, `subcategory`, `container`, `status`, `notes`, `video_type` to the `VideoMeta` type and to `VideoUploadFields`.
- [x] 3.2 Append each provided field to the upload `FormData` in `uploadVideo`.

## 4. Frontend dialog (web/src/lib/components/home3/video-management-view.svelte)

- [x] 4.1 Add dialog state for the seven fields; reset them in `openDialog`.
- [x] 4.2 In `onDialogFileChosen`, auto-detect `video_type` from the chosen file's extension.
- [x] 4.3 Derive distinct non-empty `category`/`subcategory` values from the loaded `videos` for `<datalist>` suggestions.
- [x] 4.4 Add the inputs to the dialog: `keywords` (text), `category`/`subcategory` (text bound to `<datalist>`), `container` (text), `status` (select draft/published/archived), `notes` (textarea), `video_type` (text); pass them through in `submitUpload`.

## 5. Verify

- [x] 5.1 `cd ChenWeb && go build ./server/...` (or `mise build-server`) compiles; frontend type-checks.
- [ ] 5.2 Manual smoke: upload with all fields set, and upload with none set; confirm both succeed, values round-trip in the list response, and an invalid `status` is rejected.
