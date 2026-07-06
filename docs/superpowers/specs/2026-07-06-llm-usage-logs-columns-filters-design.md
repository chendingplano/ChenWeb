# LLM Usage Logs: Columns + Filters

## Goal
On the System Admin "LLM Usage Logs" page (`home3` → `llm-usage-logs-view.svelte`):
1. Remove the **Provider** column.
2. Add **Metadata**, **Call Reason**, **Call LOC** columns.
3. Add a server-side filter section.

## Current state
- Frontend: `ChenWeb/web/src/lib/components/home3/llm-usage-logs-view.svelte` renders a paginated table from `GET /api/v1/llm/usage-events-admin?page=&page_size=`.
- Backend: `llmreporthandler.ListUsageEventsAdmin` (handler.go) → `Store.ListUsageEventsAdmin` (store.go) queries `llm_usage_event`, no filtering, only `LIMIT`/`OFFSET`.
- `call_reason` and `call_loc` columns already exist on `llm_usage_event` and are already returned in `UsageEventAdmin`, just not rendered.
- `metadata_json` (jsonb) is written per-event (`llmusage/sink.go`) but not yet selected/returned by `ListUsageEventsAdmin`.
- Table has 104k+ rows / 2000+ pages, so filtering must happen server-side to be useful.

## Backend changes
`ChenWeb/server/api/llmreporthandler/store.go`:
- Add `MetadataJSON json.RawMessage \`json:"metadata_json"\`` to `UsageEventAdmin`, select `evt.metadata_json` (COALESCE to `'{}'::jsonb` if nullable).
- Add a `UsageEventAdminFilters` struct:
  ```go
  type UsageEventAdminFilters struct {
      Model, Prompt, CallReason, CallLoc string
      StartedFrom, StartedTo             *time.Time
      InTokMin, InTokMax                 *int64
      OutTokMin, OutTokMax               *int64
      MetaKey, MetaValue                 string
  }
  ```
- `ListUsageEventsAdmin(ctx, page, pageSize, filters)` builds a `WHERE` clause dynamically (parameterized, `$n` placeholders) applying only the non-empty filters:
  - `evt.model_name ILIKE '%'||$n||'%'`
  - `evt.prompt_name ILIKE '%'||$n||'%'`
  - `evt.call_reason ILIKE '%'||$n||'%'`
  - `evt.call_loc ILIKE '%'||$n||'%'`
  - `evt.request_started_at >= $n` / `<= $n`
  - `evt.input_tokens >= $n` / `<= $n`, `evt.output_tokens >= $n` / `<= $n`
  - `evt.metadata_json ->> $n ILIKE '%'||$n||'%'` (only when both key and value are provided)
  - Same WHERE clause is reused for the `COUNT(*)` query so `total`/pagination reflect the filtered set.

`ChenWeb/server/api/llmreporthandler/handler.go`:
- `ListUsageEventsAdmin` reads optional query params: `model`, `prompt`, `call_reason`, `call_loc`, `started_from`, `started_to` (RFC3339), `in_tok_min`, `in_tok_max`, `out_tok_min`, `out_tok_max`, `meta_key`, `meta_value`. Builds `UsageEventAdminFilters` and passes to the store.
- `reportStore` interface signature updated to match.

## Frontend changes
`ChenWeb/web/src/lib/components/home3/llm-usage-logs-view.svelte`:
- `UsageEventRow` type: add `metadata_json: string`.
- Filter state: individual `$state` vars for each filter field, always-visible filter row placed above the pagination bar (inside the table card, below the header card).
  - Text inputs: Model, Prompt, Call Reason, Call LOC.
  - Datetime-local inputs: Started From, Started To.
  - Number inputs: In Tok Min/Max, Out Tok Min/Max.
  - Text inputs: Metadata Key, Metadata Value.
  - "Apply" button resets `page = 1` and calls `load()`; "Clear" resets all filter fields, resets page, and reloads.
- `load()` includes every non-empty filter value in the `URLSearchParams` sent to the API.
- Table header: remove **Provider** `<th>`; add **Metadata**, **Call Reason**, **Call LOC** `<th>`s (placed after Prompt, before token columns, matching the on-screen mock).
- Table body: remove Provider `<td>`; add Call Reason / Call LOC as plain text cells (`row.call_reason || '—'`, `row.call_loc || '—'`); add a Metadata cell using the same button style as Input/Output Body — on click, opens the existing modal with `modalTitle = 'Metadata — <id>'` and `modalContent = row.metadata_json`, reusing the existing `renderJsonHtml`/`modalHtml` pipeline (no new rendering code).
- `openBody` stays as-is; add a small `openMetadata(row)` helper that sets modal state directly (metadata has no separate fetch — it's already in the row payload).

## Out of scope
- No changes to Account/Profile columns or other tabs (Doc Processor Logs, etc.).
- No client-side-only filtering fallback.
- No new distinct-value dropdowns (all text filters are ILIKE substring match per user decision).
