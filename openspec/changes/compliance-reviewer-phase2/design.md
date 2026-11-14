## Context

Phase 1 calls `searchengine.Search` with `ArtifactTypes = ["provision","chunk"]`,
`ExcludeRecordIDs = [reviewed record]`, limit 10, no `MinCosine`. The engine already supports an
absolute floor (`Criteria.MinCosine`, with `Removed.MinCosine` counted in the response and
`Diag.Cosine` on each result) and its documentation says fused score is not a relevance
probability, so the floor must use cosine. The trial found no row of type
`compliance_no_grounding` in 151 activities, and one reviewed document matched by its own copy.
`kb.inputs.md5` exists and is indexed (`idx_kb_inputs_md5`); the dev database has 4 md5 values
shared by 8 records.

## Goals / Non-Goals

**Goals:**
- A candidate list a reader can trust to mean "worth reading", and a real "no grounding" row.
- No document is grounded by its own byte-identical copy.
- A reproducible way to score the reviewer: frozen snapshot, gold set, scorer, baseline.

**Non-Goals:**
- Applicability, requirement comparison, omission reporting, verifier (phases 3 and 4).
- Near-duplicates (a different edition, a re-typeset file). Editions are a phase 3 concern.
- Choosing the floor value or claiming accuracy before the gold set exists.
- Production data. The gold set uses a frozen snapshot of whatever knowledge base the user names.

## Decisions

**D1. Duplicates are identified by `kb.inputs.md5`.** Before searching, the reviewer reads the
md5 of the reviewed record and collects every record id sharing it; all go into
`ExcludeRecordIDs`. A record with an empty md5 excludes only itself. *Alternative:* filename or
title match; rejected, because filenames differ for the same bytes and match for different
files. *Alternative:* content hashing of indexed text at search time; rejected as heavier and
unnecessary while md5 is stored at upload.

**D2. The floor is a cosine floor, set per reviewer in config, applied through
`Criteria.MinCosine`.** The engine removes sub-floor results, so a chunk of results falls to zero
and the existing no-grounding row appears with no new code path. The default is 0 (no floor)
until the gold set sets it; a value is never invented here. Cosine goes into the row text per
candidate so a reader and the gold scorer see it. *Alternative:* top-K by fused rank with a
relative cut; rejected, because fused score is not comparable across queries.
*Consequence:* cosine from a different embedding model is not comparable, so the snapshot records
the embedding model and the floor is stored with it.

**D3. A snapshot is a version stamp of the whole indexed knowledge base (revised 2026-11-14).**
Tables in the `testbed` schema (`testbed.compliance_eval_snapshots`: id, name, created time,
embedding model, notes; and a child table with one row per indexed record: record id, md5,
chunk-index generation, counts of chunk and provision artifacts). The corpus itself stays where the
search engine reads it, in the `kb.search_artifacts_*` tables. The knowledge base has no parts, so
the snapshot stamps every indexed record rather than a chosen member list (the first build did the
latter and restricted pooling to it, while the reviewer searched everything). Verification fails
when a snapshot record's md5, generation or counts changed or the record disappeared, or the
embedding model changed, and names each. Records indexed later are allowed: gold pooling leaves
them out and the scorer reports reviewer hits on them as unjudged. *Alternative:* a separate
database copy; rejected, since it duplicates large data and the project keeps data in the
database, not in files.

