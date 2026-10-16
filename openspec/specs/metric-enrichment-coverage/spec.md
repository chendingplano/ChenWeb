# Metric enrichment coverage

## Purpose

Prevent valid but incomplete Pass 2 JSON responses from silently losing metric candidates.

## Requirements

### Requirement: Every candidate is accounted for

Each enrichment candidate SHALL appear in at least one enriched metric row or in
`dropped_candidates`. JSON/schema validity alone SHALL NOT establish completeness.
Existing candidate-ID backfilling applies before checking coverage.

### Requirement: Bounded recovery preserves successful rows

When a response omits candidates, the processor SHALL retry those candidates once
in smaller groups, keeping candidates quoting the same evidence together. It SHALL
retain the original successful rows and explicit drops, accept recovery rows only
for the recovery group's candidate IDs, and log each recovery call separately.

#### Scenario: Partial response is recovered

- **WHEN** a batch returns one metric and omits its other candidates
- **THEN** recovery SHALL request the omitted candidates and merge their returned
  metrics and explicit drops with the original response
- **AND** successful candidates SHALL NOT be requested again

### Requirement: Unresolved coverage fails extraction

If recovery still leaves candidates unaccounted for, extraction SHALL return an
error identifying their IDs. Other batches SHALL be allowed to finish; their
results SHALL remain available for diagnostics. Both the sequential and chunk-batch
entry points SHALL reject this incomplete replacement before deleting or saving
metrics. Existing behavior for unrelated batch transport failures is unchanged.

#### Scenario: Forced extraction remains incomplete

- **WHEN** a forced extraction still omits a candidate after its recovery attempt
- **THEN** extraction SHALL fail and retain the previously stored metrics
- **AND** recovery transport errors SHALL be included in the reported failure

## Incident evidence

On 2026-10-16, record 371's batch 57/65 (log row 139956) returned only candidate
`7_56` and an empty `dropped_candidates` array. Candidates `7_57` through `7_67`
were unaccounted for. Previously the warning did not trigger recovery or prevent
successful completion. The structural output contract permits such partial JSON;
coverage must be checked against the actual input candidates in application code.
