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
4. A tier-6 automatic merge requires at least one authoritative positive signal and no hard veto.
5. A plausible embedding candidate without authoritative corroboration is deferred, never merged.

The first authoritative signal is an exact external identity from a registered, versioned source. Future validators may add governed quantity/unit compatibility and strong document-context evidence, but unsupported evidence types remain explicit deferrals.

This decision supersedes the minimum loop's `tier6EmbeddingMinScore` as merge authority. It does not remove continuous scores from candidate ranking or the audit trail. Tier 5 remains a deterministic edit-distance path with its existing score and spelling guardrails; §5 defines its separate precedence and application rules so it cannot accidentally inherit or bypass tier-6 policy.

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

Two schema corrections are required:

1. Add `release TEXT NOT NULL DEFAULT ''` to `kb.keyword_surface_evidence`, replace its uniqueness key with `(surface_id, source, external_id, release)`, and add a composite foreign key to `kb.keyword_sources(source, release)`. Without the release, candidate-side evidence cannot name the same versioned identity asserted by `keyword_external_ids`.
2. Add `identity_authority BOOLEAN NOT NULL DEFAULT FALSE` to `kb.keyword_sources`. Registration preserves provenance and licensing; it does not by itself grant a source authority to merge concepts. An importer or curator must explicitly mark a source release authoritative for exact identity.

`kb.keyword_external_ids` also gains a composite foreign key to `kb.keyword_sources(source, release)`. Both halves of an authoritative identity must therefore refer to the same registered source release.

An **exact external identity** exists when a provisional concept has any surface-evidence row whose `(source, external_id, release)` equals an external-ID mapping on a candidate target and that source release has `identity_authority=TRUE`.

Conflict rules:

- The existing primary key permits one concept per `(source, external_id, release)`. An attempted duplicate mapping is a database constraint failure, not a validator outcome.
- If a provisional concept carries multiple authoritative identity triples that map to different live concepts, reject the whole candidate.
- Multiple authoritative triples that all map to the same established target strengthen one decision; they do not create multiple proposals or writes.
- `related`, `broad`, `narrow`, textual similarity, or a shared source without a shared external ID never count as exact identity.
- An automatic reconciliation always absorbs the scanned D11 provisional candidate into one established target. The target must be `status='active'`; another provisional concept is never an automatic survivor.
- Existing `never_merge`, digit, lock, scope, and `aligns_to_term` conflict gates remain authoritative and are revalidated transactionally as specified in §5.3.

## 5. Components and data flow

### 5.1 Candidate proposal and precedence

`Reconciler` retains lexical and embedding blocking. The embedding client interface stays generic: text in, vectors out. Reconciliation code does not branch on model name, vendor, dimensions, prompt format, or score range. `Reconciler.EmbeddingModelID` is immutable run metadata supplied by the composition root and required whenever an embedding client is present; it is recorded, never interpreted.

Candidate-level processing has deterministic precedence:

1. Scan only `status='provisional' AND gloss_source='auto:d11'` candidates in the requested scope.
2. Tier 5 considers only active targets in that scope. One unique top-scoring target passing the existing edit-distance threshold and spelling vetoes may proceed as `tier5_fuzzy`; when the top two scores differ by at most `1e-9`, the result is a tie and defers. Tier 5 does not require external identity because it is deterministic spelling evidence, not model output.
3. If tier 5 does not authorize a target, tier 6 forms the deduplicated union of active targets from embedding rank and exact external mappings. Deduplication is by target concept ID; all methods and evidence remain attached to the target proposal.
4. Only embedding proposals are bounded to top K. Authoritative mappings for the scanned candidate are loaded exhaustively and are never truncated or ordered by embedding score. Zero mapped active targets defers the top embedding proposal; exactly one mapped active target proceeds to validation regardless of embedding rank; more than one mapped active target rejects the entire candidate. If exhaustive authority lookup cannot complete, the run fails closed.

Each candidate-target proposal carries:

- candidate and target concept IDs;
- method and raw score;
- rank, model ID, and bounded candidate-set metadata;
- all exact-identity triples supporting that target;
- no authority to write.

Exact external mappings are added exhaustively to the candidate union; only its embedding-derived portion is bounded. This ensures authoritative evidence is not lost merely because an embedding model ranks the true cross-lingual match poorly.

