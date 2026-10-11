## ADDED Requirements

### Requirement: extract_references is an independent Phase B processor
The system SHALL provide a doc processor named `extract_references` that runs in Phase B, depends
only on `chunking`, and reads no output of any other extractor (`extract_metrics`,
`extract_provisions`, `extract_entity_relation`, `extract_inventory_items` or others). Its status
SHALL be written to `kb.inputs.status` under operation `extract_references` by the chunk batch
coordinator.

#### Scenario: Runs without any other extractor
- **WHEN** a record is processed with operation `["chunking", "extract_references"]`
- **THEN** references are extracted and stored, and the status entry `extract_references` reaches `success`

#### Scenario: Prompt is a file
- **WHEN** the processor is constructed
- **THEN** its prompt is loaded from `prompts/prompt-extract-references-v1.md` (overridable by `EXTRACT_REFERENCES_PROMPT`) and its model from `EXTRACT_REFERENCES_MODEL_NAME`

### Requirement: Every reference is stored with the reference model's labels
For each reference found, the processor SHALL store one row in `kb.ref_mentions` with:
- source location (`section`, `source_line_spans`, `source_table_rows`, `evidence`);
- `cited_raw` and the parsed `identifiers`;
- `target_type` and `target_subtype` (+ `target_subtype_label`);
- `mechanism`, `relationships`, `target_scope`, `target_locator`, `target_content`;
- `ref_form`, `list_kind`, `in_requirement`, `internal`, `confidence`;
- the named quantity when `target_content` is `criterion`;
- a stable `mention_key`.

Every label SHALL be a value of its governed vocabulary.

#### Scenario: Unknown subtype
- **WHEN** the extractor proposes a subtype that is not in the governed vocabulary of its target type
- **THEN** `target_subtype` is `others` and the proposed label is kept in `target_subtype_label`

#### Scenario: Unknown enum value
- **WHEN** the extractor returns a mechanism, relationship, scope or content value outside its vocabulary
- **THEN** the value is replaced by the vocabulary's general value (`named_reference`, `references`, `document`, `general`), and the replacement is logged

### Requirement: Identifiers are parsed and normalized in code
The processor SHALL parse identifiers from `cited_raw` deterministically. It SHALL keep a standard's
publisher prefix, number and part apart from its edition. It SHALL case-fold DOIs, and it SHALL
recognize URLs, arXiv ids, ISBN, ISSN, PMID and RFC numbers.

#### Scenario: Standard number without space
- **WHEN** `cited_raw` is `GB/T755-2019`
- **THEN** the identifier is standard `GB/T 755` with edition `2019`

#### Scenario: Part number is not an edition
- **WHEN** `cited_raw` is `ISO 14253-1`
- **THEN** the identifier is standard `ISO 14253-1` with no edition

### Requirement: Normativity comes from formal lists only
`normativity` SHALL be `normative` when the document lists the cited resource in its normative
references list. It SHALL be `informative` when the resource is listed only in an informative list,
and `unlisted` otherwise. Wording SHALL NOT change normativity. In-text mentions SHALL link to the
list entry of the same resource (`list_entry_id`). When the document has a normative references
list, `list_mismatch` SHALL be `not_listed` for an in-requirement mention of an unlisted resource,
and `never_cited` for a normative entry that no in-text mention uses.

#### Scenario: Binding citation without a list
- **WHEN** a regulation says "shall comply with Regulation (EU) 2016/679" and has no reference list
- **THEN** the mention is `unlisted` with `in_requirement = true`

#### Scenario: In-text citation of a listed standard
- **WHEN** clause 5 cites GB/T 755 and §2 lists GB/T 755 as a normative reference
- **THEN** the in-text mention is `normative` and its `list_entry_id` is the §2 entry

### Requirement: Resources are catalogued with identity separate from location
Each mention with an identifier or title SHALL resolve (R1) to one row of `kb.ref_resources`,
keyed by identity key and visibility. A cited edition SHALL resolve to a row of
`kb.ref_resource_versions`. Every identifier SHALL be recorded in `kb.ref_resource_identifiers`.

#### Scenario: Same standard written two ways
- **WHEN** one document cites `GB/T 755` and another cites `GB/T755-2019`
- **THEN** both mentions point to the same resource, and the second also to version `2019`

