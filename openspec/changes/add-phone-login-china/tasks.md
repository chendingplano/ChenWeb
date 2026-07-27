## 1. Spike: validate Kratos registration-flow auto-provisioning (de-risk D5)

- [x] 1.1 Confirmed via Kratos source (`courier/sms_test.go`, `embedx/config.schema.json:2226-2252`) that `courier.channels` (`id: "sms"`, `type: "http"`, `request_config`) works exactly as designed: a Jsonnet template renders `ctx` (which includes `to`, `login_code`/`registration_code`, `identity`, `expires_in_minutes`, `body`) into the JSON POST body sent to our relay. `identity.ChannelTypeSMS`/`sms.LoginCodeValidModel`/`sms.RegistrationCodeValidModel` (`selfservice/strategy/code/code_sender.go:125-181`, `courier/template/sms/login_code_valid.go`) confirm the code strategy has first-class SMS support once a trait's `via` is set to `"sms"`.
- [x] 1.2 Resolved by reading `selfservice/strategy/idfirst/strategy_login.go:80-196`: the login flow's code method does **not** auto-create an identity. An unrecognized identifier either surfaces `schema.NewAccountNotFoundError()` or (if `security.account_enumeration_mitigation` is on) silently renders the code-entry form without ever creating an account. So D5 is revised: the frontend must attempt a **login** flow first, and on an account-not-found response, fall back to starting a **registration** flow with the same phone number (two chained Kratos flows), rather than relying on one flow to transparently do both. Task group 5 below reflects this.

## 2. SMS relay endpoint (`shared/go/api/auth/`)

