# LLM Activities Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build ChenWeb's provider-agnostic, account-based LLM accounts, call logging, reconciliation, and `home3` admin/dashboard surfaces with DeepSeek reconciliation first.

**Architecture:** Persist account/profile/report metadata in PostgreSQL, archive full prompt/response bodies in day-partitioned files, and capture all LLM traffic through `shared/go/api/llm`. Make ChenWeb's database the source of truth after a one-time `.models.toml` import, then layer provider reconciliation and `home3` views on top.

**Tech Stack:** Go 1.25, PostgreSQL, goose SQL migrations, shared `llm` package, Echo handlers, Svelte 5, Bun/Vite tests, gzip file archives.

---

## File Map

### Shared library

- Modify: `shared/go/api/llm/types.go`
- Modify: `shared/go/api/llm/client.go`
- Modify: `shared/go/api/llm/openai_client.go`
- Modify: `shared/go/api/llm/openai.go`
- Create: `shared/go/api/llm/account_profile.go`
- Create: `shared/go/api/llm/account_profile_test.go`
- Create: `shared/go/api/llm/usage_capture.go`
- Create: `shared/go/api/llm/usage_capture_test.go`
- Create: `shared/go/api/llm/archive_store.go`
- Create: `shared/go/api/llm/archive_store_test.go`

### ChenWeb backend

- Modify: `ChenWeb/config.toml`
- Modify: `ChenWeb/server/cmd/config/config.go`
- Modify: `ChenWeb/server/api/routes.go`
- Create: `ChenWeb/project_migrations/20260619000001_create_llm_activity_tables.sql`
- Create: `ChenWeb/server/api/llmadminhandler/handler.go`
- Create: `ChenWeb/server/api/llmadminhandler/handler_test.go`
- Create: `ChenWeb/server/api/llmadminhandler/store.go`
- Create: `ChenWeb/server/api/llmadminhandler/store_test.go`
- Create: `ChenWeb/server/api/llmreporthandler/handler.go`
- Create: `ChenWeb/server/api/llmreporthandler/handler_test.go`
- Create: `ChenWeb/server/api/llmreporthandler/store.go`
- Create: `ChenWeb/server/api/llmreporthandler/store_test.go`
- Create: `ChenWeb/server/api/llmreconcile/reconcile.go`
- Create: `ChenWeb/server/api/llmreconcile/reconcile_test.go`
- Create: `ChenWeb/server/api/llmreconcile/deepseek.go`
- Create: `ChenWeb/server/api/llmreconcile/deepseek_test.go`
- Create: `ChenWeb/server/api/llmjobs/jobs.go`
- Create: `ChenWeb/server/api/llmjobs/jobs_test.go`
- Create: `ChenWeb/server/api/llmimport/models_toml.go`
- Create: `ChenWeb/server/api/llmimport/models_toml_test.go`

### ChenWeb frontend

- Modify: `ChenWeb/web/src/lib/components/home3/nav-rail.svelte`
- Modify: `ChenWeb/web/src/lib/components/home3/content-panel.svelte`
- Create: `ChenWeb/web/src/lib/components/home3/llm-activities-view.svelte`
- Create: `ChenWeb/web/src/lib/components/home3/llm-accounts-view.svelte`
- Create: `ChenWeb/web/src/lib/components/home3/llm-activities-client.ts`
- Create: `ChenWeb/web/src/lib/components/home3/llm-activities-client.test.ts`
- Create: `ChenWeb/web/src/lib/components/home3/llm-accounts-client.ts`
- Create: `ChenWeb/web/src/lib/components/home3/llm-accounts-client.test.ts`

### Docs

- Modify: `ChenWeb/docs/superpowers/specs/2026-06-19-llm-activities-design.md`
- Create: `ChenWeb/docs/llm-accounts.md`

## Current Progress Notes

Completed so far:

- shared capture foundation, including archived bodies and default sink support
- LLM schema migration
- LLM config block
- `.models.toml` parser
- account admin API for list/create/preview/apply import
- reporting read APIs
- `home3` views for LLM Activities and LLM Accounts
- persistence sink for `shared/go/api/llm` call paths in `deepdoc`

Still pending:

- account/profile edit endpoints
- reconciliation jobs
- retention jobs
- broader capture rollout for `OpenAIJSONClient`-based paths

