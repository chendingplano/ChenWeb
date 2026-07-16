## ADDED Requirements

### Requirement: Per-language label override files
The system SHALL support an optional TOML file per language at `config/knowledge-menus/labels-<lang>.toml`, each containing a `[labels]` table mapping a Wiki sidebar menu item id to a display label string. A missing file for a given language SHALL NOT be treated as an error.

#### Scenario: No label file for a language
- **WHEN** no `config/knowledge-menus/labels-<lang>.toml` file exists for the requested language
- **THEN** every menu item resolves to its hardcoded default label for that language

#### Scenario: Label file with a partial override set
- **WHEN** `config/knowledge-menus/labels-zh-cn.toml` defines `[labels]` with an entry for `kb-metrics` but not `kb-doc-wiki`
- **THEN** `kb-metrics` resolves to the configured override and `kb-doc-wiki` resolves to its hardcoded default label

### Requirement: Language-aware menu-config endpoint
`GET /api/v1/kb/menu-config` SHALL accept an optional `lang` query parameter and SHALL include a `labels` field in its JSON response containing the resolved id-to-label overrides for the requested language. Omitting or supplying an unrecognized `lang` value SHALL NOT error; it SHALL resolve to an empty `labels` map. The existing `menus` (visibility) field and its behavior SHALL be unaffected by `lang`.

#### Scenario: Requesting labels for a configured language
- **WHEN** a client calls `GET /api/v1/kb/menu-config?lang=zh-cn` and `config/knowledge-menus/labels-zh-cn.toml` defines `kb-metrics = "指标"`
- **THEN** the response includes `"labels": {"kb-metrics": "指标", ...}` alongside the existing `menus` field

#### Scenario: Omitted or unrecognized lang
- **WHEN** a client calls `GET /api/v1/kb/menu-config` with no `lang` parameter, or with a `lang` value that has no matching label file
- **THEN** the response is HTTP 200 with `"labels": {}` and the `menus` field populated exactly as it would be without any `lang` parameter

### Requirement: Sidebar resolves labels through the site-wide language control
The Wiki sidebar on `/home3/knowledge` SHALL determine "current language" from Paraglide's `getLocale()` (`$lib/paraglide/runtime`) and SHALL request menu-config labels for that language. For each visible menu item (top-level or child), the sidebar SHALL display the configured label override when present for the current language, and SHALL otherwise display the item's existing hardcoded default label. Item visibility (via `[knowledge-menus]`) SHALL be unaffected by language.

#### Scenario: Configured label overrides the default
- **WHEN** the current Paraglide locale is `zh-cn` and `config/knowledge-menus/labels-zh-cn.toml` defines `kb-doc-wiki = "知识百科"`
- **THEN** the sidebar shows "知识百科" as the label for that item instead of "Wiki"

#### Scenario: No override falls back to the hardcoded default
- **WHEN** the current Paraglide locale is `en`, or no override exists for an item under the current locale
- **THEN** the sidebar shows that item's existing hardcoded default label, unchanged from before this capability existed

#### Scenario: Switching site language updates menu labels
- **WHEN** the user switches the site-wide language via the existing Paraglide locale control (e.g. on `/semos`) and returns to or reloads `/home3/knowledge`
- **THEN** the sidebar requests and displays labels resolved for the newly selected locale
