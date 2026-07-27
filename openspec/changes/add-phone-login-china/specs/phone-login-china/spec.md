## ADDED Requirements

### Requirement: Chinese Mobile Number Input Validation
The login page SHALL provide a "Login with Phone" mode whose phone-number field accepts only strings matching `^1[3-9]\d{9}$` (11 digits, starting with `1`, second digit `3`-`9`) and SHALL reject/disable submission for any other input before a network request is made.

#### Scenario: Valid Chinese mobile number
- **WHEN** a user enters `13812345678` into the phone-login field
- **THEN** the "Send code" button becomes enabled and no client-side validation error is shown

#### Scenario: Invalid or non-Chinese number rejected client-side
- **WHEN** a user enters a value that does not match `^1[3-9]\d{9}$` (e.g. `12345`, `+1 415 555 0100`, or an 11-digit number starting with `2`)
- **THEN** the "Send code" button remains disabled and a validation message is shown, with no request sent to the backend

### Requirement: SMS Verification Code Delivery
The system SHALL deliver a one-time verification code to a validated Chinese phone number via Aliyun's SMS gateway (Dysmsapi `SendSms`), triggered through Kratos's native `code` credential method and a Kratos courier channel, without any Aliyun credentials present in source code.

#### Scenario: Code send succeeds
- **WHEN** a user with a valid, rate-limit-eligible phone number requests a code
- **THEN** Kratos's code flow invokes the courier channel, the relay endpoint signs and calls Aliyun's `SendSms` API using credentials read from environment variables, and the user is shown a "code sent" confirmation without the code itself ever being returned to the client

#### Scenario: Aliyun send failure surfaces a clear error
- **WHEN** the Aliyun API call fails (e.g. invalid sign-name, template rejected, network error)
- **THEN** the relay endpoint returns an error to Kratos's courier, and the frontend shows a user-facing "couldn't send code, please try again" message rather than a silent failure or raw error payload

### Requirement: Server-Side Send-Code Rate Limiting
The system SHALL enforce a server-side limit on how many verification codes can be sent to a given phone number within a rolling time window, independent of any client-side cooldown UI.

#### Scenario: Client-side cooldown shown after a successful send
- **WHEN** a code is successfully sent to a phone number
- **THEN** the frontend disables the "Send code" button and shows a 60-second countdown before allowing another request

#### Scenario: Server rejects excessive send requests even if client-side limit is bypassed
- **WHEN** more than the configured maximum number of send requests for a given phone number occur within the configured rolling window (bypassing or ignoring the client-side cooldown)
- **THEN** the relay endpoint SHALL reject the additional request without calling Aliyun's API, and the frontend SHALL show a "too many requests, try again later" message

### Requirement: Phone + Code Login
A user SHALL be able to log in by submitting a validated Chinese phone number together with the verification code they received, resulting in a Kratos session equivalent to any other login method.

#### Scenario: Correct code logs an existing user in
- **WHEN** a user with an existing phone-linked Kratos identity submits their phone number and the correct, unexpired code
- **THEN** Kratos's code method verifies the code, a session is established, and the user is redirected the same way email/OAuth logins already redirect (`data.redirect_url`, default `/sidebar-01`)

#### Scenario: Incorrect or expired code is rejected
- **WHEN** a user submits a code that is wrong or has expired
- **THEN** login fails with a clear error message and no session is created

### Requirement: Auto-Provisioning on First Phone Login
When a verification code is successfully validated for a phone number with no existing Kratos identity, the system SHALL create a new Kratos identity with only the `phone` trait populated and establish a session in the same step, without requiring a separate registration form.

#### Scenario: First-time phone login creates an account
- **WHEN** a phone number with no matching Kratos identity successfully verifies its code
- **THEN** a new Kratos identity is created with the `phone` trait set to that number (no email/username required), and the user is logged in immediately with a session

### Requirement: Phone Login Feature Flag
Phone login SHALL be controllable via a feature flag (`enablePhoneLogin`) served from the existing `GET /api/config` endpoint, independent of any Kratos configuration change, so it can be disabled without reverting Kratos schema/courier config.

#### Scenario: Feature flag off hides the UI
- **WHEN** `GET /api/config` reports `enablePhoneLogin: false`
- **THEN** the login page does not render the "Login with Phone" mode/tab at all

#### Scenario: Feature flag on shows the UI
- **WHEN** `GET /api/config` reports `enablePhoneLogin: true`
- **THEN** the login page renders the "Login with Phone" mode alongside existing email/OAuth options

### Requirement: No Hard-Coded SMS Provider Credentials
Aliyun SMS credentials (access key ID, access key secret, sign name, template code) SHALL be read exclusively from environment variables at runtime and SHALL NOT appear as literal values in any committed source file, unlike the bzton reference implementation.

#### Scenario: Missing configuration fails safely
- **WHEN** the relay endpoint starts without `ALIYUN_SMS_ACCESS_KEY_ID`/`ALIYUN_SMS_ACCESS_KEY_SECRET`/`ALIYUN_SMS_SIGN_NAME`/`ALIYUN_SMS_TEMPLATE_CODE` set
- **THEN** the endpoint SHALL log a clear configuration error and return a failure response for any send request, rather than sending with empty/default credentials
