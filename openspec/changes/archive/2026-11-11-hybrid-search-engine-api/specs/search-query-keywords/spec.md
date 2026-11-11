## MODIFIED Requirements

### Requirement: Query keyword extraction
When `SEARCH_QUERY_KEYWORDS` is `on`, the system SHALL turn an artifact search query into keywords
with the LLM profile named by `SEARCH_QUERY_KEYWORDS_MODEL_NAME` (default
`deepseek-flash-processor`) and the prompt named by `SEARCH_QUERY_KEYWORDS_PROMPT` (default
`prompt-search-query-keywords-v1.md`), with model reasoning disabled, unless the request supplies
its own keywords or switches extraction off. Each keyword SHALL be returned in English and Chinese.
The system SHALL keep at most 12 keyword strings after trimming, removing duplicates, removing
strings longer than 64 characters and removing strings without a letter, digit or CJK character.
Extraction SHALL be supplied to the search engine by a registered extractor; where no extractor is
registered, extraction is unavailable.

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

#### Scenario: Request supplies keywords
- **WHEN** `SEARCH_QUERY_KEYWORDS` is `on` and a request supplies the keywords "GB 16889" and "填埋"
- **THEN** no keyword LLM call is made and the BM25 clause uses the supplied keywords

#### Scenario: Request switches extraction off
- **WHEN** `SEARCH_QUERY_KEYWORDS` is `on` and a request sets keyword extraction to `off`
- **THEN** no keyword LLM call is made for that request

#### Scenario: No extractor registered
- **WHEN** the search engine runs in a process where no extractor was registered
- **THEN** no keyword LLM call is made and the fallback rules apply

### Requirement: Keyword BM25 clause
With the ParadeDB lexical backend and a non-empty keyword list, the lexical clause SHALL match an
artifact when, for at least one keyword, every token of that keyword occurs in one of the indexed
text fields (`&&&`). Keywords SHALL be combined with OR, and each keyword's BM25 contribution SHALL
be multiplied by its weight (default 1.0, so extracted keywords score as before). The same keyword
clause SHALL be used by the hybrid query's lexical list, the lexical-only search and the result
count for that query, whether the keywords were extracted, supplied by the request or taken from a
referenced artifact.

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

#### Scenario: Weighted keywords
- **WHEN** a request supplies "GB 16889" with weight 3 and "填埋" with weight 1
- **THEN** the BM25 contribution of "GB 16889" is boosted by 3 and that of "填埋" by 1

### Requirement: Fallback to the raw query
For a text query, the system SHALL search with today's raw-query clause (`|||` on the query text)
when keyword extraction is off, when no extractor is registered, when the model or prompt cannot be
loaded, when the call fails or exceeds `SEARCH_QUERY_KEYWORDS_TIMEOUT_MS` (default 3000), or when
no keyword string remains after cleaning. For a reference query without supplied or stored
keywords, the system SHALL instead use terms selected from the referenced artifact's stored search
document. A search SHALL NOT fail because of keyword extraction. Each fallback other than "off"
SHALL be logged as a warning with its reason.

#### Scenario: Extraction off
- **WHEN** `SEARCH_QUERY_KEYWORDS` is unset or `off`
- **THEN** no LLM call is made and the lexical clause of a text query is the raw-query `|||` clause

#### Scenario: Model call times out
- **WHEN** the keyword call takes longer than `SEARCH_QUERY_KEYWORDS_TIMEOUT_MS`
- **THEN** the search proceeds with the raw-query clause and returns results
- **AND** a warning with reason "timeout" is logged

#### Scenario: Model returns nothing usable
- **WHEN** the model's response cannot be parsed or yields no keyword strings after cleaning
- **THEN** the search proceeds with the raw-query clause

#### Scenario: Reference query without keywords
- **WHEN** a reference query's artifact has no stored keywords and the request supplies none
- **THEN** the lexical clause uses terms selected from the artifact's stored search document, not its whole text with `|||`
