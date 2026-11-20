## 1. Database

- [x] 1.1 Goose migration creating `kb.deployment_builds`, `kb.deployments` and `kb.deployment_steps` with the columns, status checks, foreign keys, `num_nonnulls` check and unique `(…_id, seq)` indexes from design D8 (use the `db-migration` skill)
- [x] 1.2 In the same migration, the one-active-run partial unique indexes on both run tables (`((true)) WHERE status IN (…)`); verify in the dev DB that a second active row is rejected
- [x] 1.3 Apply on the dev DB (`doc-service --migrate-only`) and confirm the tables in `project_db_migration` and `\d kb.deployment_*`

## 2. Step plans and the store

- [x] 2.1 `server/api/deployment/plan.go`: build and deploy step plans (keys, args, order) per D3
- [x] 2.2 `server/api/deployment/store.go`: create a run with all steps `pending`, claim the oldest queued deployment (`FOR UPDATE SKIP LOCKED`), start/finish steps, append capped output (256 KB tail + `output_truncated`), finish a run, list recent runs, read one run with steps; sqlmock tests
- [x] 2.3 Marker parser: `::step::<key>[::<arg>]` → step transition, other lines → output; unit tests for the D3 rules (success, failure mid-run, failure before any marker, unknown marker appended)
- [x] 2.4 Abandoned-run detection (dead `runner_pid` on the Mac; inactive `chenweb-deploy` or a queued row older than 30 s on the box) marking the run and its running step `failed`; tests

## 3. deploy-runner command

- [x] 3.1 `server/cmd/deploy-runner/main.go` with `build --build-id <id>` and `deploy` subcommands; load `.env`, `databaseutil.InitDB`, `CreateDefaultLogger`; no goose
- [x] 3.2 Run the script with combined output through the marker parser; flush output to the DB at most once a second; record exit status per D3
- [x] 3.3 `build`: run `shell_server_build_rcp.sh` with the row's name/target/os/description and `BUILD_NUMBER`; on success store `archive_name`
- [x] 3.4 `deploy`: claim the queued row, run `shell_server_deploy.sh` with `RELAY_ARCHIVE=<archive_name>`; exit cleanly when nothing is queued
- [x] 3.5 Test with a fake script that prints markers and fails at a chosen step (sqlmock or test DB with synthetic rows only)

## 4. Scripts

- [x] 4.1 `shell_server_build_rcp.sh`: `step()` helper printing markers; markers for check, documents, pdf-parser, archive, upload; set and export `BUILD_NUMBER` before calling mise; archive name `chenweb-deploy-<build_number>.tar.gz`; print the archive name for the runner
- [x] 4.2 `mise.toml` `build-server-linux`: markers for web, each `binary::<name>`, stage; add `deploy-runner:./server/cmd/deploy-runner` to the binary list (no build-info ldflags needed)
- [x] 4.3 `shell_server_deploy.sh`: markers for check, fetch, extract, documents, pdf-parser; drop the root check; `sudo -n /bin/systemctl` for pdf-parser; `--chown` only as root; `check` verifies `sudo -n /bin/systemctl status pdf-parser`; honour `RELAY_ARCHIVE`
- [x] 4.4 `scripts/deploy-server-china.sh`: markers for migrations, prompts, configs and each `binary::<name>`; add `deploy-runner` as an install-only binary
- [x] 4.5 Go test that extracts every `::step::` key from the four scripts and compares it with the plans
- [x] 4.6 `bash -n` all four scripts; `DRY_RUN=1` run of `deploy-server-china.sh` against a local staged directory to see the markers

## 5. API

- [x] 5.1 `[deployment] role` in the config loader; `role` returned by `GET /status`
- [x] 5.2 Handlers in `server/api/deployment/handler.go` for the D9 endpoints with the admin/owner check; register in `server/api/routes.go`
- [x] 5.3 `POST /builds`: insert row (new build number, requester), start the runner detached (`Setsid`), record `runner_host`/`runner_pid`; 409 on an active build; 400 when role ≠ build
- [x] 5.4 `GET /next-archive`: SSH to the relay (`bash -c` for the tcsh login shell), newest `*-deploy-*.tar.gz` with size and time, build number parsed from the name
- [x] 5.5 `POST /deployments`: insert queued row with archive name, build number and `previous_build_number` (`buildinfo.CurrentProcess("server")`), run `sudo -n /bin/systemctl start chenweb-deploy`; on error mark the row failed
- [x] 5.6 `GET /processes` (process table from D6; `systemctl show` on the box; `buildinfo` build numbers) and `POST /processes/:name/restart` (allowlist, 409 during a deployment, delayed restart for `chenweb`, logged with user and unit)
- [x] 5.7 Handler tests for the 400/403/409 cases and the restart allowlist (command runner injected, never a real `systemctl`)

## 6. System Monitor source filter

