## 1. Phase 1 store changes (prerequisites)

These change existing `cdm/store` behavior and must land before the handlers,
since the handlers depend on all three. See design D2, D3, D4.

- [ ] 1.1 Write failing tests first for the three store changes: `input_record_id` populated on create, stale-version save rejected, published-document save refused
- [ ] 1.2 Add a `tx`-accepting variant of `InputRegistrar.CreateDraft` so the input row can join another transaction; keep the existing method as a wrapper so Phase 1 callers are unaffected
- [ ] 1.3 Add `Store.Create(ctx, doc, DraftInput)` writing the `kb.inputs` row and the `kb.cdm_documents` row in one transaction and setting `input_record_id` (design D2)
- [ ] 1.4 Add a test asserting that a failure after the input insert leaves neither row behind
- [ ] 1.5 Change `Store.Save` to take an expected `content_version`, applying the increment via `ON CONFLICT ... DO UPDATE ... WHERE content_version = $expected` so the check and the increment are one statement (design D3)
- [ ] 1.6 Add `StaleVersionError{Expected, Actual}`, returned after re-reading the current version when the guarded update matches no row
- [ ] 1.7 Add a concurrency test: two saves from the same loaded version, exactly one succeeds
- [ ] 1.8 Add `FrozenError{DocumentKey}` and make `Store.Save` refuse to write when the linked `kb.inputs` row is published, joining through `input_record_id` (design D4)
- [ ] 1.9 Update the existing Phase 1 store/publish tests for the new `Save` signature; confirm the compiler has found every caller
- [ ] 1.10 Confirm all tests from 1.1 pass, and that `go test ./server/api/cdm/...` is green

## 2. HTTP handlers

- [ ] 2.1 Write failing handler tests for each scenario in `specs/cdm-http-api/spec.md`, against the live staging database as `kbhandler` tests do
- [ ] 2.2 Create `server/api/cdmhandler/` with a handler struct holding the `*Store`, `*InputRegistrar`, and `*Publisher`
- [ ] 2.3 Implement `document_key` allocation: `doc:<slug-of-title>` with a numeric suffix on collision (design D5)
- [ ] 2.4 Implement `POST /documents` (create) and `GET /documents` (list, tenant-scoped through the linked input row)
- [ ] 2.5 Implement `GET /documents/:key` (load) returning canonical JSON directly, with no intermediate DTO
- [ ] 2.6 Implement `PUT /documents/:key` (save): pass the client's expected `content_version` through to `Store.Save`, return the new version on success
- [ ] 2.7 Map store errors to HTTP: `*ValidationError` to a structured violation list, `*StaleVersionError` to 409 with both versions, `*FrozenError` to 409 naming the document, `*ConflictError` to 409 naming the block slug
- [ ] 2.8 Implement `POST /documents/:key/publish` delegating to `Publisher.Publish`, and `DELETE /documents/:key` (soft delete)
- [ ] 2.9 Implement `GET /documents/:key/render` serving cached `kb.cdm_renderings` rows for the current `content_version`, compiling through `Publisher` only on a miss (design D9)
- [ ] 2.10 Register all routes in `server/api/routes.go` under the existing `/api/v1` group, so they inherit `authmiddleware.AuthMiddleware` (design D1)
- [ ] 2.11 Add a test asserting an unauthenticated request never reaches a handler
- [ ] 2.12 Confirm all tests from 2.1 pass; `go build ./server/...` and `go vet ./server/...` clean

## 3. Frontend foundation

- [ ] 3.1 Add `@tiptap/core` and `@tiptap/pm` to `web/package.json` — not `starter-kit`, which admits nodes CDM cannot serialize (design D7)
- [ ] 3.2 Write `web/src/lib/components/cdm/types.ts` mirroring `model.Document`, `Block`, `Inline`, and the supporting types
- [ ] 3.3 Add a round-trip test parsing the shared Go fixture documents through the TypeScript types and re-serializing, so drift between the two definitions is caught (design D8)
- [ ] 3.4 Add an API client module wrapping the endpoints, carrying `content_version` on save and surfacing the four typed error cases distinctly
- [ ] 3.5 Add the block slug allocator: heading text or block type, plus a disambiguating suffix, assigned once at creation (design D6)

