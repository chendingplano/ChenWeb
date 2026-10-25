## ADDED Requirements

### Requirement: Effort levels
Hybrid search SHALL support effort levels `low`, `medium` and `high`. `low` SHALL search the input
as written without LLM query composition. `medium` and `high` SHALL analyse the input with an LLM
to decide whether to search it directly or compose new queries, and SHALL make at most
`HYBRID_SEARCH_MEDIUM_EFFORTS` (default 2) or `HYBRID_SEARCH_HIGH_EFFORTS` (default 5) attempts.

#### Scenario: Low effort
- **WHEN** a search runs with effort `low`
- **THEN** no analysis or judge LLM call is made and results equal the existing hybrid search

#### Scenario: Maximum attempts
- **WHEN** a medium-effort search's judge never finds a match
- **THEN** the loop stops after 2 attempts and returns the fused candidates

### Requirement: Search, judge and reformulate
Each attempt SHALL run the current queries through hybrid search, fuse all results seen so far,
and ask an LLM judge whether any candidate answers the request. If not, the judge's new queries
(from another angle, not repeating tried ones) SHALL be used in the next attempt. The loop SHALL
stop when the judge finds a match, when the maximum is reached, or when no new query is proposed.
Matching results SHALL be ranked first.

#### Scenario: Found on the second attempt
- **WHEN** the first attempt's candidates do not answer the request and the second attempt's do
- **THEN** the search stops after 2 attempts and the matching result is ranked first

### Requirement: Degraded operation
A failed analysis SHALL fall back to a low-effort search of the input; a failed judgement SHALL
end the loop with the fused results so far. Search SHALL NOT fail because of these LLM calls.

#### Scenario: Analysis fails
- **WHEN** the analysis call returns an error
- **THEN** the input is searched as written and the trace records the error

### Requirement: API parameter
The registry search routes SHALL accept `effort=low|medium|high` (default `low`) and reject other
values with HTTP 400. For medium and high the response SHALL include the effort and a trace of the
attempts (queries, found, matching IDs, reason).

#### Scenario: Invalid effort
- **WHEN** a search is requested with `effort=max`
- **THEN** the response is HTTP 400

### Requirement: Benchmark effort mode
The Hybrid Search Benchmark SHALL offer an effort run option. For medium or high it SHALL run the
loop for each question without revealing the target, and store the target's rank in the final
list, the attempts, whether the judge found a match, the time and the queries tried, and report
scores for mode `effort`.

#### Scenario: Benchmark at high effort
- **WHEN** a benchmark run uses effort `high`
- **THEN** each result has an effort rank (or miss), attempts between 1 and 5, and the scores include an `effort` mode
