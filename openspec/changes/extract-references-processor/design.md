## Context

The design intent, reference model and decisions are in devdoc
`KnowledgeStore/doc-repo/devdocs/202610/2026101002-devdoc-extract-references-processor.md` (§ numbers
below refer to it). This document fixes how the change is built in ChenWeb.

Current state:
- Chunk processors implement `ChunkBatchProcessor` (`InitChunkBatch` / `ProcessChunk` /
  `FinalizeChunkBatch`). The coordinator (`chunk_batch_coordinator.go`) writes their
  `kb.inputs.status` entries and shares the DeepSeek prefix through `canonicalChunkInputText`.
- `extract_metrics` persists through `MetricsSQLStore`. In force-clear mode it deletes all of the
  record's rows (`DeleteMetricsByInputRecordID`). Otherwise it merges against
  `GetMetricsByInputRecordID`. `metric_id` is positional (`<rec>_mtc_<n>`).
- 65 SQL query sites in ~30 Go files read `kb.metrics`. The `.agents` benchmark scripts read it too.
- `kb.inputs` carries `user_id`, `ks_store_id` and `doc_no` (often empty). Grants are in
  `kb.agentic_knowledge_grants` (`user_id`, `knowledge_store_id`, `document_id`, `active`,
  `expires_at`).

## Goals / Non-Goals

**Goals:**
- An `extract_references` Phase B processor that uses no output of any other processor (only
  `chunking`). It stores labelled mentions, the shared resource catalog, R1 and R2.
- The resolver contract and the "whoever comes second resolves" rule, with the metric family as the
  first resolver. Steps 5 and 6 are both live.
- Resolved metric rows in `kb.metrics` that `extract_metrics` never deletes or merges away. Every
  `kb.metrics` reader gets an explicit default.

**Non-Goals (follow-up changes):**
- reference search partition, document-graph page, candidate review pages, contradictions page;
- generic R3 location (v1 stores the locator, and the metric resolver uses it to narrow candidates);
- resolvers for provisions, entities and inventory items;
- reference/entity reconciliation;
- LLM-judge matching;
- persisting human decisions (no review UI yet, but the columns exist).

## Decisions

**D1. One processor, chunk-batched.** `ReferencesProcessor` implements `ChunkBatchProcessor`.
- `ProcessChunk` makes one LLM call per chunk. The input is `canonicalChunkInputText` and the
  prompt is `prompts/prompt-extract-references-v1.md` (env `EXTRACT_REFERENCES_PROMPT`; model env
  `EXTRACT_REFERENCES_MODEL_NAME` via `loadModelConfigFromEnvKeys`). It returns `{references:[…],
  document_identifier}`.
- `FinalizeChunkBatch` does everything that needs the whole document, in one pass:
  1. validate table row citations and render their evidence using the metrics table helpers, then
     de-duplicate overlapping chunk results by source location (including logical table rows) and cited identity;
  2. parse identifiers in code;
  3. derive normativity, list links and mismatches;
  4. replace the record's mentions (Withdraw first, then delete and insert in a transaction);
  5. upsert the catalog;
  6. link the document to its own resource;
  7. run R2;
  8. run step 5, then the incoming-holding event (§5.4).

  Status comes from the coordinator, like every other chunk processor. *Alternative*: a plain
  `HandleEvent` processor. It was rejected because it would lose the shared cache prefix and
  coordinator status.

**D2. The LLM labels, code normalizes.** The LLM returns:
- labels: `target_type`, `target_subtype`, `mechanism`, `relationships`, `target_scope`,
  `target_locator`, `target_content`, `ref_form`, `list_kind`, `in_requirement`;
- the named quantity (`metric_name(_en)`, `metric_subject(_en)`, `condition`);
- `cited_raw`, `target_title`, `target_version`, `source_line_spans`, `source_table_rows`, `evidence`, `confidence`.

Code then:
- validates every enum against the governed lists in `reference_vocabulary.go`, mapping an unknown
  subtype to `others` and keeping the label in `target_subtype_label`;
- parses identifiers from `cited_raw` with deterministic rules: standard numbers (prefix, number,
  part, edition), DOI, URL, arXiv, ISBN, ISSN, PMID, RFC and a basic legal-citation form.

*Why:* the vocabulary must be stable for search and joins. Exact identifiers are better read by
code than by a model.

**D3. Normativity is computed, never asked.** `normativity` is derived from list membership:
`normative` if a `listed` mention with `list_kind=normative` has the same resource. In-text
mentions are linked to list entries by resource key (`list_entry_id`). `list_mismatch` is computed
only when the document has a normative list. A numbered or author-year citation inherits the
identifiers of its list entry before catalog lookup.

**D4. Catalog keys and visibility.** A resource is keyed by (`identity_key`, `visibility`).
- `identity_key` is the normalized identifier without edition (`standard:GB/T 755`,
  `doi:10.1145/…`), falling back to `title:<normalized title>` when there is no identifier.
