## Why

Compliance Review phase 1 (devdoc 2026111201, research 2026111103 delivery phase 1) detects the
activities a chunk describes and lists knowledge-base search results for each. A five-document
trial on 2026-11-12 (records 416, 371, 376, 364, 799; 151 activities in all) showed two defects
that no amount of extra documents would reveal better, and one gap that blocks measuring anything:

- **"No grounding found" never happens.** Every activity got 10–20 candidates, because search
  returns the top results whatever their quality. A near-miss (another waste-sorting standard for
  a waste-sorting activity) looks the same as a rule that governs the activity.
- **A document is grounded against its own duplicate.** Record 416 and record 198 are the same
  file (same `md5`); 39 of 416's 114 candidates came from 198. The reviewer excludes only the
  record under review.
- **There is no baseline and no gold set.** Retrieval recall, activity quality and the right
  relevance threshold are unknown, and the development database cannot supply them: 64 of its 207
  records can be indexed (the rest lack parsed files on disk) and it holds 79 provisions, from
  one document.

Research §12 phase 2 is "baseline and gold set": snapshot the current reviewers, freeze a
knowledge-base snapshot and scope, record organization context, annotate gold cases. This change
does that, and fixes the two defects first because the gold set must measure the corrected
behaviour.

## What Changes

- The reviewer excludes every record that is a byte-identical copy of the reviewed record (same
  `kb.inputs.md5`), not only the reviewed record itself.
- Each candidate carries its cosine similarity, and the reviewer applies a configurable floor
  (`min_cosine` in `[reviewers.compliance]`). A candidate below the floor is not reported. An
  activity left with none gets the existing `compliance_no_grounding` row. The default stays
  "no floor" until the gold set sets the value.
- A frozen evaluation snapshot, kept in the `testbed` schema: a recorded set of indexed records (ids, `md5`,
  index generation, counts per artifact type), so a gold run is reproducible and two runs are
  comparable.
- A compliance gold set and a scorer, following the existing benchmark pattern (rules frozen per
  version, gold runs saved, scorer 1–100): activity spans and the document-level activity,
  expected grounding records per activity, and "no grounding expected" cases. Labelled by two
  independent model annotators and a model adjudicator, chosen by environment variable (research
  §11.1; labels are LLM-adjudicated, not legal validation). Gold documents are listed in
  `doc-review.local.toml`, and organization context is recorded with each run.
- A baseline: the current reviewers and the phase 1 compliance reviewer scored on the same cases.
- Row text that says "no grounding" and the candidate list are unchanged in meaning; no
  applicability, requirement comparison or omission reporting is added (phases 3 and 4).

## Capabilities

### New Capabilities
- `compliance-review`: what the Compliance Review aspect reports for a document, how it excludes
  the document's own copies, how it decides a candidate is worth reporting, and how its quality
  is measured against a frozen knowledge-base snapshot.

### Modified Capabilities
None. No existing spec describes the reviewer.

## Impact

- `ChenWeb/server/api/doc-reviews/review-compliance.go` (exclusion, floor, cosine in rows),
  `review-config.go` (`min_cosine`), `doc-review.local.toml`.
- `server/api/searchengine`: read-only use of `Criteria.MinCosine` and `Diag.Cosine`; no change.
- New benchmark rules, gold tables (migration) and scorer under the existing benchmark layout.
- Devdoc 2026111201 (limitations list shrinks; row contents gain a cosine) and research
  2026111103 §12 status. Open items for the user are in design Open Questions.
