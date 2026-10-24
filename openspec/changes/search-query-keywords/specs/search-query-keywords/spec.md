## ADDED Requirements

### Requirement: Query keyword extraction
When `SEARCH_QUERY_KEYWORDS` is `on`, the system SHALL turn an artifact search query into keywords
with the LLM profile named by `SEARCH_QUERY_KEYWORDS_MODEL_NAME` (default
`deepseek-flash-processor`) and the prompt named by `SEARCH_QUERY_KEYWORDS_PROMPT` (default
`prompt-search-query-keywords-v1.md`), with model reasoning disabled. Each keyword SHALL be
returned in English and Chinese. The system SHALL keep at most 12 keyword strings after trimming,
removing duplicates, removing strings longer than 64 characters and removing strings without a
letter, digit or CJK character.

#### Scenario: Keywords extracted
- **WHEN** `SEARCH_QUERY_KEYWORDS` is `on` and a user searches "how much intramuscular adrenaline for a child"
- **THEN** the system calls the configured model once with reasoning disabled
- **AND** the BM25 clause uses the returned keywords, such as "adrenaline" / "肾上腺素" and "child" / "儿童", and not the words "how" or "much"

#### Scenario: Keywords cleaned
- **WHEN** the model returns duplicate strings, empty strings, a string of punctuation only and more than 12 strings
- **THEN** the system drops the duplicates, empty and punctuation-only strings and keeps at most 12

#### Scenario: Unknown switch value
- **WHEN** `SEARCH_QUERY_KEYWORDS` is set to a value other than `on` or `off`
- **THEN** the system treats it as `off` and logs a warning

### Requirement: Keyword BM25 clause
With the ParadeDB lexical backend and a non-empty keyword list, the lexical clause SHALL match an
artifact when, for at least one keyword, every token of that keyword occurs in one of the indexed
text fields (`&&&`). Keywords SHALL be combined with OR. The same keyword clause SHALL be used by the
hybrid query's lexical list, the lexical-only search and the result count for that query.

#### Scenario: Artifact matches one keyword in full
- **WHEN** the keywords are "blood pressure" and "cuff" and an artifact's search document contains "blood" and "pressure" but not "cuff"
- **THEN** the artifact matches the lexical clause

#### Scenario: Artifact matches only part of a keyword
- **WHEN** the only keyword is "blood pressure" and an artifact's text contains "pressure" but not "blood"
- **THEN** the artifact does not match the lexical clause

#### Scenario: Count and results agree
- **WHEN** a search request computes its result count and its result page for the same query
- **THEN** both use the same keyword clause

#### Scenario: Filters keep their placeholders
- **WHEN** a hybrid search has keywords and structural filters
- **THEN** the query vector stays at placeholder `$2`, the filters stay at `$3` onward, the first keyword is `$1` and the other keywords follow the filters

### Requirement: BM25 queries read the artifact partitions
With the ParadeDB backend, the BM25 count, lexical-only search and hybrid lexical list SHALL read from
the artifact-type partitions of `kb.search_artifacts` that carry the BM25 indexes: the one partition
of a single requested artifact type, otherwise one `UNION ALL` branch per partition (all partitions,
or those named in the `artifact_types` filter). Partition table names SHALL come only from the fixed
list of known artifact types.

#### Scenario: Single artifact type
- **WHEN** a ParadeDB search is restricted to artifact type `metric`
- **THEN** the BM25 clause runs against `kb.search_artifacts_metric` and returns BM25 matches instead of an error

#### Scenario: All artifact types
- **WHEN** a ParadeDB search has no artifact-type restriction
- **THEN** the BM25 clause runs once per partition and the results are combined with `UNION ALL`

#### Scenario: Unknown artifact type
- **WHEN** the requested artifact type is not one of the known partitions
- **THEN** no table name is built from it and the query reads the parent table as before

### Requirement: Fallback to the raw query
The system SHALL search with today's raw-query clause (`|||` on the query text) when keyword
extraction is off, when the model or prompt cannot be loaded, when the call fails or exceeds
`SEARCH_QUERY_KEYWORDS_TIMEOUT_MS` (default 3000), or when no keyword string remains after cleaning.
A search SHALL NOT fail because of keyword extraction. Each fallback other than "off" SHALL be logged
as a warning with its reason.

#### Scenario: Extraction off
- **WHEN** `SEARCH_QUERY_KEYWORDS` is unset or `off`
- **THEN** no LLM call is made and the lexical clause is the raw-query `|||` clause

#### Scenario: Model call times out
- **WHEN** the keyword call takes longer than `SEARCH_QUERY_KEYWORDS_TIMEOUT_MS`
- **THEN** the search proceeds with the raw-query clause and returns results
- **AND** a warning with reason "timeout" is logged

#### Scenario: Model returns nothing usable
- **WHEN** the model's response cannot be parsed or yields no keyword strings after cleaning
- **THEN** the search proceeds with the raw-query clause

### Requirement: Keyword cache
The system SHALL cache successfully extracted keywords in memory, keyed by the trimmed query string,
for 24 hours and for at most 2,000 queries, evicting the oldest entries first. Failed extractions
SHALL NOT be cached.

#### Scenario: Repeated query
- **WHEN** the same query is searched twice within 24 hours
- **THEN** the model is called only for the first search

#### Scenario: Failure not cached
- **WHEN** an extraction fails and the same query is searched again
- **THEN** the system calls the model again
