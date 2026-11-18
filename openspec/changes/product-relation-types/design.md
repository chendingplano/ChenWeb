## Context

`extract_products` (merged v2/v3 path) asks the model for a short relation code `r` and expands
it through `productV2Relations` into the long value stored in `kb.products.relation_type`
(`stor` → `storage_requirement`). The response schema constrains `r` to that map's keys, so a
code outside it fails the call (issue #31). The vocabulary is also duplicated in the prompt
text, the older prompt's literal list, and `product-review.local.toml` (weights keyed by the
long value, default 0.3 for unlisted values; aspects list long values).

Constraints: schema changes go through goose; every operation is logged with the default
logger (WARN lines reach `jimo_alert_log`); all user-visible text goes through Paraglide;
model names and similar settings come from environment variables.

## Goals / Non-Goals

**Goals:**
- One table is the source of truth for relation codes, their descriptions and weights.
- An unlisted code never fails or drops a row; it becomes a reviewable proposal.
- Admins are told when a proposal appears, and cannot approve one without a description.
- The vocabulary cannot grow without bound unnoticed.

**Non-Goals:**
- Rendering the prompt's code list from the table (separate decision; until then the prompt
  and seed are edited together by hand).
- Merging or mapping a proposed code onto an existing one (`maps_to`).
- Changing the older (non-merged) extraction path beyond what it already does.
- Related-product codes (`rp[].r`: part, has, compat, alt, vs, oth) stay a fixed schema enum.

## Decisions

**D1. Table `kb.product_relation_types`.** Columns: `code` TEXT PRIMARY KEY (short code as
returned by the model, lower-cased), `value` TEXT NOT NULL UNIQUE (the string stored in
`kb.products.relation_type`; for a proposed code it equals the code), `status` TEXT
(`approved` | `proposed`), `description_en`, `description_zh`, `weight` NUMERIC(4,3) NOT NULL
DEFAULT 0.5, `first_record_id` BIGINT (input record), `first_product_id` (the kb.products row
first seen), `seen_count` BIGINT, `extra_info` JSONB NOT NULL DEFAULT '{}', `created_at`,
`approved_at`, `approved_by`. A CHECK requires non-empty descriptions when `status =
'approved'`. The natural key avoids a sequence, so no ID-manager use. Alternative considered: a
surrogate BIGSERIAL id; rejected because the code is already unique and is what the model and
the page speak.

**D2. Seed preserves current behavior.** The migration inserts the 20 codes plus `oth`
(`approved`, long values from `productV2Relations`, English and Chinese descriptions taken from
the prompt's definitions). Weights are copied from `[scoring.relation_type_weights]`; the five
values missing there (prohibited_product, exempted_product, compatible_with,
replacement_or_alternative, measurement_object) are seeded at 0.3, the current fallback, so
review scores do not move.

**D3. Validate in code.** `r` becomes `schemaString()` in the response schema. After parsing,
a resolver normalizes the code (trim, lower-case) and looks it up in a cached copy of the
table. Known (approved or proposed): use its `value`, bump `seen_count`. Unknown: insert a
`proposed` row (`INSERT … ON CONFLICT DO NOTHING`, then re-read, so concurrent pipelines
agree), accept the row. An empty code is still a contract failure and goes through the
existing retry. The resolver lives in the products package behind a small interface so tests
use a fake store.

**D4. Size limit.** `MAX_PRODUCT_RELATION_TYPES`, read from the environment, default 50. The
count is of all rows (approved + proposed). Exceeding it logs WARN
`product relation type limit exceeded` on each new insert attempt but still inserts and
accepts, since dropping rows would be the data loss this change removes. Alternative
considered: refuse the insert and map to `oth`; rejected for hiding a vocabulary problem and
because the user asked for a warning.

**D5. Alerts.** Inserting a proposed type logs WARN `new product relation type proposed`
with code, record and product id, saying an admin should review it on the Product Relation
Types page. It fires once per new code, not per row. Approval and rejection are logged INFO.

**D6. Approval.** Endpoints under `/kb/product-relation-types`: list (with counts), approve
(body: `description_en`, `description_zh`, `weight`; 400 if either description is blank or the
weight is outside 0–1; weight defaults to 0.5 in the form), and edit of descriptions/weight on
approved rows. Approved codes cannot be deleted. A proposed code can be deleted only when no
`kb.products` row uses its value.

**D7. Consistency Check card.** The review lives on System Admin → Database Maintenance →
Consistency Check as a second card under the existing `kb.inputs.status` check (component
`product-relation-types-check.svelte`, embedded by `db-consistency-view.svelte`). Check calls
the list endpoint and keeps the `proposed` rows: none → "all reviewed"; otherwise each proposal
is shown with its usage counts and a form of two description textareas (English, Chinese) and a
weight input prefilled 0.5. Approve needs both descriptions (the client mirrors the server
validation); Delete is disabled while products use the type. Approved types are not listed on
the page; editing one is possible through the API only. Keys are `prt_*` in both message files.
An earlier draft was a separate Object-Types-style page; it was folded into this check because
the Consistency Check page is where admins look for data that needs attention.

**D8. Weights in product review.** `docscope.go` reads weights from the table (keyed by
`value`, approved rows only, loaded once per scope run), falling back to the TOML map and then
0.3 when the table has no row. The TOML table is kept as a fallback in this change and marked
deprecated; removing it is a follow-up once production is seeded.

## Reused modules

- Consistency Check page (`db-consistency-view.svelte`): the host. The new card copies its
  styling tokens and Check/status banner layout; the existing card is unchanged.
- Object Types handler (`object_types_handler.go`; not catalogued): its admin check
  (`objectTypeReviewAdmin`) is reused by the new handler. Its page was not extended, since it
  is specific to `kb.object_types`.
- `module-localized-text-editor`: considered for the en/zh descriptions and not used. It edits a
  `{"en","zh-cn"}` JSON value inside the canvas property-editor framework; this table stores two
  plain columns, so two textareas are simpler.
- Searched and not fitting: `module-attention-items` (queue of KB data problems with reopen
  semantics), `module-property-editor` (canvas property editing, no approval workflow).
- No new reusable module is registered: the card is specific to this table.

## Risks / Trade-offs

- [Prompt and table drift: a code approved in the table never appears in the prompt] →
  Documented; the open decision to render the prompt from the table removes it.
- [Model invents many junk codes, e.g. `pkg`, polluting `kb.products`] → Proposals are
  visible, counted by `seen_count`, and capped by the limit warning; unreviewed values are
  stored as written, as requested.
- [A proposed code scores at the 0.3 fallback in product review until approved] → Weight
  defaults only apply at approval; proposed rows read as 0.3 so scoring is unchanged.
- [Aspects in the TOML list long values, so a new approved type matches no aspect] → Out of
  scope; the approval page shows usage counts so admins can spot it. Follow-up if wanted.
- [Concurrent doc-processor runs inserting the same code] → Primary key plus `ON CONFLICT`.

## Migration Plan

1. Apply the goose migration (creates and seeds the table, seeds the page_config entry) with
   `doc-service --migrate-only`.
2. Deploy the server; the extractor and product review start reading the table.
3. Rollback: revert the build; the table is additive and `kb.products.relation_type` values
   stay valid. Rows holding proposed values keep their strings.

## Open Questions

- Render the prompt's relation list from the table (recommended; separate change)?
- Should `status = 'rejected'` exist so a junk code is remembered and mapped to `oth` going
  forward? Not in this change.
