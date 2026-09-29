## ADDED Requirements

### Requirement: Interface text is localised through Paraglide
User-visible text in ChenWeb `.svelte` files SHALL come from Paraglide messages, and every message
key SHALL exist with a non-empty value in both `web/messages/en.json` and `web/messages/zh-cn.json`.

#### Scenario: Key missing in one language
- **WHEN** a key exists in `en.json` but not in `zh-cn.json`
- **THEN** `bun run check` fails naming the key and file

#### Scenario: New page with hard-coded text
- **WHEN** a new `.svelte` file contains literal text such as `<h2>New Page</h2>` or `placeholder="Search"`
- **THEN** `bun run check` fails listing each item with its line

#### Scenario: Existing page gains hard-coded text
- **WHEN** a file listed in `i18n-baseline.json` has more hard-coded items than its baseline count
- **THEN** `bun run check` fails

### Requirement: Side-menu labels default to Paraglide
The `/development` NavRail SHALL take each item's label and each group heading from Paraglide
messages; a `kb.page_config` label for the entry, when present, SHALL override it.

#### Scenario: Menu item without a page_config label
- **WHEN** the UI language is zh-cn and the entry has no `kb.page_config` label
- **THEN** the item shows its `zh-cn` message (e.g. Review Metrics → 指标审查)
