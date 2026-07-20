## ADDED Requirements

### Requirement: Authenticated page-config resolution endpoint
The system SHALL expose an authenticated endpoint `GET /api/v1/page-config/:pageKey?lang=<code>` that returns, for the given page and the requesting user, only the entries that are both enabled and authorized, each with its content resolved for the requested locale.

#### Scenario: Returns visible authorized entries
- **WHEN** an authenticated user requests a page's config for a locale
- **THEN** the response lists each enabled, authorized entry as `{ entry_key, content }` with content resolved for that locale
- **AND** `status` is `true`, and the response echoes `page_key` and `lang`

#### Scenario: Unauthenticated request rejected
- **WHEN** an unauthenticated request hits the endpoint
- **THEN** the auth middleware rejects it

### Requirement: Access evaluation fails closed
The system SHALL treat an entry as accessible to a user only if the entry's default-language row has `accessible = true`, its `access_role` is non-empty and contains at least one valid role key (present in `appconfig.GetAccessRoles()`), and the user holds at least one of those roles (matched case-insensitively). Otherwise the entry is inaccessible.

#### Scenario: Accessible flag off hides entry from everyone
- **WHEN** an entry's default-language row has `accessible = false`
- **THEN** the entry is omitted for every user regardless of role

#### Scenario: Empty or invalid access_role suspends entry
- **WHEN** an entry's `access_role` is null, empty, or contains no valid role key
- **THEN** the entry is suspended and omitted for every user

#### Scenario: Role membership required
- **WHEN** an entry's `access_role` is `["admin"]` and the user does not hold `admin`
- **THEN** the entry is omitted for that user
- **WHEN** the same entry is requested by a user holding `admin`
- **THEN** the entry is included

### Requirement: Omit rather than hide-flag
The system SHALL omit disabled, suspended, and unauthorized entries from the resolution response entirely, rather than returning them with a client-side hide flag.

#### Scenario: Disabled entry absent from response
- **WHEN** an entry's default-language row has `enabled = false`
- **THEN** the entry does not appear in the response at all

### Requirement: Locale fallback via configured default
The system SHALL resolve an authorized entry's content from its requested-language row when present, otherwise from its default-language row (`[languages].default`), and fall back to the frontend's built-in defaults for any content field still absent.

#### Scenario: Requested language present
- **WHEN** a `zh-cn` row exists for an authorized entry and `lang=zh-cn`
- **THEN** the `zh-cn` content is returned

#### Scenario: Requested language missing falls back to default
- **WHEN** no row exists for the requested language but the default-language row does
- **THEN** the default-language content is returned

### Requirement: Unknown-key diagnostics
The system SHALL surface diagnostics in logs for unknown page keys and unknown entry keys, while keeping the page usable (fail open on rendering).

#### Scenario: Unknown page key logged
- **WHEN** a request targets a `page_key` with no `kb.page_def` row
- **THEN** a diagnostic is logged and the response returns an empty `entries` list rather than an error page