### 5.2 Deterministic validation

A validator receives the candidate-level aggregate and returns exactly one of:

- `auto_merge`: a unique tier-5 target or one uniquely mapped authoritative tier-6 target, and no veto;
- `defer`: plausible candidate but insufficient authoritative evidence;
- `reject`: deterministic conflict, invalid evidence, or hard veto.

For tier 6, the initial validator implements exact external identity only. Quantity/unit and context validators are separate future components, not empty hooks that silently pass.

Veto aggregation is explicit:

- Conflicting authoritative mappings are candidate-global and produce `reject` before any target is chosen.
- Scope, active-target, digit, surface-lock, `never_merge`, and alignment conflicts are pair-local on the chosen absorbed/survivor pair, but any one produces the candidate's single `reject` outcome.
- Other embedding proposals are retained in the audit payload but cannot produce additional outcomes after a unique authoritative target is chosen.

### 5.3 Apply and audit

Only `auto_merge` reaches application. Direction is fixed: `absorbed=scanned D11 provisional`, `survivor=chosen active target`. `electMergeDirection` is bypassed for automatic reconciliation, so surface count, age, or lexical concept ID can never tombstone the established target.

Authorization and application occur in one database transaction under a shared identity-lock protocol:

Before reading or changing keyword identity state, acquire one transaction-scoped PostgreSQL advisory lock with a stable, namespaced key for the whole keyword identity family. The same helper is mandatory in every domain writer that can change a decision while it is in flight:

- automatic and manual concept merge;
- `NeverMergeStore.Add`/remove for the affected keyword concepts;
- `AlignmentsStore.EnsureAccepted`, retraction, and merge-follow;
- external-ID and surface-evidence create/update/delete operations.

This deliberately serializes keyword identity mutations across scopes. The minimum reconciliation loop is offline and identity writes are rare; the lower write concurrency buys a simple invariant that also covers global external identities whose affected concepts cannot be known before lookup. Concept and evidence rows are still row-locked after the family lock. This serializes an absent-veto check with a concurrent veto insertion; locking only existing rows is insufficient because an absent row cannot be row-locked. Direct SQL that bypasses domain stores is administrative repair and must hold the same family lock or run while reconciliation is stopped.

After acquiring the locks:

1. Lock both concept rows and recheck candidate status/origin, active survivor, equal concept scope, and requested run scope.
2. Lock every surface row for both concepts. Reject when an absorbed surface is locked. Recompute digit signatures over all surfaces; when either concept has digit-bearing surfaces, the two sets must agree exactly. This is conservative by design.
3. Rerun the exhaustive authoritative-mapping query for every evidence triple on the candidate while holding the family lock. Reject unless the active target set is still exactly the chosen singleton. Lock all source, surface-evidence, and external-ID rows returned by that exhaustive query and revalidate `identity_authority`; revalidating only the originally chosen rows is insufficient.
4. Recheck `kb.semid_never_merge` inside the transaction.
5. Require a wired `AlignmentsStore`; recheck its different-term conflict and perform alignment-follow inside the transaction.
6. Apply the tombstone and surface re-point with `origin_concept` provenance.
7. Append the decision-log row through the same transaction, then commit.

Implementation should refactor the existing merge transaction body into a transaction-accepting helper used by both `MergeConcept` and reconciliation. It must not validate evidence outside one transaction and then call the current self-transactional method, which would create a time-of-check/time-of-use authorization race. The shared advisory-lock helper and its use by every veto/evidence writer are part of this slice, not optional hardening.

Each scanned candidate writes exactly one auditable decision containing:

- embedding model identifier and raw ranking scores;
- authoritative evidence triples used;
- vetoes or conflicts found;
- every considered proposal, the final candidate-level outcome, and reason;
- actor, scope, absorbed/survivor IDs when applied.

Embedding scores stay observable for later evaluation but never determine the outcome. Tier-5 edit-distance scores retain their deterministic threshold and unique-top role.

`no_candidate` also writes a candidate-level decision row with an empty proposal list. `defer`, `reject`, and `no_candidate` log in their own short transaction; `auto_merge` logs atomically with the merge. The CLI reports mutually exclusive candidate counts: `merged`, `deferred_unvalidated`, `rejected`, and `no_candidate`. Their sum equals `scanned`. A missing evidence/alignment store or query error fails the run; it does not degrade to score-only merging.

