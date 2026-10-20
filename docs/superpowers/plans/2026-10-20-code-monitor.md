# Code Monitor Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a development System Admin page with daily line and frontend/backend file counts per configured ChenWeb package.

**Architecture:** A focused `server/api/codemonitor` package owns tracked-file scanning, daily collection, persistence, range queries, and the read handler. The server starts a timezone-aware wall-clock worker after migrations. The home3 System Admin UI calls that API and renders the requested three ECharts mix-line-bar charts per configured package.

**Tech Stack:** Go, PostgreSQL Goose migrations, Echo, Svelte 5, TypeScript, ECharts, Paraglide.

---

## File map

- Create `server/api/codemonitor/collector.go` for tracked-file scanning and metric calculation.
- Create `server/api/codemonitor/collector_test.go` for tracked-file filtering, line counting, and extension classification.
- Create `server/api/codemonitor/store.go` and tests for idempotent writes and date-range reads.
- Create `server/api/codemonitor/scheduler.go` and tests for the next 2:00 am run in the workspace timezone.
- Create `server/api/codemonitor/handler.go` and tests for range validation and JSON response shape.
- Create `project_migrations/20261020000001_create_kb_code_monitor.sql` for the daily snapshot table and indexes.
- Modify `server/cmd/config/config.go` to expose configured paths and their config directory.
- Modify `server/cmd/deepdoc/main.go` to start the daily worker after migrations.
- Modify `server/api/routes.go` to register the authenticated read route.
- Modify `web/src/lib/components/home3/nav-rail.svelte` and `content-panel.svelte` to add Code Monitor beneath System and render it.
- Create `web/src/lib/components/home3/code-monitor-client.ts` and `.test.ts` for the response contract and range query.
- Create `web/src/lib/components/home3/code-monitor-view.svelte` for range controls, empty/loading/error states, and three charts per package.
- Modify `web/messages/en.json` and `web/messages/zh-cn.json` with all visible labels and states.
- Create `docs/features/code-monitor.md` describing config, collection schedule, and metrics.

## Chunk 1: Collection and persistence

### Task 1: Scanner and metric definitions

**Files:**
- Create: `server/api/codemonitor/collector.go`
- Test: `server/api/codemonitor/collector_test.go`

- [x] Write tests proving the scanner counts tracked files only, counts physical lines including blank/final unterminated lines, returns zero lines for empty files, and classifies frontend/backend extensions.
- [x] Run `cd ChenWeb/server && go test ./api/codemonitor -run 'Test(Scan|Count|Classify)' -count=1` and confirm the expected missing-symbol failures.
- [x] Implement the smallest scanner using `git ls-files -z -- <package-path>` from the configured repo root; reject paths escaping the repo and skip unknown extensions.
- [x] Document the exact frontend and backend extension sets beside the classifier in `collector.go`.
- [x] Re-run the targeted tests and confirm they pass.

### Task 2: Snapshot table and store

**Files:**
- Create: `project_migrations/20261020000001_create_kb_code_monitor.sql`
- Create: `server/api/codemonitor/store.go`
- Test: `server/api/codemonitor/store_test.go`

- [x] Write store tests for package/date upsert idempotency and inclusive start/end date filtering.
- [x] Run the focused Go store tests and confirm the expected failures.
- [x] Add `kb.code_monitor` with package path, collection date, total lines, frontend/backend file counts, collected timestamp, and a unique key on package/date; add an index supporting date-range reads.
- [x] Implement parameterized UPSERT and range-read methods; keep SQL schema and tests aligned.
- [x] Re-run focused store tests; validate the migration's Up/Down contract using the repository's migration test conventions.

### Task 3: Config and daily wall-clock worker

**Files:**
- Modify: `server/cmd/config/config.go`
- Create: `server/api/codemonitor/scheduler.go`
- Test: `server/api/codemonitor/scheduler_test.go`

- [x] Write tests for next-run calculation before, at, and after 2:00 am, including a daylight-saving transition in `America/Chicago`.
- [x] Run the scheduler tests and confirm they fail before implementation.
- [x] Add `frontend.code-packages-to-monitor` as a string-slice config setting. The server loads `ChenWeb/config.toml` and merges `ChenWeb/config.local.toml`; expose `filepath.Dir(appConfigViper.ConfigFileUsed())` as the ChenWeb repo root for resolving the repo-relative package paths.
- [x] Implement a cancellable worker that computes the next 2:00 am in the workspace timezone, collects each configured path, logs failures, and continues with other packages. Upserts make retries for one local date idempotent.
- [x] Re-run config/worker tests and confirm they pass.
- [x] Configure the current workspace's frontend and backend directories in `ChenWeb/config.local.toml` under `[frontend]` using the repo-relative paths `web/src` and `server`.

## Chunk 2: Read API and startup integration

### Task 4: Range handler and route

**Files:**
- Create: `server/api/codemonitor/handler.go`
- Test: `server/api/codemonitor/handler_test.go`
- Modify: `server/api/routes.go`