## Chunk 1: Schema And Shared Capture Foundation

### Task 1: Add failing tests for archive layout and retention metadata

**Files:**
- Create: `shared/go/api/llm/archive_store_test.go`
- Create: `shared/go/api/llm/usage_capture_test.go`

- [ ] **Step 1: Write the failing tests**

Cover:
- archive path generation under `Data/llm-logs/YYYY/YYYY-MM/YYYY-MM-DD/account-<id>/bodies`
- gzip body write/read helpers
- usage event payload with prompt name, model, token counts, refs, and error message

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./api/llm -run 'TestArchive|TestUsageCapture'`
Expected: FAIL with missing types/functions

- [ ] **Step 3: Write minimal implementation**

Create `archive_store.go` and `usage_capture.go` with:
- archive root config struct
- deterministic day/account/event path builders
- gzip file writer/reader helpers
- usage event DTOs that the shared package can emit

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./api/llm -run 'TestArchive|TestUsageCapture'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/shared add go/api/llm/archive_store.go go/api/llm/archive_store_test.go go/api/llm/usage_capture.go go/api/llm/usage_capture_test.go
git -C /Users/cding/Workspace/shared commit -m "feat: add llm archive and usage capture primitives"
```

### Task 2: Add account/profile resolution types to shared `llm`

**Files:**
- Create: `shared/go/api/llm/account_profile.go`
- Create: `shared/go/api/llm/account_profile_test.go`
- Modify: `shared/go/api/llm/types.go`

- [ ] **Step 1: Write the failing tests**

Cover:
- resolving provider config from account/profile data
- carrying `account_id`, `profile_id`, `prompt_name`
- preserving backward-compatible plain provider usage where possible

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./api/llm -run 'TestAccountProfile|TestProviderConfig'`
Expected: FAIL with missing resolver/types

- [ ] **Step 3: Write minimal implementation**

Add:
- `Account`
- `AccountProfile`
- `ResolvedRequestContext`
- adapter-friendly config conversion helpers

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./api/llm -run 'TestAccountProfile|TestProviderConfig'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/shared add go/api/llm/types.go go/api/llm/account_profile.go go/api/llm/account_profile_test.go
git -C /Users/cding/Workspace/shared commit -m "feat: add llm account profile resolution types"
```

### Task 3: Create ChenWeb migration for LLM tables

**Files:**
- Create: `ChenWeb/project_migrations/20260619000001_create_llm_activity_tables.sql`

- [ ] **Step 1: Write a migration review checklist as comments in the plan task**

Include:
- account/profile foreign keys
- soft-disable friendly columns
- short-lived usage-event table
- authoritative daily report table
- balance snapshot table

- [ ] **Step 2: Write the migration**

Create tables:
- `llm_account`
- `llm_account_model_profile`
- `llm_usage_event`
- `llm_daily_account_report`
- `llm_balance_snapshot`

Include indexes on:
- `account_id`
- `workspace_day`
- `request_started_at`
- `profile_id`

- [ ] **Step 3: Run migration-focused verification**

Run: `go test ./server/cmd/config -run TestResolveMigrationDir`
Expected: PASS

Run: `rg -n "CREATE TABLE .*llm_" ChenWeb/project_migrations/20260619000001_create_llm_activity_tables.sql`
Expected: all five tables present

- [ ] **Step 4: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add project_migrations/20260619000001_create_llm_activity_tables.sql
git -C /Users/cding/Workspace/ChenWeb commit -m "feat: add llm activity schema"
```

## Chunk 2: Wire Shared `llm` Logging Into Real Calls

### Task 4: Add failing tests for non-stream call capture in `openai_client`

**Files:**
- Modify: `shared/go/api/llm/openai_client_test.go`
- Modify: `shared/go/api/llm/client_test.go`

- [ ] **Step 1: Write the failing tests**

Cover:
- successful completion logs input/output refs and token usage
- provider error logs error message
- prompt name is preserved

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./api/llm -run 'TestOpenAI.*Capture|TestClient.*Capture'`
Expected: FAIL because capture hooks do not exist

- [ ] **Step 3: Implement the minimal capture hooks**