- `visibility` is `public` when the key comes from a public identifier kind (standard, RFC, DOI,
  ISBN, ISSN, arXiv, PMID, patent, legal citation, http(s) URL). Otherwise it is
  `store:<ks_store_id>`, or `user:<user_id>` when the record has no store.
- Versions are keyed by (`resource_id`, `version_label`). An undated citation has no version.
- A shared row stores only public facts. Who cites a resource is only in `kb.ref_mentions`, which
  is per record.

**D5. Own identity.** Each processed document is attached to its resource through a
`kb_input:<id>` identifier with visibility `store:`/`user:`, never `public`. The identifier is
chosen in this order:
1. `kb.inputs.doc_no`;
2. the LLM's `document_identifier` from the first chunks;
3. a standard number parsed from the title or filename.

**D6. R2 holding with access.** For a mention with a resource, the candidate holdings are the
`kb_input` identifiers of that resource. A holding counts only if the citing record can see it:
same `user_id`, same `ks_store_id`, or an active, unexpired grant on the store or document for the
citing record's `user_id`.
- An undated citation takes the newest held version.
- A dated citation takes the exact version. If only other versions are held, the state is
  `edition_mismatch` and the newest is kept as `target_input_id` for review, but families do not
  resolve against it.
- States: `held`, `not_held`, `edition_mismatch`, `edition_ambiguous`.

**D7. The resolver registry.** The interface lives in `reference_resolver.go`:

```go
type ReferenceResolver interface {
    Family() string                                          // "metric"
    Takes(m RefMention) bool
    HasArtifacts(ctx context.Context, db *sql.DB, recordID int64) (bool, error)
    Resolve(ctx context.Context, db *sql.DB, m RefMention, targetRecordID int64) ([]ArtifactCandidate, error)
    Apply(ctx context.Context, db *sql.DB, m RefMention, cands []ArtifactCandidate) error
    Pending(ctx context.Context, db *sql.DB, m RefMention) error  // target not held or not processed yet
}
```

There is no `Withdraw`: `Apply` replaces a mention's links and rows, and deleting a mention removes
them by foreign-key cascade (`kb.metrics.ref_mention_id`, `kb.reference_artifact_links.ref_mention_id`),
which is how a re-run of `extract_references` withdraws. Resolvers are registered at init
(`RegisterReferenceResolver`). Step 5 is `ResolveOutgoing(ctx,
db, citingRecordID)`, called by `extract_references`. Step 6 is `ResolveIncoming(ctx, db, family,
targetRecordID)`, called by an artifact extractor after it saves. The incoming-holding event is
`ResolveIncoming` for every family whose `HasArtifacts` is true. All of them live in the
`docprocessing` package, so `extract_metrics` calls its own resolver directly. *Alternative*: a
separate Go package. It was rejected for v1 because every party is in `docprocessing`.

**D8. Write-then-look and idempotency.**
- Each event commits its own rows before it queries the other side.
- `Apply` takes `pg_advisory_xact_lock` on the citing record, so concurrent appliers to one record
  serialize their `<rec>_rfm_<n>` numbering.
- Links are unique on (`ref_mention_id`, `artifact_family`, `target_artifact_key`), and writes use
  `ON CONFLICT … DO UPDATE`.
- `Apply` for a mention first deletes that mention's previous links and rows, so re-resolution
  replaces and never accumulates.

**D9. Metric matching (v1, deterministic).**
- Candidates are the target's `kb.metrics` rows with `ref_mention_id IS NULL`, plus its
  `kb.metrics_dropped` rows with reason `term_definition` or `no_value`, which may be the target of
  a definition. When `target_locator` is set, candidates whose `table_name_or_section` contains
  the locator are preferred.
- Score =
  - max over zh/en of name similarity between the mention's `metric_name` and the candidate's
    `metric_name`: character bigram Dice for CJK, token Dice otherwise;
  - +0.15 when the subject matches with Dice ≥ 0.5;
  - +0.1 when the locator matches;
  - capped at 1.
- `resolved` when the top score ≥ 0.8 and leads the second by ≥ 0.1. For `criteria_set`, every
  candidate in the located table or section scores at least 0.8.
- Otherwise `needs_review` with the top 5 stored as `candidate` links. `no_match` when there are no
  candidates at all. `no_match` carries `type_doubtful` when no candidate scores 0.3 (nothing in the
  target resembles the quantity). A held target that `extract_metrics` has not processed yet keeps
  the mention `pending_target`.
- Constants live in code under the policy name `metric_reference_match` v1, recorded on each link
  (`policy_id` null, `policy_version` = "metric_reference_match/v1").

*Why not the decision-model judge now:* calibration needs real resolved references first. The
devdoc's "policy in `shared.decision_policies`" is the follow-up once an LLM judge is added.

**D10. Resolved metric rows.** `Apply` copies the target row's value columns:
- `metric_value`, `metric_unit(_en)`, `value_data_type`, `value_range_type`, `value_min`,
  `value_max`, `value_class(_en)`, `condition`, `formula_or_definition`, `threshold_or_target`,
  `metric_categories(_en)`, `keyword_concept_id`, `metric_definition_term_id`.

