# LLM Telemetry and Dual-Track Billing Design

## Goal

Route every ChenWeb AI-provider interaction through `shared/go/api/llm`, persist
auditable LLM usage with its caller user, and present separate provider-authoritative
and locally calculated billing tracks.

## Boundaries

`shared/go/api/llm` becomes the only package that performs AI-provider or AI-gateway
HTTP, including the DeepSeek balance client. ChenWeb's image-generation providers, Pi
gateway adapter, and reconciliation runner become thin callers of shared clients; their
direct HTTP implementations are removed. This includes `imagehandler`,
`productdrawings`, `agentservicehandler`, and `llmreconcile`. Every shared client
captures a usage event, including failed requests; image calls record their modality
even where the provider does not supply tokens. ChenWeb installs the usage sink at
server bootstrap after the project database is ready. The sink writes
`public.llm_usage_event` and includes a nullable text `user_id` compatible with the
external Kratos identity. A missing ID emits an error-level structured log but still
persists the NULL event; background/system calls must deliberately use a documented
system identity where one exists.

## Data model and billing

Usage events retain provider/model/token data and gain a nullable `user_id` plus an
index for administrative filtering. Hourly provider balance snapshots are account/API-key
scoped because DeepSeek exposes only an account balance, not a per-model balance. The
DeepSeek balance response can contain multiple currency entries, so every entry is
stored and reported independently. Snapshots are immutable rows with their own IDs,
capture time, currency, and raw-response archive path. A scheduler-slot claim keyed by
account and UTC hour provides idempotency only for scheduled captures; manual/retry
captures remain separately stored and marked by `capture_source`. CNY and USD
balances/deltas are never summed or converted; the scheduler uses a database/advisory
lock to prevent multi-instance concurrent capture.

Two new, non-overloaded report sources retain independent values:

- **Official Balance:** the provider-retrieved account balance and a currency-scoped
  account balance delta, separated by provider currency. A positive balance increase
  is a credit/top-up/refund candidate rather than negative spend; missing period
  boundaries yield an incomplete status. This is never represented as
  provider-confirmed per-model spend.
- **Locally Calculated:** a dedicated per-account, per-model, per-period CNY report
  calculated from logged cache-hit, cache-miss, and output tokens. It is explicitly
  labelled as an estimate and never allocates the official balance delta.

Money is stored as PostgreSQL `NUMERIC`, not floating point, with fixed documented
rounding at the event-to-period boundary. The billing parser reads only an explicit
DeepSeek billing block, ignores non-model TOML sections during model import, treats
rates as CNY per one million tokens, applies China-business-calendar/UTC peak rules,
and fails closed for unknown models or malformed prices. Pricing must be model-specific:
until a DeepSeek Pro rate is configured, Pro receives no local estimated-spend value.

The official balance chart groups by API key and currency, rendering independent CNY
and USD series when DeepSeek returns both. Model charts show only token use and local
CNY spend; there is no derived allocation of official balance deltas.

## Scheduling and UI

`RegisterRoutes` owns the LLM telemetry bootstrap: after database availability it
installs the capture sink, creates a cancellation context released through Echo server
shutdown, and starts the hourly reconciliation scheduler. The LLM Activities API
exposes hourly account-balance series and the two spend tracks, with only account name
and a stable non-secret API-key fingerprint/display label in API responses. The raw
credential remains server-only and is never selected into report DTOs or logs. The
dashboard renders a dedicated official-balance bar chart per API key and currency and
retains model token/local-spend bars, using CNY for DeepSeek local estimates.
Administrative APIs include the caller user ID only where existing admin access
controls apply.

## Validation

Tests cover startup sink installation, user-ID capture/error logging, shared image and
gateway delegation, hourly balance persistence, CNY peak/off-peak pricing, report
separation, and API/UI response shaping. Migration tests cover the additive schema.
