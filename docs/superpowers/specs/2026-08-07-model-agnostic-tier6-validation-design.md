# Model-Agnostic Tier-6 Reconciliation Validation

**Status:** Revised design, pending user re-approval (2026-08-07)
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
3. Resource-specific evidence providers normalize their native evidence into a storage-independent identity-claim contract.
4. A deterministic validator—not an evidence provider—decides whether a proposed pair may merge.
5. A tier-6 automatic merge requires at least one authoritative `exact_equivalent` claim and no hard veto.
6. A plausible embedding candidate without authoritative corroboration is deferred, never merged.

The first evidence provider is the existing triple-backed external-identity schema. It emits an authoritative `exact_equivalent` claim only when a registered source release explicitly has identity authority and both sides carry the same `(source, external_id, release)`. Future providers may adapt other local imported schemas without changing the reconciler. Related, broader, narrower, translation-only, probabilistic, and context-dependent claims remain non-authorizing until a later governed design explicitly changes the validator contract.

This decision supersedes the minimum loop's `tier6EmbeddingMinScore` as merge authority. It does not remove continuous scores from candidate ranking or the audit trail. Tier 5 remains a deterministic edit-distance path with its existing score and spelling guardrails; §5 defines its separate precedence and application rules so it cannot accidentally inherit or bypass tier-6 policy.

## 3. Alternatives rejected

### 3.1 One lower universal cosine threshold

Rejected because cosine scales and neighborhood density vary by model. The live positive scored below a sampled negative from the same model, so one lower number would trade the current false negatives for structural false merges.

### 3.2 Relative rank, percentile, or mutual-nearest-neighbor alone

Rejected as merge authority. Relative rules normalize scale but cannot establish semantic identity: every small or unrelated cohort still has a nearest pair. These signals may improve blocking or review ordering later.

### 3.3 Multi-model consensus

Rejected for the initial design because it adds runtime cost and operational coupling without producing authoritative identity evidence. Agreement between two heuristics is still heuristic evidence.

## 4. Evidence model

### 4.1 Generic identity-evidence provider contract

Tier 6 depends on normalized identity claims, not directly on the triple tables. Every enabled resource adapter implements one transaction-aware provider contract conceptually equivalent to:

```go
type IdentityEvidenceProvider interface {
	ProviderID() string
	LoadClaims(ctx context.Context, tx *sql.Tx, candidate CandidateIdentityContext) ([]IdentityClaim, error)
}

type CandidateIdentityContext struct {
	CandidateConceptID string
	Scope              string
	SurfaceIDs         []string
}

type IdentityClaim struct {
	ProviderID         string
	CandidateConceptID string
	TargetConceptID    string // empty when evidence is present but not mapped
	Relation           IdentityRelation
	Authority          IdentityAuthority
	AuthorityRef       string // EvidenceRef.Key; required when authoritative
	EvidenceRefs       []EvidenceRef
}

type EvidenceRef struct {
	Key     string // stable and unique within ProviderID
	Kind    string
	Locator string
}
```

The exact Go names may change during planning, but the fields and behavioral boundary are binding:

- A provider owns adaptation from its resource-specific local schema into normalized claims.
- `ProviderID` and `EvidenceRefs` preserve native provenance; the core does not interpret provider-specific identifier syntax.
- `IdentityRelation` has the closed values `exact_equivalent`, `related`, `broader`, `narrower`, `translation`, `probabilistic`, and `other`. `IdentityAuthority` has the closed values `non_authoritative` and `authoritative`. Provider-specific subtypes belong in evidence references, not new core policy values.
- Only `Relation=exact_equivalent` together with `Authority=authoritative` contributes a Tier-6 authoritative target. The core validator applies this fixed rule uniformly across providers. Providers cannot introduce their own score threshold or merge policy.
- `Authority=authoritative` must derive from explicit, persisted governance configuration and defaults to non-authoritative. An authoritative claim requires a non-empty `AuthorityRef` naming one of its `EvidenceRefs` with `Kind=authority_configuration`. The referenced record identifies the persisted governance decision. An adapter cannot promote a source-native confidence value, relation, or API response to authority on its own. A new provider cannot emit authoritative claims until its persisted authority configuration and reference format are added to this design or a successor design.
- A claim with no mapped active target is still returned as non-authorizing evidence so the result is `deferred_unvalidated`, not `no_candidate`.
- Each provider query is exhaustive for the scanned candidate. Provider output is never restricted by embedding top K.
- Providers run inside the candidate's SQL decision transaction and must lock or otherwise transactionally stabilize all local rows that authorize or veto the decision. A resource that cannot provide a locally imported, transactionally stable PostgreSQL snapshot cannot be enabled as an identity-evidence provider in this slice; a future design may expose it as a non-authorizing proposal source outside this interface.
- Any enabled provider failure fails the candidate run closed; the reconciler does not silently continue with fewer sources.

The core validates registration once before scanning: provider IDs must be non-empty and unique, and the mandatory `triple_external_identity` provider must be present exactly once. The core supplies `SurfaceIDs` sorted and deduplicated. It validates every returned claim before using or logging it:

