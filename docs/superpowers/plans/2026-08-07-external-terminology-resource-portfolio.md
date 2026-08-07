# External Terminology Portfolio and Model-Agnostic Tier 6

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the approved Option 3 Stage-0/1 tooling: register governed, release-pinned sources; import Stage-1 local snapshots; normalize their evidence behind a source-agnostic provider contract; and replace cosine-authorized Tier-6 merges with deterministic, transaction-safe identity validation. Production portfolio activation is a separate operator bootstrap that requires real approved artifacts, reviewed mappings, corpus coverage criteria, and a published seed release.

**Architecture:** External jobs import immutable local files into a structured source registry and adapter-owned catalog tables. Promotion writes the generic `(source, external_id, release)` mappings already used by keyword concepts and surfaces. Reconciliation never calls the Internet and never parses a source-native identifier: enabled providers normalize local evidence into deterministic claims, while embeddings only rank diagnostic proposals. Each candidate is decided under one PostgreSQL transaction and family advisory lock; exhaustive evidence reread, veto checks, fixed provisional-to-active merge direction, alignment movement, and audit logging either commit together or roll back together.

**Tech Stack:** Go 1.25, PostgreSQL/goose, `rdf2go` for Turtle, `encoding/xml` for UCUM, streaming `encoding/json` for curated/Wikidata snapshots, `go-sqlmock` plus live PostgreSQL integration tests.

## Global constraints

- Governing designs:
  - `docs/superpowers/specs/2026-08-07-model-agnostic-tier6-validation-design.md`
  - `docs/superpowers/specs/2026-08-07-external-terminology-resource-portfolio-design.md`
- All source payloads are local files. Import commands may not fetch live URLs.
- The core reconciler treats external identifiers as opaque strings and remains model/provider agnostic.
- Only an enabled, approved source policy with an authoritative `exact_equivalent` relation can grant Tier-6 write authority.
- QUDT, SIRP, IEV, Wikidata, and UCUM parsing stays outside the reconciler package.
- Licensed IEC content is represented by a reviewed, minimal seed file; no web scraping or copied definitions.
- Every schema change is a new project-track goose migration in `project_migrations/`; never modify an applied migration.
- Follow red-green-refactor for every behavior change. Commit each completed task with `jj commit`, then verify a linear `jj log`.
- A source file plus its manifest is reproducible: release/revision, retrieval time, SHA-256, license, adapter version, and authority policy are persisted before content can influence reconciliation.
- A registered `(source, release)` and each associated artifact are immutable. Exact replay is idempotent; any changed checksum, policy, scope, rights, approval, or payload fails and requires a new release/snapshot identity.

## Task 0: Corpus inventory and bootstrap acceptance baseline

**Files:**
- Create: `server/api/ontology/terminology/coverage.go`
- Create: `server/api/ontology/terminology/coverage_test.go`
- Create: `server/cmd/terminology-coverage/main.go`
- Create: `server/cmd/terminology-coverage/main_test.go`

**Interfaces:**
- Produce a deterministic scope/frequency inventory of active and provisional keyword concepts/surfaces from the selected pilot corpus, without making identity decisions.
- Compare an imported/reviewed seed release with that inventory and report exact-authority coverage, unresolved bilingual pairs, ambiguous/context-sensitive surfaces, and high-frequency uncovered concepts.
- Read an operator-authored acceptance file containing the selected corpus/scope, target coverage, risk terms, approver, and target seed release; tooling reports readiness but cannot approve itself.

- [x] Write failing tests for stable frequency ordering, scope isolation, unresolved-pair backlog generation, context-sensitive exclusions, and target pass/fail calculations.
- [x] Implement read-only inventory and coverage calculation.
- [x] Implement CLI JSON/summary output suitable for storing with a seed-release review.
- [x] Run coverage package/command tests.
- [x] Commit with `jj commit -m "ontology: measure terminology seed coverage"`.

## Task 1: Governed source registry and generic catalog schema

**Files:**
- Create: `project_migrations/20260807000001_govern_keyword_identity_sources.sql`
- Create: `server/api/ontology/keywords/identity_migration_test.go`
- Create: `server/api/ontology/keywords/source_policy.go`
- Create: `server/api/ontology/keywords/source_policy_test.go`