## 4. Block list and read-only rendering

- [ ] 4.1 Write the block list component holding `$state<Block[]>`, rendering all nine Phase 1 block types read-only
- [ ] 4.2 Verify it renders both shared fixture documents correctly, loaded from the real API
- [ ] 4.3 Add block selection, insertion, deletion, reordering, and type change, operating directly on the block array (no rich-text engine involved)
- [ ] 4.4 Add a test asserting block ids survive reorder and content edits unchanged

## 5. Inline editor

- [ ] 5.1 Write the ProseMirror schema by hand, containing exactly CDM's eight inline types and no presentation mark (design D7)
- [ ] 5.2 Implement the CDM `[]Inline` ↔ ProseMirror document mapping in both directions
- [ ] 5.3 Add a round-trip test covering every inline type, including `math`, `citation`, and `cross_reference` atoms
- [ ] 5.4 Add a test asserting pasted font/colour styling is dropped while text is retained
- [ ] 5.5 Mount the inline editor for `paragraph`, `heading`, and `quote` blocks, one instance per block
- [ ] 5.6 Build the semantic toolbar — heading, strong, emphasis, code, link, list, table, quote, code block, equation, image, callout — with no font, size, colour, or alignment control

## 6. Structured block editors

- [ ] 6.1 `table`: column add/remove/retitle/align, row add/remove, per-cell inline editing, and an optional caption
- [ ] 6.2 `list`: ordered/unordered toggle, item add/remove, nested block content per item
- [ ] 6.3 `code`: language selector and a plain textarea, verbatim and unescaped
- [ ] 6.4 `equation`: display/inline toggle and the original source with its format; Phase 1 stores `parse_status: "skipped"` with no normalized AST
- [ ] 6.5 `image`: source, alt text, and caption
- [ ] 6.6 `callout`: the five CDM roles and a title
- [ ] 6.7 Verify a document containing every block type round-trips through the editor unchanged

## 7. Save, publish, preview

- [ ] 7.1 Wire save, sending the loaded `content_version`; on success adopt the returned version
- [ ] 7.2 Surface a stale save without discarding the author's local content
- [ ] 7.3 Surface a frozen document with an explanation that continuing needs a new version — the action itself is deferred (design D4), so this must not dead-end silently
- [ ] 7.4 Attribute validation violations and block-slug conflicts to the offending block
- [ ] 7.5 Wire publish, with confirmation, since publishing freezes the document
- [ ] 7.6 Wire preview: request the rendered pages explicitly, never per keystroke, and display them

## 8. Routes and integration

- [ ] 8.1 Add `/home3/cdm` (document list) and `/home3/cdm/[key]` (editor) following the existing `home3` route conventions
- [ ] 8.2 Route all user-visible strings through the existing Paraglide i18n mechanism
- [ ] 8.3 Drive the full loop in a real browser with Playwright: create, edit each block type, save, publish, preview
- [ ] 8.4 Verify the preview shows the generated table of contents and figure/table/formula lists from the DR5d work

## 9. Verification

- [ ] 9.1 `go build ./server/...`, `go vet ./server/...`, and `go test ./server/api/cdm/...` all clean
- [ ] 9.2 `bun run check` and `bun run lint` clean in `web/`
- [ ] 9.3 Confirm every requirement in both capability specs has at least one corresponding test
- [ ] 9.4 Confirm no CDM table required a migration, as design predicted; if one did, record why
- [ ] 9.5 Update ADR 2026072603 with what implementation actually changed — the `/api/v1` path correction (design D1), the deferral of the new-version action (design D4), and any further divergence found while building
- [ ] 9.6 Update spec `2026072502-spec-cdm-editor` §5 phasing to record which MVP features shipped
