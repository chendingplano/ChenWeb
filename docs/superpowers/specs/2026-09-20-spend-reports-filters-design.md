# Spend Reports Filters and Chart Redesign

## Goal

Improve the development dashboard's LLM spend reports so operators can inspect a
specific workspace-day range and configured API key, while making each model chart
readable at full width.

## Scope

The changes are limited to the **Spend Reports** section of the development
dashboard. Current Balances and Recent Usage Events keep their existing loading,
filtering, and presentation behavior.

## Design

### Layout

- Rename `Daily Spend Reports` to `Spend Reports`.
- Keep one model chart per card, but render cards in a single vertical stack.
- Each chart card fills the available content width.
- Add a filter toolbar to the section header.

### Filters

The toolbar contains:

- `Time`: Today, Yesterday, Last 7 Days, Last 30 Days, This Month, Last Month,
  and Custom.
- `API Key`: configured key names from `.models.toml [model-api-keys]`.
- For `Custom`, inclusive start and end workspace-day inputs are shown.

The requested “Customer” option is interpreted as “Custom”. A custom range is
valid only when both dates are present and the start date is not after the end
date. Invalid ranges show an inline error and do not issue a request.

Filters apply only to Spend Reports. Preset dates are calculated in the
configured workspace timezone and sent to the server as inclusive date bounds.

### Data flow

The report API will accept optional `from`, `to`, and `api_key` query parameters.
The backend will:

1. Resolve the configured API-key name to its referenced secret without exposing
   the secret value.
2. Filter usage/report rows by workspace day and account API-key reference.
3. Aggregate model activity by provider, model, configured API-key name, and
   workspace day.
4. Return cache-hit and cache-miss input token totals in addition to output
   tokens and allocated spend.

The frontend will load API-key names alongside report data and reload only the
Spend Reports dataset when a filter changes. Existing balance, summary, and usage
event requests remain unchanged.

### Chart series

Every chart contains exactly four bar series:

1. Input (Cache Hit)
2. Input (Cache Miss)
3. Output
4. Spend

Token series use the token axis; Spend uses the currency axis. Calls and the
old combined Input Tokens series are removed from these charts.

## Error handling

- Missing or unreadable model-key configuration returns a normal API error and is
  shown in the existing dashboard error banner.
- Invalid custom dates are rejected in the UI before loading.
- Empty results show the existing empty-state message within Spend Reports.
- API keys are represented by names/references only; secret values are never sent
  to the browser.

## Verification

- Add backend store/handler coverage for date and API-key query parameters and the
  four returned metric fields.
- Run ChenWeb web checks/lint for the changed Svelte and TypeScript files.
- Run focused Go tests for `llmreporthandler`.
- Run a production web build if the focused checks pass.
