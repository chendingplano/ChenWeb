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

- [x] 2.1 Wrote 13 failing tests first from the spec scenarios, confirmed failing for the right reason (`s.Create undefined`). Against the live staging database, **not** sqlmock as `kbhandler` uses: these handlers are thin over `cdm/store`, so mocking the SQL would assert only that the mock was called, and the invariants that matter (row lock, version guard, frozen rule) all live in the database. Noted in the test file
- [x] 2.2 Created `server/api/cdmhandler/`. **Not a handler struct as planned** — the codebase convention is package-level `func(c echo.Context) error` reading `ApiTypes.ProjectDBHandle`, and every other handler package follows it. Matched that rather than introducing a second pattern
- [x] 2.3 `slugifyTitle` + `allocateDocumentKey`: `doc:<slug>` with a numeric suffix on collision. Documented that it is ASCII-oriented — a CJK title strips to empty and falls back to `document`, which is correct but not useful, and improving it needs a transliteration decision the MVP does not need to make
- [x] 2.4 `CreateDocument` (201 + canonical JSON, server-allocated key, `tenant_id` from query) and `ListDocuments` (paged, filtered by tenant through the linked input row)
- [x] 2.5 `GetDocument` returns canonical JSON directly, no wrapper, no DTO
- [x] 2.6 `SaveDocument`. **The body's own `content_version` is the expectation** — no extra header or parameter, since the version is already a first-class field of the document. A client loads v7, sends it back, the store writes v8 only if 7 is still current
- [x] 2.7 `writeStoreError` maps every typed store error: 404 for `*NotFoundError`, 400 + full `violations[]` for `*ValidationError`, 409 for the three conflict types with a `conflict` discriminator (`stale_version` / `frozen` / `block_slug`) so the client can offer the right recovery, and 500 with the detail logged rather than returned for anything else
- [x] 2.8 `PublishDocument`. **`DELETE` deferred, not implemented**: spec §2.6 makes soft delete the default and no soft-delete column exists on `kb.inputs` or `kb.cdm_documents`, so building it means either a migration (design says none) or shipping hard delete mislabelled as soft. Recorded in `routes.go` and below
- [x] 2.9 `RenderDocument` serving cached `kb.cdm_renderings` rows, compiling only on a miss. **Required splitting `Publisher.Render` out of `Publisher.Publish`**: `Publish` renders *and* transitions the input row, so reusing it for preview would freeze a draft the author only wanted to look at (D8). `Publish` is now exactly `Render` + the transition, so published artifacts come from the same code path preview used and cannot drift
- [x] 2.10 Registered six routes on the existing `apiGroup` (`/api/v1`), inheriting `authmiddleware.AuthMiddleware`. **Corrects ADR DR2's `/api/cdm`** — the authenticated group is `/api/v1`, so the paths are `/api/v1/cdm/...`
- [x] 2.11 `TestRoutesAreRegisteredBehindAuth` asserts both that `apiGroup` still carries `AuthMiddleware` and that each CDM route is registered on it
- [x] 2.12 `go build ./server/...` and `go vet ./server/...` clean; 15 handler tests pass against the live database with none skipped; `go test ./server/api/cdm/...` still green. Also drove the whole spine end to end (create → render-without-publish → confirm still a draft → load canonical JSON) against the live database

### Found while implementing group 2

- [x] 2.13 `Store.Load` flattened `sql.ErrNoRows` into `fmt.Errorf` with no `%w`, so a caller could not distinguish "no such document" from "the load failed" — the 404 test returned 500. Added `store.NotFoundError` and wrapped it properly
- [x] 2.14 Added `rendering.DefaultTheme` (`//go:embed theme.typ`) so a deployed binary carries the fallback template instead of depending on the source tree being present beside it at runtime
- [ ] 2.15 **Deferred: `DELETE /documents/:key`.** Needs a soft-delete column that does not exist. Decide whether to add one (migration) or make deletion hard-delete-with-confirmation, then build it with the lineage and inbound-reference checks D14 requires
- [ ] 2.16 **Known limitation: `tenant_id` is a client-supplied filter, not an isolation boundary.** `ApiTypes.UserInfo` carries no tenant, and the rest of this API takes `tenant_id` from the client the same way (`upload_handler.go:87`), so there is no server-side tenant identity to scope against. The spec scenario "a caller sees only their own tenant's documents" is therefore weaker than it reads. Needs a product decision about multi-tenancy before it can be made real

