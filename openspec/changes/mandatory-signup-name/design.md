## Context

Two signup paths exist today, both in `shared/go/api/auth`:

- **Email** (`/auth/email/signup` → `HandleEmailSignupKratosBase`, `kratos.go:1349-1556`): explicit signup call. The form (`LoginEmailGoogle.svelte`) already has first/last name inputs and the handler already forwards them into Kratos `traits.name` when non-empty — neither side treats them as required.
- **Phone** (`/auth/phone/send-code` + `/auth/phone/verify`, `kratos_phone.go`): *implicit* signup. `send-code` tries a Kratos login flow first; if Kratos reports "no account" (message ID `4000035`), it starts a registration flow instead and returns `flow_type: "registration"` to the client alongside `flow_type: "login"` for existing accounts. `verify` submits the SMS code to whichever flow was started; for `"registration"` it currently sends `traits: {"phone": e164Phone}` only — Kratos creates the identity with no name.

`LoginEmailGoogle.svelte` also embeds its own copy of the phone-login flow (same endpoints), so the phone-side fix must be applied in two Svelte files.

## Goals / Non-Goals

**Goals:**
- Every new account (email or phone) has a non-empty first and last name in Kratos traits (and therefore in `UserInfo.FirstName`/`LastName`) at creation time.
- For phone signup, the name is collected after the SMS code has been verified as correct, but before Kratos finalizes the registration — one combined submission (code + name), not a separate screen before code entry.
- Existing accounts (`flow_type: "login"`) are never asked for a name; login stays exactly as fast as today.

**Non-Goals:**
- Backfilling names for existing accounts that were created without one.
- Changing Google OAuth signup (Google already supplies `given_name`/`family_name`, mapped automatically).
- Any Kratos identity-schema or DB migration — `name.first`/`name.last` are already optional traits in `identity.schema.json`, and `UserInfo.FirstName`/`LastName` are already wired end-to-end; this change only makes the values mandatory at the two signup call sites.

## Decisions

**1. Phone: collect name in the same request as code verification, not a separate step before it.**
Per product decision, the extra fields appear on the *code-entry* screen once the client knows (`flow_type === "registration"`) that this is a new account, and are submitted together with the code to `/auth/phone/verify` in one call. Alternative considered: ask for the name before sending the code — rejected, since it would ask unregistered-looking users for their name before we know an account doesn't already exist (and before the SMS code, which can still fail, confirms the phone number is reachable).

**2. Server validates name presence only for the registration branch, never for login.**
`HandlePhoneVerifyCodeKratos`'s `switch req.FlowType` already separates `"registration"` from `"login"`. The check (`first_name`/`last_name` non-empty) is added only inside the `"registration"` case, before calling `UpdateRegistrationFlow`, mirroring how `HandleEmailSignupKratosBase` validates password before calling Kratos. `flowType` is caller-supplied (echoed back from `send-code`), not attacker-controlled in a way that matters here — a client claiming `"login"` for what's actually a new number still hits Kratos's real login flow and fails there, same as today.

**3. Validation pattern: reuse the `Result{Valid, Errors}` shape from `ValidatePasswordDefault`.**
Add a small `ValidateNameFields(first, last string) Result` (or inline equivalent) in `shared/go/api/auth` so both `HandleEmailSignupKratosBase` and `HandlePhoneVerifyCodeKratos` return the same shape of 400 error (`KratosSignupResponse`/`KratosErrorResponse` with the first validation error as `Message`), consistent with existing error responses on both paths.

**4. `phoneVerifyRequest` grows two optional fields.**
`FirstName`/`LastName` are added to the existing `phoneVerifyRequest` struct (`kratos_phone.go:56-61`) rather than introducing a new endpoint. They're required only when `FlowType == "registration"`; the JSON tag names match the existing email-signup convention (`first_name`/`last_name`) for consistency across the two flows.

## Risks / Trade-offs

- **Duplicated phone-flow UI logic** (`LoginCellPhoneOnly.svelte` and the phone block in `LoginEmailGoogle.svelte`) means the name-collection UI must be added twice. Mitigation: keep the added markup/logic minimal and structurally identical in both, matching how the two files already mirror each other for the rest of the phone flow. Not extracting a shared component — out of scope for this change, per "touch only what you must."
- **Client can omit fields despite `required` on inputs** (e.g. direct API calls). Server-side validation in both handlers is the actual enforcement; client-side `required` is UX only.
- **Kratos registration flow could still fail after code verification** (e.g. race where the identity now exists) — existing error handling in the `"registration"` case already surfaces Kratos's error message; no new failure mode introduced.

## Migration Plan

No data migration. Deploy as a single shared-library + ChenWeb frontend change:
1. Land the `shared/go` handler changes, `go work sync`, confirm ChenWeb (and other dependents) still build.
2. Land the two Svelte component changes.
3. No feature flag — this is a hard requirement going forward; no rollback data concerns since no existing rows are touched.

## Open Questions

None outstanding — phone-flow timing (after code verification) and scope (new accounts only) were confirmed with the user before this design was written.
