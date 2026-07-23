## Context

The video manager already persists rich metadata on `kb.videos` (`name`, `description`, `source`, `url`, `image_id`) via `server/api/videohandler` and the `video-management-view.svelte` upload dialog. This change layers seven purely-additive classification fields onto that same pipeline. The list of videos is already loaded into the component on mount, which lets the category/subcategory suggestions be derived client-side without a new endpoint.

## Goals / Non-Goals

**Goals:**
- Capture `keywords`, `category`, `subcategory`, `container`, `status`, `notes`, `video_type` at upload time.
- Make `category`/`subcategory` discoverable (suggest existing values) without forcing a fixed taxonomy.
- Auto-detect `video_type` so the operator rarely touches it, but keep it editable.
- Stay fully backward-compatible: existing rows and the existing API contract keep working.

**Non-Goals:**
- No editing of these fields after upload (no PATCH/edit dialog in this change).
- No server-side taxonomy management, validation, or normalization of category/subcategory/keywords/container (free text).
- No new column on the video list table UI, no filtering/search by the new fields.
- No changes to the Training viewer, streaming, download, delete, or cover-image flow.

## Decisions

**1. Comboboxes via native `<input list>` + `<datalist>`, sourced client-side.**
The dialog renders `category`/`subcategory` as text inputs bound to a `<datalist>` whose `<option>`s are the distinct non-empty values from the already-loaded `videos` array. This gives "suggest existing + free entry" with zero new backend surface and no dependency. *Alternative considered:* a dedicated `/api/v1/videos/categories` endpoint — rejected as premature; the list is already in memory and refreshes after every upload.

**2. `status` is a fixed dropdown, validated server-side.**
Values `draft`/`published`/`archived`, default `draft`. The handler rejects anything else (mirroring the existing `source ∈ {Recording, Web}` check) so the column stays clean regardless of client. Stored as `VARCHAR(16) NOT NULL DEFAULT 'draft'`.

**3. `video_type` auto-detected, editable fallback.**
The client derives it from the chosen file's extension on `onchange` (e.g. `doc_review.mp4` → `mp4`) and pre-fills the input. The server treats it as advisory: if the form value is empty it falls back to the stored file's extension (from `filename`), lowercased and dot-stripped. Stored as `VARCHAR(32)` nullable. *Alternative considered:* deep content-type sniffing — rejected; the browser-declared type and extension are sufficient and the field is user-overridable anyway.

**4. All seven fields optional; empty strings stored as NULL.**
Reuse the existing `nullableString` helper so blanks become `NULL` and responses `COALESCE` to safe defaults (`''`, `'draft'`). No new required-field friction in the dialog.

## Risks / Trade-offs

- **Suggestions only reflect loaded videos** → acceptable: the manager loads the full list (newest-first) on mount and re-fetches after each upload, so suggestions stay current for the realistic library size. Revisit with a dedicated endpoint only if the list is ever paginated.
- **Free-text category/subcategory can drift** (typos, casing variants) → mitigated by surfacing existing values as suggestions; a fixed taxonomy can be layered later without a contract break.
- **`status` set is hardcoded in two places** (SQL default + Go validation + Svelte options) → small, localized; documented in the migration comment so they stay in sync.

## Migration Plan

- Add one goose migration `project_migrations/<ts>_add_kb_videos_classification.sql` with `ADD COLUMN IF NOT EXISTS` (Up) and matching `DROP COLUMN IF EXISTS` (Down). Live `mise dev`/`air` auto-applies on restart.
- Ship backend + frontend together; the additive columns make the change safe to roll forward and back independently of app code (old code ignores the new columns; new code COALESCEs missing values).

## Open Questions

None — field types, `status` values, `video_type` behavior, and the combobox source were confirmed with the requester.