## 3. Frontend foundation

- [x] 3.1 Added `@tiptap/core` and `@tiptap/pm` (^3) to `web/package.json` — not `starter-kit`. `bun add` reported an unrelated pre-existing link failure for the local `@chendingplano/shared` workspace package; confirmed by stashing and reinstalling on the pre-change tree that it fails identically there, so it is not something this change caused. Both packages installed and resolve correctly (`node_modules/@tiptap/{core,pm}/package.json` present)
- [x] 3.2 Wrote `web/src/lib/components/cdm/types.ts`: `Document`, `Metadata`, `Block`, `Inline`, `TableColumn`, `TableRow`, `Equation`, `MathSource`, `MathExpr`, `RefTarget`, plus the five vocabulary unions from `validate.go` (`BlockType`, `InlineType`, `CalloutRole`, `EquationFormat`, `ParseStatus`) as `as const` arrays with derived types, so the vocabularies are enumerable at runtime for later toolbar/UI code, not just compile-time checked. Field names are snake_case, matching the JSON wire format exactly (design D8) — no camelCase translation layer
- [x] 3.3 Dumped the two shared Go fixtures (`cdmfixtures.JaroWinkler()`, `cdmfixtures.AllBlockTypes()`) to `web/src/lib/components/cdm/testdata/*.json` via a one-off `go run`, mirroring the `.typ.golden` convention already used in `rendering/testdata`. Wrote `types.test.ts`: since `JSON.parse` returns `any` and assigning it to a typed variable performs no runtime validation, a naive "parse into `Document`" test would pass even with wrong field names. Instead wrote `clone*` functions that touch only the fields each interface declares, rebuilt each fixture through them, and compared the result to the original — verified this actually catches drift by temporarily renaming `citation_key` to `citationKey` and confirming the test fails, then restored it, matching the same discipline used for the Go lock test in task group 1
- [x] 3.4 Wrote `cdm-client.ts`: six functions (`createDocument`, `getDocument`, `saveDocument`, `listDocuments`, `publishDocument`, `renderDocument`) plus `CdmApiError` and three subclasses (`CdmValidationError`, `CdmStaleVersionError`, `CdmFrozenError`, `CdmBlockConflictError`) discriminated from the handler's `conflict` field, following the existing `req<T>`/`installFetchMock` conventions from `llm-accounts-client.ts`. `saveDocument` takes the version from `doc.content_version` directly — no extra parameter, since the version is already a field of the document being saved
- [x] 3.5 Wrote `block-id.ts`: `slugify`, `allocateBlockId` (heading text preferred, block type as fallback, numeric suffix on collision), and `collectBlockIds` (walks `children` and `items` to build the existing-id set a real document needs). Verified against the spec's own example: `allocateBlockId({type: 'heading', headingText: 'Score Range'}, new Set())` → `"score-range"`

### Found while implementing group 3

- [x] 3.6 **Caught and reverted an unintended repo-wide change.** Running `bun run format` to fix lint warnings on 3 new files reformatted the entire web workspace — 483 files, +31152/-20501 lines — since `format` runs prettier with no path scoping. Reverted all 470 unintended modifications via `jj restore` (built the file list from `jj status`, excluding the two files with real changes), keeping only `package.json`/`bun.lock` (the tiptap addition) and the new `cdm/` files. Confirmed after reverting: `bun run lint` shows the 3 target files clean and the pre-existing repo-wide backlog (551 files) untouched; `bun test` and `bun run check` both still pass at the same baseline as before

## 4. Block list and read-only rendering

Business logic (mutation, defaults, id allocation) was split into plain `.ts`
modules with real unit tests, and the `.svelte` files stay thin views calling
them — this codebase has no component-testing infrastructure (13 existing
`*.test.ts` files test only plain TS modules, zero test `.svelte` files
directly), so this split is what makes 4.3/4.4 actually testable rather than
asserted by inspection, matching the project's own convention rather than
introducing a new one.

