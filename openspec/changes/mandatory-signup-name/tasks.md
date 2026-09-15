## 1. Shared validation helper

- [x] 1.1 Add a `ValidateNameFields(first, last string) Result` helper in `shared/go/api/auth` (reusing the `Result{Valid, Errors}` shape from `password_validation.go`), returning an error when either is blank.

## 2. Email signup enforcement

- [x] 2.1 In `shared/go/api/auth/kratos.go` (`HandleEmailSignupKratosBase`), call the new validation helper on `firstName`/`lastName` and return HTTP 400 (`KratosSignupResponse`) before creating the Kratos registration flow if either is blank.
- [x] 2.2 In `ChenWeb/web/src/lib/components/auth/LoginEmailGoogle.svelte`, mark the First name and Last name signup inputs (~line 377-390) as `required`.

## 3. Phone signup: request/response plumbing

- [x] 3.1 In `shared/go/api/auth/kratos_phone.go`, add `FirstName`/`LastName` fields (JSON tags `first_name`/`last_name`) to `phoneVerifyRequest`.
- [x] 3.2 In `HandlePhoneVerifyCodeKratos`'s `"registration"` case, validate `FirstName`/`LastName` with the new helper (return HTTP 400 via `KratosErrorResponse` on failure) before calling `UpdateRegistrationFlow`, and build `Traits: map[string]any{"phone": e164Phone, "name": map[string]any{"first": req.FirstName, "last": req.LastName}}`.
- [x] 3.3 Confirm the `"login"` case is untouched — no name fields read or required.

## 4. Phone signup: frontend

- [x] 4.1 In `ChenWeb/web/src/lib/components/auth/LoginCellPhoneOnly.svelte`, when `phoneFlowType === 'registration'`, show first-name/last-name inputs on the code-entry (`enter-code`) step, and include them (`required`) in the form.
- [x] 4.2 Update `handleVerifyPhoneCode` in the same file to include `first_name`/`last_name` in the `POST /auth/phone/verify` body when `phoneFlowType === 'registration'`.
- [x] 4.3 Apply the same two changes (name inputs shown conditionally on `flow_type`, included in the verify request) to the embedded phone-login block in `ChenWeb/web/src/lib/components/auth/LoginEmailGoogle.svelte`, mirroring `LoginCellPhoneOnly.svelte`'s structure.

## 5. Verification

- [x] 5.1 `cd shared/go && go test ./api/auth/...` and `go vet ./api/auth/...`.
- [x] 5.2 From workspace root: `go work sync`, then `cd ChenWeb && go build ./...` to confirm the dependent still builds. (Also verified `tax` still builds against the shared-library change.)
- [x] 5.3 `cd ChenWeb/web && bun run check` (or project's type-check command) for the two Svelte files. (3 pre-existing errors elsewhere in the codebase, unrelated test-file `bun:test` typing issues — zero diagnostics in the two files this change touched.)
- [x] 5.4 Manually exercised against the running dev server (restarted to pick up the shared/go change - see session note below):
  - Email signup, missing first name → live `curl` to `/auth/email/signup`: **400** `"first name is required"`.
  - Email signup, missing last name → live `curl`: **400** `"last name is required"`.
  - Email signup, both names present → live `curl`: name check passes, falls through to (pre-existing) password validation, confirming the new check doesn't block valid input.
  - Phone signup/login (both `flow_type` branches) — **not exercised live**: doing so would require a real `/auth/phone/send-code` call, which sends an actual SMS via Aliyun to a real number and was judged not safe/appropriate to trigger without the user present. Verified by code reading only: the `"registration"` case's name check runs before any Kratos/SMS call completes, and the `"login"` case is byte-for-byte unmodified (see 3.3). Recommend the user click through both phone paths manually once.
