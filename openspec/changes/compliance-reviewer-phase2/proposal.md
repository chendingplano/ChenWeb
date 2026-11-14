## Why

**2026-11-14 user-authorized extension:** after reviewing the domain-expert workflow, implementation now also covers metric-aware snapshots/pooling/rules and bounded original-source applicability/requirement assessment, numeric checks, semantic verification, and coverage/reference ledgers. Tasks 9.1–9.6 and the updated requirements/design specify this extension. The rationale below describes the original candidate-retrieval baseline; paid gold/baseline tasks 4.3–4.5 remain open and no expert-accuracy claim follows from the implementation tests.

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
knowledge-base snapshot and scope, annotate gold cases. This change
does that, and fixes the two defects first because the gold set must measure the corrected
behaviour.

## What Changes

- The reviewer excludes every record that is a byte-identical copy of the reviewed record (same
  `kb.inputs.md5`), not only the reviewed record itself.
- Each candidate carries its cosine similarity, and the reviewer applies a configurable floor
  (`min_cosine` in `[reviewers.compliance]`). A candidate below the floor is not reported. An
  activity left with none gets the existing `compliance_no_grounding` row. The default stays
  "no floor" until the gold set sets the value.
- An evaluation snapshot, kept in the `testbed` schema: a version stamp of the whole indexed
  knowledge base (per record `md5`, index generation, counts per artifact type), so a gold run is
  reproducible and two runs are comparable (revised 2026-11-14; first built as a chosen member list).
- A compliance gold set and a scorer, following the existing benchmark pattern (rules frozen per
  version, gold runs saved, scorer 1–100): activity spans and the document-level activity,
  expected grounding records per activity, and "no grounding expected" cases. In the first
  iteration the coding agent labels each document with the `compliance-review-benchmark` skill
  (decided 2026-11-14); the model panel of two annotators and an adjudicator is kept but deferred.
  Labels are not legal validation. Gold documents are listed in
  `doc-review.local.toml`. There is no organization profile: activity context is extracted from
  the documents (research 2026111103, activity context; design D9, revised 2026-11-14).
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