**D4. The first iteration's gold is built by the coding agent (revised 2026-11-14).** The user
judged the model panel of D7 too complicated for now. Gold is labelled by Claude Code or Codex
following the skill `.agents/skills/compliance-review-benchmark`, the pattern of the other
benchmarks (`extract-provisions-benchmark`): rules frozen per version in the skill's `rules/`
with their SHA-256 in `rules/versions.json`, the invoking model recorded, one immutable run per
document. Go does what the agent cannot: `compliance-gold chunks` prints the document as the
reviewer reads it, `pool` checks the agent's activities and pools candidates from the knowledge
base, `save` checks every label and the rules file's SHA-256 and writes the run (method `agent`),
`show` reads it back. `compliance-score` scores one or more gold runs of one snapshot. One labeller
means no inter-annotator agreement; the skill's audits and the human review sample (D7) are the
checks. *Earlier revision (during apply):* gold as Go commands calling model profiles; kept as
`compliance-gold panel`.
Gold content: per document, the activity spans (including the document-level activity); per
activity, a judgement for each pooled candidate (D10), and "no grounding expected" when none is
judged to govern.

**D6a. Activities are matched by coverage, not one-to-one (found during apply).** On record 416 the
reviewer merged neighbouring lines into 11 activities while two annotators labelled 62 clause-level
ones, so one-to-one matching would cap recall at 11/62 whatever the quality. A gold activity counts as
found when a reviewer row overlaps its span by at least half of the shorter span; a reviewer row counts
as supported when it overlaps some gold activity; grounding for a found activity is judged on all rows
covering it. The cost is that coarse rows are never penalised for being coarse; granularity is
reported through the two counts instead.

