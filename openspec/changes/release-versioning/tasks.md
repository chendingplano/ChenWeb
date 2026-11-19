## 1. Database

- [x] 1.1 Goose migration (use the `db-migration` skill): abort with a message if `public.releases` or `public.release_items` has rows; drop both; create `kb.releases`, `kb.release_items`, `kb.release_current` per design D2 (checks 0–99, kind/status/type/component checks, unique project+version, unique non-null build_number, FK with no cascade from items)
- [x] 1.2 Same migration: add `release_version` to `kb.deployment_builds`; `release_version` and `previous_release_version` to `kb.deployments` (TEXT NOT NULL DEFAULT '')
- [x] 1.3 Apply on the dev DB (`doc-service --migrate-only` from its dir) and check `\d kb.release*` and `project_db_migration`

## 2. Release core (`server/api/releaseshandler/`)

- [x] 2.1 Version type: parse/format `MM.mm.rr`, compare, next major/minor/revision with the 99 limit and the messages from D3; unit tests
- [x] 2.2 Store: read current (default `00.00.00`), create major/minor (lock `kb.release_current` row, refuse while a `building` release exists), list history with items, pending items CRUD (refuse bound/released items with a conflict error); sqlmock tests
- [x] 2.3 Store: `Reserve` (revert stale `building` per Risks, insert next revision as `building`, bind all pending items, one transaction), `SetBuildInfo` (build number, git revisions), `Commit` (released, date, current; the runner stores the version on `kb.deployment_builds` from `::release::`), `Abort` (delete `building` row → items pending); tests including failed and killed builds
- [x] 2.4 Release file: write `release.json` with full history (D7); `Import` that inserts missing releases with their items and never updates or deletes existing ones; test order-independence (import 06 then 04)
- [x] 2.5 Item validation: type, component, non-empty description, issue digits only; tests
- [x] 2.6 Remove the old free-text code paths (`naturalCompare`, wholesale `Save`) that the rewrite makes unused

## 3. Release notes

- [x] 3.1 Range selection (from, to] newest first, with major/minor headings; Markdown and plain-text renderers; issue formatting `(#n)`, `(shared#n)`, `[shared]`; English and Chinese heading table; golden-file tests for one release, a range across a new minor, an empty release
- [x] 3.2 PDF via `docbuilder.PandocConvert(DOCUMENT_PDF_ENGINE, DOCUMENT_PDF_FONT)` cached as `.cache/release-notes/<sha256>.pdf`; missing pandoc/engine → typed error the handler maps to 503 naming the tool; test with a stub converter

## 4. API and roles

- [x] 4.1 Handlers per D11 replacing the four old routes in `server/api/routes.go`; admin check as today; writes refused with 403 `read_only` unless `[deployment] role = "build"`; 400/404/409 mapping; new `CreateDefaultLogger` location codes
- [x] 4.2 `GET /api/v1/releases/notes` with `format=text|pdf` and `lang=en|zh-cn`
- [x] 4.3 Handler tests: role gating, non-admin 403, conflict on bound item, notes text/pdf content types

## 5. `release-tool` command

- [x] 5.1 `server/cmd/release-tool/main.go`: load `.env`/config/DB like `deploy-runner`; subcommands `reserve`, `notes --version --build-number --out`, `commit --version`, `abort --version`, `add-item`, `import <file>`; role check for all writers except `import`; exit codes 0/1/2
- [x] 5.2 `notes` writes `release/release.json`, `RELEASE_NOTES.txt`, `RELEASE_NOTES.pdf`, `RELEASE_NOTES.zh-cn.txt`, `RELEASE_NOTES.zh-cn.pdf`; `commit` prints `::release::<version>`
- [x] 5.3 Tests for argument handling and the role refusal

## 6. Build integration