- `claim.ProviderID` must equal the invoked provider's ID, and `CandidateConceptID` must equal the requested candidate.
- Relation and authority values must be members of the closed enums.
- An authoritative `exact_equivalent` claim must have a non-empty target, at least one evidence reference, and a valid `AuthorityRef`; any authoritative non-exact claim is malformed rather than silently downgraded.
- Every evidence reference must have non-empty `Key`, `Kind`, and `Locator`. Within one provider, the same key must always identify the same `(Kind, Locator)`; a collision is malformed.
- Malformed registration or claim output is a provider normalization failure and fails the run closed before the claim can influence a decision.

Audit normalization is deterministic. A claim's canonical key is the tuple `(ProviderID, CandidateConceptID, TargetConceptID, Relation, Authority, AuthorityRef)`. Duplicate claims with that key are collapsed by taking the set union of their evidence references. Evidence references are deduplicated and sorted by `(Key, Kind, Locator)`; claims are sorted by their canonical key. The decision payload uses typed audit DTOs with fixed JSON field order; unordered maps are prohibited unless converted to sorted key/value arrays first. All authorizing and non-authorizing claims, including targetless claims, are written to the candidate audit record. Provider registration order, provider output order, and SQL row order cannot change the serialized audit payload.

This interface is generic across imported schemas, not a license to query live Internet services during reconciliation. External data is imported or snapshotted first so its release, provenance, authority, and decision-time consistency are auditable.

### 4.2 Initial triple-backed provider

The existing step-7 schema is reused:

- `kb.keyword_sources` registers `(source, release)` and its license/provenance.
- `kb.keyword_external_ids` maps `(source, external_id, release)` to an established concept.
- `kb.keyword_surface_evidence` records the evidence supporting a surface.

Two schema corrections are required:

1. Add `release TEXT NOT NULL DEFAULT ''` to `kb.keyword_surface_evidence`, replace its uniqueness key with `(surface_id, source, external_id, release)`, and add a composite foreign key to `kb.keyword_sources(source, release)`. Without the release, candidate-side evidence cannot name the same versioned identity asserted by `keyword_external_ids`.
2. Add `identity_authority BOOLEAN NOT NULL DEFAULT FALSE` to `kb.keyword_sources`. Registration preserves provenance and licensing; it does not by itself grant a source authority to merge concepts. An importer or curator must explicitly mark a source release authoritative for exact identity.

`kb.keyword_external_ids` also gains a composite foreign key to `kb.keyword_sources(source, release)`. Both halves of an authoritative identity must therefore refer to the same registered source release.

`TripleEvidenceProvider` has the stable provider ID `triple_external_identity` and reads these three tables. It emits an authoritative `exact_equivalent` claim when a provisional concept has any surface-evidence row whose non-blank `(source, external_id, release)` equals an external-ID mapping on a candidate target and that source release has `identity_authority=TRUE`. Its `AuthorityRef` names the matching `kb.keyword_sources` row, while its other evidence references identify the surface-evidence and external-mapping rows. The triple is the initial provider's storage representation, not the core validator's universal storage contract.

Conflict rules:

- The existing primary key permits one concept per `(source, external_id, release)`. An attempted duplicate mapping is a database constraint failure, not a validator outcome.
- `kb.keyword_external_ids.external_id` must be non-blank. Legacy `keyword_surface_evidence` rows may retain `external_id=''` to mean provenance without an external identity, but `TripleEvidenceProvider` returns them only as non-authorizing targetless evidence; they can never match or authorize.
- Across all enabled providers, authoritative exact-equivalence claims mapping the provisional concept to different live concepts reject the whole candidate.
- Multiple authoritative claims—including multiple triples or claims from different providers—that all map to the same established target strengthen one decision; they do not create multiple proposals or writes.
- `related`, `broad`, `narrow`, textual similarity, or a shared source without a shared external ID never count as exact identity.
- An automatic reconciliation always absorbs the scanned D11 provisional candidate into one established target. The target must be `status='active'`; another provisional concept is never an automatic survivor.
- Existing `never_merge`, digit, lock, scope, and `aligns_to_term` conflict gates remain authoritative and are revalidated transactionally as specified in §5.3.

## 5. Components and data flow

### 5.1 Candidate proposal and precedence

`Reconciler` retains lexical and embedding blocking and gains `EvidenceProviders []IdentityEvidenceProvider`. The composition root always wires `TripleEvidenceProvider`; an empty triple dataset is a valid exhaustive result, while a missing required provider is a configuration error. Future providers are explicitly enabled and appended through the same interface. Provider results are normalized, deduplicated, and audit-sorted by stable provider/evidence keys so registration order cannot change the decision or log. The embedding client interface stays generic: text in, vectors out. Reconciliation code does not branch on model name, vendor, dimensions, prompt format, or score range. `Reconciler.EmbeddingModelID` is immutable run metadata supplied by the composition root and required whenever an embedding client is present; it is recorded, never interpreted.

Candidate-level processing has deterministic precedence:

