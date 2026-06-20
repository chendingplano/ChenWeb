# LLM Activities And Accounts Design

## Summary

Build a provider-agnostic, account-based LLM observability and billing facility for ChenWeb. The first implementation focuses on DeepSeek reconciliation, but the architecture must support OpenAI, Qwen, and future providers without redesigning the data model or UI.

The system has two distinct responsibilities:

1. Capture every LLM call made through `shared/go/api/llm` for debugging, optimization, and future self-improving workflows.
2. Produce authoritative daily spend reports from provider-side reconciliation data, grouped by account and rolled up using a configurable workspace timezone.

The ChenWeb database becomes the source of truth for LLM accounts and model profiles after an initial one-time import from `ChenWeb/.models.toml`.

## Goals

- Make LLM accounts a first-class concept.
- Support multiple accounts per provider and multiple profiles per account.
- Capture every LLM call with prompt/output references and token counts.
- Reconcile daily spend by account using provider-authoritative data.
- Provide a `home3` dashboard page for daily LLM activity and a `System Admin` page for managing accounts.
- Retain operational metadata for a configurable short window while keeping full prompt/response archives indefinitely.

## Non-Goals

- Building provider integrations for every provider in the first slice.
- Moving full LLM call storage into ClickHouse.
- Treating `.models.toml` as a long-term editable source of truth.
- Replacing ChenWeb observability traces; this system complements them.

## Source Of Truth

After initial bootstrap import, the database is the canonical registry for:

- LLM accounts
- LLM account model profiles
- Reconciliation settings

`ChenWeb/.models.toml` remains a legacy bootstrap input only. New additions or changes should be done through ChenWeb UI under `home3 -> System Admin -> LLM Accounts`.

## Configuration

Add workspace-level settings to `ChenWeb/config.toml`:

```toml
[llm]
workspace_timezone = "America/Chicago"
usage_retention_days = 30
archive_root = "Data/llm-logs"
reconciliation_run_hour = 2
```

`workspace_timezone` defines the local day boundary for all daily reporting. It must be configurable because deployment timezone may change later.

## Data Model

### `llm_account`

One row per provider account.

Suggested fields:

- `id`
- `account_name`
- `provider`
- `base_url`
- `api_key_ref`
- `status`
- `reconciliation_kind`
- `is_reconciliation_enabled`
- `default_model_name`
- `metadata_json`
- `created_at`
- `updated_at`

### `llm_account_model_profile`

One row per runnable account/model profile. This is the bridge from the current `.models.toml` structure where each section behaves like a profile rather than a pure model definition.

Suggested fields:

- `id`
- `account_id`
- `profile_name`
- `model_name`
- `thinking_type`
- `timeout_sec`
- `max_inflight`
- `max_requests_per_minute`
- `max_tokens_per_minute`
- `token_reserve_per_call`
- `is_active`
- `metadata_json`
- `created_at`
- `updated_at`

### `llm_usage_event`

One row per LLM call captured in `shared/go/api/llm`.

Suggested fields:

- `id`
- `account_id`
- `profile_id`
- `provider`
- `model_name`
- `prompt_name`
- `request_started_at`
- `request_finished_at`
- `workspace_day`
- `input_tokens`
- `output_tokens`
- `total_tokens`
- `latency_ms`
- `http_status`
- `error_message`
- `input_body_ref`
- `output_body_ref`
- `provider_request_id`
- `metadata_json`
- `created_at`

This table is for recent operational use and should be purged after a configurable retention window.

### `llm_daily_account_report`

One row per account per workspace day. This is the authoritative reporting table.

Suggested fields:

- `id`
- `account_id`
- `workspace_day`
- `timezone_name`
- `opening_balance`
- `closing_balance`
- `spend_amount`
- `currency_code`
- `input_tokens`
- `output_tokens`
- `total_tokens`
- `request_count`
- `reconciliation_status`
- `source_kind`
- `source_payload_ref`
- `created_at`
- `updated_at`

### `llm_balance_snapshot`

Stores raw balance pulls such as the daily 2:00am DeepSeek balance read.

Suggested fields:

- `id`
- `account_id`
- `captured_at`
- `workspace_day`
- `balance_amount`
- `currency_code`
- `raw_payload_ref`
- `created_at`

## Archive Storage

Full prompt and response bodies should not be stored inline in PostgreSQL. Store them in day-partitioned files under a configurable archive root, with the database holding references.

Proposed layout:

```text
Data/llm-logs/
  2026/
    2026-06/
      2026-06-19/
        account-<account_id>/
          usage-events.jsonl
          bodies/
            <event_id>-input.json.gz
            <event_id>-output.json.gz
        reconciliation/
          deepseek-account-<account_id>-balance.json
```

Why this shape:

- Efficient day-based purge for short-lived metadata if needed later.
- Avoids very large flat directories.
- Direct lookup by day and account.
- Gzip reduces repeated prompt/response storage cost.
- Retrieval is simple for UI drill-down and debugging.

Full-body archive files are retained indefinitely by default.

## Runtime Flow

### 1. Account/Profile Resolution

All LLM calls in `shared/go/api/llm` resolve through an account-aware profile instead of a bare provider config.

Resolution output:

- `account_id`
- `profile_id`
- `provider`
- `model_name`
- `base_url`
- `api_key`
- rate-limit settings
- timeout/thinking settings

### 2. Per-Call Capture

Every `Complete`, `Stream`, and legacy helper path creates one `llm_usage_event`.

Flow:

