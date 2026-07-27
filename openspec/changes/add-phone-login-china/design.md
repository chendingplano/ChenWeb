## Context

ChenWeb's login page (`web/src/lib/components/login-01.svelte`) currently drives Ory Kratos for every login method it supports:

- Email/password → `HandleEmailLoginKratos`/`HandleEmailSignupKratos` (`shared/go/api/auth/kratos.go`), which call Kratos's `self-service/login`/`self-service/registration` flows over its public API.
- Google/GitHub OAuth → `HandleGoogleLoginKratos` (`kratos.go:1861`) creates a Kratos **browser** login flow and auto-submits to it; Kratos's own `oidc` method does the provider exchange and identity creation. There is no bespoke session-minting path for these — Kratos owns the whole flow.

Kratos itself is **one shared instance** used by both `ChenWeb` and `tax` (Mirai Tax CPA): both projects' `.env`/`mise.toml` point at the same `KRATOS_PUBLIC_URL=http://127.0.0.1:4433` / `KRATOS_ADMIN_URL=http://127.0.0.1:4434`, and there is exactly one `identity.schema.json` in the workspace (`Kratos/kratos/identity.schema.json`) — it currently defines only `email` (with `password`/`totp`/`code via:"email"` credentials) and `username` traits; no `phone` trait exists. Kratos is built from source in `Kratos/src/kratos/` per `Kratos/CLAUDE.md`.

Kratos natively supports a `code` credential method that can be bound to any trait via `ory.sh/kratos.credentials.code.via`, and its courier subsystem supports custom delivery **channels** (beyond the default SMTP channel) that POST a JSON body built from a Jsonnet template to an arbitrary HTTP endpoint — this is Kratos's documented mechanism for SMS delivery, since it has no first-party SMS provider integration.

Our partner's bzton reference confirms the underlying SMS mechanics we need to replicate: Aliyun Dysmsapi `SendSms`, a 6-digit numeric code, ~5 minute code lifetime, and a client-side resend cooldown. bzton's implementation is a cautionary example, not a template to copy verbatim — it hard-codes Aliyun credentials in source, has no server-side rate limiting on the send-code endpoint, and its CN phone regex (`^1[3,4,5,7,8][0-9]{9}$`) omits currently-valid prefixes (e.g. `16x`, `19x`).

## Goals / Non-Goals

**Goals:**
- Let a user log in (and be auto-provisioned on first use) with a Chinese mobile number + SMS code, through Kratos's native `code` method — no parallel session/identity system.
- Keep Aliyun credentials out of source control (env vars only).
- Add server-side throttling on SMS sending that bzton's reference lacks.
- Ship behind a feature flag (`enablePhoneLogin`, sourced from `GET /api/config`, mirroring the existing `enableLoginWithGithub`/`enableLoginWithGoogle` toggles in `login-01.svelte`) so it can be disabled instantly without a Kratos config revert.

**Non-Goals (Phase 1):**
- US phone numbers / E.164 validation (Phase 2).
- WeChat mini-program login (`wxOpenId`/`js_code` fields in the bzton reference).
- Any change to existing email/password or OAuth behavior/UI.
- Giving `tax` (Mirai) a phone-login UI. (It shares the Kratos instance, so the *capability* becomes available at the Kratos API level — see Risks — but no `tax` frontend work is in scope here.)

## Decisions

**D1 — Extend Kratos natively rather than build a bespoke side-channel.**
Chosen because it's the only pattern this codebase already uses for non-password login (OAuth drives Kratos's own flow rather than a custom identity-creation path). A bespoke path would duplicate OTP storage, expiry, and identity/session creation that Kratos already provides, and would diverge from the one precedent (`HandleGoogleLoginKratos`) already in the codebase.

**D2 — SMS delivery via a Kratos courier *generic HTTP channel* that calls a new internal relay in `shared/go/api/auth/`, not a direct Aliyun call from Kratos's Jsonnet template.**
Aliyun's Dysmsapi is a signed RPC-style API (HMAC-SHA1 request signing over canonicalized query params) — this cannot be expressed inside Kratos's static Jsonnet request-body template. So Kratos's courier channel config points at a new endpoint, e.g. `POST /internal/sms-courier/send` (new handler in `shared/go/api/auth/`, registered in `shared/go/api/router.go`), which receives `{to, body}` from Kratos and performs the actual signed Aliyun `SendSms` call using `ALIYUN_SMS_ACCESS_KEY_ID`/`ALIYUN_SMS_ACCESS_KEY_SECRET`/`ALIYUN_SMS_SIGN_NAME`/`ALIYUN_SMS_TEMPLATE_CODE` env vars. This endpoint must be network-restricted (loopback/internal only, matching Kratos's own admin API convention) or protected by a shared secret header, since it is an unauthenticated relay from Kratos's perspective.

**D3 — `phone` trait is additive and optional, `anyOf` extended to include it.**
`identity.schema.json`'s `traits.anyOf` currently requires `email` or `username`. Add `phone` as a third alternative so existing email-only identities are unaffected, and a phone-only identity (no email) is valid — matching bzton's behavior where phone-login users don't need an email.

**D4 — CN phone validation: use a broad, low-maintenance pattern instead of enumerating carrier prefixes.**
Use `^1[3-9]\d{9}$` (11 digits, starts with 1, second digit 3-9) rather than bzton's stale enumerated-prefix regex. Carrier prefix assignments change over time (this is exactly the gap that made bzton's regex miss valid numbers); a broad structural check plus the SMS gateway's own delivery failure is a more durable validation strategy than trying to keep an exact prefix list current.