1. Scan only `status='provisional' AND gloss_source='auto:d11'` candidates in the requested scope.
2. Tier 5 considers only active targets in that scope. One unique top-scoring target passing the existing edit-distance threshold and spelling vetoes may proceed as `tier5_fuzzy`; when the top two scores differ by at most `1e-9`, tier 5 cannot authorize. Tier 5 does not require external identity because it is deterministic spelling evidence, not model output. A tier-5 tie still permits exhaustive identity-evidence validation: a unique authoritative target may resolve the ambiguity, but embedding rank alone may not.
3. If tier 5 does not authorize a target, tier 6 forms the deduplicated union of active targets from embedding rank and target-bearing normalized claims returned by all enabled identity-evidence providers. Deduplication is by target concept ID; all methods, claims, and native evidence references remain attached to the target proposal.
4. Current code has no embedding top-K list: it compares the candidate with every live concept and retains only the single best result above `0.90`. The existing compile-time `reconcileTopK=10` is actually passed only to the lexical SQL shortlist despite its broader comment. The redesign introduces a distinct compile-time `reconcileEmbeddingTopK=10`, initially not runtime-configurable, and retains the lexical bound separately as `reconcileLexicalTopK=10`. Only embedding proposals are bounded to semantic top K. Provider claims for the scanned candidate are loaded exhaustively and are never truncated or ordered by embedding score. Zero authoritative active targets defers the top embedding proposals; exactly one proceeds to validation regardless of embedding rank or absence from the embedding top 10; more than one rejects the entire candidate. If any enabled provider cannot complete its exhaustive lookup, the run fails closed.

Each candidate-target proposal carries:

- candidate and target concept IDs;
- method and raw score;
- rank, model ID, and bounded candidate-set metadata;
- all normalized identity claims and native evidence references supporting that target;
- no authority to write.

Provider-returned targets are added exhaustively to the candidate union; only its embedding-derived portion is bounded. This ensures authoritative evidence is not lost merely because an embedding model ranks the true cross-lingual match poorly or outside the top 10.

### 5.2 Deterministic validation

A validator receives the candidate-level aggregate and returns exactly one of:

- `auto_merge`: a unique tier-5 target or one unique authoritative exact-equivalence tier-6 target, and no veto;
- `deferred_unvalidated`: plausible candidate but insufficient authoritative evidence;
- `reject`: deterministic conflict, invalid evidence, or hard veto.
- `no_candidate`: no valid tier-5 proposal, provider claim, or embedding proposal.

For tier 6, the initial validator recognizes only normalized authoritative `exact_equivalent` claims as positive authority. `TripleEvidenceProvider` is the first producer of such claims. Quantity/unit and context validators are separate future components, not empty hooks that silently pass.

Veto aggregation is explicit:

- Conflicting authoritative exact-equivalence claims are candidate-global and produce `reject` before any target is chosen.
- Scope, active-target, digit, surface-lock, `never_merge`, and alignment conflicts are pair-local on the chosen absorbed/survivor pair, but any one produces the candidate's single `reject` outcome.
- Other embedding proposals are retained in the audit payload but cannot produce additional outcomes after a unique authoritative target is chosen.

The candidate-level decision table is binding:

| Tier-5 state | Authoritative exact-equivalence active targets | Other proposals/evidence | Outcome before pair vetoes |
|---|---:|---:|---|
| unique valid target | any | any | validate the tier-5 target; conflicting authority to a different target rejects |
| tie | exactly 1 | any | validate the authoritative target |
| tie | more than 1 | any | `reject` |
| tie | 0 | any | `deferred_unvalidated` |
| none | exactly 1 | any | validate the authoritative target |
| none | more than 1 | any | `reject` |
| none | 0 | at least 1 | `deferred_unvalidated` |
| none | 0 | 0 | `no_candidate` |

“Validate” produces `auto_merge` only when all pair vetoes pass; otherwise it produces `reject`. A tier-5 target and a unique authoritative target that name the same concept are one proposal. If they name different concepts, authoritative identity is a candidate-global conflict and rejects rather than allowing the tier-5 merge.

“Other proposals/evidence” means an embedding proposal or any normalized provider claim that cannot authorize: a non-exact relation, non-authoritative evidence, no mapped target, or a mapping to a non-active target. For `TripleEvidenceProvider`, examples include a missing external mapping or a registered but non-authoritative source release. Any such evidence prevents `no_candidate` and produces `deferred_unvalidated` when no stronger path decides the candidate. A claim targeting a provisional concept never makes it an automatic survivor.

### 5.3 Apply and audit

Only `auto_merge` reaches application. Direction is fixed: `absorbed=scanned D11 provisional`, `survivor=chosen active target`. `electMergeDirection` is bypassed for automatic reconciliation, so surface count, age, or lexical concept ID can never tombstone the established target.

Authorization and application occur in one database transaction under a shared identity-lock protocol:

Before reading or changing keyword identity state, call `AcquireKeywordIdentityMutationLock(ctx, tx)`, which executes exactly:

```sql
SELECT pg_advisory_xact_lock(1264011588, 1);
```

`1264011588` is the fixed namespace value for ASCII `KWID`; the second key is lock-contract version 1. These constants are part of the persistence contract and must not be recomputed with a process- or database-version-dependent hash. The same helper is mandatory in every domain writer that can change a decision while it is in flight:

- automatic and manual concept merge;
- concept create/update/status operations that change `pref_label`, `scope`, `status`, or `gloss_source`;
- surface create/update/lock/delete operations;
- `NeverMergeStore.Add`/remove for the affected keyword concepts;
- `AlignmentsStore.EnsureAccepted`, retraction, and merge-follow;
- all enabled provider backing-data create/update/delete operations, including external-ID and surface-evidence changes for `TripleEvidenceProvider`.