- [x] 2.1 Added `shared/go/api/auth/sms_relay.go` with `HandleSMSCourierRelay` at `POST /internal/sms-courier/send`, accepting `{to, code}` (payload shape decided by our own Jsonnet template in task 4.2, not Kratos's default — see design.md D2)
- [x] 2.2 Implemented the Aliyun Dysmsapi `SendSms` call directly (RPC signing: sorted params, HMAC-SHA1, base64), reading `ALIYUN_SMS_ACCESS_KEY_ID`/`ALIYUN_SMS_ACCESS_KEY_SECRET`/`ALIYUN_SMS_SIGN_NAME`/`ALIYUN_SMS_TEMPLATE_CODE` from env vars only (`loadAliyunSMSConfig`)
- [x] 2.3 `loadAliyunSMSConfig` fails fast with a logged error + 500 response if any required env var is missing
- [x] 2.4 Restricted via a required `X-Internal-Relay-Secret` header compared (constant-time) against `SMS_RELAY_SHARED_SECRET`; fails closed (500) if that env var itself isn't set
- [x] 2.5 Registered `POST /internal/sms-courier/send` in `shared/go/api/router.go` (Kratos-mode only)

## 3. Server-side send-rate limiting

- [x] 3.1 (Revised — see design.md D6) No migration needed: reuse the existing in-process `RateLimiter` from `shared/go/api/auth/rate_limiter.go`. Added an `smsRateLimiter` keyed by phone number.
- [x] 3.2 N/A — no new table.
- [x] 3.3 In `HandleSMSCourierRelay`, check `CheckSMSSendRateLimit(phone)` before calling Aliyun, rejecting with a clear error once the limit is exceeded (spec: "Server rejects excessive send requests even if client-side limit is bypassed")

## 4. Kratos configuration (separate `Kratos/` repo)

- [x] 4.1 Added a `phone` trait to `Kratos/kratos/identity.schema.json`. **Corrected after live testing**: the pattern is `^\+861[3-9][0-9]{9}$` (E.164), not the originally-sketched bare `^1[3-9][0-9]{9}$` — see 4.7.
- [x] 4.2 Added a `courier.channels` entry in `Kratos/kratos/kratos.yml` (`id: sms`, `type: http`), with `Kratos/kratos/templates/courier/sms/request.config.jsonnet` producing a `{to, code}` payload. No `courier.templates` override was needed — Kratos ships default SMS templates for the `code` method out of the box. Header `X-Internal-Relay-Secret` set to match `SMS_RELAY_SHARED_SECRET` in `ChenWeb/.env`; `kratos.yml` is gitignored so this plaintext value never reaches git. **URL corrected after live testing** — see 4.8.
- [x] 4.3 Added `code: {hooks: [{hook: session}]}` under `selfservice.flows.registration.after` in `kratos.yml` (additive alongside existing `password`/`oidc` hooks)
- [x] 4.4 Kratos restarted twice by the user during live testing (config-only; no `mise build-kratos` rebuild needed). `identity.schema.json` changes turned out to hot-reload without a restart; `kratos.yml` changes (courier channels, `passwordless_enabled`) did require one.
- [x] 4.5 Smoke-tested via a direct Kratos API call (`GET /self-service/login/api`): the login flow still renders the `password`/`oidc` nodes correctly after the schema change, confirming the `phone` trait addition didn't break the existing structure. (A full real-credentials `tax` login wasn't run — no test account credentials available in this session — but the flow-rendering check is a strong structural signal nothing broke.)
- [ ] 4.6 Still **blocked** on real `ALIYUN_SMS_*` credentials for an actual SMS send — see 4.9 for how far live testing got without them.
- [x] 4.7 **Bug found via live testing, fixed**: `selfservice.methods.code.enabled: true` alone does *not* enable code-based **login** — `Strategy.Login()` (`selfservice/strategy/code/strategy_login.go`) bails out with `ErrStrategyNotResponsible` unless `selfservice.methods.code.passwordless_enabled: true` is also set, in which case Kratos returned a generic "no strategy found" (message 4010002) instead of an account-not-found error. Added `passwordless_enabled: true` to `kratos.yml`.
- [x] 4.8 **Bug found via live testing, fixed**: Kratos's phone-identifier normalization (`Kratos/src/kratos/x/normalize.go`, `phonenumbers.Parse(value, "")` — empty default region) requires full **E.164** format and rejects bare 11-digit numbers with "invalid country code" (message 4000001). Fixed by: (a) `identity.schema.json`'s `phone` pattern now requires `+86` prefix, (b) `kratos_phone.go` added a `toE164CN()` helper that prepends `+86` to the user-typed 11-digit number before ever sending it to Kratos as `Identifier`/`Traits.phone`, (c) `sms_relay.go` now validates the *E.164* form (`cnMobileE164Pattern`) on the value it receives from Kratos's courier, and strips the `+86` prefix before calling Aliyun (whose domestic `SendSms` action expects the bare national number).
- [x] 4.9 **Bug found via live testing, fixed**: the relay endpoint was originally registered at `/internal/sms-courier/send`, which doesn't match any of the path prefixes (`/api`, `/auth`, `/shared_api`, `/ws`) that ChenWeb's global route middleware (`server/api/routes.go`) exempts from its session-auth gate — Kratos's courier call (no session cookie) was rejected with 401 before ever reaching the handler. Moved the route to `/auth/internal/sms-courier/send` (and updated `kratos.yml`'s `courier.channels` URL to match), consistent with how `/auth/phone/*` already worked without a session.
- [x] 4.10 **Corrected message-ID assumption**: the login flow's account-not-found error for the code method is actually `schema.NewNoCodeAuthnCredentials()` / message ID **4000035** (`ErrorValidationNoCodeUser`), not `schema.NewAccountNotFoundError()` / 4000037 as originally assumed from reading source alone — confirmed via a direct curl trace against Kratos. `kratosMessageIDAccountNotFound` in `kratos_phone.go` corrected to 4000035.
- [x] 4.11 End-to-end verified (minus the actual Aliyun send, blocked per 4.6): `POST /auth/phone/send-code` with a never-seen phone number returns `flow_type: "registration"`; Kratos's courier reaches the relay at the corrected `/auth/internal/sms-courier/send` path with the E.164-formatted number (`"to":"+8613800005555"` observed in logs, confirming 4.8's fix); the relay then fails cleanly with a clear, logged error (`"aliyun sms send failed" ... error="missing one or more of ALIYUN_SMS_ACCESS_KEY_ID, ..."`) rather than crashing or silently swallowing the failure — exactly the fail-safe behavior specced in "No Hard-Coded SMS Provider Credentials". Kratos's courier retried the dispatch once on its own (expected built-in retry behavior, not a bug). The only remaining unverified step is the Aliyun call itself succeeding, which needs real credentials.

## 5. Backend phone-login flow handlers + config flag (new — not in original breakdown)

