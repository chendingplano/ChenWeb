## ADDED Requirements

### Requirement: Root account exists after server startup

When ChenWeb runs with Kratos authentication enabled, the server SHALL ensure a
privileged root account exists in the Kratos identity store before it begins
serving requests. If no such account exists, the server SHALL create it from
environment configuration. The check and creation SHALL run on every startup and
SHALL be idempotent.

The root account SHALL be a single Kratos identity that:

- carries the configured username (default `root`) as a `password`-method login
  identifier, AND
- carries the configured synthetic email (default `root@chenweb.local`) as a
  `password`-method login identifier, with that email marked verified so no
  verification message is attempted, AND
- has `metadata_public.admin = true`, `metadata_public.is_owner = true`, and
  `metadata_public.roles` containing both `root` and `admin`.

#### Scenario: Fresh identity store, valid configuration

- **WHEN** the server starts, `AUTH_USE_KRATOS` is `"true"`,
  `CHENWEB_ROOT_ACCOUNT_PASSWORD` is set, and no identity with the configured
  username or email identifier exists in Kratos
- **THEN** the server creates one Kratos identity with both identifiers, sets
  `admin = true`, `is_owner = true`, and `roles = ["root", "admin"]` in
  `metadata_public`, marks the synthetic email verified, logs the created
  identity id at info level, and continues startup

#### Scenario: Root account already present

- **WHEN** the server starts and a Kratos identity already exists for the
  configured username identifier (or the configured email identifier)
- **THEN** the server makes no create, update, or delete call for that identity,
  logs at info level that the root account already exists, and continues startup

#### Scenario: Subsequent restarts do not change an existing account

- **WHEN** the server has already created the root account and is restarted,
  including after `CHENWEB_ROOT_ACCOUNT_PASSWORD` in `.env` has been changed
- **THEN** the stored password, traits, and `metadata_public` of the existing
  identity are left exactly as they are (no auto-reset, no role rewrite)

#### Scenario: Root account is protected from removal via user management

- **WHEN** an administrator attempts to delete the root account, or to remove its
  admin flag, through the ChenWeb user-management API
- **THEN** the request is rejected because the identity is an owner
  (`metadata_public.is_owner = true`)

### Requirement: Bootstrap is safely skipped when not configured

The startup bootstrap SHALL be skipped, without failing the server, whenever it is
not applicable or not configured.

#### Scenario: Kratos auth disabled

- **WHEN** the server starts and `AUTH_USE_KRATOS` is not `"true"`
- **THEN** the bootstrap is skipped, a single informational log line records that
  it was skipped because Kratos auth is disabled, and startup proceeds

#### Scenario: Password not provided

- **WHEN** the server starts with `AUTH_USE_KRATOS="true"` but
  `CHENWEB_ROOT_ACCOUNT_PASSWORD` is empty or unset
- **THEN** no identity is created, a warning log line records that the root
  account bootstrap was skipped for lack of a configured password, and startup
  proceeds

#### Scenario: No password is ever hard-coded

- **WHEN** the source tree is inspected
- **THEN** no root account password value appears in committed code or in
  committed configuration files; the only sources are `.env` / environment
  variables

### Requirement: Bootstrap failures are non-fatal

A failure to reach Kratos or to create the identity SHALL NOT stop the ChenWeb
server from starting.

#### Scenario: Kratos Admin API unreachable during boot

- **WHEN** the existence check or the create call to the Kratos Admin API returns
  an error or times out
- **THEN** the server logs the error at error level with enough context to
  diagnose it, does not exit, and completes the rest of its startup sequence

#### Scenario: Kratos rejects the configured password

- **WHEN** Kratos rejects the create request because the configured password
  violates its password policy
- **THEN** the server logs the rejection reason at error level and continues
  startup, leaving no partial identity behind

### Requirement: Root account credentials are configurable via environment

The username, password, and synthetic email of the root account SHALL be read from
environment variables, with documented defaults for the non-secret values.

#### Scenario: Defaults applied

- **WHEN** `CHENWEB_ROOT_ACCOUNT_USERNAME` and/or `CHENWEB_ROOT_ACCOUNT_EMAIL` are
  unset but `CHENWEB_ROOT_ACCOUNT_PASSWORD` is set
- **THEN** the username defaults to `root` and the email defaults to
  `root@chenweb.local`

#### Scenario: Overrides applied

- **WHEN** `CHENWEB_ROOT_ACCOUNT_USERNAME` and `CHENWEB_ROOT_ACCOUNT_EMAIL` are
  set to non-empty values
- **THEN** those values are used verbatim (trimmed of surrounding whitespace) as
  the identity's identifiers

#### Scenario: Keys documented for operators

- **WHEN** an operator reads `ChenWeb/.env.example`
- **THEN** it lists `CHENWEB_ROOT_ACCOUNT_USERNAME`,
  `CHENWEB_ROOT_ACCOUNT_PASSWORD`, and `CHENWEB_ROOT_ACCOUNT_EMAIL` with
  explanatory comments