## 6. Error handling and operational behavior

- Missing, unregistered, or non-authoritative source/release: defer with an explicit reason.
- Conflicting exact identities: reject and surface the conflicting triples and concepts.
- Evidence query failure: fail the reconciliation run so no unaudited partial policy is applied.
- Decision-log failure rolls back an automatic merge because audit and application share a transaction.
- Existing concepts produced before this schema correction remain readable. Migration-created source placeholders have `identity_authority=FALSE`, so legacy evidence gains no merge authority accidentally.

### 6.1 Migration procedure

Create a new goose migration; never edit applied migration `20260805000003`.

Up sequence:

1. Add nullable `keyword_surface_evidence.release` and `keyword_sources.identity_authority BOOLEAN NOT NULL DEFAULT FALSE`.
2. Backfill evidence release to `''`.
3. Insert missing `(source, '')` registry placeholders referenced by legacy surface evidence, with empty license, `identity_authority=FALSE`, and notes identifying migration backfill.
4. Insert missing registry placeholders for every exact `(source, release)` pair already referenced by `keyword_external_ids`, preserving non-empty releases and using `identity_authority=FALSE`.
5. Make evidence release non-null with default `''`.
6. Replace the evidence uniqueness constraint with `(surface_id, source, external_id, release)`.
7. Run diagnostic queries and abort with source/release counts if any evidence or external-mapping orphan remains.
8. Add composite foreign keys from both evidence and external mappings to `keyword_sources(source, release)` using restrictive update/delete behavior.

Down sequence first aborts with a clear error if dropping release would collapse two evidence rows onto the old uniqueness key. Otherwise it drops the two foreign keys, restores the old uniqueness key, drops evidence release, and drops `identity_authority`. Placeholder source rows are retained as provenance rather than destructively guessed away.

## 7. Tests and acceptance

### 7.1 Unit and SQL-store tests

- High cosine without exact external identity defers and writes one candidate-level audit row.
- Low or differently scaled cosine with matching exact identity may merge.
- Results remain identical when fake embeddings are rescaled or shifted while candidate ordering is preserved.
- Matching `(source, external_id)` with a different release does not authorize a merge.
- Multiple identity triples to one target merge once; triples mapping to different targets reject once.
- Unregistered/non-authoritative source releases defer.
- Tier-5 behavior remains score/guardrail-driven and does not consult embedding scores.
- Tier-5 equal top scores within `1e-9` defer; one unique top score may proceed.
- Automatic direction always absorbs the scanned D11 provisional into an active target; provisional targets defer.
- Existing `never_merge`, all-surface digit, absorbed-surface lock, alignment, and scope vetoes dominate positive evidence and are rechecked in the merge transaction.
- Retracting evidence concurrently with reconciliation either completes before validation and prevents the merge, or blocks until the merge transaction commits; it cannot invalidate authorization between validation and apply.
- Concurrent insertion of `never_merge`, a conflicting alignment, new surface evidence, or a conflicting external mapping follows the same family-lock protocol: either the mutation commits first and the exhaustive in-transaction reread rejects the merge, or the merge commits first and the later writer observes the tombstone/current identity before deciding.
- A dedicated concurrency test inserts a second authoritative mapping to another active target after proposal assembly; it must block on the family lock or commit first and force the exhaustive reread to reject.
- Exhaustive authoritative mappings are never truncated by embedding top K; authority overflow or lookup failure fails closed.
- `merged + deferred_unvalidated + rejected + no_candidate == scanned`, with one decision-log row per scanned candidate.
- SQL tests cover evidence lookup, the new release/authority columns, placeholder backfill, uniqueness, both source/release foreign keys, and guarded Down behavior.

### 7.2 Live PostgreSQL proof

Against rebuilt `chenweb_test`:

1. Register a versioned bilingual fixture source.
2. Create an established `Luminance` concept with an external-ID mapping.
3. Create a D11 provisional `亮度` concept with surface evidence carrying the same identity triple.
4. Run `cmd/keyword-reconcile` through either configured multilingual embedding model.
5. Assert one merge in the fixed provisional-to-active direction, the surface moves with origin provenance, and the decision log names exact external identity as authority while retaining model/score diagnostics.
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