- [x] 6.1 `shell_server_build_rcp.sh`: in `check`, run `release-tool reserve` and export `RELEASE_VERSION`; trap that runs `release-tool abort` (and removes uploaded relay files if the failure is after upload); new `release` step before `archive`; upload the sidecar `runshen-deploy-<build>.release.json`; new `release-commit` step after `upload`
- [x] 6.2 `mise.toml` `build-server-linux`: add `release_version` to the build metadata JSON; MANIFEST lines `release_version:`, `shared_git_rev:`, `shared_git_dirty:` (from `../shared`); works unchanged when `RELEASE_VERSION` is unset
- [x] 6.3 `server/buildinfo`: `ReleaseVersion` in `Metadata`; Build Info API and `build-info-view.svelte` show it
- [x] 6.4 `deployment/plan.go`: `release` and `release-commit` build steps; tracker records `::release::` and `deploy-runner` stores it; update the script/plan key test (deployment-page task 4.5)
- [ ] 6.5 `bash -n` the scripts; run one real page build on the Mac with two pending items and check the archive's `release/`, the sidecar on the relay, the committed release and that both items are released
- [x] 6.6 Failure check: force a failure after `reserve` (e.g. a bad `BINS`) and confirm the release is gone and the items are pending again — verified 2026-11-19: hand run with BINS=bogus reserved 00.00.01, the trap gave it back, the test item returned to pending (then deleted)

## 7. Deploy integration

- [x] 7.1 `shell_server_deploy.sh`: `--list` (all archives, newest first), `--release-file <name>` (print one sidecar, name validated); new `release` step copying `release/` from the extracted archive into the ChenWeb dir; plan key added
- [x] 7.2 Server start-up (role = deploy): import `<ChenWeb>/release/release.json` if present and set `kb.release_current`; log and continue on error
- [x] 7.3 `GET /api/v1/deployment/archives`: list, fetch and import the newest sidecar, return archives with build number, version, date, size, item count and the running release; tests with a fake script
- [x] 7.4 `StartDeployment`: fill `release_version` and `previous_release_version`; history API returns them

## 8. Frontend

- [x] 8.1 `releases-client.ts` for the new API
- [x] 8.2 `releases-view.svelte` rebuild per the `release-versions` page requirement: current/next, New major/New minor dialogs with resulting version, pending items add/edit/delete, building release with age, history with expandable items and text/PDF notes (plain text in a `<pre>`; Markdown Viewer not needed); read-only mode when not editable; "builds remaining" hint from revision 90
- [x] 8.3 `deployment-view.svelte` (build role): "Next release … · N items" on the build card; release version column in build history
- [x] 8.4 `deployment-view.svelte` (deploy role): release picker replacing the newest-archive confirmation; notes from the running release to the selection; rollback warning; View PDF; version in deployment history
- [x] 8.5 Paraglide keys (`releases_*`, `deployment_*`) in `en.json` and `zh-cn.json` with real Chinese; follow `docs/frontend-rules.md`; `bun run check` passes with no new hard-coded text

## 9. Rollout

- [ ] 9.1 On the Mac: create major `01`, add items (one via `release-tool add-item`), build, and view text and PDF notes on the Releases page
- [ ] 9.2 Root on the box (user): confirm `pandoc`, `typst` and a CJK-capable font are installed; set `DOCUMENT_PDF_FONT` if needed
- [ ] 9.3 Deploy that build with the current page; confirm the box's Releases page shows the history and the running release
- [ ] 9.4 Next build: deploy it with the picker; check notes since the running release, the PDF, a skipped-release range and the rollback warning (without confirming the rollback)

## 10. Documentation and catalogue

- [x] 10.1 Rewrite devdoc `2026111901-devdoc-release-management.md` with the `create-devdoc` skill; add the release steps to devdoc `2026111602` and the runbook's "Deployment page" section
- [x] 10.2 `ChenWeb/CLAUDE.md`: rule to record each fix with `release-tool add-item` (issue number when one exists)
- [x] 10.3 Catalogue: `docbuilder.PandocConvert` is now used outside Document Builder — update module 20261109-02's `kb.reusable_modules` row and doc (`create-reusable-module`) and run `modules.py check`
- [ ] 10.4 When `deployment-page` is archived, note that `release-deployment` replaces its "Deploy shows the archive before it starts" requirement
