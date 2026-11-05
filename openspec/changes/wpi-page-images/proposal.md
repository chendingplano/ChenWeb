## Why

A page stored in `kb.frontend_pages` is only JSON. Authors searching for a page to reuse
(*Load Page*, `GET /api/v1/wpi/pages`) cannot see what it looks like. A picture of the page,
made when it is saved, lets them recognise it at a glance.

## What Changes

- When the WPI editor creates or saves a page, it captures a PNG of the page as drawn on the
  canvas preview and uploads it before the page is stored.
- A new admin endpoint stores the PNG under `DATA_HOME_DIR/page-images/` and returns its file name.
- A new optional page root attribute, `page_image`, holds that file name. The page validator
  accepts it and checks its form.
- A read endpoint serves a stored image by name, so the editor can show it later.
- A failed capture or upload never blocks the page save: the page is saved without a new image
  and the editor shows a warning.

## Capabilities

### New Capabilities
- `wpi-page-images`: capturing, storing and serving page images.

### Modified Capabilities
- `wpi-pages`: the page attribute list gains `page_image`.

## Impact

- Backend: `server/api/wpi` (new image handler, `validate.go`), `server/api/routes.go`.
- Frontend: `web/src/lib/components/shared-ui/wpi-editor.svelte`, `web/src/lib/wpi/api.ts`,
  a new capture helper, `properties/page.ts`, Paraglide messages (en and zh).
- New frontend dependency: `html-to-image`.
- Needs `DATA_HOME_DIR` set; the upload is refused with 503 otherwise.
- Docs: devdoc 2026102601 (Page attributes, Storage), `openspec/specs/wpi-pages`.
