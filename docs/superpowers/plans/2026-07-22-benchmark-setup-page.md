# Benchmark Setup Page Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a browser-driven Benchmark Setup page under `home3 > System Admin > Benchmark > Setup` that replaces the CLI-only setup flow with persisted admin config, state inspection, and browser-triggered benchmark operations through `validate`, `run`, `report`, and `compare`.

**Architecture:** Add a benchmark-admin backend layer that persists setup config in the database, inspects current readiness, and runs long operations as tracked jobs. Add a `home3` admin view and typed client that render step state from the server, let users edit config, and trigger one-step or full-flow operations without shell access.

**Tech Stack:** Go, Echo, PostgreSQL, existing `server/api/doc-benchmark` package, existing admin handler patterns, Svelte 5, TypeScript.

---

## File Map

### Backend

- Create: `server/api/appdatastores/table-doc-benchmark-admin-config.go`
- Create: `server/api/appdatastores/table-doc-benchmark-admin-jobs.go`
- Create: `server/api/doc-benchmark-admin/types.go`
- Create: `server/api/doc-benchmark-admin/store.go`
- Create: `server/api/doc-benchmark-admin/config.go`
- Create: `server/api/doc-benchmark-admin/inspect.go`
- Create: `server/api/doc-benchmark-admin/jobs.go`
- Create: `server/api/doc-benchmark-admin/service.go`
- Create: `server/api/doc-benchmark-admin/service_test.go`
- Create: `server/api/doc-benchmark-admin/inspect_test.go`
- Create: `server/api/doc-benchmark-admin/jobs_test.go`
- Create: `server/api/docbenchmarkadminhandler/handler.go`
- Create: `server/api/docbenchmarkadminhandler/handler_test.go`
- Modify: `server/api/database/createtables.go`
- Modify: the main API route registration file(s) that currently wire `pageconfighandler`, `llmadminhandler`, and other admin handlers

### Frontend

- Create: `web/src/lib/services/docBenchmarkAdminService.ts`
- Create: `web/src/lib/services/docBenchmarkAdminService.test.ts`
- Create: `web/src/lib/components/home3/benchmark-setup-view.svelte`
- Create: `web/src/lib/components/home3/benchmark-step-card.svelte`
- Create: `web/src/lib/components/home3/benchmark-job-list.svelte`
- Modify: `web/src/lib/components/home3/content-panel.svelte`
- Modify: `web/src/lib/components/home3/nav-rail.svelte`

### Docs

- Modify: `docs/doc-processor-benchmark-operations.md`
- Modify: `KnowledgeStore/doc-repo/misc/202607/2026072101-misc-benchmark-user-manual.md` only if the implementation intentionally changes the user flow

---

## Chunk 1: Backend Persistence and Domain Model

### Task 1: Define persisted benchmark-admin config and job records

**Files:**
- Create: `server/api/appdatastores/table-doc-benchmark-admin-config.go`
- Create: `server/api/appdatastores/table-doc-benchmark-admin-jobs.go`
- Modify: `server/api/database/createtables.go`
- Test: `server/api/doc-benchmark-admin/service_test.go`

- [ ] Add failing Go tests that describe the stored config shape and job lifecycle.
- [ ] Run the focused tests and confirm they fail because the store/table code does not exist yet.
- [ ] Add table creation helpers for:
  - one row per benchmark admin config scope;
  - append-only or update-safe benchmark admin jobs with status, timestamps, payload, and result summary.
- [ ] Wire the new tables into `server/api/database/createtables.go`.
- [ ] Run the focused tests and make them pass.
- [ ] Commit the backend persistence slice.

Run:
```sh
go test ./server/api/doc-benchmark-admin -run 'TestConfigStore|TestJobStore' -count=1
```

### Task 2: Define shared benchmark-admin types

**Files:**
- Create: `server/api/doc-benchmark-admin/types.go`
- Test: `server/api/doc-benchmark-admin/service_test.go`

- [ ] Add failing tests for config DTOs, setup-state DTOs, step IDs, and job status serialization.
- [ ] Define typed request/response structs for:
  - saved config;
  - step inspection result;
  - setup-state response;
  - action request payloads;
  - job summary and job detail.
- [ ] Keep the step enum aligned with the agreed browser flow:
  - runtime config
  - roots
  - working copy
  - validate
  - run
  - report
  - compare
- [ ] Re-run tests and confirm the DTO contract is stable.
- [ ] Commit the domain types.

Run:
```sh
go test ./server/api/doc-benchmark-admin -run 'TestTypes|TestStepIDs' -count=1
```

---

## Chunk 2: Backend Inspection and Orchestration Service

### Task 3: Build the setup-state inspection service

**Files:**
- Create: `server/api/doc-benchmark-admin/store.go`
- Create: `server/api/doc-benchmark-admin/config.go`
- Create: `server/api/doc-benchmark-admin/inspect.go`
- Create: `server/api/doc-benchmark-admin/inspect_test.go`