**Interfaces:**
- Extend `kb.keyword_sources` with structured governance fields: provider/subset/checksum/license review/authority role/authoritative relations/scopes/languages/adapter version/provenance/approval and `identity_authority`.
- Add `release` to `kb.keyword_surface_evidence`; enforce the full evidence triple uniqueness and both source-release foreign keys.
- Add fail-closed nonblank constraints and legacy placeholder backfill exactly as specified by Tier-6 design §6.
- Add immutable adapter-owned staging tables for catalog entries, labels, relations, negative decisions, artifact manifests, and UCUM unit codes. Staging rows retain native payload provenance and never themselves authorize a merge.
- Add a separate lock-protected, audited deployment/enablement pointer (and append-only deployment history) so activation or rollback never mutates the immutable source-release policy or its artifacts.
- `SourcePolicyStore.Register` validates closed enums, authoritative relation policy, checksum shape, and approval requirements before insert-or-verify-identical registration; it never updates a historical source release in place.

- [x] Write migration contract tests first: assert new columns, named constraints/FKs, placeholder backfill, guarded Down order, and staging-table authority separation.
- [x] Run the focused tests and record the expected red result.
- [x] Add the goose migration with a reversible, guarded Down section.
- [x] Write store tests for rejected incomplete authority policies, exact-replay idempotence, and rejection of any attempt to mutate historical source-release or artifact metadata.
- [x] Implement the smallest source-policy model/store that passes them.
- [x] Run keyword-package tests and migration contract tests.
- [x] Commit with `jj commit -m "keywords: add governed terminology source registry"`.

## Task 2: Source-agnostic identity evidence contract

**Files:**
- Create: `server/api/ontology/keywords/identity_evidence.go`
- Create: `server/api/ontology/keywords/identity_evidence_test.go`
- Create: `server/api/ontology/keywords/triple_evidence_provider.go`
- Create: `server/api/ontology/keywords/triple_evidence_provider_test.go`

**Interfaces:**
- `IdentityEvidenceProvider` exposes a stable nonblank provider ID and exhaustive candidate claims against a transactionally stable `DBX`.
- `IdentityClaim` uses closed relation and authority enums and canonical evidence references.
- Canonicalization sorts providers/claims/references, rejects malformed or colliding claims, deduplicates exact claims while unioning references, and produces byte-identical audit JSON.
- `TripleEvidenceProvider` reads candidate surfaces → evidence triple → registered source policy → optional external mapping → optional target independently of embedding rank. It retains targetless claims, missing mappings, and inactive/provisional targets as non-authorizing evidence; it does not erase them with an inner join or active-target filter.
- Authorizing provider calls run inside the candidate transaction and lock/stabilize every source-policy/deployment, surface-evidence, and external-mapping row they read after the family advisory lock. Providers may read mapped-concept status to normalize claims but do not row-lock every possible target; Task 4 locks only the chosen target after targetless/conflict outcomes are selected.
- Evidence-reference keys use unpadded base64url components and remain reversible for Unicode, delimiters, and empty releases.

- [x] Add table-driven failing tests for every valid/invalid relation-authority combination, duplicate providers, malformed claims, ordering, deduplication, and canonical JSON.
- [x] Add failing SQL-provider tests for matching and mismatched releases, non-authoritative sources, blank legacy IDs, missing mappings, inactive/provisional mappings, corrupt/unregistered source rows, multiple triples to one target, conflicting targets, provider-only targets, and the required backing-row locks.
- [x] Implement the generic contract and normalizer without source-specific branches.
- [x] Implement `TripleEvidenceProvider` using the full triple and policy relation.
- [x] Run the focused tests twice with shuffled inputs to prove determinism.
- [x] Commit with `jj commit -m "keywords: normalize external identity evidence"`.

## Task 3: Transaction-scoped merge and audit primitives

**Files:**
- Modify: `server/api/ontology/keywords/concepts_store.go`
- Modify: `server/api/ontology/keywords/concepts_store_test.go`
- Modify: `server/api/ontology/semid/stores.go`
- Modify: `server/api/ontology/semid/stores_test.go`
- Modify: `server/api/ontology/keywords/alignment.go`
- Create: `server/api/ontology/keywords/reconcile_lock_test.go`

