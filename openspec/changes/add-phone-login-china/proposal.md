## Why

ChenWeb's `/login` page (`web/src/lib/components/login-01.svelte`) currently only supports email/password (via Kratos) and Google/GitHub OAuth. Our partner in China needs to onboard users who authenticate primarily by mobile phone number with an SMS verification code — the dominant login pattern for Chinese consumer web/mobile products, where many users don't have or don't want to use an email address. This change adds "Login through Phone" as a first-class login method, scoped to Chinese mobile numbers first (Phase 1). US mobile number support is an explicit Phase 2, out of scope here.

## What Changes

- Add a `phone` trait to the Kratos identity schema (`Kratos/kratos/identity.schema.json`), backed by Kratos's native `code` credential method (`via: "sms"`), following the same pattern already used for `email` + `code`/`via: "email"`.
- Configure a Kratos courier **generic/HTTP channel** (`Kratos/kratos/kratos.yml`) that delivers the one-time code via Aliyun's SMS gateway (Dysmsapi `SendSms` action), instead of building a bespoke OTP/session side-channel. This mirrors how Google/GitHub login already drive Kratos's own `self-service/login` flow rather than a parallel auth path.
- Add a small Go relay in `shared/go/api/auth/` that Kratos's courier HTTP channel calls to actually invoke Aliyun's SMS API (Aliyun credentials must not be hard-coded, unlike bzton's reference implementation — see Impact).
- Add a "Login with Phone" tab/mode to `login-01.svelte`: phone-number input (Chinese mobile regex, improved over bzton's incomplete `1[3,4,5,7,8]` pattern to cover current MIIT-assigned prefixes), "Send code" button with a **server-enforced** cooldown (Kratos's login flow already rate-limits via `CheckLoginRateLimit`-equivalent behavior for the code method) plus a client-side 60s countdown for UX, and a code-input step that submits to Kratos's native code-verification flow.
- On first successful phone login with no matching identity, auto-create a minimal Kratos identity with only the `phone` trait set (no forced email), matching the "phone login doubles as signup" behavior from the bzton reference, but without bzton's global `platform.code.enable` kill-switch anti-pattern.
- Phase 1 explicitly excludes: US phone numbers/E.164 validation, WeChat mini-program login (`wxOpenId` fields in the bzton reference), and any change to existing email/OAuth login behavior.

## Capabilities

### New Capabilities
- `phone-login-china`: Phone-number + SMS-verification-code login and just-in-time account creation for Chinese mobile numbers, integrated into Kratos's native identity/session flows.

### Modified Capabilities
(none — no existing `openspec/specs/` capabilities exist yet in this repo; email/OAuth login behavior is unchanged)

## Impact

- **Affected repos**: `ChenWeb/` (frontend `web/src/lib/components/login-01.svelte`, `web/src/routes/login/`), `shared/go/api/auth/` (new SMS-courier relay handler + route registration in `shared/go/api/router.go`), `Kratos/` (`kratos/identity.schema.json`, `kratos/kratos.yml` courier config) — three separate git repositories, each needs its own commit.
- **Rebuild/restart required**: Kratos config changes require `mise migrate` (schema traits are not itself a DB migration, but Kratos process restart is required) and a Kratos restart; no Kratos source rebuild is needed since courier HTTP channels and identity traits are config-only, not compiled-in code.
- **New external dependency**: Aliyun SMS (Dysmsapi) account, sign-name, and template ID, configured via environment variables (`ALIYUN_SMS_ACCESS_KEY_ID`, `ALIYUN_SMS_ACCESS_KEY_SECRET`, `ALIYUN_SMS_SIGN_NAME`, `ALIYUN_SMS_TEMPLATE_CODE`) — never hard-coded in source, unlike the bzton reference.
- **Database**: No new ChenWeb/shared Postgres tables are needed — phone identities and OTP codes live entirely inside Kratos's own `identities`/`identity_credentials` tables, consistent with how email/password already works. The existing unused `users.user_mobile` column (legacy non-Kratos path) is left untouched.
- **Docs**: `Kratos/CLAUDE.md` needs a new section documenting the `phone` trait and SMS courier channel (it currently only documents adding a `phone` field as an unimplemented "Customization" bullet). A new ADR should record the "extend Kratos natively" decision vs. the bespoke-side-channel alternative that was considered and rejected.