This deliberately serializes keyword identity mutations across scopes. The minimum reconciliation loop is offline and identity writes are rare; the lower write concurrency buys a simple invariant that also covers global external identities whose affected concepts cannot be known before lookup. Concept and evidence rows are still row-locked after the family lock. This serializes an absent-veto check with a concurrent veto insertion; locking only existing rows is insufficient because an absent row cannot be row-locked. Direct SQL that bypasses domain stores is administrative repair and must acquire the same two-key advisory lock or run while reconciliation is stopped.

Lexical queries and embedding calls may assemble provisional proposals outside this lock because they have no write authority. Identity-evidence provider calls that may authorize run inside the lock and transaction. Every scanned candidate opens one decision transaction, acquires the family lock, performs exhaustive provider, authority, and veto reads, selects exactly one outcome from the decision table, appends exactly one decision-log row, and commits. `auto_merge` additionally applies the merge in that transaction. `defer`, `reject`, and `no_candidate` hold the same lock through their decision-log write, so the logged evidence and outcome share one linearization point.

After acquiring the family lock, execute in this order:

1. Lock and recheck the scanned candidate row (`status='provisional'`, `gloss_source='auto:d11'`, requested scope), then lock all candidate surfaces.
2. Recompute the complete tier-5 active-target query, edit-distance scores, unique-top/tie rule, and spelling guardrails from current transactional state. The pre-lock tier-5 proposal is diagnostic only and cannot authorize a merge.
3. Call every enabled identity-evidence provider inside the transaction and require an exhaustive result. Each provider locks or otherwise transactionally stabilizes its local backing rows. Normalize and deduplicate authoritative active targets and retain all non-authorizing claims. `TripleEvidenceProvider` locks all source, candidate surface-evidence, and external-ID rows it reads.
4. Select the one candidate-level outcome using the binding table. Target-less branches (`no_candidate`, tier-5 tie without authority, invalid/provisional external target, or multi-authority conflict) do not attempt to lock a second concept. Append their audit row and commit.
5. Only for a `validate` branch, lock the chosen target concept and all its surfaces. Recheck target `status='active'`, equal concept scope, and requested run scope. Reject when an absorbed candidate surface is locked. Recompute digit signatures over every surface on both concepts; when either concept has digit-bearing surfaces, the two sets must agree exactly.
6. Apply method-specific authority revalidation:
   - for `tier6_identity_claim`, the authoritative exact-equivalence active-target set across all enabled providers must be exactly the chosen singleton;
   - for `tier5_fuzzy`, zero authoritative targets or one authoritative target equal to the chosen tier-5 target may proceed, while any different or multiple authoritative targets reject as a candidate-global identity conflict.
7. Recheck `kb.semid_never_merge`. Require a wired `AlignmentsStore`, recheck its different-term conflict, and prepare alignment-follow inside the transaction.
8. If any validate-branch veto fires, append the candidate's `reject` audit row and commit without merging. Otherwise apply the tombstone, surface re-point with `origin_concept`, alignment-follow, and `auto_merge` audit row, then commit.

Implementation should refactor the existing merge transaction body into a transaction-accepting helper used by both `MergeConcept` and reconciliation. It must not validate evidence outside one transaction and then call the current self-transactional method, which would create a time-of-check/time-of-use authorization race. The shared advisory-lock helper and its use by every veto/evidence writer are part of this slice, not optional hardening.

Each scanned candidate writes exactly one auditable decision containing:

- embedding model identifier and raw ranking scores;
- every canonical normalized claim—authorizing, non-authorizing, mapped, and targetless—and its native evidence and authority references;
- vetoes or conflicts found;
- every considered proposal, the final candidate-level outcome, and reason;
- actor, scope, absorbed/survivor IDs when applied.

Embedding scores stay observable for later evaluation but never determine the outcome. Tier-5 edit-distance scores retain their deterministic threshold and unique-top role.

`no_candidate` means no valid tier-5 proposal, no normalized provider evidence/proposal of any strength, and no embedding proposal after blocking. It writes a candidate-level decision row with an empty proposal list. Non-authorizing evidence—including a non-exact relation, missing target mapping, non-authoritative source, or inactive/provisional mapped target—produces `deferred_unvalidated` when no stronger path decides the candidate. `defer`, `reject`, and `no_candidate` log in their candidate decision transaction; `auto_merge` logs atomically with the merge. The CLI reports mutually exclusive candidate counts: `merged`, `deferred_unvalidated`, `rejected`, and `no_candidate`. Their sum equals `scanned`. A missing required provider/alignment store or query error fails the run; it does not degrade to score-only merging.

## 6. Error handling and operational behavior

- Non-exact, non-authoritative, unmapped, and inactive/provisional-target claims defer with explicit reasons. For `TripleEvidenceProvider`, an unregistered source/release violates the post-migration foreign-key contract and fails the run as data corruption rather than becoming a decision.
- Conflicting authoritative exact-equivalence claims: reject and surface the provider IDs, native evidence references, and concepts.
- Any enabled provider query or normalization failure: fail the reconciliation run so no incomplete evidence set is treated as authoritative.
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
8. Abort with a count if any existing `keyword_external_ids.external_id` is blank, then add a non-blank external-ID CHECK to that mapping table. Do not add the CHECK to `keyword_surface_evidence`, where a blank ID remains valid non-authorizing provenance.
9. Add composite foreign keys from both evidence and external mappings to `keyword_sources(source, release)` using restrictive update/delete behavior.

The migration uses explicit constraint names:

- drop legacy unique `keyword_surface_evidence_surface_id_source_external_id_key`;
- add `uq_keyword_surface_evidence_source_external_release` on `(surface_id, source, external_id, release)`;
- add `ck_keyword_external_ids_nonblank_external_id` as `CHECK (btrim(external_id) <> '')`;
- add `fk_keyword_surface_evidence_source_release` and `fk_keyword_external_ids_source_release`, both referencing the existing primary key `keyword_sources_pkey (source, release)` with `ON UPDATE RESTRICT ON DELETE RESTRICT`.

Legacy placeholder inserts supply only `source`, `release`, `license`, and `notes`; `retrieved_at` remains NULL. Evidence placeholders select distinct `source, ''`. External-mapping placeholders select distinct `source, release` from `kb.keyword_external_ids`, preserving each stored release. Both use `ON CONFLICT (source, release) DO NOTHING`. A `DO` block checks both left joins for NULL source rows and raises an exception with orphan counts before either foreign key is added.

Down sequence first checks for duplicate `(surface_id, source, external_id)` groups and raises an exception if dropping release would collapse them. Otherwise it drops the two named foreign keys, drops `ck_keyword_external_ids_nonblank_external_id`, drops the new named unique constraint, restores `keyword_surface_evidence_surface_id_source_external_id_key`, drops evidence release, and drops `identity_authority`. Placeholder source rows are retained as provenance rather than destructively guessed away.

## 7. Tests and acceptance

### 7.1 Unit and SQL-store tests

- High cosine without exact external identity defers and writes one candidate-level audit row.
- Low or differently scaled cosine with matching exact identity may merge.
- Results remain identical when fake embeddings are rescaled or shifted while candidate ordering is preserved.
- Provider contract tests prove that different resource-specific schemas normalize to the same claim shape and therefore produce the same validator outcome.
- Only authoritative `exact_equivalent` claims authorize; related, broader, narrower, translation-only, probabilistic, non-authoritative, and unmapped claims defer when no stronger path exists.
- A provider target outside the embedding top 10 is still considered and may merge; embedding rank never truncates provider claims.
- Embedding positions 10 and 11 prove the new semantic bound exactly, while a provider-only target absent from all embedding results remains eligible.
- Any enabled provider failure fails closed, and a provider without transactionally stable local evidence cannot emit authoritative claims.
- Empty/duplicate provider IDs and malformed claims—including wrong provider/candidate IDs, unknown enums, authoritative claims without a target or authority reference, and colliding evidence-reference keys—fail closed before decision.
- Shuffling provider registration, provider output, SQL rows, duplicate claims, and evidence-reference order produces byte-identical canonical audit JSON; duplicate claims retain the union of all distinct evidence references.
- Matching `(source, external_id)` with a different release does not authorize a merge.
- Blank external IDs in legacy surface evidence remain non-authorizing; blank external IDs in `keyword_external_ids` fail the migration CHECK and can never authorize.
- Multiple identity triples to one target merge once; triples mapping to different targets reject once.
- Registered but non-authoritative source releases defer. An attempted unregistered insert fails the foreign key; a simulated/corrupt unregistered lookup fails the reconciliation run.
- Tier-5 behavior remains score/guardrail-driven and does not consult embedding scores.
- Tier-5 equal top scores within `1e-9` cannot authorize by themselves: unique external authority may resolve the tie; otherwise the candidate defers.
- Automatic direction always absorbs the scanned D11 provisional into an active target; provisional targets defer.
- Existing `never_merge`, all-surface digit, absorbed-surface lock, alignment, and scope vetoes dominate positive evidence and are rechecked in the merge transaction.
- Retracting evidence concurrently with reconciliation either completes before validation and prevents the merge, or blocks until the merge transaction commits; it cannot invalidate authorization between validation and apply.
- Concurrent insertion of `never_merge`, a conflicting alignment, new surface evidence, or a conflicting external mapping follows the same family-lock protocol: either the mutation commits first and the exhaustive in-transaction reread rejects the merge, or the merge commits first and the later writer observes the tombstone/current identity before deciding.
- A dedicated concurrency test inserts a second authoritative mapping to another active target after proposal assembly; it must block on the family lock or commit first and force the exhaustive reread to reject.
- A concurrent concept-label/status or surface mutation after pre-lock tier-5 assembly must block or commit first; the in-transaction tier-5 recomputation must then change/defer/reject the stale proposal rather than applying it.
- Exhaustive authoritative claims are never truncated by embedding top K; authority overflow or provider lookup failure fails closed.
- `merged + deferred_unvalidated + rejected + no_candidate == scanned`, with one decision-log row per scanned candidate.
- Table-driven tests cover every row of the candidate-level decision table, including the exact distinction between `deferred_unvalidated` and `no_candidate`.
- SQL tests cover evidence lookup, the new release/authority columns, placeholder backfill, uniqueness, the non-blank mapped external-ID CHECK, both source/release foreign keys, and guarded Down behavior.

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
- No standards-glossary or Wikidata production importer in this slice; the live proof uses the initial triple-backed provider with a versioned fixture.
- No provider-specific automatic-merge policies. New adapters may normalize new storage schemas and relation types, but only the core's fixed authoritative `exact_equivalent` rule can grant Tier-6 write authority.
- No quantity/unit or context validator until their governed evidence is available.
- No change to online resolution tiers or `aligns_to_term` semantics.

## 9. Documentation impact