Modify:
- `types.go` to include optional prompt name/capture context on request
- `client.go` / `openai_client.go` / `openai.go` to start, finish, and fail usage captures

Avoid broad refactors; wrap existing request execution paths.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./api/llm -run 'TestOpenAI.*Capture|TestClient.*Capture'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/shared add go/api/llm/types.go go/api/llm/client.go go/api/llm/openai_client.go go/api/llm/openai.go go/api/llm/openai_client_test.go go/api/llm/client_test.go
git -C /Users/cding/Workspace/shared commit -m "feat: capture llm usage for non-stream calls"
```

### Task 5: Add failing tests for streaming capture completion

**Files:**
- Modify: `shared/go/api/llm/openai_test.go`

- [ ] **Step 1: Write the failing tests**

Cover:
- stream final chunk usage updates the usage event
- stream errors persist partial metadata and error text

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./api/llm -run 'TestStream.*Capture'`
Expected: FAIL with missing stream capture behavior

- [ ] **Step 3: Implement minimal stream capture updates**

Ensure:
- one usage event per streamed request
- final usage lands when stream ends
- output body archive is written at end of stream

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./api/llm -run 'TestStream.*Capture'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/shared add go/api/llm/openai_test.go go/api/llm/openai_client.go
git -C /Users/cding/Workspace/shared commit -m "feat: capture llm usage for streaming calls"
```

## Chunk 3: ChenWeb Account Registry And TOML Import

### Task 6: Add failing tests for `.models.toml` import parsing

**Files:**
- Create: `ChenWeb/server/api/llmimport/models_toml_test.go`
- Create: `ChenWeb/server/api/llmimport/models_toml.go`

- [ ] **Step 1: Write the failing tests**

Cover:
- parsing one profile section into account/profile records
- deduplicating same-account profiles
- preserving timeout/thinking/rate-limit fields

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/api/llmimport -run TestModelsToml`
Expected: FAIL with missing package/functions

- [ ] **Step 3: Implement minimal importer**

Parse `ChenWeb/.models.toml` into:
- import DTOs
- account upsert requests
- profile upsert requests

Do not add file watching.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/api/llmimport -run TestModelsToml`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add server/api/llmimport/models_toml.go server/api/llmimport/models_toml_test.go
git -C /Users/cding/Workspace/ChenWeb commit -m "feat: add llm models toml importer"
```

### Task 7: Add failing tests for admin store and handlers

**Files:**
- Create: `ChenWeb/server/api/llmadminhandler/store.go`
- Create: `ChenWeb/server/api/llmadminhandler/store_test.go`
- Create: `ChenWeb/server/api/llmadminhandler/handler.go`
- Create: `ChenWeb/server/api/llmadminhandler/handler_test.go`
- Modify: `ChenWeb/server/api/routes.go`

- [ ] **Step 1: Write the failing tests**

Cover:
- create/list/update/disable account
- create/list/update profile
- API key write-only semantics
- import endpoint triggers one-time `.models.toml` import

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/api/llmadminhandler`
Expected: FAIL with missing store/handler

- [ ] **Step 3: Implement minimal store and handlers**

Add endpoints:
- `GET /api/v1/llm/accounts`
- `POST /api/v1/llm/accounts`
- `PUT /api/v1/llm/accounts/:id`
- `POST /api/v1/llm/accounts/:id/profiles`
- `PUT /api/v1/llm/accounts/:id/profiles/:profile_id`
- `POST /api/v1/llm/accounts/import-models-toml`
 - `POST /api/v1/llm/accounts/import-models-toml/apply`

Implementation note:

- This slice currently ships `GET /api/v1/llm/accounts`, `POST /api/v1/llm/accounts`, preview import, and apply import.
- Update/disable/profile-management endpoints remain follow-up work.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/api/llmadminhandler`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add server/api/llmadminhandler/store.go server/api/llmadminhandler/store_test.go server/api/llmadminhandler/handler.go server/api/llmadminhandler/handler_test.go server/api/routes.go
git -C /Users/cding/Workspace/ChenWeb commit -m "feat: add llm account admin api"
```

## Chunk 4: Reconciliation, Jobs, And Retention

### Task 8: Add config tests for LLM settings

**Files:**
- Modify: `ChenWeb/server/cmd/config/config.go`
- Create or Modify: `ChenWeb/server/cmd/config/config_migration_test.go`

- [ ] **Step 1: Write the failing tests**

Cover:
- reading `workspace_timezone`
- defaulting `usage_retention_days` to `30`
- resolving `archive_root`

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/cmd/config -run 'Test.*LLM|Test.*Config'`
Expected: FAIL because config fields are absent

