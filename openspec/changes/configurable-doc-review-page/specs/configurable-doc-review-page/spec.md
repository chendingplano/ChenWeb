## ADDED Requirements

### Requirement: Page-owned static text is resolved from page-config

The Document Review wizard SHALL keep its structure, layout, step machine, and
hardcoded default strings page-owned, and SHALL resolve every configurable
page-owned static string through the DB-backed page-config resolver under the
stable `page_key` `doc-review`. Each configurable string SHALL be addressed by a
stable `entry_key` that maps to an id the page owns (not display text). The
tiers and aspects loaded from `doc-review.local.toml` are NOT part of this
surface and SHALL remain unchanged.

#### Scenario: Configurable strings resolve by entry_key

- **WHEN** the page renders any of its page-owned static strings (title,
  subtitle, the four step-indicator labels, the four step headings, the
  Next/Back/Start Review buttons, the labeled form fields, and the P1–P6 group
  labels)
- **THEN** each string is rendered via `labelFor(entry_key, hardcodedDefault)`
  so a page-config override for that `entry_key` replaces the default while any
  string without an override renders its hardcoded default

#### Scenario: Tiers and aspects are untouched

- **WHEN** the page loads the check-level tiers and aspect chips
- **THEN** they are still sourced from `listTiers()` / `listAspects()`
  (`doc-review.local.toml`) and are not read from or written to `kb.page_config`

### Requirement: Config fetch fails open to hardcoded defaults

The page SHALL hold the resolved config as a nullable state
(`PageConfig | null`), fetch `getPageConfig('doc-review', getLocale())` on
mount, and SHALL render the full page with its hardcoded default text whenever
the config is not yet loaded or the fetch fails.

#### Scenario: Fetch not yet resolved or errored

- **WHEN** the page-config fetch is still pending, or rejects
- **THEN** the config state is `null` and every string renders its hardcoded
  default and every item is visible (no blank or missing UI)

#### Scenario: Locale drives the requested language

- **WHEN** the page mounts with Paraglide `getLocale()` returning `zh-cn`
- **THEN** the resolver is called with `lang=zh-cn` and seeded `zh-cn` labels
  are rendered, falling back to the hardcoded default for any entry without a
  `zh-cn` override

### Requirement: Visibility uses the overlay model

Visibility SHALL follow the overlay model: an entry is visible by default and
is hidden only when the resolver reports its `entry_key` in `hidden` (disabled,
suspended, or unauthorized for the caller). An `entry_key` present in neither
`overrides` nor `hidden` has no config row and SHALL render its hardcoded
default. Every configurable entry SHALL be translatable/renamable via
`labelFor`. Because hiding wizard-critical chrome (navigation buttons, step
headings, grid-paired summary rows) would break the linear flow or layout,
`isVisible` gating SHALL be applied to the entries that render as standalone,
genuinely-optional blocks — the page subtitle and the two optional Step 4
template fields (report template, doc template) — mirroring how
`/semos/workspace` gates optional masthead sections while translating all
content.

#### Scenario: Operator disables an optional entry

- **WHEN** an operator sets `dr-subtitle` (or `dr-s4-report-label` /
  `dr-s4-doctpl-label`) `enabled = false` via `/semos/admin/page-config` and the
  page is reloaded
- **THEN** the resolver returns that `entry_key` in `hidden`, `isVisible` is
  false, and that standalone block is not rendered — with no backend restart
  required

#### Scenario: Deleted config row reverts to default

- **WHEN** an entry's config row is deleted
- **THEN** the `entry_key` appears in neither `overrides` nor `hidden` and the
  page renders that string's hardcoded default (visible to all authenticated
  users)

### Requirement: Unknown entry ids surface a diagnostic

The page SHALL warn when the resolver returns an `entry_key` (in `overrides` or
`hidden`) that does not match any real page-owned id, rather than failing
silently, while still rendering the page normally.

#### Scenario: Stale or mistyped entry_key

- **WHEN** the resolver returns an `entry_key` that no page-owned string uses
- **THEN** the page logs a `console.warn` naming the unrecognized id(s) and
  otherwise renders unaffected

### Requirement: Baseline seed reproduces current rendering

A goose migration in `project_migrations/` SHALL insert one `kb.page_def` row
for `doc-review` and one `kb.page_config` row per configurable entry per
configured language (`en`, `zh-cn`), idempotently
(`ON CONFLICT (page_key, entry_key, language) DO NOTHING`). The `en` rows SHALL
carry empty content (`{}`) so English rendering is unchanged; the `zh-cn` rows
SHALL carry translated labels. The default-language (`zh-cn`) rows SHALL carry
`access_role` (the current `[system].access_roles` keys), `accessible`, and
`enabled` so existing users retain access.

#### Scenario: Seed leaves English rendering unchanged

- **WHEN** the migration is applied and the page loads with locale `en`
- **THEN** every string renders exactly as before the change (all `en` content
  is `{}`, so the hardcoded default is used)

#### Scenario: Seed adds Chinese labels

- **WHEN** the migration is applied and the page loads with locale `zh-cn`
- **THEN** the seeded Chinese labels render for entries that have a `zh-cn`
  override, and the hardcoded default renders for entries left as `{}`
