## Why

Product relation types (`sc`, `stor`, `ban`, …) are hard-coded in four places: the merged
prompt text, `productV2Relations` and the response-schema enum in `products_v2.go`, the older
prompt's literal list in `extract-products.go`, and the weights/aspects in
`product-review.local.toml`. `kb.products.relation_type` is free text. When the model returns a
code outside the list, the schema enum rejects the whole call (bug 2026111703, issue #31:
`r:"pkg"`), costing a retry and risking a lost chunk, and there is no way for an operator to
see, describe or weight a new type.

## What Changes

- New table `kb.product_relation_types`: one row per relation code, seeded with the 20 current
  codes plus `oth`, each `approved` with descriptions (en/zh) and a weight.
- The extractor validates the relation code in code, not in the response schema. The `r` field
  becomes a plain string. An unknown code is looked up in the table; if absent it is inserted as
  `proposed` (with the input record, the first product ID seen, and an `extra_info` JSON) and the
  row is accepted either way. The product row stores the code's long value.
- A hard limit `MAX_PRODUCT_RELATION_TYPES` (default 50) on the table size. Above it, a
  warning is logged. Existing and new rows are still accepted.
- A WARN alert whenever a new `proposed` type is added, asking an admin to review it.
- A new check card on System Admin → Database Maintenance → Consistency Check (English and
  Chinese): Check lists the proposed types with usage counts; each can be approved in place with
  non-empty descriptions and a weight (default 0.5), or deleted when unused.
- Product-review scoring reads relation-type weights from the table instead of
  `[scoring.relation_type_weights]` in `product-review.local.toml`.
- Not in scope: rendering the prompt's code list from the table (decided separately; until then
  the prompt text and the table seed must be kept in step by hand), a `maps_to` merge feature.

## Capabilities

### New Capabilities
- `product-relation-types`: the relation-type vocabulary table, extractor validation and
  proposal behavior, the size limit and warnings, and the Consistency Check card.

### Modified Capabilities
<!-- None: no existing spec covers extract_products relation handling. -->

## Impact

- Code: `server/api/doc-processing/products_v2.go` (schema, expansion), `extract-products.go`
  (validation, proposal insert), `server/api/product-reviews/` (`config.go`, `docscope.go`
  weight lookup), a new `server/api/kbhandler/` handler and routes, and a web view under
  `web/src/lib/components/home3/` (a card embedded in `db-consistency-view.svelte`).
- Database: one goose migration creating and seeding `kb.product_relation_types`.
- Config: `MAX_PRODUCT_RELATION_TYPES`; the TOML weights table becomes a fallback or is removed
  (decided in design).
- i18n: new Paraglide keys in `web/messages/en.json` and `zh-cn.json`.
- Docs: bug 2026111703 (#31) is corrected (`stor` already covers transport);
  a devdoc for the relation vocabulary.
- Reusable modules searched: the catalogue (`KnowledgeStore/doc-repo/modules/`) and the code.
  Reused: the Consistency Check page (`db-consistency-view.svelte`) as the host and its card
  styling, and the Object Types handler's admin check (not catalogued). Considered and not used: `module-localized-text-editor` (canvas JSON value, not
  two plain columns). Nothing catalogued covers a
  vocabulary table with proposal and approval, so this is new code. IDs: the table's key is the
  relation code itself, so no shared-ID sequence is needed.
