## 1. Env var consolidation: `UPLOAD_FILE_STAGING_DIR`

- [x] 1.1 `server/api/kbhandler/upload_handler.go`: read `UPLOAD_FILE_STAGING_DIR`
      instead of `STAGING_DIR`
- [x] 1.2 `server/cmd/service-pdf-parser/main.go`: read
      `UPLOAD_FILE_STAGING_DIR` instead of `DATA_STAGING_DIR` — **turned out
      to be dead code, see task 7**; `doc-service`, the real staging
      service, was already reading `UPLOAD_FILE_STAGING_DIR` before this
      change and needed no edit here
- [x] 1.3 `mise.local.toml`: replace the two `STAGING_DIR`/`DATA_STAGING_DIR`
      lines with a single `UPLOAD_FILE_STAGING_DIR`
- [x] 1.4 `python/pdf-parser/pdf_parser.py`: add `UPLOAD_FILE_STAGING_DIR` as
      the first-checked name in the existing `_env(...)` fallback chain
      (keep `STAGING_DIR`/`PDF_STAGING_DIR`/`DATA_STAGING_DIR` as fallbacks)

## 2. Staging poller: ignore `.pending` files

- [x] 2.1 In `service-pdf-parser/main.go`'s `processStagingOnce`, skip any
      entry whose name ends in `.pending` before stat/copy/insert —
      **applied to the wrong (dead) service, see task 7 for the real fix in
      `doc-service`**
- [x] 2.2 Add/verify a unit test asserting a `.pending` file is left in place
      and produces no copy/insert side effects (in `service-pdf-parser`'s
      test file; a matching test for `doc-service` is task 7.2)

## 3. Backend: list pending files endpoint

- [x] 3.1 Add `GET /kb/pending-files` in `kbhandler` (new file
      `pending_files_handler.go`), admin-gated via the same pattern as
      `keyword_handlers.go`'s `requireKeywordRewriteAdmin`
- [x] 3.2 List `.pending`-suffixed files in `UPLOAD_FILE_STAGING_DIR`; apply
      one two-sample size-stability check per request (read all sizes, sleep
      a short fixed interval, re-read, keep only unchanged files)
- [x] 3.3 Return each file's stripped name and mtime
- [x] 3.4 Add tests: unauthenticated gets 401 (no admin-session test harness
      exists yet in this package — see note below); `typeFromExtension`
      covered directly

## 4. Backend: claim pending files endpoint

- [x] 4.1 Add `POST /kb/pending-files/claim` accepting selected pending
      filenames, `processing_mode`, `parser_name`, `ks_store_id`,
      admin-gated the same way as the list endpoint; derive the input's
      `user_id` from the authenticated session
- [x] 4.2 Port the frontend's extension→type map
      (`kb-import-view.svelte`'s `typeExtensions`) to Go for per-file type
      derivation; reject files with unrecognized extensions with a per-file
      error
- [x] 4.3 For each selected file, in one DB transaction: call the existing
      `insertUploadedInputRecord` with `StagingAbsPath` set to the
      post-`.pending`-strip path, then `os.Rename` the pending file to that
      path; commit on success, roll back and report a per-file
      "no longer available" error if the rename fails (file already claimed
      or gone) — do not fail the rest of the batch
- [x] 4.4 Add tests: `claimOnePendingFile` insert+rename success, rollback
      when the pending file is already gone, unauthenticated gets 401.
      **Note:** a true admin-authenticated end-to-end test (200 path through
      the HTTP handler) isn't possible without new test scaffolding — no
      handler in `kbhandler` currently has one, since `EchoFactory`'s
      `IsAuthenticated()` calls a package-level `DefaultAuthenticator` wired
      up only by `authmiddleware.Init()`, which none of this package's tests
      call. Covered the admin-gate's reachable branch (401 when
      unauthenticated) and tested the post-auth business logic
      (`claimOnePendingFile`) directly instead.

## 5. Frontend: Pending Files button and dialog

- [x] 5.1 In `kb-import-view.svelte`, add a "Pending Files" button next to
      "Upload Files"; on mount, call `GET /kb/pending-files` and show the
      button only if the call succeeds (401/403 hides it — no separate
      admin-check endpoint)
- [x] 5.2 On click, call `GET /kb/pending-files` again for a fresh list; if
      empty, show a prompt instead of opening the dialog
- [x] 5.3 Build the selection dialog: checkbox per pending file, filename and
      mtime shown, and the same Auto / Upload Files Only / PDF Parsing mode
      selector `submitUpload()` already uses (also added the parser_name
      selector, since the backend requires a valid one for every claim just
      like `UploadInputs` does)
- [x] 5.4 Wire the dialog's "Upload Files" action to
      `POST /kb/pending-files/claim`, reusing the same `ks_store_id` payload
      as `submitUpload()`; the server derives `kb.inputs.user_id` from the
      authenticated requester's `user_id`; show per-file results
- [x] 5.5 On a "no longer available" error for any file, re-fetch the
      pending files list in the dialog rather than leaving it stale
- [ ] 5.6 Manually verify in a live `mise dev` session: copy a file named
      `<name>.pending` into `UPLOAD_FILE_STAGING_DIR`, confirm it's invisible
      to normal upload processing, claim it as admin, confirm it's ingested
      and attributed to the authenticated user's ID stored in
      `kb.inputs.user_id`, regardless of the active store's tenant value

## 6. Documentation

- [x] 6.1 Update `KnowledgeStore/Capsules/coding-capsules/input-management/inputs-processing-spec.md`
      to document the `.pending` convention, the `UPLOAD_FILE_STAGING_DIR`
      env var, and the Pending Files claim flow — and correct its staging
      service name and zip-handling description to match `doc-service`

## 7. Fix: tasks 1.2/2.1/2.2 initially patched the wrong service

Manual verification (task 5.6) surfaced that a real `.pending` file placed
in staging was ingested unattributed by the live `doc-service` process —
tasks 1.2/2.1/2.2 above had patched `server/cmd/service-pdf-parser`
instead, which is dead code not wired into any `mise` task and does not run
in this environment. `server/cmd/doc-service` is the real staging service.

- [x] 7.1 Add the `.pending` skip filter to `doc-service/main.go`'s
      `processStagingOnce` (the actual fix; `service-pdf-parser`'s copy is
      harmless dead code left in place, not reverted, since it does no harm
      and isn't this session's call to delete)
- [x] 7.2 Add `TestProcessStagingOnceSkipsPendingFiles` in
      `doc-service/main_test.go`
- [x] 7.3 Clean up the bad state the bug produced during manual testing:
      `kb.inputs` record `726` (inserted unattributed, `type='pending'`)
      deleted; its misnamed file moved back from
      `DATA_HOME_DIR/Artifacts/0/726/jixie_pdf_new.zip.pending` to
      `UPLOAD_FILE_STAGING_DIR/jixie_pdf_new.zip.pending` for the user to
      retry; the stray backup copy removed
- [x] 7.4 Correct `design.md`, `tasks.md`, and the KnowledgeStore spec doc's
      references from `service-pdf-parser` to `doc-service`, and restore the
      accurate description of `doc-service`'s real zip child extraction
      (wrongly described as nonexistent in the first pass, since that
      investigation was against the wrong file)
