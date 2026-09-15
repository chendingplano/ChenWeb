## Why

Email and phone signup can currently complete without a name: the email signup form's first/last name fields aren't enforced (client or server), and phone signup has no name fields at all — a new account is created from just a phone number. Downstream personalization (greetings, display name, admin/user lists) depends on `UserInfo.FirstName`/`LastName`, so accounts created without a name leave those fields empty indefinitely.

## What Changes

- Email signup: make First name / Last name required on the signup form (`LoginEmailGoogle.svelte`), and reject signup server-side (400) if either is blank, following the existing `ValidatePasswordDefault`-style validation-result pattern.
- Phone signup: when `/auth/phone/send-code` reports `flow_type: "registration"` (no existing account), the code-verification step additionally collects first/last name and submits them together with the code — after the SMS code is confirmed valid, but before the account is finalized. Reject (400) if either is blank for a registration flow.
- Existing accounts (`flow_type: "login"`) are never prompted for a name — this only applies to first-time account creation.
- `HandlePhoneVerifyCodeKratos`'s registration branch builds Kratos traits `name: {first, last}` (previously phone-only traits), consistent with how email signup already builds `name` traits.

## Capabilities

### New Capabilities
- `signup-name-requirement`: first/last name is mandatory when creating a new account via email or phone signup (not required for login, not required for Google OAuth which already supplies a name).

### Modified Capabilities
(none — no existing spec covers signup/auth in `openspec/specs/`)

## Impact

- **Frontend:** `ChenWeb/web/src/lib/components/auth/LoginEmailGoogle.svelte` (email signup form fields + its embedded phone flow), `ChenWeb/web/src/lib/components/auth/LoginCellPhoneOnly.svelte` (phone verify step UI).
- **Backend:** `shared/go/api/auth/kratos.go` (`HandleEmailSignupKratosBase`), `shared/go/api/auth/kratos_phone.go` (`phoneVerifyRequest`, `HandlePhoneVerifyCodeKratos`).
- **Shared library:** changes land in `shared/go`, so they affect every app that uses these handlers, not just ChenWeb — a `go work sync` and dependent-build check is required per the shared-library workflow.
- No DB schema or Kratos identity-schema changes — `name.first`/`name.last` traits and `UserInfo.FirstName`/`LastName` already exist end-to-end.
- Out of scope: Google OAuth signup (name already supplied by Google), any change to existing users' data.
