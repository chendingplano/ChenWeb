## ADDED Requirements

### Requirement: Class-contract hybrid-search index table

The system SHALL maintain a dedicated table `kb.ontology_class_contract_search` holding one row
per governed class term, created by a goose migration. A "governed class term" here is a
`kb.ontology_term_headers` row that has a current class-contract revision
(`current_contract_revision_id` set) OR is referenced by some metric assertion's
`instance_of_term_id` — it is NOT keyed on `term_kind`, because synthesized metric classes carry
`term_kind = 'metric_definition'`, not `'class'`. Each row SHALL
carry at least: `class_term_id` (primary key), the class's `current_contract_revision_id`, its
`definition_state`, its `module_id`, a `search_document` text field, a `search_vector` tsvector
over that text, an optional `embedding_text`, an optional `embedding` of type `vector(1536)`, an
`instance_count`, and an `updated_at` timestamp. The migration SHALL create a GIN index on
`search_vector` and an HNSW (`vector_cosine_ops`) index on `embedding`. The table SHALL NOT be
partitioned and SHALL be independent of `kb.search_artifacts`.

#### Scenario: Migration creates the table and indexes

- **WHEN** the goose migration is applied
- **THEN** `kb.ontology_class_contract_search` exists with the columns above, a GIN index on
  `search_vector`, and an HNSW index on `embedding`
- **AND** the migration's down step drops the table and leaves the `vector` extension installed

### Requirement: Class-contract search document composition

For a class term, its `search_document` SHALL be composed from: the `class_term_id` slug itself;
its `module_id`; the class term's `definition` and `scope` from its latest `kb.ontology_terms`
row when one exists; the flattened facets of its current contract payload (`value_type`, each
permitted unit term id) and its `definition_state`; and a bounded sample (about 20) of distinct
`metric_name` / `metric_name_en` / `metric_subject` values of metrics resolved to that class. The
`search_vector` SHALL be `to_tsvector('simple', <search_document>)`. The `embedding` SHALL be
computed only when semantic search is enabled and an embedding model is configured; otherwise the
row SHALL be stored lexical-only with a null `embedding`.

#### Scenario: Instances contribute to the document

- **WHEN** a class has three resolved metrics named "收运频次", "collection frequency", and
  "清运间隔"
- **THEN** those names appear in that class's `search_document` and are reflected in its
  `search_vector`

#### Scenario: Lexical-only when embeddings are off

- **WHEN** semantic search is disabled or no embedding model is configured and a class row is
  (re)indexed
- **THEN** the row is written with a populated `search_document` / `search_vector` and a null
  `embedding`, and no error is raised

### Requirement: Similar-class match query

The capability SHALL provide a `MatchSimilar(class_term_id, k)` operation that returns up to `k`
other class terms ranked by similarity to the given class, computed as Reciprocal Rank Fusion
(constant 60) of a lexical candidate list (`ts_rank_cd` of `search_vector` against the given
class's `search_document`) and a semantic candidate list (pgvector cosine distance of `embedding`
against the given class's embedding). The given class SHALL be excluded from its own results. When
the given class has no stored embedding, the operation SHALL embed its `search_document` on the
fly for the semantic half; when no embedding is available at all, it SHALL fall back to the
lexical list alone and still return a ranked result. Each returned entry SHALL carry the matched
`class_term_id` and its fused score.

#### Scenario: Fused ranking

- **WHEN** `MatchSimilar('measurement:collection_frequency_x', 30)` runs with embeddings present
- **THEN** it returns up to 30 class terms other than `measurement:collection_frequency_x`, ordered
  by fused lexical+semantic score

#### Scenario: Lexical fallback

- **WHEN** `MatchSimilar` runs for a class whose row and query both have no embedding
- **THEN** it returns a ranked list computed from the lexical candidate list alone, not an error

#### Scenario: Self excluded

- **WHEN** `MatchSimilar(C, k)` runs
- **THEN** `C` is never present in the returned entries

### Requirement: Index kept current on contract-revision change

The system SHALL bring a class term's `kb.ontology_class_contract_search` row up to date whenever
that class's contract revision is created or promoted (a new
`kb.ontology_class_contract_revisions` row and an advanced
`kb.ontology_term_headers.current_contract_revision_id`). This reindex SHALL run outside the
metric-write database transaction (it performs network I/O for the embedding), SHALL be
best-effort — a failure SHALL be logged and SHALL NOT fail the metric write or the request that
triggered it — and SHALL be idempotent (repeating it for an unchanged class produces the same
row).

#### Scenario: New class becomes searchable

- **WHEN** a metric write creates a brand-new `identity_only` class contract and its transaction
  commits
- **THEN** that class term has a `kb.ontology_class_contract_search` row after the write completes

#### Scenario: Reindex failure does not break the write

- **WHEN** the post-commit reindex of a class fails (for example the embedding call errors)
- **THEN** the failure is logged and the metric write and its response are unaffected

#### Scenario: Reindex never runs inside the metric transaction

- **WHEN** the metric semantic transaction in the lossless writer is open
- **THEN** no class-contract search reindex or embedding call is issued until after that
  transaction commits

### Requirement: Class-contract search backfill endpoint

The API SHALL expose `POST /api/v1/kb/ontology/class-contracts/backfill-search` that (re)indexes
class-term rows in `kb.ontology_class_contract_search`, accepting an optional `limit` (rows per
call) and an optional `reembed_all` flag, and returning counts of scanned, embedded, skipped,
failed, and remaining rows so it can be called repeatedly until `remaining` is zero. It SHALL be
usable to first populate the index and to repair it, independently of whether semantic search is
enabled.

#### Scenario: Repeated backfill drains the corpus

- **WHEN** the endpoint is called repeatedly (default mode) with `limit=500`
- **THEN** each call indexes up to 500 not-yet-indexed class terms and reports `remaining`, and
  once `remaining` is zero every governed class term (per the definition above) has a
  `kb.ontology_class_contract_search` row

#### Scenario: Re-embed pass

- **WHEN** the endpoint is called with `reembed_all=true` after an embedding model is configured
- **THEN** it recomputes `embedding` for rows that already had one, not only the null ones
