## ADDED Requirements

### Requirement: Email signup requires first and last name
The system SHALL require a non-empty first name and non-empty last name to complete an email signup, both client-side (form validation) and server-side (`POST /auth/email/signup`). A request missing either field SHALL be rejected with HTTP 400 and the account SHALL NOT be created in Kratos.

#### Scenario: Email signup with both names provided
- **WHEN** a user submits the email signup form with email, password, first name, and last name all filled in
- **THEN** the account is created in Kratos with `traits.name.first` and `traits.name.last` set to the provided values

#### Scenario: Email signup missing first name
- **WHEN** a user submits the email signup form (or calls `POST /auth/email/signup` directly) with an empty or missing first name
- **THEN** the server returns HTTP 400 with a validation error message and no Kratos identity is created

#### Scenario: Email signup missing last name
- **WHEN** a user submits the email signup form (or calls `POST /auth/email/signup` directly) with an empty or missing last name
- **THEN** the server returns HTTP 400 with a validation error message and no Kratos identity is created

### Requirement: Phone signup requires first and last name, collected after code verification
The system SHALL require a non-empty first name and non-empty last name before finalizing a phone (SMS) signup for a phone number with no existing account. These fields SHALL be collected together with the SMS verification code — after the code has been confirmed valid, and before the Kratos registration flow completes — not as a separate step before the code is sent or entered.

#### Scenario: New phone number completes verification with name provided
- **WHEN** `POST /auth/phone/send-code` reports `flow_type: "registration"` for a phone number, and the user then submits `POST /auth/phone/verify` with a valid code plus non-empty first and last name
- **THEN** the account is created in Kratos with `traits.phone` and `traits.name.first`/`traits.name.last` set to the provided values, and a session is established

#### Scenario: New phone number verifies code but omits name
- **WHEN** `POST /auth/phone/verify` is called for a `flow_type: "registration"` flow with a valid code but an empty or missing first name or last name
- **THEN** the server returns HTTP 400 with a validation error message, no Kratos identity is created, and no session is established

#### Scenario: Existing phone number logging in is never asked for a name
- **WHEN** `POST /auth/phone/send-code` reports `flow_type: "login"` for a phone number with an existing account
- **THEN** the verification step does not require or accept first/last name, and login proceeds using only the phone number and code as it does today

### Requirement: Google OAuth signup is exempt
The system SHALL NOT require separate first/last name entry for signup via Google OAuth, since Google-supplied `given_name`/`family_name` claims already populate `traits.name.first`/`traits.name.last` at account creation.

#### Scenario: Google signup proceeds without a manual name prompt
- **WHEN** a user signs up via Google OAuth
- **THEN** the account is created using the name from Google's OIDC claims, with no additional name entry required from the user