- [ ] Write failing tests for inspection of:
  - experiment file existence;
  - model ref presence;
  - artifact/work/evidence root existence and writability;
  - clean vs dirty working copy;
  - last successful validate/run/report/compare state when persisted data is present.
- [ ] Implement a small store layer that reads and writes config and recent job/run metadata.
- [ ] Implement inspection helpers that classify each step as `completed`, `ready`, `blocked`, `running`, `failed`, or `unknown`.
- [ ] Make the service attach detected values and timestamps where they can be inferred safely.
- [ ] Re-run tests until the setup-state logic passes.
- [ ] Commit the inspection slice.

Run:
```sh
go test ./server/api/doc-benchmark-admin -run 'TestInspect|TestSetupState' -count=1
```

### Task 4: Build the benchmark-admin action service

**Files:**
- Create: `server/api/doc-benchmark-admin/service.go`
- Create: `server/api/doc-benchmark-admin/jobs.go`
- Create: `server/api/doc-benchmark-admin/jobs_test.go`
- Create: `server/api/doc-benchmark-admin/service_test.go`
- Reference: `server/api/doc-benchmark/application.go`
- Reference: `server/api/doc-benchmark/report_service.go`
- Reference: `server/api/doc-benchmark/experiment.go`

- [ ] Write failing tests for:
  - saving config;
  - running one step;
  - running “next unfinished step”;
  - creating tracked jobs for validate/run/report/compare;
  - refusing actions when required config is missing.
- [ ] Implement a benchmark-admin service that:
  - persists config;
  - delegates readiness logic to the inspection layer;
  - creates job rows for long-running actions;
  - records outputs like experiment ID and generated report paths.
- [ ] Reuse existing `doc-benchmark` application logic internally instead of shelling out to a raw CLI command.
- [ ] Keep fast state checks synchronous and job-backed operations asynchronous.
- [ ] Re-run focused tests until service orchestration passes.
- [ ] Commit the service slice.

Run:
```sh
go test ./server/api/doc-benchmark-admin -run 'TestSaveConfig|TestRunStep|TestRunNext|TestCreateJob' -count=1
```

### Task 5: Decide and document the job execution model

**Files:**
- Modify: `server/api/doc-benchmark-admin/service.go`
- Modify: `docs/doc-processor-benchmark-operations.md`
- Test: `server/api/doc-benchmark-admin/jobs_test.go`

- [ ] Add a failing test that proves job state moves through `queued` → `running` → terminal states.
- [ ] Implement the minimal job runner pattern used by this feature.
- [ ] Ensure job results persist enough data for page refresh and later report viewing.
- [ ] Update the benchmark operations doc with the browser-driven job behavior.
- [ ] Re-run the job tests and confirm terminal-state persistence works.
- [ ] Commit the job execution slice.

Run:
```sh
go test ./server/api/doc-benchmark-admin -run 'TestJobLifecycle' -count=1
```

---

## Chunk 3: Backend HTTP Handler and Routes

### Task 6: Add admin HTTP endpoints

**Files:**
- Create: `server/api/docbenchmarkadminhandler/handler.go`
- Create: `server/api/docbenchmarkadminhandler/handler_test.go`
- Modify: the central route registration file(s) that mount admin handlers

- [ ] Write failing handler tests for:
  - `GET /api/v1/admin/benchmark/config`
  - `PUT /api/v1/admin/benchmark/config`
  - `GET /api/v1/admin/benchmark/setup-state`
  - `POST /api/v1/admin/benchmark/steps/:stepId/run`
  - `POST /api/v1/admin/benchmark/run-next`
  - `GET /api/v1/admin/benchmark/jobs`
  - `GET /api/v1/admin/benchmark/jobs/:jobId`
- [ ] Implement the handler with the same auth/error style used by existing admin handlers.
- [ ] Register the routes under the admin API namespace.
- [ ] Return stable JSON payloads designed for frontend polling and refresh.
- [ ] Re-run handler tests until they pass.
- [ ] Commit the handler and routes slice.

Run:
```sh
go test ./server/api/docbenchmarkadminhandler -count=1
```

### Task 7: Add one end-to-end backend integration test

**Files:**
- Modify: `server/api/docbenchmarkadminhandler/handler_test.go`
- Modify: `server/api/doc-benchmark-admin/service_test.go`

- [ ] Add one failing integration-style test that saves config, requests setup-state, starts an action, and reads back job state.
- [ ] Implement only the missing glue needed to make that test pass.
- [ ] Re-run both handler and service suites together.
- [ ] Commit the backend vertical slice.

Run:
```sh
go test ./server/api/doc-benchmark-admin ./server/api/docbenchmarkadminhandler -count=1
```

---

## Chunk 4: Frontend Service Layer

### Task 8: Add a typed benchmark admin client

**Files:**
- Create: `web/src/lib/services/docBenchmarkAdminService.ts`
- Create: `web/src/lib/services/docBenchmarkAdminService.test.ts`