Implementation must update the keyword spec's §13.1 claim that embedding similarity may itself auto-accept, record the new validation contract in §13 R6 and §21, and close or narrow the I2 reconciliation gap. The master handoff update remains pending. `KnowledgeStore` edits must be made only after its pre-existing working-copy changes are resolved.

## 10. Background

This section explains the discussions that led to the design. Sections 2–8 are normative if explanatory wording here ever drifts from them.

| Tier | Current code | Revised behavior |
|---|---|---|
| Tier 5 | Fuzzy spelling matching | Still deterministic fuzzy spelling matching, with stronger transactional checks |
| Tier 6 | Embedding similarity can directly authorize a merge at cosine ≥ `0.90` | Embeddings only find/rank candidates; exact authoritative identity evidence authorizes the merge |

### 10.1 Tier 5: Spelling-Error Reconciliation

Tier 5 handles terms that are probably the same because their normalized spellings are close.

Examples:

- `kubernets` → `Kubernetes`
- `luminence` → `luminance`

#### 10.1.1 Online Resolver

When resolving a newly encountered keyword, Tier 5 currently:

- Runs only after Tiers 0–4 fail.
- Uses trigram similarity to shortlist stored surfaces.
- Uses normalized edit distance to recognize spelling mistakes.
- Rejects digit changes and negation changes.
- Disables fuzzy matching for inputs of four characters or fewer.
- Auto-accepts one unambiguous candidate with score ≥ `0.8`.

This is implemented in [keywordfamily.go](/Users/cding/Workspace/ChenWeb/server/api/ontology/keywords/keywordfamily.go:285).

#### 10.1.2 Offline Reconciler: partially implemented now

The current offline reconciler also uses the Tier-5 edit-distance function, but it has
weaknesses that the redesign corrects:

- It can consider provisional targets, not just active targets.
- It does not properly model a tied top result.
- Its proposal is not recomputed inside the merge transaction.
- Its merge direction can theoretically preserve the provisional concept and absorb the established concept.
- The online “four characters or fewer” exclusion is not explicitly applied by the offline path.

#### 10.1.3 Revised Design

The redesign keeps the existing spelling rules but makes offline Tier 5 deterministic and safe:

- Only active targets in the same scope are eligible.
- There must be exactly one top-scoring valid target.
- A tie cannot authorize a merge.
- Eligibility and scores are recomputed inside the protected transaction.
- External identity evidence must not contradict the Tier-5 target.
- The D11 provisional concept is always absorbed into the active target.

### 10.2 Tier 6: Cross-Language or Semantic Reconciliation

Tier 6 handles cases spelling cannot detect.

Examples:

- `亮度` → `Luminance`
- Potentially an abbreviation or terminology variant with no lexical resemblance

The current implementation:

1. Embeds the provisional concept and existing concepts.
2. Calculates cosine similarity.
3. Treats cosine ≥ `0.90` as sufficient merge evidence.
4. Lets a higher-scoring Tier-6 result replace the Tier-5 proposal.
5. Automatically merges through the normal merge guardrails.

Relevant code: [reconcile.go](/Users/cding/Workspace/ChenWeb/server/api/ontology/keywords/reconcile.go:24).

This current rule is the problem: `亮度`/`Luminance` scored only `0.6711` with Qwen, while some incorrect English pairs scored higher. A universal cosine threshold therefore cannot provide model-independent identity proof.

Under the revised model-agnostic design:

1. Embeddings produce only a bounded, ranked candidate list.
2. Embedding score, model name, and rank are recorded for audit but never authorize a merge.
3. The reconciler loads normalized claims from all enabled identity-evidence providers independently of embedding rank. The first provider adapts the triple-backed external-identity tables.
4. Outcomes are deterministic:

   - Exactly one authoritative active target → validate and potentially merge.
   - No authoritative target but some semantic/evidence proposal → defer.
   - Multiple conflicting authoritative targets → reject.
   - No Tier-5 target and no evidence or embedding proposal → no candidate.

5. Scope, digit, surface-lock, `never_merge`, and alignment conflicts can still veto the merge.
6. Validation, merge, alignment movement, and audit logging occur atomically in one serialized transaction.

The binding provider and decision contracts are in §§4–5 above.

### 10.3 Where External-Identity Mappings Come From

They are stored database records. The reconciler will not search the Internet during reconciliation.

The intended lifecycle is:

1. A governed catalog or terminology source is imported into the system.
2. The source and its exact release are registered, including provenance and license.
3. The importer maps external identifiers to established keyword concepts.
4. Surface evidence records which external identifier supports a particular surface.
5. A curator or governed import configuration explicitly marks that source release as an identity authority.

The relevant tables are:

- `keyword_concepts`
- `keyword_sources`: registered source and release
- `keyword_external_ids`: external identifier → established concept
- `keyword_surface_evidence`: surface → external identifier evidence

For example:

```text
Source:       example-medical-catalog
Release:      2026.1
External ID:  METRIC-0042
Authority:    true

"亮度" surface evidence
    └── (example-medical-catalog, METRIC-0042, 2026.1)

"Luminance" established concept
    └── (example-medical-catalog, METRIC-0042, 2026.1)
```

Because the complete triples match, the catalog asserts that both labels have the same identity.

The data could originate from:

- A standards or terminology catalog imported by a system job.
- An internal governed catalog maintained by the organization.
- A human curator entering or approving mappings through a future administration workflow.