- [ ] **Step 3: Implement minimal config support**

Update:
- config structs
- normalization/default logic

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/cmd/config -run 'Test.*LLM|Test.*Config'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add config.toml server/cmd/config/config.go server/cmd/config/config_migration_test.go
git -C /Users/cding/Workspace/ChenWeb commit -m "feat: add llm config settings"
```

### Task 9: Add failing tests for DeepSeek reconciliation and retention jobs

**Files:**
- Create: `ChenWeb/server/api/llmreconcile/reconcile.go`
- Create: `ChenWeb/server/api/llmreconcile/reconcile_test.go`
- Create: `ChenWeb/server/api/llmreconcile/deepseek.go`
- Create: `ChenWeb/server/api/llmreconcile/deepseek_test.go`
- Create: `ChenWeb/server/api/llmjobs/jobs.go`
- Create: `ChenWeb/server/api/llmjobs/jobs_test.go`

- [ ] **Step 1: Write the failing tests**

Cover:
- one account DeepSeek balance snapshot ingestion
- daily report generation for prior workspace day
- fallback reconciliation status when only local tokens exist
- retention job deletes `llm_usage_event` rows older than config threshold

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/api/llmreconcile ./server/api/llmjobs`
Expected: FAIL with missing packages/functions

- [ ] **Step 3: Implement minimal job/reconcile code**

Add:
- provider interface
- DeepSeek reconciler using balance endpoint data
- daily report writer
- retention cleanup job

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/api/llmreconcile ./server/api/llmjobs`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add server/api/llmreconcile/reconcile.go server/api/llmreconcile/reconcile_test.go server/api/llmreconcile/deepseek.go server/api/llmreconcile/deepseek_test.go server/api/llmjobs/jobs.go server/api/llmjobs/jobs_test.go
git -C /Users/cding/Workspace/ChenWeb commit -m "feat: add llm reconciliation and retention jobs"
```

## Chunk 5: Reporting APIs And Home3 UI

### Task 10: Add failing tests for report store and handlers

**Files:**
- Create: `ChenWeb/server/api/llmreporthandler/store.go`
- Create: `ChenWeb/server/api/llmreporthandler/store_test.go`
- Create: `ChenWeb/server/api/llmreporthandler/handler.go`
- Create: `ChenWeb/server/api/llmreporthandler/handler_test.go`
- Modify: `ChenWeb/server/api/routes.go`

- [ ] **Step 1: Write the failing tests**

Cover:
- list daily reports by day/account
- list recent usage events
- fetch one usage event with input/output refs

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/api/llmreporthandler`
Expected: FAIL with missing packages/functions

- [ ] **Step 3: Implement minimal report API**

Add endpoints:
- `GET /api/v1/llm/reports/daily`
- `GET /api/v1/llm/reports/daily/:account_id`
- `GET /api/v1/llm/usage-events`
- `GET /api/v1/llm/usage-events/:id`

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/api/llmreporthandler`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add server/api/llmreporthandler/store.go server/api/llmreporthandler/store_test.go server/api/llmreporthandler/handler.go server/api/llmreporthandler/handler_test.go server/api/routes.go
git -C /Users/cding/Workspace/ChenWeb commit -m "feat: add llm reporting api"
```

### Task 11: Add failing frontend tests for `LLM Accounts` and `LLM Activities`

**Files:**
- Create: `ChenWeb/web/src/lib/components/home3/llm-activities-client.ts`
- Create: `ChenWeb/web/src/lib/components/home3/llm-activities-client.test.ts`
- Create: `ChenWeb/web/src/lib/components/home3/llm-accounts-client.ts`
- Create: `ChenWeb/web/src/lib/components/home3/llm-accounts-client.test.ts`
- Modify: `ChenWeb/web/src/lib/components/home3/nav-rail.svelte`
- Modify: `ChenWeb/web/src/lib/components/home3/content-panel.svelte`
- Create: `ChenWeb/web/src/lib/components/home3/llm-activities-view.svelte`
- Create: `ChenWeb/web/src/lib/components/home3/llm-accounts-view.svelte`

- [ ] **Step 1: Write the failing tests**

Cover:
- nav exposes `Dashboard -> LLM Activities`
- nav exposes `System Admin -> LLM Accounts`
- report client parses daily report payloads
- accounts client parses account/profile payloads

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/cding/Workspace/ChenWeb/web && bun test`
Expected: FAIL for missing modules/routes

