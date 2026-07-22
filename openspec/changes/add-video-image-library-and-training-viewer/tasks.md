## 1. Database

- [x] 1.1 Migration: create `kb.images` (id, filename, stored_path, size_bytes, content_type, origin, prompt, created_by, created_at) with a created_at index.
- [x] 1.2 Migration: `ALTER TABLE kb.videos` add `name`, `description`, `source` (NOT NULL DEFAULT 'Recording'), `url`, `image_id` (nullable).
- [x] 1.3 Migration: extend the `development` page-config seed with `sysadmin-resources` and `sysadmin-resources-videos` (en + zh-cn labels), granting the standard access roles.

## 2. Backend — image library + generation

- [x] 2.1 Add `IMAGE_DIR` resolution (env override + `DATA_HOME_DIR/Images` fallback) and `.env` entry; add `IMAGE_GEN_BASE_URL`, `IMAGE_GEN_API_KEY`, `IMAGE_GEN_MODEL` env keys.
- [x] 2.2 Create `server/api/imagehandler/`: `POST /api/v1/images` (upload), `GET /api/v1/images` (list), `GET /api/v1/images/:id/content` (serve), `DELETE /api/v1/images/:id`.
- [x] 2.3 Add OpenAI-compatible image-gen client + `POST /api/v1/images/generate`: build prompt, call `{base}/v1/images/generations`, handle `b64_json`/`url` responses, save to `kb.images` (origin='generated'), return metadata; fail-soft when unconfigured or on provider error.
- [x] 2.4 Register image routes in `server/api/routes.go` under the authenticated `/api/v1` group.

## 3. Backend — enriched videos

- [x] 3.1 Update `videohandler` upload to read `name`, `description`, `source` (validate Recording/Web), `url` (require when Web), `image_id`; persist on `kb.videos`.
- [x] 3.2 Update list/detail responses to include `name`, `description`, `source`, `url`, and a resolved cover-image URL (from `image_id`).

## 4. Frontend — move + wire the manager

- [x] 4.1 In `nav-rail.svelte`, add `sysadmin-resources` (label "Resources") sub-group with grandchild `sysadmin-resources-videos` (label "Videos") under the `system-admin` item.
- [x] 4.2 In `content-panel.svelte`, route `childId === 'sysadmin-resources-videos'` → `VideoManagementView`; change `videos-training` → the new viewer.
- [x] 4.3 Add nav label message keys if needed (en/zh) consistent with existing System Admin labels.

## 5. Frontend — enriched upload dialog + image picker

- [x] 5.1 Add `imageService.ts` (list/upload/generate + content URL) and extend `videoService.ts` types + upload to send name/description/source/url/image_id.
- [x] 5.2 Build an image-library picker component (grid of `kb.images`, select one; upload-into-library affordance).
- [x] 5.3 Rework `video-management-view.svelte` upload UI into a dialog with name, description, source select, conditional url, file input, and a cover control (selected preview + Pick an Image + Auto-Generate; each Auto-Generate click swaps in a new image).

## 6. Frontend — Training viewer (impeccable)

- [x] 6.1 Use the impeccable skill to design and build a responsive card-gallery viewer: each card uses the cover image and shows name, description, size, upload time; placeholder when no cover; empty state.
- [x] 6.2 Clicking a card plays the video (modal player streaming from `/api/v1/videos/:id/stream`, with seeking).

## 7. Verify & docs

- [x] 7.1 `go vet ./server/...` + `go build ./server/...`; `bun run check` in `web/` — no errors.
- [ ] 7.2 Manual: upload with metadata + cover (pick and auto-generate), manager under SYSTEM ADMIN → Resources → Videos, viewer cards + playback on `/resources`.
- [x] 7.3 Record knowledge changes per the ChenWeb doc protocol; note the new env keys.
