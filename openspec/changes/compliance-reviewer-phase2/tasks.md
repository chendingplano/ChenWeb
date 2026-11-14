## 1. Reviewer fixes (no data needed)

- [x] 1.1 Duplicate exclusion: read the reviewed record's `kb.inputs.md5`, collect every record id sharing it, and send all in `ExcludeRecordIDs`. Empty md5 excludes only the record itself.
- [x] 1.2 Add optional `min_cosine` to `ReviewAspectConfig` and pass it as `Criteria.MinCosine`. Absent or 0 means no floor. Reject a negative value or a value above 1 at config load.
- [x] 1.3 Show each candidate's cosine (`Diag.Cosine`) in the row description. An activity with zero results after the floor produces `compliance_no_grounding`.
- [x] 1.4 Tests with the fake search function: duplicate ids in the request, empty md5, floor in the request, all-below-floor gives the no-grounding row, search error still aborts the chunk, cosine appears in the description.
- [x] 1.5 Update `doc-review.local.toml` comments (key present, unset) and devdoc 2026111201 (limitations, row contents).

## 2. Evaluation snapshot

- [x] 2.1 Before writing, run the `find-reusable-module` skill for a snapshot or manifest mechanism and for the benchmark layout; report what was searched. *Searched 2026-11-12 (catalogue: snapshot, manifest, benchmark, gold, scorer, freeze, generation; code and devdocs for snapshot/manifest): no snapshot module exists. Reused: Artifact Search (`searchengine`) and `kb.chunk_search_state.generation`; built new: `server/api/compliance-eval` (small, evaluation-only, not registered).*
- [x] 2.2 Project migration (see `db-migration` skill): `testbed.compliance_eval_snapshots` and `testbed.compliance_eval_snapshot_records` (record id, md5, chunk-index generation, chunk and provision counts), with down migration. Apply to the dev database and confirm the columns.
- [x] 2.3 A command that records a snapshot from a named list of records, and a check that compares current state with a manifest and names every member that differs. *(Replaced by a stamp of the whole knowledge base in 6.2.)*

## 3. Gold set and scorer (Go; see design D4, D10)

