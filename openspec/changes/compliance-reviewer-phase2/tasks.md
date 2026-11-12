## 1. Reviewer fixes (no data needed)

- [x] 1.1 Duplicate exclusion: read the reviewed record's `kb.inputs.md5`, collect every record id sharing it, and send all in `ExcludeRecordIDs`. Empty md5 excludes only the record itself.
- [x] 1.2 Add optional `min_cosine` to `ReviewAspectConfig` and pass it as `Criteria.MinCosine`. Absent or 0 means no floor. Reject a negative value or a value above 1 at config load.
- [x] 1.3 Show each candidate's cosine (`Diag.Cosine`) in the row description. An activity with zero results after the floor produces `compliance_no_grounding`.
- [x] 1.4 Tests with the fake search function: duplicate ids in the request, empty md5, floor in the request, all-below-floor gives the no-grounding row, search error still aborts the chunk, cosine appears in the description.
- [x] 1.5 Update `doc-review.local.toml` comments (key present, unset) and devdoc 2026111201 (limitations, row contents).

## 2. Evaluation snapshot

- [x] 2.1 Before writing, run the `find-reusable-module` skill for a snapshot or manifest mechanism and for the benchmark layout; report what was searched. *Searched 2026-11-12 (catalogue: snapshot, manifest, benchmark, gold, scorer, freeze, generation; code and devdocs for snapshot/manifest): no snapshot module exists. Reused: Artifact Search (`searchengine`) and `kb.chunk_search_state.generation`; built new: `server/api/compliance-eval` (small, evaluation-only, not registered).*
- [x] 2.2 Project migration (see `db-migration` skill): `testbed.compliance_eval_snapshots` and `testbed.compliance_eval_snapshot_records` (record id, md5, chunk-index generation, chunk and provision counts), with down migration. Apply to the dev database and confirm the columns.
- [x] 2.3 A command that records a snapshot from a named list of records, and a check that compares current state with a manifest and names every member that differs.

## 3. Gold set and scorer (.agents skills, pattern of `extract-provisions-benchmark` / `score-extract-provisions`)

- [ ] 3.1 Project migration for `testbed` gold tables: runs (snapshot id, rules version, model), activities (document, line spans, document-level flag), and expected grounding (activity, snapshot record or "no grounding expected", adjudication state). Down migration.
- [ ] 3.2 Gold skill `extract-compliance-benchmark`: `rules/1.0.0.md` (what counts as an activity, the document-level activity, governing versus topical record, no-grounding rule, ambiguity recording), a validate-and-save script that checks spans against existing lines and records against the snapshot, tests, and the skill docs.
- [ ] 3.3 Scorer skill `score-compliance-review`: activity span precision and recall, scope stability, record-level grounding recall, precision of "no grounding" and the missed-by-search share, counts behind each figure.
- [ ] 3.4 Tests: one refusal or scoring case per spec scenario.

## 4. Configuration, annotation and baseline

- [ ] 4.1 Read `[compliance-review.covered-docs]`, `[compliance-review.uncovered-docs]` and `[compliance-review.organization]` from `doc-review.local.toml`; validate as in the spec (both-tables, missing record, outside snapshot, status vocabulary). Ship the organization table with every fact `unknown` and both document tables empty, with comments.
- [ ] 4.2 Labelling runner: resolve `COMPLIANCE_REVIEWER_ANNOTATOR_1`, `_2` and `COMPLIANCE_ADJUDICATOR` through `MODEL_DEF_FILE`; fail naming an unset variable; refuse equal annotators; store label source and adjudication rationale. Prompts in `prompts/`, versioned.
- [ ] 4.3 With the user's chosen documents and organization facts: record the snapshot, run the labelling, save the gold run with a copy of the organization context.
- [ ] 4.4 Run the baseline (current legal, regulatory and standards reviewers; Compliance Review with exclusion and floor off, then on) on the same snapshot and store the scores with the gold run, marked LLM-adjudicated.
- [ ] 4.5 From the gold set's cosine distribution for governing versus non-governing candidates, propose a `min_cosine` and report the misses it would cause. Set it only with the user's agreement.

## 5. Close out

- [ ] 5.1 Update research 2026111103 §12 status and devdoc 2026111201; answer the coding-practice questions (what knowledge changed, which docs are stale).
- [ ] 5.2 Register any reusable module built here with the `create-reusable-module` skill.
- [ ] 5.3 `go build ./server/...` and `go test` for `doc-reviews` and any new package; commit with explicit paths via `jj`.
