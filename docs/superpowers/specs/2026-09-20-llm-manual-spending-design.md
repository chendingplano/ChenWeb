# LLM Manual Spending Design

## Purpose

Allow administrators to record provider total spending when a provider does not expose it, retain an audit trail of manual entries, and show a currency-aware Total Spending series alongside existing balance charts.

## Manual entries

The LLM Accounts page uses one form for two actions. **Add Deposit** preserves the existing behavior. **Set Total Spend** uses the selected API key, currency, amount, timestamp, and note, then appends an `llm_balance_snapshot` record whose `entry_kind` is `set-total-spending` and whose `deposit_amount` holds the supplied total.

The server resolves API-key names to account IDs as it does for deposits. It does not return key references or values to the browser. A set-total-spending entry must not participate in provider-balance calculations or become a current balance.

## Manual-record list

The page lists all manual records (both `deposit` and `set-total-spending`), newest first. Each row shows timestamp, configured API-key name, type, currency, amount, and note. The API-key name is resolved server-side from the account reference and configuration; secrets are never returned.

## Chart semantics

For every account, selected frequency bucket, and currency, only the latest `set-total-spending` entry is used. Older entries remain audit records but do not affect the chart.

- **Total Spending (CNY)** = existing provider-derived CNY spending + the latest manual CNY total in the bucket.
- **Total Spending (USD)** = the latest manual USD total in the bucket, because provider-derived USD spending does not exist today.

The Total Spending bars appear with the existing hourly, daily, and monthly account charts. Manual total-spend snapshots are excluded from the provider-balance pivot and current-balance queries.

## Verification

Server tests cover set-total-spend persistence, safe manual-record responses, selection of the latest manual amount per bucket, and balance-query exclusion. Client tests cover the new requests and chart response field; frontend checks cover the two form modes and chart series.
