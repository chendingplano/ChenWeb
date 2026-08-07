# Model-Agnostic Tier-6 Reconciliation Validation

**Status:** Approved design (2026-08-07)  
**Governing spec:** `KnowledgeStore/doc-repo/specs/202608/2026080403-spec-keyword-canonicalization-and-reconciliation.md`  
**Scope:** Keyword reconciliation only; no online resolver or consumer changes

## 1. Problem and live evidence

The minimum tier-6 reconciler currently treats one raw cosine threshold as sufficient evidence for an automatic concept merge. That is not model-agnostic. Embedding models produce materially different score distributions, so a universal raw threshold cannot mean the same thing across models.

The I2 PostgreSQL run on 2026-08-06/07 demonstrated the defect with the exact motivating pair:

| Model | `亮度` / `Luminance` cosine | Current threshold | Result |
|---|---:|---:|---|
| Qwen3-Embedding-0.6B Q8 | 0.6711 | 0.90 | deferred |
| nomic-embed-text-v2-moe Q4 | 0.4524 | 0.90 | deferred |

Sampling Qwen against nearby metric names also showed why merely lowering the threshold is unsafe: `Luminance` / `Contrast ratio` scored 0.8035 with bare labels. Qwen's official model card recommends an English task instruction on the query side for retrieval, but that is a model usage optimization, not portable merge evidence. The reconciliation decision must remain correct if an operator changes the configured embedding model.

References:

- Qwen3-Embedding model card: <https://huggingface.co/Qwen/Qwen3-Embedding-0.6B>
- Keyword spec §13 R3/R6 and §13.1: embeddings block candidates; deterministic gates own writes.

## 2. Decision

Tier 6 remains model-agnostic by separating **candidate generation** from **merge authority**:

1. Embeddings produce a bounded, ranked candidate set.
2. Raw cosine and rank are recorded for audit and diagnostics only.
3. A deterministic validator decides whether a proposed pair may merge.
4. Automatic merge requires at least one authoritative positive signal and no hard veto.
5. A plausible embedding candidate without authoritative corroboration is deferred, never merged.

The first authoritative signal is an exact external identity from a registered, versioned source. Future validators may add governed quantity/unit compatibility and strong document-context evidence, but unsupported evidence types remain explicit deferrals.

This decision supersedes the minimum loop's `tier6EmbeddingMinScore` as merge authority. It does not remove continuous scores from candidate ranking or the audit trail.

## 3. Alternatives rejected

### 3.1 One lower universal cosine threshold

Rejected because cosine scales and neighborhood density vary by model. The live positive scored below a sampled negative from the same model, so one lower number would trade the current false negatives for structural false merges.

### 3.2 Relative rank, percentile, or mutual-nearest-neighbor alone

Rejected as merge authority. Relative rules normalize scale but cannot establish semantic identity: every small or unrelated cohort still has a nearest pair. These signals may improve blocking or review ordering later.

### 3.3 Multi-model consensus

Rejected for the initial design because it adds runtime cost and operational coupling without producing authoritative identity evidence. Agreement between two heuristics is still heuristic evidence.

## 4. Evidence model

The existing step-7 schema is reused:

- `kb.keyword_sources` registers `(source, release)` and its license/provenance.
- `kb.keyword_external_ids` maps `(source, external_id, release)` to an established concept.
- `kb.keyword_surface_evidence` records the evidence supporting a surface.

One schema correction is required: add `release TEXT NOT NULL DEFAULT ''` to `kb.keyword_surface_evidence`, replace its uniqueness key with `(surface_id, source, external_id, release)`, and add a composite foreign key to `kb.keyword_sources(source, release)`. Without the release, the candidate-side evidence cannot name the same versioned identity asserted by `keyword_external_ids`.

An **exact external identity** exists when a provisional concept has any surface-evidence row whose `(source, external_id, release)` equals an external-ID mapping on a candidate target. Only sources registered in `keyword_sources` participate.

Conflict rules:

- If one external identity maps to more than one live concept, reject automatic reconciliation and record a data-integrity conflict.
- If a provisional concept carries multiple exact identities that point to different live concepts, reject.
- `related`, `broad`, `narrow`, textual similarity, or a shared source without a shared external ID never count as exact identity.
- Existing `never_merge`, digit, lock, scope, merge-direction, and `aligns_to_term` conflict gates remain authoritative and run before application.

