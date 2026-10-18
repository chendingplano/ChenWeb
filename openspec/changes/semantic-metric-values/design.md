## Context

The user approved a focused semantic correction covering four concerns: value-to-condition mappings, safe numeric consumers, structured multi-values, and extraction/scoring consistency. Existing unrelated working-copy edits must remain untouched.

## Decisions

1. One row is one independently interpretable assertion. Split distinct properties or source-supported conditions, never an alternatives set or mandatory collection merely because it contains several numbers. Preserve the existing condition/frame separation.
2. Add nullable `value_structure JSONB` on `kb.metrics` and `testbed.metrics`. Shape: `{ "kind": "alternatives|required_points|composite|expression|unresolved", "values": ["10", "20", "30"] }`. Values are canonical strings in the row's unit; alternatives and required_points require at least two nonempty members. Composite members are ordered; expressions and unresolved values retain their complete display text in metric_value. Existing scalar/range fields and display text remain available. Null is legacy/scalar, not proof of scalar validity.
3. New prompt versions apply the same semantic rule in both candidate extraction and enrichment. Combined-row recovery requests explicit per-candidate decisions: `split`, `keep`, or `unresolved`, with a nonempty reason. Split replacements must preserve evidence/numbers and contain independent rows. Keep requires valid non-scalar structure and one evidence-preserving retained row. Unresolved or invalid responses preserve the original, tagged as unresolved. Valid collections require no corrective call and generate no multi-value warning.
4. Scalar consumers parse only complete supported scalar forms. Any populated non-scalar structure (including malformed/unknown structures) blocks scalar interpretation. Legacy combined/composite/expression text also cannot be reduced to its first number. Unsupported numeric comparisons defer through the existing unparsed path, not a fabricated scalar or qualitative requirement. Supported simple fractions/tolerances may be handled explicitly without weakening the guard.
5. Persistence, API reads/writes, reference copies and merge identity retain value_structure. Different set relationships cannot merge simply because display numbers match.
6. Release new benchmark rules without editing frozen releases; persist structure in gold and compare relationship plus all members. Bump scoring semantics and update the in-app scorer prompt. Historical scores remain versioned.

## Validation and rollout

Test alternatives versus required points, conditional values, different properties, intervals, tolerances, composites, ambiguity, recovery failures and full persistence. Verify that a set never becomes its first member or its bounding interval. Run focused Go and Python tests, migration up/down/up in an isolated schema/database where available, frontend checks if interfaces change, and review all diffs. Deploy through the existing release process; do not deploy or reprocess production during this implementation. Old extractions/derived assertions require explicit reprocessing after deployment, not automatic mutation.

## Verification record

Regression tests reproduced blind splitting, keep-field loss, legacy list-to-interval conversion, first-number parsing, structured/display disagreement and merge relationship loss before their fixes. Focused Go suites and Python benchmark/scoring suites pass. Real disposable-database tests verify migration rollback/reapply, worker save/load/update/artifact, and reference-copy round trips. `go build ./server/...` and strict OpenSpec validation pass. The doc-processing suite passes with the pre-existing scene prompt default test excluded (that test expects v1 while HEAD already embeds v2); no unrelated scene behavior was changed.

The `.agents` benchmark/scoring files and the older conditions devdoc already contained uncommitted changes. Their new edits remain available in the working tree; baseline copies and a task-only skill patch are preserved under `/tmp/semantic-metric-values-baseline/` and `/tmp/semantic-metric-values-task-agents.patch`. They are not folded into the isolated ChenWeb commit. No production deployment, database mutation or historical reprocessing occurred.
