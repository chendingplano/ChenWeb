## 1. ADDED Requirements

### 1.1 Requirement-01: Registered artifact types

Retrieval SHALL be defined over a registered artifact type rather than over a specific table. Each
registered type SHALL supply an adapter providing: listing artifacts by `input_record_id`, matching
artifacts by a subject concept id, the `kb.search_artifacts` partition to fuse for hybrid matching,
and the projection of an artifact into a result row. Results SHALL be keyed by
`(artifact_type, artifact_id)` using the identifier convention of `kbsearch.BuildArtifactID`.
This change SHALL register `metric` and no other type.

#### 1.1.1 Scenario A: Metric type is registered

- **WHEN** a run requests `artifact_types = ["metric"]`
- **THEN** retrieval executes through the `metric` adapter
- **AND** result rows carry `artifact_type` `metric` and an `artifact_id` of the form
  `<record_id>_mtc_<seq>`

#### 1.1.2 Scenario B: Unregistered type is refused

- **WHEN** a run requests `artifact_types = ["provision"]`
- **THEN** the request is rejected with `CWB_KB_PMR_020` naming the unregistered type
- **AND** no run is created

### 1.2 Requirement-02: Document scoping

The system SHALL compute a scored document scope set for a profile before retrieving artifacts. A
document SHALL enter the set through any of:

- **Path A — product record match**: a `kb.products` row for that document whose identity matches an
  accepted scope node, matched by `object_id`/`concept_id` where the node is grounded and by
  normalized name or alias otherwise. The row's `relation_type` SHALL weight the score, with
  `scope`, `regulated_object`, and `requirement_target` weighted highest.
- **Path B — hybrid document match**: reciprocal-rank fusion of lexical and vector matches over the
  `product`, `summary`, and `topic` partitions of `kb.search_artifacts` using the node's labels.
- **Path C — document kind boost**: a document whose `document.doc_kind` facet value in
  `kb.doc_facet_values` is `da:doc_kind_standard`, `da:doc_kind_specification`, or
  `da:doc_kind_regulation` SHALL receive a configured score boost. Where that facet is absent, the
  system SHALL fall back to `kb.doc_facets.input_doc_type`. Document kind SHALL NOT be used as a
  filter.

Each document in the set SHALL record every matching node, every path that matched it, and its
fused score.

#### 1.2.1 Scenario A: Standard about the product is scoped

- **WHEN** a `kb.products` row for document 42 has `canonical_name` `Ventilator` and `relation_type`
  `scope`, and the profile root is grounded to the same product identity
- **THEN** document 42 enters the scope set with a Path A reason naming the root node and the
  `scope` relation type

#### 1.2.2 Scenario B: Standard is boosted over a non-standard

- **WHEN** two documents match a node with equal fused text scores and one has
  `doc_kind = standard` while the other has `doc_kind = manual`
- **THEN** the standard ranks above the manual

#### 1.2.3 Scenario C: Non-standard document is not excluded

- **WHEN** a document with `doc_kind = manual` matches a scope node
- **THEN** it enters the scope set, ranked below equally-matching standards, and is not filtered out

#### 1.2.4 Scenario D: Document matched through a part, not the product

- **WHEN** a document's only product record is `Display` and `Display` is an accepted part node
- **THEN** the document enters the scope set with its match attributed to the `Display` node

#### 1.2.5 Scenario E: Aspect-scoped document

- **WHEN** a document has a `kb.products` row for the product identity with `relation_type`
  `storage_requirement` and the profile has an accepted `storage` aspect node mapped to that
  relation type
- **THEN** the document enters the scope set attributed to the `storage` aspect node

### 1.3 Requirement-03: Two-path artifact retrieval

The system SHALL retrieve artifacts by two paths and union them:

- **Path D — document-first**: every artifact of the registered type belonging to a document in the
  scope set.
- **Path E — direct match**: artifacts whose subject concept equals an accepted scope node's
  `concept_id`, or whose subject matches a node through reciprocal-rank fusion over that type's
  `kb.search_artifacts` partition. Path E SHALL NOT be restricted to the document scope set.

An artifact found by both paths SHALL appear once, retaining both paths.

#### 1.3.1 Scenario A: Part metric found inside a product standard

- **WHEN** document 42 is in the scope set and contains a metric whose `metric_subject` is `Display`
- **THEN** the metric is returned once, with `paths` containing both `document_first` and `direct`

#### 1.3.2 Scenario B: Part metric found outside the document scope

- **WHEN** a metric with `subject_concept_id` equal to the `Display` node's concept belongs to
  document 77, which is not in the scope set
- **THEN** the metric is returned with `paths` containing `direct` only
- **AND** its result row names document 77

#### 1.3.3 Scenario C: Deduplication across paths

- **WHEN** the same `(artifact_type, artifact_id)` is produced by both paths
- **THEN** exactly one result row exists for it

### 1.4 Requirement-04: Relevance tiering and inclusion reasons

Every result SHALL be assigned exactly one tier, determined by what it matched and independent of
which path found it, in this precedence: `direct` when its subject matches the `product` root node;
`part` when its subject matches a `module` or `part` node; `aspect` when its subject or context
matches an `aspect` node; `document_scope` when it matched no node but belongs to a document in the
scope set. Every result SHALL carry the matched node id (null only for `document_scope`), the
`paths` that found it, a score, and a human-readable inclusion reason naming the node and the
evidence.

#### 1.4.1 Scenario A: Tier reflects the matched node, not the path

- **WHEN** a metric whose subject is `Display` is found only through the document-first path
- **THEN** its tier is `part`, not `document_scope`

#### 1.4.2 Scenario B: Unattributed metric is retained

- **WHEN** a metric in a scoped document matches no scope node
- **THEN** it is returned with tier `document_scope` and a null matched node
- **AND** its inclusion reason names the document and why the document was in scope

#### 1.4.3 Scenario C: Inclusion reason is populated

- **WHEN** any result is returned
- **THEN** its inclusion reason is a non-empty string naming the matched node or the scoping
  document

### 1.5 Requirement-05: Result caps

The system SHALL apply configured caps to the number of artifacts contributed per document and to
the total results per run. When a cap truncates results, the system SHALL keep the highest-scoring
results, SHALL record the truncated count on the run, and SHALL NOT silently drop a tier above
`document_scope` in favour of a `document_scope` result.

#### 1.5.1 Scenario A: Per-document cap truncates the lowest-scoring hits

- **WHEN** a single document contributes more metrics than the per-document cap
- **THEN** the highest-scoring metrics up to the cap are kept
- **AND** the run records the number truncated for that document

#### 1.5.2 Scenario B: Attributed results survive truncation

- **WHEN** the run-level cap is reached and both `part`-tier and `document_scope`-tier results are
  candidates for removal
- **THEN** `document_scope` results are removed first

### 1.6 Requirement-06: Retrieval determinism

Given the same profile version, the same corpus, and the same configuration, two runs SHALL produce
the same result set with the same tiers. Retrieval SHALL NOT invoke an LLM.

#### 1.6.1 Scenario A: Re-run over an unchanged corpus is stable

- **WHEN** a run is repeated with no document ingested and no profile edit in between
- **THEN** the two runs return identical `(artifact_type, artifact_id)` sets with identical tiers