## 5. Components and data flow

### 5.1 Candidate proposal

`Reconciler` retains lexical and embedding blocking. The embedding client interface stays generic: text in, vectors out. Reconciliation code does not branch on model name, vendor, dimensions, prompt format, or score range.

The ranked embedding candidates become proposals carrying:

- candidate and target concept IDs;
- method and raw score;
- rank and bounded candidate-set metadata;
- no authority to write.

Exact external mappings are also added to the bounded candidate union. This ensures authoritative evidence is not lost merely because an embedding model ranks the true cross-lingual match poorly.

### 5.2 Deterministic validation

A validator receives a proposal and returns one of:

- `auto_merge`: one authoritative exact-identity signal, and no veto;
- `defer`: plausible candidate but insufficient authoritative evidence;
- `reject`: deterministic conflict, invalid evidence, or hard veto.

The initial validator implements exact external identity only. Quantity/unit and context validators are separate future components, not empty hooks that silently pass.

### 5.3 Apply and audit

Only `auto_merge` reaches `ConceptStore.MergeConcept`. Existing transactional merge, alignment-follow, and conflict behavior remain unchanged.

Every considered proposal writes an auditable decision containing:

- embedding model identifier and raw ranking scores;
- authoritative evidence triples used;
- vetoes or conflicts found;
- final outcome and reason;
- actor, scope, absorbed/survivor IDs when applied.

Scores stay observable for later evaluation but never determine the outcome.

The CLI reports `merged`, `deferred_unvalidated`, `rejected`, and `no_candidate` separately. A missing evidence store or query error fails the run; it does not degrade to score-only merging.

## 6. Error handling and operational behavior

- Missing or unregistered source/release: defer with an explicit reason.
- Conflicting exact identities: reject and surface the conflicting triples and concepts.
- Evidence query failure: fail the reconciliation run so no unaudited partial policy is applied.
- Decision-log failure after a committed merge remains the minimum loop's known non-atomic limitation; this design does not widen that issue.
- Existing concepts produced before this schema correction remain readable because the new release column defaults to the empty release; they gain authority only when the corresponding registered source and external mapping use that same explicit release value.

## 7. Tests and acceptance

### 7.1 Unit and SQL-store tests

- High cosine without exact external identity defers.
- Low or differently scaled cosine with matching exact identity may merge.
- Results remain identical when fake embeddings are rescaled or shifted while candidate ordering is preserved.
- Matching `(source, external_id)` with a different release does not authorize a merge.
- Conflicting external identities reject.
- Unregistered source/release defers.
- Existing `never_merge`, digit, alignment, lock, and scope vetoes dominate positive evidence.
- SQL tests cover evidence lookup, the new release column, uniqueness, and source/release referential integrity.

### 7.2 Live PostgreSQL proof

Against rebuilt `chenweb_test`:

1. Register a versioned bilingual fixture source.
2. Create an established `Luminance` concept with an external-ID mapping.
3. Create a D11 provisional `亮度` concept with surface evidence carrying the same identity triple.
4. Run `cmd/keyword-reconcile` through either configured multilingual embedding model.
5. Assert one merge, the surface moves with origin provenance, and the decision log names exact external identity as authority while retaining model/score diagnostics.
6. Repeat with high fake/live similarity but no identity evidence and assert defer.
7. Repeat import/reconciliation and assert idempotence.

The live test is about the invariant, not achieving a particular cosine.

## 8. Non-goals and deferred work

- No per-model thresholds, prompts, score normalization, or vendor branches.
- No `on`-mode retrieval wiring.
- No full R1-R7 orchestration, runs/watermark table, or adjudication UI.
- No standards-glossary or Wikidata production importer in this slice; the live proof uses a versioned fixture through the same evidence tables.
- No quantity/unit or context validator until their governed evidence is available.
- No change to online resolution tiers or `aligns_to_term` semantics.

## 9. Documentation impact

Implementation must update the keyword spec's §13.1 claim that embedding similarity may itself auto-accept, record the new validation contract in §13 R6 and §21, and close or narrow the I2 reconciliation gap. The master handoff update remains pending. `KnowledgeStore` edits must be made only after its pre-existing working-copy changes are resolved.

