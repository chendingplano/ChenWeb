# LLM Deposit API Key Selector Design

## Goal

Let an administrator record a deposit against a configured API key without exposing its secret or requiring it to be manually linked to an account in the UI.

## UI

The Add Deposit form labels the selector **API Key**. It lists the names declared in `.models.toml` under `[model-api-keys]`, such as `deepseek-chen`. The API Key and Currency native selects use the existing dark input fill with a higher-contrast border so both controls are visually apparent in dark mode.

## Data flow

The browser obtains only API-key names from a new read-only endpoint. When a deposit is submitted, it sends the selected name rather than a database account ID. The server reads `.models.toml`, resolves that name to its configured `api_key`, then selects the first `public.llm_account` row whose `api_key_ref` equals the configured key. The existing snapshot insert continues to use that row's ID.

No API key value is included in API responses, client state, or rendered HTML. A missing configuration entry or matching account returns a validation error and inserts nothing.

## Verification

Handler tests cover safe option output, resolution to the first matching account, and rejection when no matching account exists. Client tests cover the new options request and request payload. The frontend build verifies the Svelte markup and styling.