But ordinary user keywords, Internet search results, or LLM output do **not** automatically become authoritative.

One important current limitation: the schema foundations exist, but the revised design does not yet build the catalog importer or curator UI. Selecting and bootstrapping authoritative sources remains a separate required step.

### 10.4 Current Status of External Resources

No external resource is currently imported. No importer or application code currently inserts into the three external-evidence tables. Therefore, the situation is:

- The generic tables exist.
- No terminology source has been selected or imported.
- No source release is currently authoritative.
- The proposed `identity_authority` column is part of the redesign and does not exist yet.
- Without catalog bootstrap, redesigned Tier 6 would safely **defer every embedding-only match**; it could not auto-merge one.

Selecting and bootstrapping authoritative sources remains a separate required step.

The current schema is in [20260805000003_create_kb_keyword_occurrences_and_evidence_shapes.sql](/Users/cding/Workspace/ChenWeb/project_migrations/20260805000003_create_kb_keyword_occurrences_and_evidence_shapes.sql:44).

For Tier 6 to be operational, we still need to decide:

- Which catalogs are suitable for the system’s domains.
- Which releases are authoritative.
- How they are imported and updated.
- Whether authority is assigned automatically by configuration or approved by a curator.
- How internal concepts are initially created and mapped.

### 10.5 How the Reconciler Works

- Normalized authoritative `exact_equivalent` claims provide positive authorization. The initial triple-backed provider produces these from exact external mappings.
- Cosine values: used to find and rank proposals, but not to authorize or select the authoritative target.
- Other information: used primarily as safety vetoes.

The initial `TripleEvidenceProvider` performs this exact join and returns normalized claims to the reconciler:

```text
provisional candidate's surfaces
    ↓
surface evidence: (source, external_id, release)
    ↓ exact triple match
registered source release where identity_authority = true
    ↓ exact triple match
external-ID mapping
    ↓
active established concept in the same scope
```

The reconciler combines these normalized claims with every other enabled provider's exhaustive result, deduplicates the authoritative active target concept IDs, and counts them:

| Distinct authoritative active targets | Result |
|---:|---|
| 0 | Tier 6 cannot merge; defer if there is a plausible proposal |
| 1 | Validate that target |
| More than 1 | Reject because authoritative evidence conflicts |

Cosine does not participate in this count.

For example:

```text
Embedding ranking:
1. Contrast Ratio     cosine 0.81
2. Luminance          cosine 0.67
3. Display Intensity  cosine 0.63

Exact authoritative mapping:
亮度 → external ID METRIC-0042
METRIC-0042 → Luminance
```

The result is `Luminance`, even though it ranked second by cosine. The embedding suggested
where to look; the exact identity mapping made the decision.

#### Embedding candidate bound

Current code does not select an embedding top N. It compares against every live concept and retains only the single best result above the fixed `0.90` threshold. The existing compile-time `reconcileTopK=10` is used by the lexical SQL shortlist, not the embedding loop.

The revised design introduces `reconcileEmbeddingTopK=10` as a new compile-time semantic proposal bound; it is initially not runtime-configurable. Identity-evidence providers are queried independently and exhaustively: an authoritative target outside the embedding top 10—or absent from embedding results entirely—is still added to the candidate union and can be selected. The normalized authoritative claim, not cosine or rank, authorizes Tier 6.

Before merging, the reconciler still checks:

- Target is active.
- Candidate and target scopes agree.
- Digits across their surfaces do not conflict.
- No relevant surface is locked.
- No `never_merge` rule exists.
- Existing governed-term alignments do not conflict.
- Provider authority or backing evidence has not changed during the transaction.

Any failed check rejects the merge.

Tier 5 has precedence:

- A unique valid Tier-5 spelling target can authorize without external identity.
- If authoritative exact-equivalence claims point to the same target, they are consistent.
- If an authoritative claim points elsewhere, the candidate is rejected.
- If Tier 5 is tied or has no result, exactly one authoritative active target across all enabled providers can resolve the candidate.

The complete decision table is in §5.2.

### 10.6 Keyword Concepts

The internal keyword concept itself is stored in:

```text
kb.keyword_concepts
```

Example:

```text
concept_id:   kwc_luminance
pref_label:   Luminance
scope:        display_metrics
status:       active
```

Its schema is defined in [20260803000001_create_kb_keyword_concepts.sql](/Users/cding/Workspace/ChenWeb/project_migrations/20260803000001_create_kb_keyword_concepts.sql:7).

`kb.keyword_external_ids` does not store the concept itself. It is a mapping table connecting an external catalog identity to the internal concept:

```text
source                  example-medical-catalog
external_id             METRIC-0042
release                 2026.1
concept_id              kwc_luminance
```

The relationship is:

```text
kb.keyword_concepts
        1
        │
        │ concept_id
        │
        *
kb.keyword_external_ids
```

One internal concept can have:

- No external IDs.
- One external ID.
- Multiple IDs from different sources.
- Multiple IDs from different releases of a source.

Labels and aliases are stored separately in `kb.keyword_surfaces`, while `kb.keyword_surface_evidence` connects a surface to the catalog identity supporting it.

### 10.7 External Authoritative Entity Identification

For the initial triple-backed provider, an external authoritative entity identifier has the shape:

```text
(source, external_id, release)
```

For example:

```text
(example-medical-catalog, METRIC-0042, 2026.1)
```