**D5 — Auto-provisioning on first phone login uses Kratos's registration-flow code strategy, not a manual Admin-API identity-creation step. Confirmed: login and registration are two separate flows, chained by the frontend, not one flow that transparently does both.**
Reading `selfservice/strategy/idfirst/strategy_login.go:80-196` confirmed that Kratos's login flow with the code method does **not** auto-create an identity for an unrecognized identifier — it returns an account-not-found error (or, with account-enumeration mitigation on, silently renders the form without creating anything). So the frontend flow is: attempt a **login** flow with the phone identifier first; if it comes back as account-not-found, start a **registration** flow with the same phone number instead. Both flows use the `code`/`sms` method underneath, and the registration flow creates the identity and issues a session in one step via `selfservice.flows.registration.after.code.hooks: [{hook: session}]` (paralleling the existing `password`/`oidc` hooks already in `Kratos/CLAUDE.md`'s documented `kratos.yml`).

**D6 — Server-side send-code rate limiting reuses the existing in-process `RateLimiter` from `shared/go/api/auth/rate_limiter.go`, not a new Postgres table.**
That file already implements exactly this pattern (sliding window, keyed by string, used today for login/signup/password-reset — see `loginRateLimiter`/`signupRateLimiter`/`accountLockoutRateLimiter`). Adding an `smsRateLimiter` keyed by phone number is a small, consistent addition — no new migration or table needed. This has the same trade-off as the existing limiters (in-process, resets on restart, not shared across horizontally-scaled instances), which is an accepted trade-off already made for this codebase's other auth rate limits. This directly addresses the gap identified in the bzton reference (its send-code endpoint has no server-enforced cooldown, only a client-side 60s timer that's trivially bypassed).

## Risks / Trade-offs

- **[Shared Kratos blast radius]** `Kratos/kratos/identity.schema.json` and `kratos.yml` are shared by `ChenWeb` and `tax` (same instance, same ports, in this environment). Adding a `phone` trait + `code`/`sms` credential method makes phone-login *available at the Kratos API level* to `tax` as well, even though no `tax` UI will expose it. → **Mitigation**: verify the schema change is purely additive (existing `email`/`username`-only identities and flows are unaffected — confirm via a smoke test of `tax`'s email login after the change, before considering this change done), and call out in the ADR that a long-term follow-up could give `ChenWeb` its own Kratos instance if the two apps' auth requirements diverge further. Per workspace CLAUDE.md this is a staging environment, so a Kratos restart is an acceptable destructive-ish action here, but should still be a deliberate, communicated step, not incidental.
- **[New unauthenticated-ish attack surface]** The `/internal/sms-courier/send` relay is called by Kratos, not a logged-in user, so it can't use normal session auth. → **Mitigation**: restrict by network/loopback binding (same trust model as Kratos's own admin API on 4434) and/or a shared-secret header validated in the handler; apply the D6 rate limiter here regardless of caller.
- **[Aliyun cost/abuse]** SMS sends cost money per message. → **Mitigation**: D6 rate limiting, plus alerting/logging (per workspace logging convention) on send volume so abuse is visible quickly.
- **[Aliyun sign-name/template approval]** Aliyun requires manual approval of the SMS sign-name and template before sends succeed in production — this is a business/operational lead-time item, not something this change can control. → Track as a parallel non-engineering task; the relay code should fail gracefully (clear error surfaced to the frontend) if Aliyun rejects a send.
- **[Registration-vs-login flow selection prototype risk]** D5's approach (registration flow auto-creates identity + session via code method) is the standard Kratos pattern but hasn't been exercised in this codebase yet (unlike password/OAuth, which were already working before this change). → De-risk early: a tasks.md item should get one end-to-end phone signup+login working against a local Kratos instance before building the full Svelte UI around it.

## Migration Plan

1. Update `Kratos/kratos/identity.schema.json` (additive `phone` trait) and `Kratos/kratos/kratos.yml` (courier `channels` entry for `sms` pointing at the new relay endpoint; `selfservice.flows.registration.after.code.hooks`). Commit in the `Kratos` repo.
2. Implement and deploy the `/internal/sms-courier/send` relay handler in `shared/go/api/auth/` (dormant/unused until step 1 lands) and register its route. Commit in the `shared` repo.
3. Restart Kratos (config-only change, no `mise build-kratos` rebuild needed) and smoke-test: (a) existing `tax` email login still works, (b) a manual `curl` against Kratos's registration flow with a phone identifier successfully triggers a courier message to the relay.
4. Add the "Login with Phone" UI to `login-01.svelte` plus the `enablePhoneLogin` config flag. Commit in the `ChenWeb` repo.
5. Roll out with the flag off by default; enable once end-to-end testing (steps 1-3) is confirmed working, then flip the flag on.

**Rollback**: disable via `enablePhoneLogin` flag first (instant, no Kratos changes needed). If the Kratos schema/courier change itself needs reverting, revert `identity.schema.json`/`kratos.yml` and restart Kratos — safe because the trait/method addition is additive and no existing identity data is migrated or altered.

## Open Questions

- Confirm with the partner team whether the Aliyun account/sign-name/template used will be shared with bzton's existing one or provisioned separately for ChenWeb.
- ~~Does Kratos's registration-flow `code` strategy correctly auto-create a session on first phone verification with only a `phone` trait set (no email/username)?~~ Resolved — see D5: confirmed via source, no live prototype needed; the frontend chains a login attempt and a registration fallback rather than relying on one flow to do both.
- ~~Where should the per-phone send-rate-limit counter live?~~ Resolved — see D6: reuse the existing in-process `RateLimiter` from `rate_limiter.go`, no new table needed.