- [x] 3.1 Project migration for `testbed` gold tables: runs (snapshot id, rules version and SHA-256, annotator and adjudicator profiles, pooling parameters, organization-context copy), activities (document, line spans, document-level flag, label sources), and pooled judgements (activity, candidate artifact, cosine, each annotator's label, final label, rationale, ambiguity mark). Down migration. *(The organization-context copy was removed by 4.6.)*
- [x] 3.2 Rules file (`prompts/`, versioned and hashed): what counts as an activity, the document-level activity, `governs` versus `topical` versus `unrelated`, when "no grounding expected" applies, recording ambiguity.
- [x] 3.3 `compliance-gold` command (package `server/api/compliance-eval`): for each listed document, two independent activity extractions and adjudicated merge by span overlap; deep-search pool per activity; two independent judgements per pooled candidate and adjudication; save in one transaction. Fake-model tests for each step and for the refusals (unset variable, equal annotators, snapshot drift).
- [x] 3.4 `compliance-score` command: activity span precision and recall, record-level grounding recall within the pool, precision of "no grounding" and the missed-by-search share, scope stability, counts behind each figure; a reviewer's findings are read from its saved run.
- [x] 3.5 Tests: one refusal or scoring case per spec scenario.

## 4. Configuration, annotation and baseline

- [x] 4.1 Read `[compliance-review.covered-docs]`, `[compliance-review.uncovered-docs]` and `[compliance-review.organization]` from `doc-review.local.toml`; validate as in the spec (both-tables, missing record, outside snapshot, status vocabulary). Ship the organization table with every fact `unknown` and both document tables empty, with comments. *(The organization table was removed by 4.6.)*
- [x] 4.2 Labelling configuration (feeds 3.3): resolve `COMPLIANCE_REVIEWER_ANNOTATOR_1`, `_2` and `COMPLIANCE_ADJUDICATOR` through `MODEL_DEF_FILE`; fail naming an unset variable; refuse equal annotators; store label source and adjudication rationale. Prompts in `prompts/`, versioned.
- [ ] 4.3 With the user's chosen documents: record the snapshot, have the coding agent label each document with the `compliance-review-benchmark` skill, save one gold run per document.
- [ ] 4.4 Run the baseline (current legal, regulatory and standards reviewers; Compliance Review with exclusion and floor off, then on) on the same snapshot and store the scores with the gold run, marked LLM-adjudicated.
- [ ] 4.5 From the gold set's cosine distribution for governing versus non-governing candidates, propose a `min_cosine` and report the misses it would cause. Set it only with the user's agreement.
- [x] 4.6 Retire the organization profile (design D9, revised 2026-11-14): remove `[compliance-review.organization]` from the config parser, `compliance-gold` and `doc-review.local.toml`; drop `testbed.compliance_gold_runs.organization` (migration `20261114000001`, applied to the dev database); update the config tests.

## 6. Agent-built gold and whole-KB snapshot (design D3, D4, D10 revised 2026-11-14)

- [x] 6.1 Before writing, run the `find-reusable-module` skill. *Searched 2026-11-14 (catalogue: benchmark, rules, version, gold, snapshot; code: `versions.json`, `rules_sha256`): no Go helper verifies a skill's rules versions (each benchmark skill does it in its own Python script). Reused the benchmark-skill pattern of `extract-provisions-benchmark`, `searchengine` (Artifact Search) and the existing `compliance-eval` code; built the rules check in Go because the save step is Go.*
- [x] 6.2 Snapshot = stamp of the whole indexed knowledge base (`Record`, `Verify` returning changed and added records, `CurrentIndexed`, `Added`); `compliance-snapshot record -name | verify [-id] | list`; the snapshot-membership refusal removed.
- [x] 6.3 Pool over the whole knowledge base, leaving out the document, its copies and records added since the snapshot; two queries interleaved by rank; cap `[reviewers.compliance].max_candidates` (default 100, positive); candidates carry their source title.
- [x] 6.4 `compliance-gold chunks | pool | save | show` for the agent; `panel` keeps the model panel; pool search usage attributed to `-user` or the document's owner.
- [x] 6.5 Project migration `20261114000002` (method, rules version, audit notes, `max_candidates`, one-labeller runs), applied to the dev database; Down/Up tested in a rolled-back transaction.
- [x] 6.6 Scorer: several gold runs of one snapshot; hits on records added after the snapshot reported as unjudged; output names the labelling method.
- [x] 6.7 Skill `.agents/skills/compliance-review-benchmark` (SKILL.md, rules 1.0.0 with SHA-256, CHANGELOG, Codex metadata), linked for Claude Code and Codex.
- [x] 6.8 Tests for activities, labels, rules versions, pool cap and unjudged scoring; a smoke run on record 416 (snapshot, chunks, pool, save, show, score) on the dev database, its data deleted.

## 7. Complete pool and candidate-level scoring (design D11, D12, second revision 2026-11-14)

- [x] 7.1 Before writing, run the `find-reusable-module` skill for loading artifacts by id and for resolving a cited document number to records; report what was searched. *Searched 2026-11-14 (catalogue: search artifact load lookup; document number citation reference resolve doc_no; code: doc_no normalization, citation resolution): Artifact Search had no load-by-id, so it is extended with `searchengine.Load`; the extract_references identifier parser (`reference_identifiers.go`, uncatalogued) gives edition-free keys and is reused through two exported wrappers. The references catalogue (`kb.ref_resource_identifiers`) was not used to resolve citations: on dev it covers 5 of 67 indexed records, `doc_no` 49.*
- [x] 7.2 `searchengine.Load` (artifacts by type and id, display fields, no ranking) and exported citation identity keys in `doc-processing` (citation text, and a record's own identifier from `doc_no`, title, file name), with tests.
- [x] 7.3 Pool: agent `queries` (at most 6, keyword extraction off) and `cited_docs` (at most 10) in the activities file; fused, keyword and vector rankings per query; cited documents resolved edition-insensitively to snapshot records and searched within (up to 10 new passages per record); unresolved citations and searched records in the pool file; `found_by` on every candidate.
- [x] 7.4 `compliance-gold expand`: search within every record with a `governs` label not yet searched for that activity; `save` refuses an unexpanded governing record.
- [x] 7.5 Scorer: reported passages parsed per row; candidate precision (best label over covered gold activities); unjudged counts for new records and for passages outside the pool; false grounding only over activities with a judged reported passage. *Added during apply: passages on the document's own copies count as unrelated (D12).*
- [x] 7.6 `compliance-gold augment -run -review` and saving an augmentation pool as a new run (base run's activities and judgements plus the new ones; same snapshot and rules version).
- [x] 7.7 Project migration `20261114000003` (activities `queries`, `cited_docs`; judgements `found_by`; runs `base_run_id`, `review_run_id`), applied to the dev database; Down/Up tested in a rolled-back transaction. *Applied by the running dev server at 14:45; Down/Up then tested in a rolled-back transaction.*
- [x] 7.8 Skill: rules 1.1.0 (queries, cited documents, judging added passages), SKILL.md steps for expand and augment, CHANGELOG, `versions.json`.
- [x] 7.9 Tests for each part; `go build ./server/...`; update devdoc 2026111201 and the phase 2 handoff; register the reused identifier parser and update Artifact Search with `create-reusable-module`. *Smoke run on record 416 with review run 101 (synthetic labels, data deleted); module `20261114-02` Document Identifier Keys registered (new capability `document-identifiers`), Artifact Search 1.2.0.*

## 8. One gold document list (design D8 revised 2026-11-14)

- [x] 8.1 `[compliance-review.gold-docs]` replaces `covered-docs` and `uncovered-docs` in the config parser, `compliance-gold` and `doc-review.local.toml`; the old tables, even empty, are refused with a message naming the new one; a record listed twice is refused. (Supersedes the two tables of 4.1.)
- [x] 8.2 `save` and `compliance-score` report how many gold activities have a governing passage and how many have none (`show` already did).
- [x] 8.3 Project migration `20261114000004` drops `testbed.compliance_gold_activities.covered`, applied to the dev database; Down/Up tested in a rolled-back transaction.
- [x] 8.4 Tests; skill SKILL.md and CHANGELOG; devdoc 2026111201 and the phase 2 handoff.

## 5. Close out

- [x] 5.1 Update research 2026111103 §12 status and devdoc 2026111201; answer the coding-practice questions (what knowledge changed, which docs are stale).
- [x] 5.2 Register any reusable module built here with the `create-reusable-module` skill. *None registered: `compliance-eval` is evaluation-only and used by one feature; the reviewer's md5 lookup is exported for the gold pooling and stays in `doc-reviews`.*
- [x] 5.3 `go build ./server/...` and `go test` for `doc-reviews` and any new package; commit with explicit paths via `jj`.