- [x] Write handler tests for all six range names, invalid ranges, workspace-local boundary calculations, and grouped response JSON.
- [x] Run the handler tests and confirm the expected failures.
- [x] Implement ranges: today; yesterday; today plus prior six days; trailing 30 dates including today; trailing 6 and 12 calendar months, clamping start day to the last day of the target month. Both ends are inclusive.
- [x] Register `GET /api/v1/code-monitor?range=<range>` behind the existing authenticated API middleware.
- [x] Re-run focused API tests and confirm they pass.

### Task 5: Start worker after database setup

**Files:**
- Modify: `server/cmd/deepdoc/main.go`
- Test: `server/cmd/config/config_migration_test.go` only if startup/config coverage is needed.

- [x] Start the code monitor worker only after project migrations complete and the project DB is available.
- [x] Use the configured workspace timezone and a logger created with a unique `CreateDefaultLogger` location.
- [x] Confirm worker cancellation uses the server context and startup logs the configured package count and timezone.

## Chunk 3: Home3 page

### Task 6: Typed API client

**Files:**
- Create: `web/src/lib/components/home3/code-monitor-client.ts`
- Test: `web/src/lib/components/home3/code-monitor-client.test.ts`

- [x] Write tests for the requested range query, response parsing, and HTTP errors.
- [x] Run the targeted Bun test and confirm it fails before implementation.
- [x] Implement the typed client with daily rows grouped by package.
- [x] Run the targeted Bun test and confirm it passes.

### Task 7: Navigation, view, and translations

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte`
- Modify: `web/src/lib/components/home3/content-panel.svelte`
- Create: `web/src/lib/components/home3/code-monitor-view.svelte`
- Modify: `web/messages/en.json`
- Modify: `web/messages/zh-cn.json`

- [x] Add the localized Code Monitor menu entry beneath System and render its view in the home3 content panel.
- [x] Build the view with six localized range controls, one package section per response package, and three mix-line-bar ECharts charts per package. Keep text selectable at the view root.
- [x] Add localized loading, error, empty, chart-title, and date-range text to both message catalogs; use no user-visible literals in Svelte markup.
- [x] Run `cd ChenWeb/web && bun run check` and fix only feature-related issues.

## Chunk 4: Documentation and final verification

### Task 8: Feature documentation and focused verification

**Files:**
- Create: `docs/features/code-monitor.md`

- [x] Document the `frontend.code-packages-to-monitor` setting, tracked source-file counting rules, timezone, daily timing, and range semantics.
- [x] Run focused Go tests: `cd ChenWeb/server && go test ./api/codemonitor -count=1`.
- [x] Run targeted frontend client tests and `cd ChenWeb/web && bun run check`.
- [x] Review the complete diff, ensure the pre-existing metrics-handler edits are untouched and excluded from commits, and verify `jj log` has only expected linear feature commits.

## User-requested revision: projects and deltas

### Task 9: Project model, deltas, and empty-table initialization

**Files:**
- Modify: `server/cmd/config/config.go`, `config.local.toml`, `server/api/codemonitor/monitor.go`, `server/api/codemonitor/monitor_test.go`, `server/api/routes.go`, `server/cmd/deepdoc/main.go`
- Create: next Goose migration to rename `kb.code_monitor.package_path` to `project_path`

- [x] Replace `frontend.code-packages-to-monitor` with `frontend.projects-to-monitor`, treating each workspace-relative project directory name as a project group. Count frontend source files under that project's `web/src` and backend source files under its `server` directory.
- [x] Add delta fields to each API row; each delta is the current value minus the previous recorded row for that project, with zero for the first recorded row, including previous rows outside the selected range.
- [x] If the table has no rows at startup, collect once using today's workspace-local date and a synthetic 2:00 am collection timestamp.
- [x] Keep scheduled subsequent collections at 2:00 am and preserve the existing date ranges.
- [x] Write focused tests before code, verify the failing behavior, implement, and verify green.

### Task 10: Project charts with total and delta bars

**Files:**
- Modify: `web/src/lib/components/home3/code-monitor-client.ts`, `.test.ts`, `code-monitor-view.svelte`, `web/messages/en.json`, `web/messages/zh-cn.json`

- [x] Change the response model and page grouping from packages to projects.
- [x] Render two bar series in each metric chart, Total on the left axis and Delta on the right axis.
- [x] Add localized Total/Delta labels and update chart accessibility descriptions.
- [x] Verify client tests and `bun run check`.

### Task 11: Updated documentation and final review

**Files:**
- Modify: `docs/superpowers/specs/2026-10-20-code-monitor-design.md`, `docs/features/code-monitor.md`, and the Code Monitor KnowledgeStore devdoc.

- [x] Record the revised config key, workspace-relative project naming, fixed frontend/backend directories, grouping, delta definition, dual-axis charts, and empty-table initial snapshot behavior.
- [x] Run focused backend and frontend checks; review the final diff and commit only feature files.
