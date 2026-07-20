## ADDED Requirements

### Requirement: Admin management of page defs and entries
The system SHALL provide admin API endpoints and an admin page under `/semos/admin/page-config` to create, read, update, and delete `kb.page_def` rows and `kb.page_config` entries, editing an entry's locales together and exposing its `access_role`, `accessible`, and `enabled` controls.

#### Scenario: Create an entry with both locales
- **WHEN** an admin creates an entry with `en` and `zh-cn` content
- **THEN** two `kb.page_config` rows are written sharing `page_key + entry_key`, one per language
- **AND** access/enable/accessibility set on the entry are stored on the default-language row

#### Scenario: Edit access and suspension controls
- **WHEN** an admin sets an entry's `enabled` to false, or clears its `access_role`, or sets `accessible` to false
- **THEN** the resolution API omits that entry on the next request without a backend restart

#### Scenario: Delete an entry
- **WHEN** an admin deletes an entry
- **THEN** all its per-language `kb.page_config` rows are removed

### Requirement: Admin write access restricted
The system SHALL require an authenticated user for admin read endpoints and restrict admin write endpoints (create/update/delete) to users who are admin, owner, or hold the `admin` role.

#### Scenario: Non-admin write rejected
- **WHEN** an authenticated non-admin user calls a write endpoint
- **THEN** the request is rejected as unauthorized

#### Scenario: Admin write allowed
- **WHEN** an admin/owner/`admin`-role user calls a write endpoint
- **THEN** the operation proceeds

### Requirement: Stable-key referencing in admin tooling
The admin tooling SHALL reference entries by `page_key + entry_key`, and SHALL surface diagnostics for unknown page keys and entry keys rather than creating text-keyed references.

#### Scenario: Unknown entry key surfaced
- **WHEN** admin tooling encounters a referenced `entry_key` with no matching row
- **THEN** it surfaces a diagnostic rather than silently creating or ignoring it
