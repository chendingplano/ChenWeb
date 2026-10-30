## Why

The existing text renderer cannot implement the human-approved R2 text element contract: localized content, optional caption/icon/tooltip and descriptor-driven appearance. The existing `text` descriptor has a draft revision awaiting implementation.

## What Changes

- Implement the approved 30-attribute contract and its 26 concrete defaults.
- Preserve existing class copies, string conversion, source exclusivity and unsupported-attribute warnings.
- Add bilingual typed Properties groups, accessible tooltip interactions and overflow handling.
- Activate a new version under the existing `text` identity after verification; update catalogue/documentation records.

## Capabilities

### New Capabilities

- `wpi-text-element`: Configurable localized read-only paragraphs, supporting content, defaults, validation and authoring.

### Modified Capabilities

- `wpi-page-rendering`: Clarify localized text-module descriptor content while keeping plain strings and bound conversion literal.

## Impact

ChenWeb renderer/types, Properties panel, English/Chinese messages, backend validator, project data migration, WPI OpenSpec and KnowledgeStore documentation. Existing module/class identities and saved pages are retained. No dependencies or schema changes.