### Requirement: Public resources are shared, private ones are not
A resource keyed by a public identifier kind SHALL have visibility `public` and SHALL be shared
across all customers. It SHALL hold only public facts. A resource known only from a private source
SHALL be scoped to the citing record's knowledge store, or to its user when the record has no
store. The attachment of a document (`kb_input` identifier) SHALL never be public.

#### Scenario: Internal file path
- **WHEN** a document cites `docs/architecture.md`
- **THEN** the resource's visibility is `store:<ks_store_id>` of the citing record

### Requirement: Holding resolution respects access
R2 SHALL link a mention to a `kb.inputs` record only when that record is attached to the mention's
resource and the citing record's owner can access it (same user, same knowledge store, or an
active unexpired grant). An undated citation SHALL take the newest held version. A dated citation
with only other versions held SHALL be `edition_mismatch`.

#### Scenario: Another customer's copy
- **WHEN** the only held copy of the cited standard belongs to a different user and store, with no grant
- **THEN** the mention's `r2_state` is `not_held`

### Requirement: Each processed document is attached to its own resource
The processor SHALL attach the document it processes to its resource. It SHALL take the identifier
from `kb.inputs.doc_no`, else from the document's own identifier read from its first chunks, else
from a standard number in the title or filename.

#### Scenario: Missing doc_no
- **WHEN** `kb.inputs.doc_no` is empty and the filename contains `GB_T 46121-2025`
- **THEN** the document is attached to resource `standard:GB/T 46121`, version `2025`

### Requirement: Re-running replaces the record's references
Re-running `extract_references` on a record SHALL first withdraw what resolvers applied for the
record's previous mentions, then replace all of the record's `kb.ref_mentions` rows.

#### Scenario: Re-run
- **WHEN** `extract_references` runs twice on the same record
- **THEN** the record has only the second run's mentions, and no links or resolved rows from the first run remain

### Requirement: Overlapping chunks do not duplicate references
A list line (or logical table row for a table-based list) SHALL yield one listed reference, and an in-text occurrence SHALL be identified by what
it cites (its identity key when it has one), so the same occurrence seen by two overlapping chunks
is stored once.

#### Scenario: Same list line with different raw text
- **WHEN** two chunks report line 47 of the normative references list as `GB/T755` and `GB/T755 旋转电机 定额和性能`
- **THEN** one listed reference is stored for line 47

### Requirement: Only listable documents can be "not listed"
`list_mismatch = not_listed` SHALL be set only for references to a resource with a public
identifier. A reference to manufacturer's instructions or similar unidentified material SHALL NOT
be flagged.

#### Scenario: Manufacturer's instructions in a requirement
- **WHEN** a clause says "shall be installed according to the manufacturer's instructions" in a document with a normative references list
- **THEN** the reference has no `list_mismatch`

### Requirement: Table references retain logical row provenance
For references extracted from tables, the processor SHALL request `source_table_rows` as logical
row citations (`116#r1`), validate them against the source table grid, and store normalized JSONB
`[{"line":116,"rows":["r1"],"row_hash":{"r1":"<system hash>"}}]` in
`kb.ref_mentions.source_table_rows`, using the same shape and grid hashes as `extract_metrics`.
The system SHALL compute hashes itself and SHALL exclude nonexistent rows and rows outside the
mention's source line spans. Evidence SHALL include the caption and the selected rows rendered
with column labels. Mixed prose/table references SHALL retain prose evidence alongside the rows.
When no valid explicit rows exist, row selection SHALL use metrics' evidence matching and small
table fallback; unmatched references SHALL retain their original evidence and have no stored rows.
Prose references SHALL have no table provenance. The references list API SHALL return stored
`source_table_rows`, and resolved metric rows SHALL carry the referencing mention's rows.
Deduplication and stable mention keys SHALL distinguish different row locations on the same line.

#### Scenario: Different occurrences in one table
- **WHEN** rows `116#r1` and `116#r2` both cite ISO 9001 and overlapping chunks repeat row `r1`
- **THEN** two mentions are stored with distinct mention keys and their respective row provenance

#### Scenario: Citation uses multiple rows
- **WHEN** a reference cites `116#h0`, `116#r1` and `116#r2`, plus a nonexistent `116#r99`
- **THEN** all three valid rows are retained with system hashes and rendered evidence, and `r99` is excluded

#### Scenario: Resolved metric preserves the source table
- **WHEN** a table-based criterion reference resolves to a metric of another document
- **THEN** the resolved metric row stores `source_table_rows` from the referencing document's mention