- [x] 5.0a Added `shared/go/api/auth/kratos_phone.go`: `HandlePhoneSendCodeKratos` (`POST /auth/phone/send-code`) creates a native Kratos login flow first; if Kratos's response carries message ID 4000035 (`ErrorValidationNoCodeUser` — corrected per 4.10), falls back to a native registration flow instead (per the D5 login-then-registration-fallback finding). Both the login identifier and registration `traits.phone` are sent to Kratos in E.164 form via `toE164CN()` (per 4.8) — the bare 11-digit form is only for user input/display. `HandlePhoneVerifyCodeKratos` (`POST /auth/phone/verify`) submits the code to whichever flow type the frontend echoes back, sets the `session_token` cookie via the existing `setSessionTokenCookie` on success, and logs via `sysdatastores.AddSessionLog` (`LoginMethod: "kratos_phone_login"/"kratos_phone_registration"`).
- [x] 5.0b Registered both routes in `shared/go/api/router.go` (Kratos-mode only)
- [x] 5.0c Extended `identityInfo`/`extractIdentityInfo` in `kratos.go` with a `Phone` field for consistency with the new `phone` trait (currently unused by the phone handlers themselves, which pass the phone number directly, but available for any future code reading identity info generically)
- [x] 5.0d Added `EnablePhoneLogin` (defaults **false**, unlike the OAuth flags which default true) to `ChenWeb/server/cmd/config/config.go`'s `FrontendConfigSection` + `GetEnablePhoneLogin()`, and wired it into `ChenWeb/server/api/confighandler/handler.go`'s `ConfigResponse`/`GetConfig` as `enable_phone_login`
- [x] 5.0e **Verified end-to-end via live testing** (see 4.7-4.11): three real bugs found and fixed (login needs `passwordless_enabled`, Kratos requires E.164 phone format, the relay route needed to live under `/auth`). The login→registration fallback logic itself, once the message-ID was corrected, now behaves as designed against the live Kratos instance.

## 6. Frontend UI (`ChenWeb/web/src/lib/components/login-01.svelte`)

- [x] 6.1 Added `enablePhoneLogin` to the `onMount` `GET /api/config` fetch in `login-01.svelte` (defaults `false`, matching the backend flag)
- [x] 6.2 Added a `phone` mode gated on `enablePhoneLogin` (link "Log in with Phone" shown next to the login form when enabled), with a phone-number input enforcing `cnPhonePattern = /^1[3-9]\d{9}$/` client-side (native HTML `required` + JS check) before calling `/auth/phone/send-code`
- [x] 6.3 `handleSendPhoneCode()` posts to `/auth/phone/send-code`, stores `flow_id`/`flow_type` from the response, advances to the code-entry step, and starts a 60-second client-side cooldown (`startPhoneCooldown`) disabling the resend button
- [x] 6.4 Added the code-input step (`phoneStep === 'enter-code'`) and `handleVerifyPhoneCode()`, posting to `/auth/phone/verify` with `{phone, code, flow_id, flow_type}` and redirecting via `data.redirect_url` on success, same as `handleEmailLogin`
- [x] 6.5 Both handlers `alert()` the backend's `message` field on non-OK responses (invalid/expired code, rate-limit, send failure all surface through the same `KratosErrorResponse.message` shape) — consistent with this component's existing error-handling style (plain `alert()`, no toast system in use here)

## 7. Documentation

- [x] 7.1 Updated `Kratos/CLAUDE.md`'s "Add More User Fields" section with the actual `phone` trait + `sms` courier channel configuration
- [x] 7.2 Wrote `KnowledgeStore/doc-repo/adrs/202607/2026072801-adr-phone-login-china.md`, recording the "extend Kratos natively" decision (DR1-DR7) vs. rejected alternatives, and the shared-Kratos-instance caveat
- [x] 7.3 ADR's "Documentation Impact" section covers what changed/stayed stale

## 8. Verification

- [x] 8.1 `cd shared/go && go test ./...` — all pass
- [x] 8.2 `cd ChenWeb && go work sync && go vet ./...` — clean
- [x] 8.3 `cd ChenWeb && mise build-server` — builds cleanly (`.cache/server.exe`); `cd ChenWeb/web && bun run check` also passes for `login-01.svelte` (pre-existing unrelated error in `doc-processor-dashboard-state.test.ts`, not touched by this change)
- [ ] 8.4 **Blocked**: needs a real Aliyun account/credentials + explicit go-ahead to restart the shared Kratos instance
- [ ] 8.5 **Blocked on the same Kratos restart** as 4.4/4.5
- [x] 8.6 Verified against the live dev server (air auto-restarted after the Go edits in this session): `curl http://127.0.0.1:8080/api/config` returns `"enable_phone_login": false`, and `login-01.svelte`'s `{#if enablePhoneLogin}` gates both the "Log in with Phone" link and the whole `phone` mode block, so the UI stays hidden by default
