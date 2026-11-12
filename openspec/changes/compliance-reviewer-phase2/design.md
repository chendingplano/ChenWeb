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

**D3. A frozen evaluation snapshot is a recorded manifest, not a copy of the data.** Tables in the `testbed` schema
(`testbed.compliance_eval_snapshots`: id, name, created time, embedding model, notes; and a child table
of member records (record id, md5, chunk-index generation, counts of chunk and provision
artifacts). The searched corpus itself stays where the search engine reads it, in the
`kb.search_artifacts_*` tables; the snapshot only records which records of it a gold run may rely
on. A gold run refuses to start when a member's current generation or md5 differs from
the manifest, and says which. *Alternative:* a separate database copy; rejected, since it
duplicates large data and the project keeps data in the database, not in files.

**D4. Gold set and scorer follow the existing benchmark pattern.** Rules are frozen per version
in a rules file; a gold run is saved with its model and rules version; a scorer produces a
1–100 score with per-group detail, as `extract-provisions-benchmark` and
`extract-references-benchmark` do (reuse their layout; the tasks list the exact modules to
inspect first). Gold content: per document, the activity spans and the document-level activity;
per activity, the snapshot records that govern it (target: obligation-level source spans where
the annotator can name them, otherwise record level) or an explicit "no grounding expected".
Two annotators label independently; disagreements are adjudicated and genuine ambiguity is
recorded, not forced (research §11.1).

**D5. Baseline = same cases through current reviewers.** The phase 1 compliance reviewer (with
D1 and D2 off, then on) and the existing legal, regulatory and standards compliance reviewers are
run on the gold documents against the same snapshot, and their findings are compared by the
scorer. The baseline is stored with the gold run so later phases are measured against it.

**D6. Metrics reported separately (research §11.2).** Activity span precision and recall;
knowledge-base independence (activity lists identical across full, narrowed and empty scope);
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
- **LLM labels share model blind spots.** → Different profiles for the two annotators, an
  adjudicator with rationale, the `llm:` label source, and the human-sample follow-up (D7).

## Migration Plan

One project goose migration for the `testbed` snapshot and gold tables (shared migrations are not touched). Config
gains optional `min_cosine`; absent means today's behaviour. Rows gain a cosine only in the
description text, so stored findings and the report view keep working. Rolling back means
dropping the tables and removing the config key.

**D7. Annotators and adjudicator are model profiles named by environment variables.**
`COMPLIANCE_REVIEWER_ANNOTATOR_1`, `COMPLIANCE_REVIEWER_ANNOTATOR_2` and
`COMPLIANCE_ADJUDICATOR` each name a profile resolved through `MODEL_DEF_FILE`; no model name is
in code, and an unset variable fails with the variable's name. The two annotators label
independently and must be different profiles; the adjudicator sees both labels with their
reasons and the source text and decides, recording a rationale, or marks the case ambiguous.
Every label carries its source (`llm:<profile>`). *Caveat:* research §6.6 and §13 say model
agreement is not independent legal validation and that interpretation needs qualified owners.
So scores from this gold set are reported as LLM-adjudicated, never as release-gate evidence, and
a human review of a sample is listed as a follow-up, not silently skipped.

**D8. Gold documents are listed in `ChenWeb/doc-review.local.toml`.** Two tables,
`[compliance-review.covered-docs]` (documents with activities the snapshot is expected to cover)
and `[compliance-review.uncovered-docs]` (documents with activities it is expected not to cover).
Each key is a `kb.inputs` record id and each value a one-line reason. The tables select what
the gold run labels; they are not labels. A record in both tables, a missing record, or a record
outside the snapshot is a startup error naming it.

**D9. Organization context is recorded now, used in phase 3.** A table
`[compliance-review.organization]` in the same file holds the facts of research §6.1 (operating
territories, legal entities, regulated roles, sectors, data categories, deployment regions,
adopted standards). Each is `{ value = [...], status = "asserted|inferred|disputed|unknown",
source = "..." }`; an absent fact is `unknown`. A gold run stores a copy of the context it ran
with. Phase 2 does not read it to decide anything; applicability is phase 3. The values are the
user's to supply: this change ships every fact as `unknown`.

## Open Questions

1. The organization facts (D9) have no values yet; they need the user.
2. The first covered and uncovered documents (D8) need choosing by the user, 10–15 in all.
3. A human review sample of LLM-adjudicated labels (D7): who, how many, when.
