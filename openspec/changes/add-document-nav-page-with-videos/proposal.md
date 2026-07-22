## Why

ChenWeb has no place to host end-user documentation and training videos. Users want a top-level "Resources" area — structured like the existing "Development" dashboard — where they can browse manuals/docs and, most importantly, manage training videos (upload, download, view, delete). This change adds that area and delivers a working video-management page under it.

## What Changes

- Add a **"Resources"** entry to the shared `SiteHeader` nav, inserted between "Development" and "About Us", linking to a new `/resources` route. It is auth-gated exactly like "Development"/"Knowledge Base".
- `/resources` renders the **same shared `Dashboard` component** as `/development` (header + nav rail + content panel + context shelf), passing `pageKey="resources"`, so it needs no new layout logic.
- Give the Resources page its **own left-rail menu tree** (distinct from the workspace menu shown on `/home3` and `/development`):
  - `Documents` (folder) → `User's Manual`, `Development` — placeholder stubs for now.
  - `Videos` (folder) → `Training` — the functional video-management page.
  These menu items are added to the hardcoded `nav-rail.svelte` tree and made visible **only** on the `resources` page via the DB-backed `kb.page_config` overlay (seeded for `en` + `zh-cn`); they are hidden on all other pages.
- Add a **video-management capability**: a new `kb.videos` metadata table plus authenticated CRUD/stream endpoints so the Training page can **upload, list, view (stream), download, and delete** video files. Video bytes are stored on the filesystem under a configured directory (reusing the existing `STAGING_DIR`-style pattern from `kbhandler.UploadInputs`); metadata rows live in `kb.videos`.
- Add the Training video-management **content component** wired into `content-panel.svelte` (new `childId === 'videos-training'` branch) and the front-end service that calls the video API.
- New i18n message keys for the "Resources" nav label and the new menu labels (`en.json` + `zh-cn.json`).

## Capabilities

### New Capabilities
- `document-nav-page`: a "Resources" entry in the shared `SiteHeader` nav linking to a real `/resources` route that renders the shared `Dashboard` with `pageKey="resources"` (its own `+page.svelte` + `+layout.ts`), plus its i18n label.
- `document-page-menu`: the Resources page's left-rail menu tree (`Documents` folder → `User's Manual`, `Development`; `Videos` folder → `Training`), added to `nav-rail.svelte` and scoped to the `resources` page via seeded `kb.page_def`/`kb.page_config` rows so it appears only there; `User's Manual` and `Development` are non-functional placeholder views.
- `video-management`: the `kb.videos` metadata table, filesystem video storage, and authenticated API to upload, list, stream/view, download, and delete videos, surfaced by the Training content component under the Videos folder.

### Modified Capabilities
<!-- openspec/specs/ is empty; no previously spec'd capability's requirements change. -->
(none)

## Impact

- **DB**: new goose migration in `ChenWeb/project_migrations/` for `kb.videos` (id, filename, stored path, size, content_type, uploaded_by, created_at). A data-seed migration adds the `resources` page's `kb.page_def` row and the `kb.page_config` visibility/label rows (`en` + `zh-cn`) for the new menu items.
- **Backend**: new handler package (e.g. `server/api/videohandler/`) for video upload/list/stream/download/delete; routes registered in `server/api/routes.go` under `/api/v1/videos/...`; new config/env key for the video storage directory (mirroring `STAGING_DIR`).
- **Frontend**:
  - `web/src/routes/semos/components/SiteHeader.svelte` — add the "Resources" nav entry.
  - `web/src/routes/resources/+page.svelte`, `web/src/routes/resources/+layout.ts` — new route rendering `<Dashboard pageKey="resources">` (copying the `/development` pattern).
  - `web/src/lib/components/home3/nav-rail.svelte` — add the `Documents`/`Videos` folders and their items to the menu tree (visibility scoped via page-config).
  - `web/src/lib/components/home3/content-panel.svelte` — add the `videos-training` branch rendering the new video-management component; placeholder views for `User's Manual`/`Development`.
  - New Training video-management component + a front-end video service under `web/src/lib/`.
  - `web/messages/en.json`, `web/messages/zh-cn.json` — new label keys.
- **No changes** to `/home3` or `/development` behavior, the workspace/knowledge menus, or existing `kbhandler` upload flow — this is additive.
