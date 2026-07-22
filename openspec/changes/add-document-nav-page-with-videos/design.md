## Context

The `/development` route was added by `add-development-nav-route`: a plain entry in `SiteHeader.svelte`'s `nav` array, a thin `routes/development/+page.svelte` rendering the shared `$lib/components/home3/dashboard.svelte`, and a copied `+layout.ts` that fetches `siteConfig`. `dashboard.svelte` accepts an optional `pageKey` prop that it forwards to `NavRail`.

`nav-rail.svelte` holds the workspace menu as a hardcoded tree (`dashboard`, `chat`, `agents`, … `system-admin`, …). When `pageKey` is set, it fetches `GET /api/v1/page-config/:pageKey` and overlays **labels + visibility** onto that tree. Crucially the overlay is **fail-open**: `isVisible(id) = pageConfig === null || !pageConfig.hidden.has(id)` — an id is hidden only if the resolver explicitly lists it, and `/home3` passes **no** `pageKey`, so its `pageConfig` is `null` and *every* menu id renders.

`content-panel.svelte` maps the selected menu item to a content view through an `{#if activeMenu?.childId === '...'}` chain.

Video storage has an existing precedent: `kbhandler.UploadInputs` streams multipart files to a filesystem directory named by the `STAGING_DIR` env var and records metadata in a DB table.

## Goals / Non-Goals

**Goals:**
- A real `/resources` route reachable from a new auth-gated "Resources" nav entry (between Development and About Us), rendering the shared dashboard under `pageKey="resources"`.
- The Resources page shows its **own** menu tree (`Documents` → `User's Manual`, `Development`; `Videos` → `Training`) and nothing from the workspace tree — and those document items must not leak onto `/home3` or `/development`.
- A working Training video-management page: upload, list, view (stream), download, delete — filesystem bytes + `kb.videos` metadata, all authenticated.

**Non-Goals:**
- No content/behavior for `User's Manual` and `Development` beyond placeholder views.
- No object-storage/CDN/transcoding/thumbnail pipeline — filesystem only, reusing the `STAGING_DIR` pattern.
- No changes to `/home3` or `/development` behavior or to the workspace/knowledge menus.
- No per-role access control on videos beyond "authenticated" (matches the auth-gated Resources page).

## Decisions

- **Nav entry + route mirror `add-development-nav-route`.** Add `{ label: m.semos_nav_resources(), href: '/resources', requiresAuth: true }` to the `nav` array between the Development and About entries. Add `routes/resources/+page.svelte` rendering `<Dashboard siteConfig={data.siteConfig} pageKey="resources" />` and a copied `routes/resources/+layout.ts`. New i18n key `semos_nav_resources` ("Resources" / "文档").

- **The Resources page uses a dedicated menu tree selected by `pageKey`, NOT a DB hide-overlay of the workspace tree.** Because the overlay is fail-open and `/home3` sends no `pageKey`, adding the document items to the shared workspace tree would surface them on `/home3` (and require hiding the entire workspace tree on `/resources`). Instead, `nav-rail.svelte` gains a small separate constant `documentMenu` and renders it *in place of* the workspace tree when `pageKey === 'resources'`. The workspace tree remains the default for `undefined`/`development`/other keys. This keeps the two trees isolated and leak-free.
  - `documentMenu` structure (ids used as `childId`s in content-panel):
    - `documents` (folder) → `docs-users-manual` (User's Manual), `docs-development` (Development)
    - `videos` (folder) → `videos-training` (Training)
  - The existing DB page-config overlay (labels + enable/disable via `hidden`) still applies to whichever tree is active, so operators can relabel/disable document items through the seeded rows. Seed `kb.page_def` (`page_key='resources'`) and `kb.page_config` rows for all five ids in `en` + `zh-cn` so the resolver recognizes them (no "unrecognized nav entry id" warnings) and provides localized labels.

- **Menu labels come from seeded page-config, mirroring the existing DB-backed label path.** English + `zh-cn` rows are seeded in the same migration. (The nav-bar "Resources" label itself stays a paraglide key like the other `semos_nav_*` entries, since the header isn't page-config-driven.)

- **content-panel gets three new branches.** `videos-training` → new `VideoManagement` component (the functional page); `docs-users-manual` and `docs-development` → a shared lightweight placeholder view (empty-state "coming soon"). No other branches change.

- **Backend: new `videohandler` package, filesystem + `kb.videos`.**
  - Migration `kb.videos`: `id BIGSERIAL PK`, `filename TEXT`, `stored_path TEXT`, `size_bytes BIGINT`, `content_type TEXT`, `uploaded_by TEXT`, `created_at TIMESTAMPTZ DEFAULT now()`. Reserved-word-safe names. Created via goose in `project_migrations/` (tax/ChenWeb project tables live in `database.CreateTables`/project migrations, not `sysdatastores`).
  - Storage dir from a new env key `VIDEO_DIR` (set in `.env` — the server loads it via `godotenv` at startup — and in `mise.local.toml`). When `VIDEO_DIR` is unset it falls back to `<DATA_HOME_DIR>/Videos` (DATA_HOME_DIR is already present in every deployment), so uploads work out of the box; it fails closed (CWB_VID_010) only when neither is set.
  - Endpoints under `/api/v1/videos`, all behind the standard auth middleware:
    - `POST /api/v1/videos` — multipart upload; validate content type against a video allow-list + a max-size limit; write bytes to `VIDEO_DIR`, insert metadata; return the new row.
    - `GET /api/v1/videos` — list metadata, newest first.
    - `GET /api/v1/videos/:id/stream` — serve inline with stored content type and HTTP range support (use `http.ServeContent`/`ServeFile` so seeking works).
    - `GET /api/v1/videos/:id/download` — serve as `Content-Disposition: attachment` with the original filename.
    - `DELETE /api/v1/videos/:id` — delete file then metadata row; treat a missing file as success (still remove the row).
  - Routes registered in `server/api/routes.go`. All operations logged per the workspace logging guidance.

- **Frontend service `videoService.ts`** wraps the five endpoints; the `VideoManagement` component uses it for the list/upload/delete UI and points a `<video>`/download link at the stream/download URLs.

## Risks / Trade-offs

- **New branching in `nav-rail.svelte` (tree selection by `pageKey`).** Slightly more logic than a pure data overlay, but it's the only leak-free way to give `/resources` a distinct menu given the fail-open overlay and `/home3`'s missing `pageKey`. Isolated to one `pageKey === 'resources'` check.
- **Large uploads over a single multipart request.** Acceptable for the training-video use case; the max-size limit and fail-closed dir check bound the blast radius. Streaming/chunked/resumable upload is a future concern, not needed now.
- **Filesystem storage isn't horizontally scalable** and ties videos to one host's disk — accepted per Non-Goals; the `kb.videos.stored_path` indirection leaves room to swap in object storage later without changing the metadata contract.
- **Two dashboard-hosting routes plus a page-specific menu tree** mean `nav-rail` now knowingly serves more than one page shape; documented here so a future page follows the same `pageKey`-selects-tree pattern rather than re-deriving it.
