# product-gold-benchmark Specification

## Purpose
A frozen, versioned answer key for `extract_products`: the `extract-products-benchmark` skill
annotates one document under a numbered rule book (candidate rules A/X/D, relation rules R1–R8), and saves
each run immutably in `testbed.product_benchmark_runs`, `testbed.products` and `testbed.products_dropped`.
The read-only Gold Products page (System Admin › Benchmark › Products) shows the runs and rejected candidates.
## Requirements
### Requirement: Versioned, frozen annotation rules
The `extract-products-benchmark` skill SHALL annotate under exactly one released rules version.
Each version is a file `rules/<version>.md` listed in `rules/versions.json` with its SHA-256.

The helper SHALL refuse:
- an unknown version;
- a rules file whose hash differs from the manifest.

With no version named, the newest released, non-pre-release version SHALL be used.

#### Scenario: Edited released rules are refused
- **WHEN** `rules/1.0.0.md` is changed after release and `benchmark_io.py prepare --skill-version 1.0.0` runs
- **THEN** the helper exits 1 with a message that released rules are frozen

#### Scenario: Default version
- **WHEN** `prepare` runs without `--skill-version` and versions `1.0.0` and `2.0.0-rc.1` exist
- **THEN** the payload's `skill_version` is `1.0.0`

### Requirement: Gold product relation unit
Rules 1.0.0 SHALL define a gold row as one (product, relation_type) pair. Every row SHALL carry a
`product_key`. All rows with the same key SHALL have the same `product_name`, `canonical_name`
and `product_type`, and no two rows of one key SHALL have the same `relation_type`.

A relation type SHALL be a gold row only when the evidence directly and clearly supports it. A
product with no specific supported relation SHALL have exactly one `scope` row.

#### Scenario: Two rows of one product disagree on type
- **WHEN** two payload rows share `product_key` but have different `product_type`
- **THEN** `validate` exits 1 naming the product key

#### Scenario: Repeated relation of one product
- **WHEN** two payload rows share `product_key` and `relation_type`
- **THEN** `validate` exits 1

### Requirement: Producthood decisions
Rules 1.0.0 SHALL accept the following as products:
- physical products, product classes, equipment, devices, components, accessories;
- software products, consumables;
- materials, substances and regulated waste streams;
- specified packaging.

Waste categories and waste streams (such as 可回收物, 易腐垃圾, 有害垃圾) SHALL be products of
type `product_class` or `material`.

Rules 1.0.0 SHALL exclude, under X rules:
- places, facilities and works;
- mentions supported only by citation or normative-reference lines;
- organizations, people, locations, abstract concepts and technologies, activities and services,
  legal acts, and document sections.

Every candidate SHALL record the bold rule ID that decided it.

#### Scenario: Rule ID not in the rules file
- **WHEN** a candidate's `rule_id` is `X99` and the rules file declares no bold `X99`
- **THEN** `validate` exits 1

#### Scenario: Decision prefix mismatch
- **WHEN** an excluded candidate has `rule_id = A1`
- **THEN** `validate` exits 1

### Requirement: Governed labels and source-grounded fields
Every gold row SHALL use the extractor's stored vocabularies:
- `product_type`: the ten values, without `unknown`;
- `relation_type`: the 21 values;
- `obligation_level`: the seven values;
- `related_products[].relationship`: the six values.

`validate` SHALL also refuse a payload unless:
- `related_products` is non-empty exactly when `relation_type` is `component_of`,
  `contains_product`, `compatible_with` or `replacement_or_alternative`;
- `evidence_lines` uses the forms `"N"`, `"N-M"`, `"N#rK"` and `"N#hK"`, and names only existing
  lines and table rows;
- `product_name` occurs in a cited line, unless `ext_info.name_expanded` is true, in which case
  each part must occur;
- `evidence_quote` is an exact substring of a cited line or cited table row;
- each ` … `-separated part of a non-empty `requirement_text` occurs in the joined cited lines;
- every `parameters` item has a non-empty `name` and `value`;
- `product_name_en` and `canonical_name_en` are present for non-English documents.

#### Scenario: Related products on a non-inter-product relation
- **WHEN** a row with `relation_type = scope` has a non-empty `related_products`
- **THEN** `validate` exits 1

#### Scenario: Paraphrased requirement text
- **WHEN** a row's `requirement_text` does not occur in its cited lines
- **THEN** `validate` exits 1

#### Scenario: Unknown table row
- **WHEN** a row cites `"131#r9"` and table line 131 has rows r1–r3
- **THEN** `validate` exits 1

### Requirement: Complete, evidence-backed run
`validate` SHALL refuse a payload unless all of the following hold:
- Its coverage regions account for every source line exactly once.
- Every candidate has a decision (`accepted|excluded|duplicate`), a rationale and a rule ID.
- Every accepted candidate links to a `product_key` that has rows.
- Every `product_key` is linked from an accepted candidate.
- `limitations` is empty.

#### Scenario: Uncovered line
- **WHEN** coverage omits line 57 of a 168-line document
- **THEN** `validate` exits 1 with "Coverage does not account for every source line"

#### Scenario: Orphan product
- **WHEN** a row's `product_key` is linked from no accepted candidate
- **THEN** `validate` exits 1

### Requirement: Immutable storage in testbed
`save` SHALL write the following in one transaction:
- the run audit row (`testbed.product_benchmark_runs`);
- the gold rows (`testbed.products`);
- the excluded and duplicate candidates (`testbed.products_dropped`).

The run key is (`skill_name`, `skill_version`, `model_name`, `run_id`). `save` SHALL refuse a run
key that already exists, and SHALL never write `kb.products` or `kb.product_names`.

#### Scenario: Re-saving a run is refused
- **WHEN** `save` runs twice with the same payload
- **THEN** the second call fails and the stored run is unchanged

#### Scenario: Zero gold products
- **WHEN** a document has no products and the payload has zero rows
- **THEN** the run row and the dropped candidates are saved, and `testbed.products` gets no rows for the run

### Requirement: First gold run for record 416
A rules-1.0.0 gold run of record 416 SHALL be saved and read back. The run SHALL record the
invoking model's exact ID. Read-back SHALL match the validated payload's counts and substantive
fields.

#### Scenario: Read-back
- **WHEN** the 416 run is saved
- **THEN** reading `testbed.products` by its run key returns as many rows as the payload has, with equal fields

