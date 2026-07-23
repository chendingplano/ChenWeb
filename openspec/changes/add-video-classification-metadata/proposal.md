## Why

The video manager (SYSTEM ADMIN → Resources → Videos) captures only `name`, `description`, `source`, `url`, and a cover image. Operators building out a training-video library need to classify and triage videos — group them, tag them for search, track publication state, and jot internal notes — but the upload dialog offers no place to record any of that. Adding the classification fields at upload time avoids a second edit pass and keeps the library organized from day one.

## What Changes

- **Enrich the upload dialog** with seven fields, all optional:
  - `keywords` — free-text tags (text input).
  - `category` / `subcategory` — editable comboboxes: a native `<datalist>` suggests values already used by other videos (derived client-side from the loaded list), and the operator can also type a new value.
  - `container` — free-text grouping/collection name (text input).
  - `status` — dropdown constrained to `draft` / `published` / `archived`, defaulting to `draft`.
  - `notes` — free-text internal notes (textarea).
  - `video_type` — auto-detected from the uploaded file (e.g. `mp4`) and pre-filled; the operator can override it if detection fails.
- **Enrich the video model** (`kb.videos` gains `keywords`, `category`, `subcategory`, `container`, `status`, `notes`, `video_type`). Additive and backward-compatible: existing rows keep working; responses fall back to safe defaults.
- **Extend list/detail responses** so the new fields round-trip to the client (and become the source of the combobox suggestions).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `video-management`: the upload flow captures `keywords`, `category`, `subcategory`, `container`, `status` (default `draft`), `notes`, and an auto-detected-but-editable `video_type`; list/detail responses expose these fields; `status` is validated against a fixed set.

## Impact

- **DB**: one additive goose migration in `project_migrations/` — `ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS` for `keywords TEXT`, `category TEXT`, `subcategory TEXT`, `container TEXT`, `status VARCHAR(16) NOT NULL DEFAULT 'draft'`, `notes TEXT`, `video_type VARCHAR(32)`. Live `mise dev`/`air` auto-applies it.
- **Backend**: `server/api/videohandler/handler.go` — extend the `videoMeta` struct, read the new form values in `UploadVideo` (validate `status`, derive `video_type` from the file extension when absent), and extend the `INSERT ... RETURNING` and `ListVideos` `SELECT`/`Scan`.
- **Frontend**:
  - `web/src/lib/services/videoService.ts` — add the fields to `VideoMeta` and `VideoUploadFields`; append them to the upload `FormData`.
  - `web/src/lib/components/home3/video-management-view.svelte` — add dialog state + inputs (comboboxes with a `<datalist>` fed by distinct `category`/`subcategory` values from the loaded list), auto-detect `video_type` on file choose, and include the fields on submit.
- **No change** to streaming/download/delete, the cover-image flow, routing, the Training viewer, or unrelated System Admin tools.