The tuple shape is shared, but its values and identifier format are specific to each source.

For example, the same internal concept could theoretically have:

```text
(standard-A, TERM-0042, 2026.1) → kwc_luminance
(ontology-B,  Q123456,  2025-11) → kwc_luminance
(company-C,   MET-77,   v4)      → kwc_luminance
```

These are three different external identities mapped to one internal concept.

The tuple is not the keyword concept’s primary identifier. The internal identity remains:

```text
kb.keyword_concepts.concept_id
```

For example:

```text
kwc_luminance
```

**How Triples Are Stored**

```text
kb.keyword_surface_evidence
(surface_id, source, external_id, release)
                         │
                         │ exact triple match
                         ▼
kb.keyword_external_ids
(source, external_id, release, concept_id)
```

`kb.keyword_concepts` stores only the internal concept. The shared triple connects evidence about an observed surface to that concept indirectly.

The external tuple means:

> “According to this particular release of this particular source, this external identifier corresponds to the internal concept.”

`external_id` cannot safely be interpreted without `source` and `release`. `METRIC-0042` in one catalog may mean something entirely different in another.

Consequently, not every keyword concept must have an external tuple. A concept without a suitable authoritative claim from any enabled provider cannot be the basis of a Tier-6 automatic merge. It can still be resolved by Tier 0–5, manually curated, or left deferred.

### 10.8 `kb.keyword_external_ids`

`kb.keyword_external_ids` acts as a translation layer between source-specific identities
and SemOS’s internal keyword concepts:

```text
External identity                                      Internal identity
(QUDT, quantitykind:Luminance, 3.1.0)              ┐
(another-standard, METRIC-0042, 2026.1)            ├→ kwc_luminance
(company-catalog, DISPLAY-LUMINANCE, v4)           ┘
```

The external IDs do not need to have the same format. Each importer preserves the source-native identifier as an opaque string and maps it to the appropriate `kb.keyword_concepts.concept_id`.

The table does not itself “massage” IDs. The importer or curator establishes the mappings;
the table stores them and gives the reconciler a uniform way to query them.

The triple is generic enough when a resource provides:

- A recognizable source namespace
- A stable entry identifier
- A release/version identifier, or `''` when genuinely unversioned
- An assertion that the entry represents exact identity

Resources that provide only similarity, related-term, broader/narrower, or uncertain translation evidence can still be imported, but those relationships should not be treated as exact identity or independently authorize a merge.

The triple-backed schema remains the first supported evidence representation. The reconciler nevertheless consumes it through the generic provider contract in §4.1, so future source schemas can be adapted without changing the deterministic validator.

### 10.9 Blockers

#### 10.9.1 Blocker 01

The practical gap is therefore real: **before Tier 6 can provide useful automatic cross-lingual reconciliation, an authoritative catalog strategy and bootstrap process must be selected and built.**

#### 10.9.2 Evidence-provider extensibility decision

This is a design decision, not a current operational blocker.

The current schema can support resources with stable, versioned identifiers, but not necessarily resources that provide:

- Unstable or unversioned identifiers that lack a durable source-scoped identity; stable genuinely unversioned identifiers remain supported with `release=''`
- Synonym sets rather than IDs
- Cross-references between different vocabularies
- Broader/narrower relationships
- Translation pairs
- Probabilistic mappings
- Context-dependent identity
- APIs whose native evidence cannot sensibly become one exact triple

Therefore, the triple join is the initial storage adapter rather than the Tier-6 core contract.

The safer architecture is:

```text
External resource/import format
        ↓
resource-specific evidence adapter
        ↓
generic normalized identity claims
        ↓
Tier-6 deterministic validator
```

The existing triple tables become the first adapter:

```text
TripleEvidenceProvider
    reads keyword_sources
          keyword_external_ids
          keyword_surface_evidence
    returns normalized claims
```

Future providers can use different tables and schemas:

```text
TranslationEvidenceProvider
StandardsCrosswalkProvider
SynonymSetProvider
CuratorAssertionProvider
```

The reconciler does not know provider storage layouts. It consumes a generic result such as:

```go
type IdentityClaim struct {
    ProviderID         string
    CandidateConceptID string
    TargetConceptID    string
    Relation           IdentityRelation
    Authority          IdentityAuthority
    AuthorityRef       string
    EvidenceRefs       []EvidenceRef
}
```

Cosine ranking remains a separate proposal source, with no merge authority.

The alternatives considered for normalized merge authority were:

- **Option A — exact-equivalence only (recommended):** Providers may return many relation types, but only an authoritative `exact_equivalent` claim can authorize Tier 6. Related, broader, translation-only, or probabilistic claims cause defer/review.
- **Option B — policy-driven claims:** Each provider can define which relations and confidence levels authorize. More flexible, but introduces source-specific decision rules and weakens the fixed model-agnostic contract.
- **Option C — evidence aggregation:** Combine several weaker claims into an automatic decision. Potentially useful later, but requires calibration before we know the available resources.

**Decision: Option A.** Providers may normalize arbitrary evidence and relation types, but only an authoritative `exact_equivalent` claim may authorize Tier 6. The core validator owns this rule; adapters cannot opt into source-specific confidence thresholds or merge policies. Non-exact or non-authoritative claims remain useful for audit and review prioritization, but produce `deferred_unvalidated` when no stronger path decides the candidate.
