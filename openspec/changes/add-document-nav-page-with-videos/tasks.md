## 1. Nav entry + /resources route

- [x] 1.1 Add `semos_nav_resources` message key to `web/messages/en.json` ("Resources") and `web/messages/zh-cn.json` ("文档").
- [x] 1.2 Add `{ label: m.semos_nav_resources(), href: '/resources', requiresAuth: true }` to the `nav` array in `web/src/routes/semos/components/SiteHeader.svelte`, between the Development and About Us entries.
- [x] 1.3 Add `web/src/routes/resources/+layout.ts` (copy of `routes/development/+layout.ts`, fetching `siteConfig`).
- [x] 1.4 Add `web/src/routes/resources/+page.svelte` rendering `<Dashboard siteConfig={data.siteConfig} pageKey="resources" />`.
- [ ] 1.5 Verify `/resources` loads the shared dashboard with the URL bar showing `/resources`, the Resources nav entry highlights when active, and `/home3` + `/development` are unchanged.

## 2. Resources page menu tree (nav-rail)

- [x] 2.1 In `web/src/lib/components/home3/nav-rail.svelte`, add a `documentMenu` constant: `documents` folder (children `docs-users-manual`, `docs-development`) and `videos` folder (child `videos-training`), with icons consistent with the existing tree.
- [x] 2.2 Select the active menu tree by `pageKey`: render `documentMenu` when `pageKey === 'resources'`, otherwise the existing workspace tree. Ensure the DB page-config overlay (labels + `hidden`) still applies to the active tree.
- [ ] 2.3 Confirm the workspace tree is unaffected on `/home3` and `/development`, and the document items do not appear there.

## 3. Page-config seed for the document page

- [x] 3.1 Add a goose data-seed migration in `ChenWeb/project_migrations/` inserting a `kb.page_def` row for `page_key='resources'` (route `/resources`).
- [x] 3.2 Seed `kb.page_config` rows for all five ids (`documents`, `docs-users-manual`, `docs-development`, `videos`, `videos-training`) in `en` and `zh-cn`, enabled and visible, with localized labels.
- [ ] 3.3 Verify no "unrecognized nav entry id" warnings are logged for the `resources` page and that labels resolve per locale.

## 4. Video backend (videohandler + kb.videos)

- [x] 4.1 Add a goose migration in `ChenWeb/project_migrations/` creating `kb.videos` (`id`, `filename`, `stored_path`, `size_bytes`, `content_type`, `uploaded_by`, `created_at`), with reserved-word-safe column names; wire creation through the project's table-creation path.
- [x] 4.2 Add a `VIDEO_DIR` env/config key for the storage directory, validated fail-closed (mirroring `STAGING_DIR` handling in `kbhandler.UploadInputs`).
- [x] 4.3 Create `server/api/videohandler/` with a `POST /api/v1/videos` upload handler: multipart parse, content-type allow-list + max-size check, write bytes to `VIDEO_DIR`, insert `kb.videos` row, return the new row; clean up on failure.
- [x] 4.4 Add `GET /api/v1/videos` list handler (newest first).
- [x] 4.5 Add `GET /api/v1/videos/:id/stream` handler serving inline with stored content type and HTTP range support (`http.ServeContent`).
- [x] 4.6 Add `GET /api/v1/videos/:id/download` handler serving `Content-Disposition: attachment` with the original filename.
- [x] 4.7 Add `DELETE /api/v1/videos/:id` handler: delete file (treat missing file as success) then remove the metadata row.
- [x] 4.8 Register all routes under `/api/v1/videos` in `server/api/routes.go` behind the standard auth middleware; ensure every operation is logged.

## 5. Video frontend (Training page)

- [x] 5.1 Add `web/src/lib/services/videoService.ts` wrapping upload/list/stream/download/delete with `credentials: 'same-origin'`.
- [x] 5.2 Create the `VideoManagement` content component: list with metadata (filename/size/date/uploader), upload control, inline `<video>` viewer pointing at the stream URL, download link, and delete-with-confirm.
- [x] 5.3 Add a `videos-training` branch in `web/src/lib/components/home3/content-panel.svelte` rendering `VideoManagement`; add placeholder branches for `docs-users-manual` and `docs-development` (shared empty-state view).
- [ ] 5.4 Verify end-to-end on `/resources` → Videos → Training: upload, list refresh, inline play (with seeking), download, delete.

## 6. Verification & docs

- [x] 6.1 Run `bun`/svelte build + `go vet ./...` / `go test ./...` for touched modules; confirm no regressions.
- [ ] 6.2 Manually verify auth gating: unauthenticated `/api/v1/videos/...` requests are rejected.
- [x] 6.3 Record what knowledge changed / which docs are affected per the workspace "Coding Best Practice" doc protocol; update any relevant ChenWeb docs.
