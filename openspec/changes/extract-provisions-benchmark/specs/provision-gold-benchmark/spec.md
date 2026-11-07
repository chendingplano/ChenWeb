## ADDED Requirements

### Requirement: Versioned, frozen annotation rules
The `extract-provisions-benchmark` skill SHALL annotate under exactly one released rules version.
Each version is a file `rules/<version>.md` listed in `rules/versions.json` with its SHA-256. The
helper SHALL refuse an unknown version and a rules file whose hash differs from the manifest.
With no version named, the newest released, non-pre-release version SHALL be used.

#### Scenario: Edited released rules are refused
- **WHEN** `rules/1.0.0.md` is changed after release and `benchmark_io.py prepare --skill-version 1.0.0` runs
- **THEN** the helper exits 1 with a message that released rules are frozen

#### Scenario: Default version
- **WHEN** `prepare` runs without `--skill-version` and versions `1.0.0` and `2.0.0-rc.1` exist
- **THEN** the payload's `skill_version` is `1.0.0`

### Requirement: Gold provision unit and labels
Rules 1.0.0 SHALL define a gold provision as one normative statement. Normally that is a sentence,
or a list item or table cell that is not a full sentence. A sentence SHALL be split only where its
provision type or explicit grammatical subject changes. Imperative test-method steps SHALL be
provisions of kind `instruction` and type `mandatory`.

Every provision SHALL carry a `provision_kind` and a `provision_type`, related as follows:
`requirement`, `prohibition` and `instruction` map to `mandatory`; `recommendation` maps to
`recommended`; `permission` maps to `optional`. Every provision SHALL also carry a
`location_type` from `sentence paragraph bullet table_row table_cell heading_context mixed` and an
`is_explicit` boolean.

#### Scenario: Kind/type mismatch is refused
- **WHEN** a payload provision has `provision_kind = permission` and `provision_type = mandatory`
- **THEN** `validate` exits 1 naming the provision

#### Scenario: Table line needs a table location
- **WHEN** a provision cites only a `table` line with `location_type = sentence`
- **THEN** `validate` exits 1

### Requirement: Complete, evidence-backed run
`validate` SHALL refuse a payload unless all of the following hold:
- Its coverage regions account for every source line exactly once.
- Every candidate has a decision (`accepted|excluded|duplicate`), a rationale and a `rule_id`
  whose prefix matches the decision (`A`, `X`, `D`) and that is declared in bold in the rules file.
- Every provision is linked from an accepted candidate.
- Every evidence quote is an exact substring of a cited line.
- `provision` is source text: each ` … `-separated part occurs in the joined cited lines.
- No two provisions share the same spans and normalised text.
- `limitations` is empty.

#### Scenario: Uncovered line
- **WHEN** coverage omits line 57 of a 168-line document
- **THEN** `validate` exits 1 with "Coverage does not account for every source line"

#### Scenario: Paraphrased provision text
- **WHEN** a provision's `provision` text does not occur in its cited lines
- **THEN** `validate` exits 1

### Requirement: Immutable storage in testbed
`save` SHALL write the run audit row (`testbed.provision_benchmark_runs`), the gold provisions
(`testbed.provisions`) and the excluded and duplicate candidates (`testbed.provisions_dropped`)
in one transaction. The run key is (`skill_name`, `skill_version`, `model_name`, `run_id`).
`save` SHALL refuse a run key that already exists. It SHALL never write `kb.provisions`.

#### Scenario: Re-saving a run is refused
- **WHEN** `save` runs twice with the same payload
- **THEN** the second call fails and the stored run is unchanged

#### Scenario: Zero gold provisions
- **WHEN** a document has no provisions and the payload has zero provisions
- **THEN** the run row and the dropped candidates are saved and `testbed.provisions` gets no rows for the run

### Requirement: First gold run for record 416
A rules-1.0.0 gold run of record 416 SHALL be saved and read back. The run SHALL record the
invoking model's exact ID. Read-back SHALL match the validated payload's counts and substantive
fields.

#### Scenario: Read-back
- **WHEN** the 416 run is saved
- **THEN** reading `testbed.provisions` by its run key returns as many rows as the payload's provisions, with equal fields

### Requirement: Gold provision table highlights
The Gold Provisions viewer SHALL use saved table row and cell references to locate PDF
highlights, including continuation pages. A resolved table reference SHALL suppress the
ordinary canonical line-box highlight. For a legacy `table_cell` provision with only one
cited row and no cell reference, the viewer SHALL narrow to the column containing the
provision text only when exactly one source table cell contains that text after Unicode
and whitespace normalization. Missing or ambiguous text SHALL retain the row reference.
Stored gold records SHALL remain unchanged.

#### Scenario: Record 40 highlights its continuation-page cell
- **WHEN** record 416's provision `416_gprv_40` cites line 121, row r2, with `location_type = table_cell`
- **AND** its provision text occurs uniquely in the technical-requirements cell
- **THEN** the viewer highlights r2:c4 on PDF page 7 and does not highlight the table box on page 6
