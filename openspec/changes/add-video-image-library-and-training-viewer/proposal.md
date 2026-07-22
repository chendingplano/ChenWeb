## Why

The training-video feature currently mixes management and consumption on one page (`/resources` > Videos > Training) and stores only a filename. Operators want richer video metadata (name, description, source, cover image, external URL), a reusable image library with AI-generated covers, and a clean split: admins **manage** videos in the workspace admin area, while end users **browse and watch** them on a polished gallery page.

## What Changes

- **Restructure the two surfaces:**
  - Move the video **manager** (upload/edit/delete) into the workspace tree under **SYSTEM ADMIN → Resources → Videos** (appears on `/home3` and `/development`, like other System Admin tools).
  - Turn **`/resources` → Videos → Training (培训)** into a read-only **card-gallery viewer**: each video is a card showing its cover image, name, description, size, and upload time; clicking a card plays the video.
- **Enrich the video model** (`kb.videos` gains `name`, `description`, `source`, `url`, `image_id`). Every video still uploads a file (size/stream unchanged); `source` ∈ {`Recording`, `Web`}; when `source = Web`, a `url` is provided as metadata; `image_id` references the cover in the image library.
- **Enriched upload dialog** in the manager: fields for `name`, `description`, `source` (select), `url` (shown/required when `source = Web`), the video file, and a cover `image` chosen via **Pick an Image** (open the image-library picker) or **Auto-Generate** (each click generates a new AI cover and selects it).
- **New minimal image library** (`kb.images`): admins upload images into it; auto-generated covers are saved into it too; a picker grid selects one. Images are served from a filesystem dir (like videos).
- **AI cover generation**: an OpenAI-compatible text-to-image client (`POST {base}/v1/images/generations`) configured via env (`IMAGE_GEN_BASE_URL`, `IMAGE_GEN_API_KEY`, `IMAGE_GEN_MODEL`). Auto-Generate builds a prompt from the video name/description, saves the result to `kb.images`, and returns it. Unconfigured or failed generation returns a clear error; Pick/upload still work.

## Capabilities

### New Capabilities
- `image-library`: the `kb.images` store, filesystem image storage under `IMAGE_DIR`, and an authenticated API to upload, list, and serve library images, surfaced by a picker UI.
- `image-generation`: an OpenAI-compatible text-to-image integration (env-configured) that generates a cover from a prompt, persists it into the image library, and fails gracefully when unconfigured.
- `training-video-viewer`: the read-only card-gallery page at `/resources` → Videos → Training that renders videos as image cards (name, description, size, upload time) and plays a video on click.

### Modified Capabilities
- `video-management`: the manager moves to SYSTEM ADMIN → Resources → Videos; uploads capture `name`, `description`, `source`, `url`, and a cover `image_id`; list/detail responses expose these fields and a cover-image URL; the Training page no longer hosts the manager.

## Impact

- **DB**: goose migrations in `project_migrations/` — create `kb.images`; alter `kb.videos` to add `name`, `description`, `source` (default `Recording`), `url`, `image_id`; update the `development` page-config seed to register the new nav ids (`sysadmin-resources`, `sysadmin-resources-videos`) for `en` + `zh-cn`.
- **Backend**: new `server/api/imagehandler/` (library CRUD + serve + `/generate`), an OpenAI-compatible image-gen client, enriched `server/api/videohandler/`; routes under `/api/v1/images/...`; new env keys `IMAGE_DIR`, `IMAGE_GEN_BASE_URL`, `IMAGE_GEN_API_KEY`, `IMAGE_GEN_MODEL`.
- **Frontend**:
  - `nav-rail.svelte` — add SYSTEM ADMIN → Resources → Videos to the workspace tree; keep Resources-page `videos-training` (now the viewer).
  - `content-panel.svelte` — route `sysadmin-resources-videos` → the (moved, enriched) manager; `videos-training` → the new viewer.
  - New/updated components: enriched `video-management-view.svelte`, an image-library picker, a training-viewer gallery (designed with the impeccable skill), and `imageService.ts`; enriched `videoService.ts`.
  - `web/messages/en.json`, `web/messages/zh-cn.json` — labels for the new nav nodes.
- **No change** to unrelated System Admin tools, `/home3`/`/development` behavior beyond the new menu node, or the knowledge/workspace menus.