**Interfaces:**
- Expose transaction-scoped guard/apply methods reused by both `MergeConcept` and the reconciler; do not duplicate merge SQL.
- `DecisionLogStore` accepts a package-local minimal database interface satisfied by both `*sql.DB` and `*sql.Tx`, so audit rows can be part of the caller transaction without introducing a `semid` → `keywords` dependency.
- Every mutation that can invalidate a reconciliation decision uses the keyword-family advisory transaction lock: `SELECT pg_advisory_xact_lock(1264011588, 1);`.
- Automatic merge always absorbs the scanned `status='provisional', gloss_source='auto:d11'` concept into an active target.

- [x] Add failing tests proving audit rollback with merge rollback and fixed merge direction.
- [x] Add failing lock-order tests for merge, evidence/source/mapping mutations, `never_merge`, alignment, concept-label/status, and surface mutations used by reconciliation.
- [x] Refactor merge/audit stores to accept a caller-owned transaction while preserving existing public behavior.
- [x] Add the lock helper and wire every relevant mutation path before its decision-sensitive write.
- [x] Run concept/alignment/semid tests.
- [x] Commit with `jj commit -m "keywords: make reconciliation writes transaction atomic"`.

## Task 4: Replace cosine authority with deterministic Tier-6 validation

**Files:**
- Rewrite: `server/api/ontology/keywords/reconcile.go`
- Rewrite/add: `server/api/ontology/keywords/reconcile_test.go`
- Modify: `server/cmd/keyword-reconcile/main.go`
- Add/modify: `server/cmd/keyword-reconcile/main_test.go`

**Interfaces:**
- Stats are `Scanned`, `Decided`, `Failed`, `Merged`, `DeferredUnvalidated`, `Rejected`, and `NoCandidate`, with the design invariants enforced.
- Tier 5 recomputes only active, same-scope candidates inside the protected transaction, applies the four-character rule and deterministic `1e-9` tie handling, and never reads embeddings. A unique valid Tier-5 target may proceed with zero external authority; authority to a different target or multiple authoritative targets rejects it.
- Embeddings contribute a bounded positions-1-through-10 proposal list and audit diagnostics only. They cannot choose or authorize a target.
- Providers are exhaustive and fail closed. When Tier 5 is absent/tied, exactly one authoritative active target may authorize, zero defers/no-candidate, and multiple targets reject; a unique valid Tier-5 target follows the binding Tier-5 rule above.
- Each decided candidate writes exactly one canonical decision row inside its transaction, including outcome, authority, claims, vetoes, embedding model/rank/score diagnostics, and candidate ID.
- CLI wires required alignments and triple provider, requires a model ID when embeddings are configured, and prints partial counters on failure.

- [x] Replace old threshold-oriented tests with the complete candidate decision table before changing implementation.
- [x] Add tests for high cosine/no identity defer, low cosine/exact identity merge, affine-rescaled embeddings, top-10 boundary, provider-only target, unique Tier-5/no-authority merge, Tier-5/contradictory-authority reject, all non-authorizing relation types, conflicting authority, ties, fixed direction, veto dominance, and partial failure counters.
- [x] Run tests to capture the expected failures against the old reconciler.
- [x] Implement proposal assembly separately from transaction-time decision validation.
- [x] Inside each candidate transaction follow the binding order: acquire the family lock; lock/recheck candidate and all candidate surfaces; recompute Tier 5; load exhaustive provider claims while providers lock/stabilize all backing rows; decide and audit targetless outcomes before target locking; for validate branches lock/recheck target and surfaces; revalidate method authority and every veto; merge if authorized; append audit; commit.
- [x] Delete the `tier6EmbeddingMinScore` authority rule and old direction election.
- [x] Wire the command and update summary output.
- [x] Run keyword reconciler package/command tests.
- [x] Commit with `jj commit -m "keywords: make tier 6 identity-authorized and model agnostic"`.

## Task 5: Deterministic adapter library and import runner

**Files:**
- Create: `server/api/ontology/terminology/manifest.go`
- Create: `server/api/ontology/terminology/manifest_test.go`
- Create: `server/api/ontology/terminology/importer.go`
- Create: `server/api/ontology/terminology/importer_test.go`
- Create: `server/cmd/terminology-import/main.go`
- Create: `server/cmd/terminology-import/main_test.go`
- Create: `server/api/ontology/terminology/testdata/*`