**D10. Grounding labels come from a judged pool, not from the whole corpus (revised 2026-11-14).**
A labeller cannot read the corpus, so for each gold activity the system builds a candidate pool by a
deep search of the whole knowledge base: two queries (the activity's source text, and its name with
its object), no floor, the document, its byte-identical copies and the records indexed after the
snapshot left out. The two result lists are interleaved by rank and the pool stops at
`[reviewers.compliance].max_candidates` per activity (default 100; it is the labelling workload, and
the reviewer's own search ignores it). The agent judges every pooled candidate as `governs`,
`topical`, `unrelated` or `ambiguous`, with a reason for `governs` and `ambiguous`; in the model
panel both annotators judge and the adjudicator resolves disagreements. This is standard pooled evaluation and has a known bias: a governing
record that the pooling search misses is never judged, so recall measured against the pool is an
upper bound. The scorer reports it as "recall within the pool" and the gold run stores the
pooling parameters. The judged pool is also what gives the cosine distribution for choosing
`min_cosine` (task 4.5): governing versus topical versus unrelated.

**D5. Baseline = same cases through current reviewers.** The phase 1 compliance reviewer (with
D1 and D2 off, then on) and the existing legal, regulatory and standards compliance reviewers are
run on the gold documents against the same snapshot, and their findings are compared by the
scorer. The baseline is stored with the gold run so later phases are measured against it.

**D6. Metrics reported separately (research §11.2).** Activity span precision and recall;
knowledge-base independence (activity lists identical for runs of one document made against
different knowledge-base states, `compliance-score -compare`); hits on records indexed after the
snapshot counted as unjudged, never as false grounding;
grounding recall at the record level; precision of "no grounding" and the share where the
snapshot did hold a governing record that search missed. Obligation-level recall and the 95%
gates are phase 3 and later.

## Risks / Trade-offs

- **A floor hides true hits.** → The floor stays 0 until chosen from the gold set's cosine
  distribution for governing versus non-governing candidates, and the scorer reports misses
  caused by the floor (`Removed.MinCosine`).
- **Hash duplicates are not the only duplicates.** → Documented non-goal; the gold set includes
  edition pairs, labelled so phase 3 can measure them.
- **Annotation is the cost.** → Start with a small, deliberately mixed set (research suggests
  10–15 documents for phase 1; this change targets that many with both covered and uncovered
  activities) and report sample size and intervals, not a headline score.
- **Dev corpus is thin.** 143 of 207 dev records cannot be indexed and provisions are nearly
  absent. → The snapshot lists only records that are indexed; its counts show how thin it is, and
  scores say so. Provision grounding stays unmeasured until provisions exist.
- **One agent labels alone.** → No inter-annotator agreement; the skill's frozen rules, its
  completeness and consistency audits (recorded with the run), and the human-sample follow-up (D7).
  The model panel remains available for a later iteration.

## Migration Plan

One project goose migration for the `testbed` snapshot and gold tables (shared migrations are not touched). Config
gains optional `min_cosine`; absent means today's behaviour. Rows gain a cosine only in the
description text, so stored findings and the report view keep working. Rolling back means
dropping the tables and removing the config key. A second project migration
(`20261114000001`) drops `testbed.compliance_gold_runs.organization` (D9); its Down restores the
column with `{}` as the value. A third (`20261114000002`) lets a run have one labeller: `method`
(`agent` or `model_panel`), `rules_version`, `audit_notes`, `pool_limit` renamed `max_candidates`,
optional second annotator and adjudicator, agreement `single`, `label_1` may be `ambiguous`,
`label_2` optional. Its Down deletes agent runs, which the old shape cannot hold.

**D7. Model-panel labelling (deferred 2026-11-14, kept as `compliance-gold panel`).**
`COMPLIANCE_REVIEWER_ANNOTATOR_1`, `COMPLIANCE_REVIEWER_ANNOTATOR_2` and
`COMPLIANCE_ADJUDICATOR` each name a profile resolved through `MODEL_DEF_FILE`; no model name is
in code, and an unset variable fails with the variable's name. The two annotators label
independently and must be different profiles; the adjudicator sees both labels with their
reasons and the source text and decides, recording a rationale, or marks the case ambiguous.
Every label carries its source (`llm:<profile>`). *Caveat:* research §6.6 and §13 say model
agreement is not independent legal validation and that interpretation needs qualified owners.
So scores from this gold set are reported as LLM-adjudicated, never as release-gate evidence, and
a human review of a sample (including a check for governing records the pool missed) is listed as a follow-up, not silently skipped.

**D8. Gold documents are listed in `ChenWeb/doc-review.local.toml`.** Two tables,
`[compliance-review.covered-docs]` (documents with activities the snapshot is expected to cover)
and `[compliance-review.uncovered-docs]` (documents with activities it is expected not to cover).
Each key is a `kb.inputs` record id and each value a one-line reason. The tables select what
the gold run labels; they are not labels. A record in both tables, or one the agent's `pool` or
`save` step is given without being listed, is an error naming it. Covered and uncovered are
relative to the whole knowledge base.

**D9. No organization profile (revised 2026-11-14).** The first build had a
`[compliance-review.organization]` table of organization facts (operating territories, legal
entities, regulated roles, sectors, data categories, deployment regions, adopted standards), copied
into each gold run. Research 2026111103 (activity context) now determines context automatically:
each activity carries its actor, the actor's role, its object, the document's relationship to it,
and context facts with source passages and an evidence status (`explicit`, `inferred`, `unknown`,
`conflicting`). Users answer only questions whose answer changes an applicability decision. One
profile per run could not describe gold documents from different issuers anyway. The table, its
validation and the `organization` column of `testbed.compliance_gold_runs` are removed (migration
`20261114000001`).

Expected context is labelled in phase 3, as a separate pass over the frozen gold activities: it
attaches context to spans already labelled, so a gold run made under rules v1 keeps its activities
and judgements. The reviewer is unchanged here; its context extraction is built in phase 3
together with the storage that keeps it, so the phase 2 baseline is not disturbed.

## Open Questions

0. *Resolved 2026-11-14:* the knowledge base has no parts. The snapshot stamps the whole
   knowledge base and pooling searches all of it (D3, D10), as the reviewer does; activity-list
   stability is checked between runs made against different knowledge-base states.

1. *Resolved 2026-11-14:* no organization facts are needed (D9).
2. The first covered and uncovered documents (D8) need choosing by the user, 10–15 in all.
3. A human review sample of the gold labels (agent-made in the first iteration; D4, D7): who, how many, when.