- [x] 4.1 Wrote `InlineView.svelte` (all 8 inline types, recursive via self-import) and `BlockView.svelte` (all 9 Phase 1 block types, recursive into `children`/`items` via self-import) as read-only views, plus `BlockList.svelte` holding `blocks` as a `$bindable` prop (not internal `$state` — the caller, eventually the editor page, owns the one canonical copy; no duplicate view model, per design D8)
- [x] Added supporting pure-logic modules, each with its own test file: `createIdAllocator` in `block-id.ts` (a stateful allocator so one insert minting several ids, e.g. a `list` block plus its first item, can't hand out the same slug twice); `block-defaults.ts` (`createDefaultBlock`, a minimal valid shape for each of the 9 types — checked against the content/children/items exclusivity and table-cell-key invariants from spec §1.2, deliberately *not* a client-side port of `model.Validate`, which stays server-side by design); `block-ops.ts` (`insertBlockAt`, `deleteBlockById`, `moveBlock`, `changeContentBlockType`)
- [x] 4.3 Wired selection (click to reveal a toolbar), insertion (a type-picker + Insert control before/between/after every block), deletion, reordering, and type change into `BlockList.svelte`. **Reordering is up/down buttons, not drag-and-drop** — a deliberate simplification for this iteration; `@dnd-kit` is already a project dependency (used in `data-table.svelte`) so richer reordering can be layered in later without a new dependency. **Type change is scoped to paragraph/heading/quote** (`changeContentBlockType`) — the three CDM types that carry `content`; every other type has no content-preserving equivalent (a table's columns/rows don't become paragraph text), so converting into or out of one is delete-then-insert-a-default, not an in-place function. Both scoping decisions are documented in the code, not silently reduced
- [x] 4.4 `block-ops.test.ts` is the direct proof: `moveBlock` asserts the *same* block object references survive a swap (not rebuilt copies with fresh identity), and `changeContentBlockType` asserts `id` is unchanged across a type conversion. Also verified live in a real browser (below) — selecting a block, moving it down, and reading its id back from the DOM confirmed the same id survives the reorder, matching the unit-test result end to end

### 4.2: verification, and what it actually covers

**Full authenticated browser verification through the live server was not
achieved.** Auth is wired through Kratos (`libmanager.go` sets
`authmiddleware.KratosAuthenticator`), no Kratos container was running in this
environment, and no dev-mode auth bypass exists. Scripting a real login was
judged disproportionate to this task, so it was not attempted further; this
is a known gap, not a claimed pass.

What was actually done, and it is real, not a substitute performed for lack
of better options:

- [x] Built a temporary scratch route (`web/src/routes/dev-cdm-verify/`, deleted after use — not part of the shipped route tree) that renders `BlockList` against the exact same fixture JSON used by the group 3 round-trip test — byte-identical to what the live API serves, which the round-trip test already proved
- [x] Loaded it in a real headless Chromium via Playwright (`webapp-testing` skill) and screenshotted it. Found and fixed two real cosmetic bugs this way: the app's global Tailwind preflight strips list markers and heading font-size/weight and link color, making lists/headings/links unreadable in the block list. Added targeted scoped-CSS overrides in `BlockView.svelte` and `InlineView.svelte`, re-screenshotted, confirmed fixed. Every block type and inline type visually matched the source data: table cell alignment, code language label, callout role coloring, equation source/format/status, citation brackets, cross-reference styling
- [x] Drove the interactive controls (4.3) in the same real browser: selecting a block reveals the toolbar with the correct id; moving a block down changes rendered order and the moved block's id is unchanged afterward (the live proof for 4.4); inserting grows the block count by exactly one; deleting returns it to the original count. All assertions passed
- [x] `bun test src/lib/components/cdm/` — 44 tests, `bun run check` — no new errors introduced (same one pre-existing, unrelated error as before this task), `bun run lint` — clean on all new files (format applied *scoped to the specific files*, having learned from task group 3 not to run the project-wide format script again)

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
