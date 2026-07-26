## 1. Phase 1 store changes (prerequisites)

These change existing `cdm/store` behavior and must land before the handlers,
since the handlers depend on all three. See design D2, D3, D4.

- [x] 1.1 Wrote failing tests first (`create_test.go`, `concurrency_test.go`), confirmed failing for the right reason — `s.Create undefined`, not a typo — before writing any implementation
- [x] 1.2 Added `createDraftTx` taking an `execQuerier` (the `QueryRowContext` subset shared by `*sql.DB` and `*sql.Tx`); `InputRegistrar.CreateDraft` is now a one-line wrapper, so Phase 1 callers are untouched
- [x] 1.3 Added `Store.Create(ctx, doc, DraftInput)` writing both rows in one transaction and setting `input_record_id`. Extracted `writeContentTx` so `Create` and `Save` share one definition of how content is persisted rather than drifting
- [x] 1.4 `TestCreate_InvalidDocumentWritesNeitherRow` — counts `kb.inputs` rows before and after, asserting no orphan is left behind
- [x] 1.5 **Implemented differently from the plan.** The planned `ON CONFLICT ... DO UPDATE ... WHERE content_version = $expected` handles a version mismatch but silently *creates* a document when the caller expects version 7 and no row exists. Replaced with `lockDocStateTx`: one `SELECT ... FOR UPDATE OF d` reading version and publication state together, then an explicit four-way branch (frozen / stale / missing / proceed). Clearer, handles every case, and reads the frozen state in the same query rather than a second round trip
- [x] 1.6 Added `StaleVersionError{DocumentKey, Expected, Actual}` with `Actual == 0` meaning "no such document", and a distinct message for that case
- [x] 1.7 Added `TestSave_ConcurrentSavesExactlyOneWins`. **It does not prove the row lock**, verified by deleting `FOR UPDATE` and watching it stay green across repeated runs — one goroutine commits before the other reads, so the lost-update window never opens. Added `TestLockDocStateTx_BlocksConcurrentReader` (`lock_internal_test.go`, internal to package `store`) as the deterministic proof: it holds a transaction, asserts a second reader blocks for 500ms, then asserts the waiter observes the committed version. Confirmed it fails with `FOR UPDATE` removed and passes with it. The goroutine test's comment now states what it does and does not cover
- [x] 1.8 Added `FrozenError{DocumentKey}`; `lockDocStateTx` derives publication from the absence of the `doc_processing` status entry — the same signal the worklist uses — rather than introducing a second source of truth. A NULL `input_record_id` reads as a draft, keeping pre-`Create` documents editable
- [x] 1.9 Updated all 18 call sites across `store_test.go` and `publish_test.go`; every one was a test, as design predicted. Noted in `publish_test.go` that `TestPublisher_RepublishSupersedesArtifacts` builds its document with `Save` + a separate `CreateDraft`, so `input_record_id` is NULL and the frozen rule does not apply — deliberate, since that test is about artifact superseding, not the editorial lifecycle
- [x] 1.10 `go build ./server/...`, `go vet ./server/...` clean; `go test ./server/api/cdm/...` green; 20 store tests pass against the live `miner` database with none skipped

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