**Interfaces:**
- A strict JSON manifest describes source policy plus one or more local artifacts and their expected SHA-256 checksums.
- `Adapter` is a plug-in interface returning normalized catalog entries/labels/relations/unit codes; it cannot write keyword concepts or mark itself authoritative.
- The runner verifies files before opening a transaction, registers policy, inserts or verifies the immutable exact source-release staging snapshot, and reports stable counts. Changed content under an existing release is an error, never a replacement.
- A release-diff operation reports additions, retirements, replacements, label/relation changes, artifact/checksum changes, and policy/license changes before a new release may be approved. Activation appends an audited deployment event and advances a separate enabled-release pointer. Rollback appends a compensating event and advances that pointer to the prior immutable release; source-release policy, artifacts, historical deployment events, and audit evidence are never rewritten or deleted.
- Promotion from staging to identity triples is a separate explicit reviewed mapping input, never an inference from labels.

- [x] Add failing manifest tests for path confinement, checksum mismatch, missing release/revision, invalid authority combinations, duplicate artifacts, and deterministic JSON.
- [x] Add failing runner tests for all-or-nothing first import, identical replay, changed-payload/reused-release rejection, and rollback on adapter failure.
- [x] Implement strict manifest parsing/checksum validation and adapter registry.
- [x] Implement transaction-scoped immutable catalog import/verification and the local-file-only CLI.
- [x] Add release-diff tests/output and tested lock-protected activation/rollback operations over the separate deployment pointer, including audit history and rejection when the selected release is absent or unapproved.
- [x] Add fixture manifests with synthetic data only.
- [x] Run terminology package/command tests.
- [x] Commit with `jj commit -m "ontology: add governed terminology import runner"`.

## Task 6: Stage-1 SIRP, IEC seed, Wikidata, and UCUM adapters

**Files:**
- Create: `server/api/ontology/terminology/sirp.go` and `_test.go`
- Create: `server/api/ontology/terminology/iec_seed.go` and `_test.go`
- Create: `server/api/ontology/terminology/wikidata.go` and `_test.go`
- Create: `server/api/ontology/terminology/ucum.go` and `_test.go`
- Add: `server/api/ontology/terminology/testdata/{sirp,iec,wikidata,ucum}/*`

**Interfaces:**
- SIRP reads local Turtle, preserves persistent IDs and language tags, and emits exact native quantity identity only under manifest policy.
- IEC reads a minimal reviewed JSON seed containing local EN/ZH surfaces, stable IEV references, scope/constraints, relation decision, reviewer, and timestamp; it rejects copied definitions and unreviewed exact decisions.
- Wikidata streams revision-pinned JSONL containing Q-ID/revision, labels/aliases, external IDs, different-from/broader/narrower/unit statements; all output is proposal/context only.
- UCUM reads official essence XML into the unit-code staging registry and emits no keyword identity claims.

- [ ] Write conformance fixtures first, including malformed, deprecated/replaced, multilingual, relation-distinction, and duplicate cases.
- [ ] Add shared adapter contract tests proving deterministic ordering, idempotent keys, provenance retention, and fail-closed unknown relations.
- [ ] Implement each adapter behind the same interface; keep source-specific parsing out of the keyword package.
- [ ] Add a luminance/brightness fixture proving distinct IEV identities and exact EN/ZH luminance mapping.
- [ ] Run all terminology tests and the import CLI against every fixture twice.
- [ ] Commit with `jj commit -m "ontology: import stage 1 terminology snapshots"`.

## Task 7: Extend QUDT and connect reviewed promotion

**Files:**
- Refactor: `server/cmd/qudt-import/main.go`
- Create/modify: `server/cmd/qudt-import/main_test.go`
- Create: `server/api/ontology/terminology/qudt.go` and `_test.go`
- Create: `server/api/ontology/keywords/identity_import.go` and `_test.go`

**Interfaces:**
- QUDT preserves all approved language-tagged labels and alternatives, source IRIs, release, deprecation/replacement, and exact/broader/narrower/related/Wikidata relation distinctions.
- Configured QUDT `siExactMatch`/approved exact-match predicates normalize explicit SIRP persistent-identifier crosswalks; generic `wikidataMatch` remains proposal-only and cannot authorize.
- Existing governed quantity import behavior remains compatible, but parsing is shared with the terminology adapter rather than duplicated.
- A reviewed promotion file maps native catalog identities/local surfaces to established keyword concepts and records full evidence triples. It validates concept/scope/status, source policy, required unit/dimension constraints, existing alignments, `never_merge`, and authoritative negative evidence; it is idempotent and acquires the family lock.
- A separate provenance-preserving negative promotion records a reviewed scoped non-equivalence as a `never_merge` veto linked to its source triples, reviewer, and decision timestamp. Non-equivalence never becomes a lowered score.
- Proposal-only, context-only, or non-exact relations cannot be promoted as authoritative evidence.