1. Create event row at request start.
2. Write prompt/input body to archive storage.
3. Execute provider call.
4. On completion, update the event with token counts, latency, output reference, and provider request metadata.
5. On failure, persist the error and any available metadata while still keeping the input body reference.

Minimum captured fields:

- model name
- prompt name
- input content reference
- output content reference
- input token count
- output token count
- timestamp
- error message

### 3. Provider Reconciliation

Run a scheduled ChenWeb backend job using workspace timezone from `config.toml`.

DeepSeek first slice:

- Fetch current balance around 2:00am workspace time.
- Write a `llm_balance_snapshot`.
- Produce a `llm_daily_account_report` for the just-finished local day.

Authoritative precedence:

- Provider-side reconciliation data is the source of truth for spend.
- Local `llm_usage_event` data is supporting evidence for debugging and backfill.
- If provider token totals are unavailable for a day, local captured token totals may be used with a clearly marked reconciliation status.

### 4. Retention

Retention policy:

- `llm_usage_event` rows: purge after `usage_retention_days`, default `30`
- Full prompt/response archive files: retain indefinitely
- `llm_daily_account_report`: retain indefinitely
- `llm_balance_snapshot`: retain indefinitely

Retention should be implemented as a scheduled cleanup job.

## UI

### `home3 -> Dashboard -> LLM Activities`

Add a dashboard child view that reads authoritative daily reports plus recent call activity.

First slice:

- Daily spend chart by account
- Daily token chart by account
- Account summary table
  - account name
  - provider
  - opening balance
  - closing balance
  - spend
  - input/output/total tokens
  - reconciliation status
- Recent LLM calls table
  - time
  - account
  - model
  - prompt name
  - input/output tokens
  - latency
  - error status
- Call detail drawer
  - metadata
  - input text
  - output text
  - provider request id
  - archive references

Implementation fit:

- new dashboard child in `web/src/lib/components/home3/nav-rail.svelte`
- new branch in `web/src/lib/components/home3/content-panel.svelte`
- dedicated `llm-activities-view.svelte`
- backend endpoints under `/api/v1/llm/...`

### `home3 -> System Admin -> LLM Accounts`

Add an admin page for live account/profile management.

First slice:

- Account list
  - account name
  - provider
  - base URL
  - reconciliation enabled
  - status
  - number of profiles
- Account create/edit drawer
  - account name
  - provider
  - base URL
  - API key
  - reconciliation enabled
  - reconciliation kind
- Profile subtable per account
  - profile name
  - model name
  - timeout
  - thinking type
  - RPM/TPM/max inflight
  - active/inactive
- Actions
  - add account
  - add profile
  - edit
  - disable
  - rotate API key
  - test connection
  - one-time import from `.models.toml`

Deletion behavior:

- Prefer soft disable over hard delete.
- Historical usage and reporting rows must remain valid.
- API keys are write-only from the UI and should never be returned in plaintext.

## Backend Interfaces

Suggested endpoint families:

- `/api/v1/llm/accounts`
- `/api/v1/llm/accounts/:id`
- `/api/v1/llm/accounts/:id/profiles`
- `/api/v1/llm/accounts/import-models-toml`
- `/api/v1/llm/reports/daily`
- `/api/v1/llm/reports/daily/:account_id`
- `/api/v1/llm/usage-events`
- `/api/v1/llm/usage-events/:id`

## Import Strategy For `.models.toml`

The first import should:

1. Read `ChenWeb/.models.toml`.
2. Convert each section into:
   - one `llm_account` row if it maps to a new provider account
   - one `llm_account_model_profile` row
3. Mark imported rows with `metadata_json.source = "toml_import"`.
4. Avoid future automatic syncing from the file.

The system should not watch `.models.toml` for subsequent changes because the database/UI becomes the source of truth.

## Testing Strategy

Backend tests:

- account/profile resolution tests in `shared/go/api/llm`
- usage-event creation and update tests
- archive path generation tests
- retention job tests
- reconciliation aggregation tests
- import tests for `.models.toml`

Frontend tests:

- nav route selection for `LLM Activities`
- nav route selection for `LLM Accounts`
- dashboard rendering for daily reports
- account admin create/edit flows

Integration checks:

- one DeepSeek account can reconcile a daily report
- one captured call appears in recent call table
- one archived body can be retrieved in detail view

## Risks And Mitigations

### Secret Handling

Risk:

- API keys must not leak to logs, responses, or archived payloads.

Mitigation:

- Store only references or encrypted values.
- Treat UI API key fields as write-only.
- Reuse existing log-redaction discipline.

### Storage Growth

Risk:

- Full-body archives can grow quickly.

Mitigation:

- Compress bodies with gzip.
- Partition by day/account.
- Keep operational rows short-lived.

### Reconciliation Gaps

Risk:

- Provider billing APIs may expose incomplete daily token or spend data.

Mitigation:

- Keep provider-side data authoritative for spend.
- Mark fallback-derived days with reconciliation status.
- Retain local captured evidence for comparison.

## Documentation Impact

Knowledge changed:

- ChenWeb gains an account-based LLM telemetry, reconciliation, and admin-management design.

Docs/specs/ADRs/tests affected:

- New design spec in `docs/superpowers/specs/`
- Future docs should cover config, account admin workflow, and retention behavior
- Tests will be added for account resolution, call capture, reconciliation, and UI routing

Docs updated:

- This design spec only

Docs now stale:

- None identified yet, but any internal notes that assume `.models.toml` is the long-term source of truth will become stale after implementation

Intentionally left undocumented:

- Final provider-specific DeepSeek reconciliation wire details
- Exact SQL schema and migration file names
- Final API response shapes for the dashboard/admin pages

