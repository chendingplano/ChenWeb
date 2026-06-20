# LLM Accounts

## Summary

ChenWeb now treats LLM provider credentials as first-class accounts. Accounts are the unit for:

- daily spend reconciliation
- model-profile configuration
- recent usage-event reporting
- future provider-specific jobs

The database is the source of truth after bootstrap import.

## Current UI

`home3 -> System Admin -> LLM Accounts`

Current actions:

- list accounts
- create an account manually
- preview `.models.toml`
- import `.models.toml` into accounts and profiles

Current non-actions:

- edit account
- disable account
- delete account
- manage profiles directly in the UI
- rotate API key
- test connection

These are follow-up items.

## Import Flow

Bootstrap import uses:

- `POST /api/v1/llm/accounts/import-models-toml` for preview
- `POST /api/v1/llm/accounts/import-models-toml/apply` for actual import

The importer:

1. Reads `CHENWEB_MODELS_TOML` if set, otherwise `./.models.toml`.
2. Groups profiles into provider accounts by provider + base URL + API key.
3. Upserts `llm_account` rows.
4. Upserts `llm_account_model_profile` rows.

Important behavior:

- Multiple accounts on the same host get distinct generated names.
- Re-running import is intended to be idempotent for account/profile rows.
- ChenWeb does not watch `.models.toml` for ongoing sync.

## What To Test

1. Open `LLM Accounts`.
2. Click `Preview .models.toml`.
3. Confirm accounts and profiles look correct.
4. Click `Import Into Accounts`.
5. Confirm imported accounts appear in the table.
6. Re-run import once to confirm it does not duplicate obvious rows.

## Telemetry Coverage

Usage-event persistence exists now, and DeepSeek has the first provider-side reconciliation slice.

Covered now:

- `shared/go/api/llm` call paths in the main `deepdoc` server can persist `llm_usage_event` rows and archive input/output bodies.
- `OpenAIJSONClient`-based JSON extraction calls can capture prompt name, model, token counts, request/response bodies, and provider request IDs.
- `OpenAIJSONClient` embedding calls can now capture request/response bodies and provider token usage when the embedding API returns it.
- when the client supplies `provider + base_url + api_key + profile_name`, ChenWeb can resolve the matching account/profile row before inserting the usage event.
- embedding clients that are built from `.models.toml` in doc-processing and kbhandler now also preserve the profile name on the client configuration.
- `deepdoc` now runs LLM usage retention automatically: it deletes `llm_usage_event` rows and whole archived day directories older than `llm.usage_retention_days`.
- `deepdoc` now auto-generates daily usage-based rows in `llm_daily_account_report` for yesterday and today. These rows use `reconciliation_status = "usage_aggregated"` until a provider-side reconciliation pass overwrites the just-finished day.
- `deepdoc` now runs a daily DeepSeek reconciliation pass at `llm.reconciliation_run_hour` in the configured workspace timezone. For reconciliation-enabled DeepSeek accounts, it:
  - calls `GET /user/balance`
  - writes a `llm_balance_snapshot`
  - archives the raw payload under the configured archive root
  - reconciles yesterday's `llm_daily_account_report` row with `reconciliation_status = "provider_verified"`
- ChenWeb also exposes a manual trigger at `POST /api/v1/llm/reconciliation/run`, and `home3 -> Dashboard -> LLM Activities` now includes a `Run Reconciliation` button for ad hoc testing.

Not fully covered yet:

- provider-side reconciliation for providers other than DeepSeek
- broader account/profile resolution across every existing caller that still constructs `OpenAIJSONClient` without profile/account metadata
- the remaining non-JSON helper paths that do not yet emit shared usage captures

## DeepSeek Reconciliation Notes

DeepSeek reconciliation currently uses balance deltas between consecutive daily snapshots:

- opening balance: latest snapshot captured on the report day
- closing balance: latest snapshot captured on the following day
- spend amount: `opening_balance - closing_balance`

This means the first reconciled DeepSeek slice is useful for daily spend monitoring, but it does not yet separate usage spend from same-day account top-ups.