- [x] 6.1 `systemmonitor.List`: optional `source` query parameter returning only that source (400 for an unknown id); test
- [x] 6.2 `system-monitor-view.svelte`: send `source=<sourceId>` when embedded; check the Doc Processor dashboard's pdf-parser window still works

## 7. Page

- [x] 7.1 `web/src/lib/services/deploymentService.ts` for the D9 endpoints
- [x] 7.2 `deployment-view.svelte`: action section (Build, or Deploy with archive confirmation), step list with status icons, durations, expandable output, failed step opened with the run error; polling every 2 s while active, 10 s otherwise; "server restarting" on failed requests during a deploy
- [x] 7.3 History section; selecting a run shows its steps
- [x] 7.4 Processes section: state, PID, uptime, build number, Restart with confirmation (box only), reconnecting state after a `chenweb` restart, embedded `SystemMonitorView` per process
- [x] 7.5 Menu entry `sysadmin-system-deployment` (development page only) in `nav-rail.svelte`; route in `content-panel.svelte`
- [x] 7.6 `dep_*` and `nav_sysadmin_system_deployment` keys in `en.json` and `zh-cn.json` with real Chinese, including `dep_step_<key>` for every plan key
- [x] 7.7 `bun run check` from `web/` (no new hard-coded text, message files in sync); sweep compiler warnings

## 8. Verification on the Mac

- [x] 8.1 `go test ./server/api/deployment/... ./server/cmd/deploy-runner/... ./server/api/systemmonitor/...` and `go vet`
- [x] 8.2 Set `[deployment] role = "build"` locally; run a full build from the page; confirm the steps, the build row, the archive on the relay and `kb.build_activities` rows with the same build number
- [x] 8.3 Force a failure (e.g. a temporary compile error) and confirm the failed step, skipped steps, no upload and the `failed` row; then revert
- [x] 8.4 Confirm a build survives an `air` restart and the page picks it up again
- [x] 8.5 Browser check of the page in English and Chinese, light and dark (`webapp-testing`)
  (Done through a temporary preview route and an auth-stubbed API harness, both removed: no test login exists. Build branch against real data; deploy branch against mocked responses.)

## 9. China box rollout (needs the user: root password or root SSH key)

- [x] 9.1 Release this change once with the manual procedure; confirm the tables exist on the box and `deploy-runner-linux` is installed
  (2026-11-16: build 20261116122029 deployed as root the old way, then again by hand as `gui` with the new script; every step passed, including migrations.)
- [x] 9.2 Read the existing sudoers lines (exact `/bin/systemctl <verb> <unit>` entries; `pdf-parser` missing) and `PROJECT_WORKSPACE_DIR` (in the box's `.env`); write `scripts/chenweb-deploy.service` (User=gui, Type=simple, chenweb's EnvironmentFile and `SHARED_LIB_CONFIG_DIR`)
- [ ] 9.3 Root setup: install the unit, add `pdf-parser` (start/stop/restart/status) and `start chenweb-deploy` to `/etc/sudoers.d/gui-chenweb-services`, `visudo -c`, `daemon-reload`; `chown -R gui:gui` the directories earlier root deploys created (`ChenWeb/chenweb-deploy`, `ChenWeb/deploy_staging`, `~/Workspace/deploy`)
  (2026-11-16: sudoers for pdf-parser and the chown are done; the by-hand `gui` deploy passed. Unit install and `start chenweb-deploy` not yet confirmed.)
- [x] 9.4 Authorize `gui`'s SSH key on the relay; test `ssh -p 8896 cding@47.189.245.217 true` as `gui`
  (2026-11-16: `ssh -o BatchMode=yes` as `gui` succeeded.)
- [ ] 9.5 Set `[deployment] role = "deploy"` in the box's `config.local.toml`; restart `chenweb`
- [ ] 9.6 Restart ChenWeb from its own page and confirm the page reloads when it is back
- [ ] 9.7 First button deploy, watched against `journalctl -u chenweb-deploy`; confirm the row, the steps across the `chenweb` restart, and the pdf-parser step
- [ ] 9.8 Restart one worker (e.g. `parser-result-converter`) from the page

## 10. Docs and catalogue

- [x] 10.1 Update the production runbook `KnowledgeStore/doc-repo/devdocs/202609/2026090701-devdoc-deploy-start-system-onto.md`: one-time setup, button workflow, running the scripts as `gui`, the new archive name
- [x] 10.2 New devdoc for the Deployment page with the `create-devdoc` skill (runner, markers, tables, failure handling)
- [x] 10.3 Register `system-monitor-view.svelte` (embedded log window with `sourceId`) with the `create-reusable-module` skill; run the catalogue check
  (It was already catalogued as `20261115-01`; updated its row and doc to 1.2.0 instead.)
- [x] 10.4 Answer the workspace "Coding Best Practice" questions in the final summary; commit ChenWeb and KnowledgeStore changes separately with `jj commit <paths>`
