## Implementation plan

Goal: solve all four concerns without losing source meaning or interpreting unsupported values as scalars.

- [x] 1. Add regression tests and guards in ontology/assertions/metric_normalizer.go for combined values, composites, expressions and explicit value_structure; preserve supported scalar/range behavior.
- [x] 2. Add value_structure goose migration and preserve it through production/gold persistence, metric APIs, reference copies and merge identity; verify round trips.
- [x] 3. Add failing semantic recovery tests, implement split/keep/unresolved outcomes, preserve evidence and numbers, and report only unresolved combinations.
- [x] 4. Version candidate/enrichment/merge/scoring prompts and switch defaults; apply semantic rules before candidate splitting.
- [x] 5. Release benchmark rules and scoring updates with validation and relationship-aware collection comparison, preserving old releases.
- [x] 6. Update specs and existing developer documentation; document deferred numeric set comparisons and historical reprocessing.
- [x] 7. Run targeted tests, migration verification and review; commit isolated task-owned paths through jj and verify linear history; preserve overlapping pre-existing skill/doc edits uncommitted with a task-only skill patch (see design verification record).