- [ ] Write failing frontend service tests for:
  - loading config;
  - saving config;
  - loading setup-state;
  - starting a step;
  - starting “run next”;
  - listing jobs.
- [ ] Implement a typed fetch client that mirrors the new admin endpoints.
- [ ] Reuse the error-handling style used by existing `pageConfigService.ts` and `skillService.ts`.
- [ ] Re-run the service tests until they pass.
- [ ] Commit the frontend service slice.

Run:
```sh
pnpm test -- docBenchmarkAdminService
```

If this repo uses a different web test runner command, replace the command with the project-standard equivalent and record it in the commit note.

---

## Chunk 5: Frontend Benchmark Setup UI

### Task 9: Build the benchmark setup page view

**Files:**
- Create: `web/src/lib/components/home3/benchmark-setup-view.svelte`
- Create: `web/src/lib/components/home3/benchmark-step-card.svelte`
- Create: `web/src/lib/components/home3/benchmark-job-list.svelte`

- [ ] Write or scaffold component tests for the page state model if the repo already tests comparable home3 views.
- [ ] Build the top-level layout with:
  - benchmark config editor;
  - setup progress lane;
  - operations lane;
  - recent activity/job list.
- [ ] Show completed steps compactly and unfinished/failed steps expanded.
- [ ] Show detected values, timestamps, and server explanations per step.
- [ ] Add actions for:
  - save config;
  - refresh status;
  - run individual step;
  - run next unfinished step.
- [ ] Poll active jobs until terminal state, then refresh setup-state.
- [ ] Re-run component tests if present, otherwise do a local browser sanity pass later in the plan.
- [ ] Commit the page UI slice.

### Task 10: Wire the page into `home3` navigation

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte`
- Modify: `web/src/lib/components/home3/content-panel.svelte`

- [ ] Add a new System Admin menu child path for benchmark setup.
- [ ] Render `BenchmarkSetupView` from `content-panel.svelte`.
- [ ] Keep labels consistent with the agreed IA:
  - `System Admin`
  - `Benchmark`
  - `Setup`
- [ ] Re-run typecheck/build and fix any route/view wiring issues.
- [ ] Commit the navigation slice.

Run:
```sh
pnpm check
```

If the repo standard is `bun`, `npm`, or `vite` directly, use the existing ChenWeb frontend check command instead.

---

## Chunk 6: Verification, Documentation, and Cleanup

### Task 11: Verify the end-to-end flow manually

**Files:**
- No file changes required unless bugs are found

- [ ] Start the ChenWeb app locally.
- [ ] Open `home3 > System Admin > Benchmark > Setup`.
- [ ] Save benchmark config through the UI.
- [ ] Confirm setup-state reflects completed, missing, and blocked steps correctly.
- [ ] Trigger:
  - one direct step action;
  - one “run next unfinished step” action;
  - one job-backed action;
  - one refresh after completion.
- [ ] Confirm completed timestamps and last-known experiment/report outputs render correctly.
- [ ] If issues appear, fix them in the smallest responsible layer and re-run the checks.

Suggested verification commands:
```sh
go test ./server/api/doc-benchmark-admin ./server/api/docbenchmarkadminhandler -count=1
pnpm check
```

### Task 12: Finish docs and handoff notes

**Files:**
- Modify: `docs/doc-processor-benchmark-operations.md`
- Modify: `KnowledgeStore/doc-repo/misc/202607/2026072101-misc-benchmark-user-manual.md` if browser behavior changes the operator workflow materially

- [ ] Update the ops doc so browser users know:
  - what config is now stored in-app;
  - which actions run as jobs;
  - which CLI paths still exist for fallback/debugging.
- [ ] Update the benchmark manual only if the implemented browser flow changes the user guidance.
- [ ] Record any intentionally deferred items:
  - richer logs/streaming output;
  - job cancellation;
  - multiple saved benchmark profiles;
  - deeper result inspection.
- [ ] Commit the doc updates.

---

## Final Verification Checklist

- [ ] `go test ./server/api/doc-benchmark-admin ./server/api/docbenchmarkadminhandler -count=1`
- [ ] `go test ./server/api/doc-benchmark -count=1` for regression confidence in reused benchmark code
- [ ] frontend service/component tests for the new benchmark page
- [ ] frontend typecheck/build command passes
- [ ] manual browser test confirms config save, status refresh, job polling, and step transitions
- [ ] `jj status` shows only intended files

---

## Notes for Execution

- Keep the first version narrow: one benchmark setup page, one persisted config scope, one job model, and the agreed step flow.
- Do not add shell-command execution from the browser. The server should own benchmark operations through internal Go code.
- Prefer direct reuse of `server/api/doc-benchmark` logic over introducing a second benchmark engine.
- Treat unknown detection conservatively. If the backend cannot safely prove completion, return `unknown` with an explanation instead of guessing.
- Preserve existing `home3` visual language rather than introducing a separate design system.

Plan complete and saved to `docs/superpowers/plans/2026-07-22-benchmark-setup-page.md`. Ready to execute?