- [ ] **Step 3: Implement minimal clients and views**

Add:
- fetch wrappers
- `llm-activities-view.svelte`
- `llm-accounts-view.svelte`
- `nav-rail.svelte` child items
- `content-panel.svelte` branches

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/cding/Workspace/ChenWeb/web && bun test`
Expected: PASS for new unit tests

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add web/src/lib/components/home3/llm-activities-client.ts web/src/lib/components/home3/llm-activities-client.test.ts web/src/lib/components/home3/llm-accounts-client.ts web/src/lib/components/home3/llm-accounts-client.test.ts web/src/lib/components/home3/llm-activities-view.svelte web/src/lib/components/home3/llm-accounts-view.svelte web/src/lib/components/home3/nav-rail.svelte web/src/lib/components/home3/content-panel.svelte
git -C /Users/cding/Workspace/ChenWeb commit -m "feat: add llm accounts and activities home3 views"
```

## Chunk 6: Final Wiring, Verification, And Docs

### Task 12: Wire scheduled jobs into startup paths

**Files:**
- Modify: `ChenWeb/server/cmd/deepdoc/main.go`
- Modify: `ChenWeb/server/cmd/dataservice/main.go`
- Modify: any startup helper used to launch recurring jobs

- [ ] **Step 1: Write/extend the failing tests**

Cover:
- startup registers LLM jobs once
- disabled reconciliation accounts do not run provider jobs

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/cmd/...`
Expected: FAIL where job registration is absent

- [ ] **Step 3: Implement minimal startup wiring**

Register:
- reconciliation scheduler
- retention scheduler

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/cmd/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add server/cmd/deepdoc/main.go server/cmd/dataservice/main.go
git -C /Users/cding/Workspace/ChenWeb commit -m "feat: wire llm jobs into service startup"
```

### Task 13: Update docs and run final verification

**Files:**
- Modify: `ChenWeb/docs/superpowers/specs/2026-06-19-llm-activities-design.md`
- Create: `ChenWeb/docs/llm-accounts.md`

- [ ] **Step 1: Document operator workflow**

Include:
- import from `.models.toml`
- create/edit/disable account
- archive retention behavior
- workspace timezone behavior

- [ ] **Step 2: Run focused backend verification**

Run: `go test ./server/api/llmadminhandler ./server/api/llmreporthandler ./server/api/llmreconcile ./server/api/llmjobs`
Expected: PASS

Run: `go test ./api/llm`
Expected: PASS

- [ ] **Step 3: Run focused frontend verification**

Run: `cd /Users/cding/Workspace/ChenWeb && bun run check`
Expected: PASS

- [ ] **Step 4: Run workspace-level sanity checks if changes touched shared behavior broadly**

Run: `cd /Users/cding/Workspace && go work sync`
Expected: completes successfully

Run: `cd /Users/cding/Workspace && go test ./...`
Expected: PASS or any pre-existing failures identified explicitly

- [ ] **Step 5: Commit**

```bash
git -C /Users/cding/Workspace/ChenWeb add docs/superpowers/specs/2026-06-19-llm-activities-design.md docs/llm-accounts.md
git -C /Users/cding/Workspace/ChenWeb commit -m "docs: add llm accounts operator guide"
```

## Notes For Execution

- Keep shared library commits separate from ChenWeb commits.
- Prefer soft-disable for account deletion.
- Never return plaintext API keys from admin handlers.
- Do not auto-sync `.models.toml` after initial import.
- Preserve backward compatibility for existing `shared/go/api/llm` callers where practical, then migrate ChenWeb callers incrementally.
- If full workspace `go test ./...` is too noisy, record exact failing packages and continue with targeted verification.
