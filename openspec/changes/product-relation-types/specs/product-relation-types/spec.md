## ADDED Requirements

### Requirement: Relation type vocabulary table
The system SHALL store product relation types in `kb.product_relation_types`, keyed by the
short code, holding the stored value, status (`approved` or `proposed`), English and Chinese
descriptions, a weight, the first input record and first product seen, a usage count and an
`extra_info` JSON. The 20 relation codes in use at deployment plus `oth` SHALL be seeded as
`approved` with non-empty descriptions and the weights product review used before the change.

#### Scenario: Seed matches current behavior
- **WHEN** the migration has run
- **THEN** the table contains `sc`, `reg`, `req`, `perf`, `des`, `mat`, `test`, `cert`, `use`,
  `inst`, `maint`, `stor`, `ban`, `ex`, `part`, `has`, `compat`, `alt`, `meas`, `risk` and `oth`
  as approved, and each weight equals the value product review used for that relation

### Requirement: Relation code validated in code
The extractor SHALL NOT constrain the relation code in the model response schema. It SHALL
resolve each returned code against the table and accept the row in every case where the code
is non-empty.

#### Scenario: Known code
- **WHEN** the model returns `r:"stor"`
- **THEN** the product row stores `storage_requirement` and the code's usage count increases

#### Scenario: Unknown code becomes a proposal
- **WHEN** the model returns `r:"pkg"` and `pkg` is not in the table
- **THEN** a row with status `proposed`, value `pkg`, the input record, the product ID and
  an `extra_info` object is inserted, the product row is stored with `relation_type = 'pkg'`,
  and the call is not retried

#### Scenario: Known proposed code
- **WHEN** the model returns a code already in the table as `proposed`
- **THEN** no new table row is created, the usage count increases, and the product row is stored

#### Scenario: Empty code
- **WHEN** the model returns an empty relation code
- **THEN** the call fails the contract check and is retried as before

### Requirement: Vocabulary size limit
The system SHALL read `MAX_PRODUCT_RELATION_TYPES` from the environment (default 50). When an
insert would take the table above that number, the system SHALL log a WARN naming the limit
and the count, and SHALL still insert the row and accept the product.

#### Scenario: Limit exceeded
- **WHEN** the table holds 50 rows and a new unknown code arrives
- **THEN** the row is inserted, the product is accepted, and a WARN is logged

### Requirement: Proposal alert
The system SHALL log one WARN when a new proposed relation type is added, stating the code, the
record and the product, and asking an admin to review it.

#### Scenario: One alert per new code
- **WHEN** an unknown code appears in 10 rows of one run
- **THEN** exactly one proposal WARN is logged for that code

### Requirement: Approval
An admin SHALL approve a proposed relation type only with non-empty English and Chinese
descriptions and a weight between 0 and 1. The approval form SHALL prefill the weight with 0.5.

#### Scenario: Missing description rejected
- **WHEN** an approval request has an empty description
- **THEN** the server returns 400 and the type stays `proposed`

#### Scenario: Approved with weight
- **WHEN** an admin approves a proposed type with both descriptions and weight 0.6
- **THEN** the type is `approved`, the weight is 0.6, and approver and time are recorded

### Requirement: Consistency Check review card
The Consistency Check page under System Admin → Database Maintenance SHALL include a check
"Product relation types awaiting review", in English and Chinese. Its Check action SHALL list
every proposed type with its usage counts, and report that all are reviewed when there are
none. Each listed type SHALL offer Approve (with both descriptions and a weight prefilled 0.5)
and, while no product uses it, Delete. The server SHALL also allow editing an approved type's
descriptions and weight through the API.

#### Scenario: Review a proposal
- **WHEN** an admin runs the check while `pkg` is proposed
- **THEN** the card reports one proposed type and shows `pkg` with a form that requires both
  descriptions and shows a weight of 0.5

#### Scenario: Nothing to review
- **WHEN** an admin runs the check and no type is proposed
- **THEN** the card reports that every type has been reviewed

### Requirement: Weights feed product review
Product review SHALL take the relation-type weight from the table by stored value, falling back
to the configured weights and then 0.3 when the table has no row.

#### Scenario: Approved weight used
- **WHEN** `pkg` is approved with weight 0.8 and a product with `relation_type = 'pkg'` matches
- **THEN** the match is scored with weight 0.8