- [ ] Add failing QUDT tests demonstrating preservation of `en`, `zh`, `fr`, and `und`, exact versus `wikidataMatch`, and deprecation/replacement.
- [ ] Add a SIRP/QUDT fixture proving QUDT Luminance → SIRP LUMA normalization through an explicit exact crosswalk, and proving the adjacent Wikidata mapping stays non-authoritative.
- [ ] Add failing promotion tests for unknown concepts, provisional targets, scope mismatch, non-authoritative policy, relation mismatch, conflicting unit/dimension, conflicting alignment, authoritative non-equivalence, and full-triple idempotence.
- [ ] Add failing negative-promotion tests for incomplete provenance/scope, proposal-only distinctions, and a conflict where positive identity exists but the authoritative `never_merge` veto wins.
- [ ] Extract/implement the QUDT adapter and keep the existing command functional.
- [ ] Implement reviewed positive promotion into `keyword_external_ids`/`keyword_surface_evidence` and negative promotion into the provenance-linked `never_merge` path.
- [ ] Run QUDT, terminology, and keyword tests.
- [ ] Commit with `jj commit -m "ontology: preserve QUDT semantics and promote reviewed identities"`.

## Task 8: Live acceptance, documentation, and final review

**Files:**
- Create: `server/api/ontology/keywords/reconcile_identity_integration_test.go`
- Modify: keyword spec/ADR documentation in the owning repository only if its worktree is clean
- Modify: `docs/superpowers/specs/2026-08-07-model-agnostic-tier6-validation-design.md`
- Modify: `docs/superpowers/specs/2026-08-07-external-terminology-resource-portfolio-design.md`

- [ ] Rebuild/apply migrations to `chenweb_test` and verify Up plus guarded Down/Up.
- [ ] Import the versioned bilingual fixture and reviewed mapping twice.
- [ ] Prove `亮度` merges into active `Luminance` by exact identity regardless of cosine, while reviewed luminance/brightness non-equivalence is promoted and vetoes a deliberately conflicting positive proposal.
- [ ] Prove only a reviewed mapping to IEV `845-22-059` can converge `brightness`/`视亮度`; bare brightness surfaces remain unpromoted and context-sensitive.
- [ ] Prove high cosine without identity defers, conflicting identities reject, and every decided candidate has one audit row.
- [ ] Run concurrency tests for evidence retraction, conflicting mapping insertion, `never_merge`, alignment, concept/surface mutation, and family-lock serialization.
- [ ] Run `go test ./server/api/ontology/keywords/... ./server/api/ontology/terminology/... ./server/cmd/keyword-reconcile/... ./server/cmd/terminology-import/... ./server/cmd/qudt-import/...`.
- [ ] Run `go test ./...` and `go vet ./...`; investigate every failure.
- [ ] Run the coverage command against the selected pilot scope, store the unresolved-pair backlog and pass/fail report, and require operator approval before calling a production seed release ready.
- [ ] Update implementation status, operational commands, source-manifest/diff/rollback format, and the five documentation-impact questions required by workspace policy.
- [ ] Synchronize the governing KnowledgeStore keyword spec (§13.1, R6, §21, and I2). If that separate repository has unrelated changes, stop and have the user resolve them; do not declare completion while the governing spec is stale.
- [ ] Obtain independent spec-compliance review, then code-quality review; fix and re-run verification.
- [ ] Commit documentation/test closure, verify a single linear `jj log`, and report actual resource files intentionally not distributed with the repository.

## Completion boundary

This plan implements the validator plus Stage-0/1 coverage, import, governance, promotion, negative-evidence, release-diff, and rollback tooling. It does not claim that the production portfolio is bootstrapped: it does not download or redistribute external datasets, scrape IEC/CIE/ISO pages, create a curator UI, choose the operator's coverage target, publish a production seed release, or implement Stage-2/3 licensed/clinical adapters. Production activation requires operators to supply approved local SIRP/QUDT/Wikidata/UCUM artifacts and the reviewed IEC seed/promotion files, run the corpus/coverage report, approve the acceptance criteria, and publish a versioned seed release before enabling Tier 6.
