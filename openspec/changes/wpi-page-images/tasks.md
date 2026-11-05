## 1. Backend

- [x] 1.1 Add `page_image` to the page keys and a name check in `validate.go`, with tests
- [x] 1.2 Add `server/api/wpi/images.go`: `PutPageImage`, `GetPageImage` (D3), with tests
- [x] 1.3 Register both routes in `server/api/routes.go`

## 2. Frontend

- [x] 2.1 `bun add html-to-image` (pinned version)
- [x] 2.2 Add `uploadPageImage` to `web/src/lib/wpi/api.ts` and a capture helper (D6), with tests
- [x] 2.3 In `save()` of `wpi-editor.svelte`, capture, upload, set `page_image`, then save (D2, D5)
- [x] 2.4 Show `page_image` read-only in the page properties (`properties/page.ts`); Paraglide en and zh messages

## 3. Verification and docs

- [x] 3.1 `go test ./server/api/wpi/...`, `bun run check`, frontend tests
- [ ] 3.2 Save a page in the running editor; confirm file in `DATA_HOME_DIR/page-images` and attribute
- [x] 3.3 Update devdoc 2026102601 (Page attributes, Storage) and `openspec/specs/wpi-pages`