It fills context from the citing mention:
- `metric_name(_en)` and `metric_subject(_en)`, falling back to the target's;
- `metric_context` = evidence;
- `source_line_spans`, `table_name_or_section` = the mention's section;
- `reasoning_tags` = `["reference_resolved"]`.

It sets `ref_mention_id`, `ref_input_record_id`, `ref_metric_id` (the target `metric_id`) and
`ref_metric_key` (fingerprint, see D11). `metric_id` = `<rec>_rfm_<n>`. A target row from
`kb.metrics_dropped` is never copied into `kb.metrics`: it has no value. It is linked only.

**D11. Stable keys.**
- `mention_key` = sha1(record id, section, normalized `cited_raw`, normalized metric name,
  locator).
- Target artifact key = sha1(record id, `table_name_or_section`, normalized `metric_name`,
  `metric_subject`, `condition`).

They are stored now so later human decisions can reattach.

**D12. `extract_metrics` guard and step 6.**
- `DeleteMetricsByInputRecordID` adds `AND ref_mention_id IS NULL`.
- `GetMetricsByInputRecordID` (merge and objects) reads only own rows.
- After a successful save, in both force-clear and merge paths, `FinalizeChunkBatch` calls
  `ResolveIncoming(ctx, db, "metric", recordID)`. A failure there is logged at WARN and never
  fails `extract_metrics`.

**D13. Reader defaults.**
- Every query site reading `kb.metrics` adds `ref_mention_id IS NULL`, except:
  - the metric list API (`kbhandler/metrics_handler.go`) returns resolved rows with their `ref_*`
    fields, plus an `include_resolved` flag defaulting to true;
  - the dropped-rows view, which reads `kb.metrics_dropped` and is untouched.
- The `.agents` `score_io.py` and `benchmark_io.py` filter the same way.
- *Why exclude by default:* every computation (reviews, Phase D, class contracts, ontology, wiki,
  search indexing, benchmark) must treat a resolved row as "value asserted by another document".
  Until each one models that, excluding is the safe default (devdoc §6.3).

**D14. Registration.**
- `processor_plan.go` spec: Phase B, `DependsOn: chunking`, `Produces: references`.
- `runtime.go` constructor.
- `control.go` `requiresChunkingDependency`.
- `config.toml` `required_processors`. Default selection is the user's call; it is not added to
  `default_processors` in this change.
- Dashboard processor lists in `doc-processor-dashboard-view.svelte`.

## Risks / Trade-offs

- [Missing a `kb.metrics` reader, so resolved rows leak into a computation] → a grep-based test
  asserts every `kb.metrics` query in non-test Go files carries `ref_mention_id` or is on an
  allowlist.
- [Deterministic name matching misses cross-language or paraphrased quantities] → those land in
  `needs_review` with candidates, never a wrong silent match. The LLM judge is the follow-up.
- [The LLM mislabels `criterion` vs `method`] → the prompt carries the devdoc tests with examples
  from several domains. Wrong `criterion` labels mostly end as `no_match` / `type_doubtful`, which
  is visible.
- [Concurrent processing of two documents that reference each other] → D8: write-then-look plus an
  advisory lock and upserts.
- [Cost: one more LLM call per chunk] → shares the cached chunk prefix, and the processor is not
  in `default_processors` until the user enables it.
- [A private resource accidentally made public] → visibility is computed from the identifier kind
  only. `kb_input` identifiers are never public.

## Migration Plan

1. Goose migrations (project): create the five `kb.ref_*` / `kb.reference_artifact_links` tables
   and add the four nullable `kb.metrics` columns plus a partial index on `ref_mention_id`. All
   are additive.
2. Deploy code. Existing rows have null `ref_*`, so every reader behaves as today.
3. Enable `extract_references` per pipeline or request. Re-run it on records to backfill.
4. **Rollback:** drop the processor from config. Resolved rows can be removed with
   `DELETE FROM kb.metrics WHERE ref_mention_id IS NOT NULL`, and the tables dropped by down
   migrations.

## Open Questions

- Whether `extract_references` joins `default_processors` (cost vs coverage). Left to the user.

**D15. Table provenance shares the metrics representation.** `kb.ref_mentions.source_table_rows`
is nullable JSONB with the same `{line, rows, row_hash}` entries as `kb.metrics.source_table_rows`.
At finalize, the processor builds a table index from its own chunk lines; it reads no other
extractor's output. It validates explicit row IDs and computes hashes from the grid before using
metrics' context builder. Explicit valid rows take precedence and are all preserved, including
headers. Without explicit rows, the existing evidence/small-table fallback applies. Captions and
column-labelled rows become evidence, with prose evidence retained when the source also spans
prose. Row locations participate in deduplication and mention keys, preventing two occurrences
on one table line from collapsing. The references API exposes provenance; the metric resolver
copies the citing mention's rows into resolved metrics rather than using the target's rows.
