## Context

Built on `add-document-nav-page-with-videos`: `kb.videos` (filesystem bytes + metadata), `videohandler` (`/api/v1/videos` upload/list/stream/download/delete), the Resources page (`pageKey='resources'`) with its own `resourcesNav` tree (Videos → Training), and the workspace `mainNav` tree (used by `/home3` and `/development`) whose `system-admin` item carries sub-groups with grandchildren. `content-panel.svelte` maps `childId` → a view. Nav visibility/labels for a `pageKey` come from seeded `kb.page_config`; `/home3` passes no `pageKey`. LLM infra is text-only (no image provider). Video/image bytes use a filesystem dir resolved with an env override + `DATA_HOME_DIR` fallback (the `videoDir()` pattern).

## Goals / Non-Goals

**Goals:**
- Split manager (workspace SYSTEM ADMIN → Resources → Videos) from viewer (Resources → Videos → Training, a designed card gallery).
- Enrich videos with name/description/source/url/cover image; every video still uploads a file.
- A minimal image library (upload, list, serve, pick) plus AI cover generation via a configurable OpenAI-compatible endpoint.

**Non-Goals:**
- No video transcoding/thumbnail-from-frame; the cover is a separate library image.
- No full image-library management UI beyond upload + pick (delete is optional/basic).
- No provider-specific image features beyond the standard `/v1/images/generations` contract.
- No role-scoping of the manager beyond "authenticated" + its System Admin placement.

## Decisions

- **Manager placement — extend the workspace `system-admin` item.** Add a `sysadmin-resources` sub-group (label "Resources") with a grandchild `sysadmin-resources-videos` (label "Videos") to `mainNav` in `nav-rail.svelte`. `content-panel.svelte` routes `childId === 'sysadmin-resources-videos'` → the (moved, enriched) `VideoManagementView`. Because `mainNav` renders on `/home3` (no pageKey) and `/development` (pageKey='development'), the `development` page-config seed must register the two new ids (en + zh-cn) to avoid "unrecognized nav entry id" warnings; `/home3` shows them by fail-open default. The Resources page's `videos-training` now renders the viewer instead of the manager.
- **Video model — additive columns.** `ALTER TABLE kb.videos ADD` `name TEXT`, `description TEXT`, `source TEXT NOT NULL DEFAULT 'Recording'`, `url TEXT`, `image_id BIGINT` (nullable, references `kb.images(id)`). Existing rows keep working (`name` falls back to `filename` in responses). `source` is validated server-side against `{Recording, Web}`; `url` is required when `source = Web`. Every upload still writes a file (unchanged storage/stream path).
- **Image library — `kb.images` + `imagehandler`, mirroring the video pattern.** `kb.images(id, filename, stored_path, size_bytes, content_type, origin TEXT ['upload','generated'], prompt TEXT, created_by, created_at)`. Bytes under `IMAGE_DIR` (env override + `DATA_HOME_DIR/Images` fallback). Endpoints under the authenticated `/api/v1` group:
  - `POST /api/v1/images` (multipart upload to library), `GET /api/v1/images` (list), `GET /api/v1/images/:id/content` (serve bytes), `POST /api/v1/images/generate` (AI), `DELETE /api/v1/images/:id` (optional).
  - Videos expose a cover URL by resolving `image_id` → `/api/v1/images/:id/content` (empty when null).
- **AI generation — OpenAI-compatible client, env-configured, fail-soft.** `POST {IMAGE_GEN_BASE_URL}/v1/images/generations` with `{model: IMAGE_GEN_MODEL, prompt, n:1}` and `Authorization: Bearer {IMAGE_GEN_API_KEY}`. Support both `b64_json` and `url` response shapes: decode/fetch the bytes, write to `IMAGE_DIR`, insert a `kb.images` row (origin='generated', prompt recorded), return its metadata. Missing config or provider error → clear error, no partial row/file. The prompt defaults to the video's name + description.
- **Upload dialog** (in `VideoManagementView`) gains: `name`, `description`, `source` `<select>` (Recording/Web), `url` (visible/required when Web), file input, and a cover control showing the selected image with **Pick an Image** (opens an image-library picker modal — grid of `kb.images` served via the content endpoint) and **Auto-Generate** (calls `/images/generate`, selects the returned image; each click replaces the selection with a new one). Submits `image_id` with the video.
- **Training viewer** is a new read-only component rendered for `videos-training`, built with the **impeccable** skill: a responsive card gallery where each card uses the cover image and shows name, description, size, and upload time; clicking plays the video (modal player streaming from `/api/v1/videos/:id/stream`). No management controls.

## Risks / Trade-offs

- **`mainNav` is shared, so the manager shows on `/home3` too** (not only `/development`). Accepted per the confirmed layout; consistent with every other System Admin tool. Page-config could hide it on `/home3` later if desired.
- **AI generation depends on external config/credentials.** Fail-soft design keeps Pick/upload working when unset; the env keys are documented. Response-shape variance (`b64_json` vs `url`) is handled explicitly.
- **Cover images are decoupled from video frames.** Simpler and library-reusable, but a cover can be unrelated to the footage — acceptable and user-controlled (pick or generate).
- **Additive migration on `kb.videos`** avoids a rebuild; `source` default keeps old rows valid. `image_id` is a soft reference (nullable, no cascade); deleting a library image may orphan a cover URL → the content endpoint 404s and the card falls back to its placeholder.
