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

- [x] Write conformance fixtures first, including malformed, deprecated/replaced, multilingual, relation-distinction, and duplicate cases.
- [x] Add shared adapter contract tests proving deterministic ordering, idempotent keys, provenance retention, and fail-closed unknown relations.
- [x] Implement each adapter behind the same interface; keep source-specific parsing out of the keyword package.
- [x] Add a luminance/brightness fixture proving distinct IEV identities and exact EN/ZH luminance mapping.
- [x] Run all terminology tests and the import CLI against every fixture twice.
- [x] Commit with `jj commit -m "ontology: import stage 1 terminology snapshots"`.

## Task 7: Extend QUDT and connect reviewed promotion

**Files:**
- Refactor: `server/cmd/qudt-import/main.go`
- Create/modify: `server/cmd/qudt-import/main_test.go`
- Create: `server/api/ontology/terminology/qudt.go` and `_test.go`
- Create: `server/api/ontology/keywords/identity_import.go` and `_test.go`

**Interfaces:**
- QUDT preserves all approved language-tagged labels and alternatives, source IRIs, release, deprecation/replacement, and exact/broader/narrower/related/Wikidata relation distinctions.
- SKOS similarity links such as `skos:closeMatch` are retained as
  non-authoritative `close_match` relations, never upgraded to exact
  equivalence; unknown SKOS/OWL predicates still fail closed.
- Configured QUDT `siExactMatch`/approved exact-match predicates normalize explicit SIRP persistent-identifier crosswalks; generic `wikidataMatch` remains proposal-only and cannot authorize.
- Existing governed quantity import behavior remains compatible, but parsing is shared with the terminology adapter rather than duplicated.
- A reviewed promotion file maps native catalog identities/local surfaces to established keyword concepts and records full evidence triples. It validates concept/scope/status, source policy, required unit/dimension constraints, existing alignments, `never_merge`, and authoritative negative evidence; it is idempotent and acquires the family lock.
- A separate provenance-preserving negative promotion records a reviewed scoped non-equivalence as a `never_merge` veto linked to its source triples, reviewer, and decision timestamp. Non-equivalence never becomes a lowered score.
- Proposal-only, context-only, or non-exact relations cannot be promoted as authoritative evidence.

- [x] Add failing QUDT tests demonstrating preservation of `en`, `zh`, `fr`, and `und`, exact versus `wikidataMatch`, and deprecation/replacement.
- [x] Add a SIRP/QUDT fixture proving QUDT Luminance → SIRP LUMA normalization through an explicit exact crosswalk, and proving the adjacent Wikidata mapping stays non-authoritative.
- [x] Add failing promotion tests for unknown concepts, provisional targets, scope mismatch, non-authoritative policy, relation mismatch, conflicting unit/dimension, conflicting alignment, authoritative non-equivalence, and full-triple idempotence.
- [x] Add failing negative-promotion tests for incomplete provenance/scope, proposal-only distinctions, and a conflict where positive identity exists but the authoritative `never_merge` veto wins.
- [x] Extract/implement the QUDT adapter and keep the existing command functional.
- [x] Implement reviewed positive promotion into `keyword_external_ids`/`keyword_surface_evidence` and negative promotion into the provenance-linked `never_merge` path.
- [x] Run QUDT, terminology, and keyword tests.
- [x] Commit with `jj commit -m "ontology: preserve QUDT semantics and promote reviewed identities"`.

## Task 8: Live acceptance, documentation, and final review

**Files:**
- Create: `server/api/ontology/keywords/reconcile_identity_integration_test.go`
- Modify: keyword spec/ADR documentation in the owning repository only if its worktree is clean
- Modify: `docs/superpowers/specs/2026-08-07-model-agnostic-tier6-validation-design.md`
- Modify: `docs/superpowers/specs/2026-08-07-external-terminology-resource-portfolio-design.md`

- [x] Rebuild/apply migrations to `chenweb_test` and verify Up plus guarded Down/Up.
- [x] Import the versioned bilingual fixture and reviewed mapping twice.
- [x] Prove `亮度` merges into active `Luminance` by exact identity regardless of cosine, while reviewed luminance/brightness non-equivalence is promoted and vetoes a deliberately conflicting positive proposal.
- [x] Prove only a reviewed mapping to IEV `845-22-059` can converge `brightness`/`视亮度`; bare brightness surfaces remain unpromoted and context-sensitive.
- [x] Prove high cosine without identity defers, conflicting identities reject, and every decided candidate has one audit row.
- [x] Run concurrency coverage: lock-order unit tests for every identity
  mutation writer (concept create/update/status, surface create/lock,
  `never_merge`/alignment/external-mapping/evidence writes) plus the live
  family-lock serialization test and the transactional in-transaction reread
  that defeats concurrent evidence retraction and conflicting mapping
  insertion.
- [x] Run `go test ./server/api/ontology/keywords/... ./server/api/ontology/terminology/... ./server/cmd/keyword-reconcile/... ./server/cmd/terminology-import/... ./server/cmd/qudt-import/...`.
- [x] Run `go test ./...` and `go vet ./...`; investigate every failure.
- [x] Run the coverage command against the selected pilot scope, store the unresolved-pair backlog and pass/fail report, and require operator approval before calling a production seed release ready.
- [x] Update implementation status, operational commands, source-manifest/diff/rollback format, and the five documentation-impact questions required by workspace policy.
- [x] Synchronize the governing KnowledgeStore keyword spec (§13.1, R6, §21, and I2). If that separate repository has unrelated changes, stop and have the user resolve them; do not declare completion while the governing spec is stale.
- [x] Obtain independent spec-compliance review, then code-quality review; fix and re-run verification.
- [x] Commit documentation/test closure, verify a single linear `jj log`, and report actual resource files intentionally not distributed with the repository.

## Task 9: Automated resource download and admin page

**Files:**
- Create: `server/api/ontology/terminology/fetch.go` + `fetch_test.go`
- Create: `server/api/terminologyresourcehandler/handler.go` + `handler_test.go`
- Create: `server/cmd/terminology-fetch/main.go` + `main_test.go`
- Create: `web/src/lib/services/terminologyResourceService.ts`
- Create: `web/src/lib/components/home3/external-terminology-resources-view.svelte`
- Modify: `server/api/routes.go`, `web/src/lib/components/home3/nav-rail.svelte`,
  `web/src/lib/components/home3/content-panel.svelte`
- Modify: `docs/superpowers/specs/2026-08-07-external-terminology-resource-portfolio-design.md`

**Interfaces:**
- Downloads stay an explicit bootstrap step separate from import: the fetch
  tool/API writes local artifacts, computes SHA-256, and drafts an unapproved
  manifest (`license_review_status=pending_review`, no `approved_by`/`approved_at`)
  that `terminology-import` rejects until the operator completes the license
  review. Import commands remain offline.
- Freely downloadable sources (no permission): QUDT (`qudt.org/download/3.5.0/qudt-all.ttl`,
  CC-BY-4.0), UCUM (`raw.githubusercontent.com/ucum-org/ucum/v2.2/ucum-essence.xml`,
  UCUM License 1.1), BIPM SIRP (`si-digital-framework.org/quantities`, CC-BY-3.0-IGO),
  Wikidata pilot entity subset via `wbgetentities` pinned by `lastrevid` (CC0).
  IEC 60050-845 (IEV) is copyright-gated: the tool refuses it and the page shows
  "Requires license" instead of a download button.
- The admin page (System Admin > Resources > External Terminology Resources)
  lists every resource as a card with name, source URL, release, license,
  download status, download time, SHA-256, size, artifact/draft-manifest names,
  and a Download / Re-download button.

- [x] Verify live URLs are reachable and record real artifact sizes/releases.
- [x] Implement the shared fetch package with SHA-256, size limits, persisted
  `status.json`, and unapproved draft manifests; refuse permission-gated sources.
- [x] Add `GET /api/v1/terminology-resources` and
  `POST /api/v1/terminology-resources/:source/download`.
- [x] Add the `terminology-fetch` CLI (`list`, `status`, `fetch`).
- [x] Add the admin page under System Admin > Resources and wire the nav.
- [x] Run fetch/handler/CLI tests, `go build ./...`, `go vet ./...`,
  `svelte-check`, and eslint on the new frontend files; fix every new issue.
- [x] Update §13.2 operational commands and the documentation-impact section.
- [x] Commit with `jj commit` and verify a single linear `jj log`.

## Task 10: Review page for downloaded, unreviewed resources

**Files:**
- Create: `web/src/lib/components/home3/review-external-resources-view.svelte`
- Modify: `server/api/ontology/terminology/fetch.go` (+ `fetch_test.go`),
  `server/api/terminologyresourcehandler/handler.go` (+ `handler_test.go`),
  `web/src/lib/services/terminologyResourceService.ts`,
  `web/src/lib/components/home3/nav-rail.svelte`,
  `web/src/lib/components/home3/content-panel.svelte`
- Modify: `docs/superpowers/specs/2026-08-07-external-terminology-resource-portfolio-design.md`

**Interfaces:**
- The resources API now reports the draft manifest's `review_status`
  (`pending_review`, `approved`, `disapproved`, or empty) plus the saved
  review comments, reviewer, and review time per downloaded resource.
- The Review page (System Admin > Resources > Review External Resources) lists
  only downloaded resources whose draft manifest is still `pending_review`,
  in the same card style as the External Terminology Resources page.
- `POST /api/v1/terminology-resources/:source/approve` completes the operator
  license review in place: it writes `license_review_status=approved` plus
  `approved_by` (authenticated user email unless overridden), `approved_at`,
  and the operator's review comments into the draft manifest, then starts the
  offline import of the now-approved local manifest. It fails closed when the
  source is not downloaded, has no draft, or is already decided. Approval is
  persisted even if the import fails so the operator can retry; a release
  already registered with byte-identical content is replayed idempotently
  rather than rejected by the immutable-release guard.
- `POST /api/v1/terminology-resources/:source/disapprove` saves the operator's
  review comments and reviewer, marks the draft `disapproved`, and never
  imports anything. It fails closed on the same preconditions.
- Each pending card has a "Review" button that opens a dialog showing the
  resource details, a multi-line comments field, and Approve / Disapprove /
  Cancel actions. Approve (with confirmation) marks the entry reviewed and
  approved and starts the import; Disapprove (with confirmation) saves the
  comments and marks the resource `disapproved`. A direct "Mark approved"
  button (with confirmation) remains below it.

- [x] Surface `review_status` from the draft manifest in the resources API.
- [x] Add the Review page and filter to downloaded + `pending_review` only.
- [x] Add fetch-package and handler tests for pending/approved/no-draft states.
- [x] Implement `POST .../:source/approve` with fail-closed state checks,
      comment saving, and import-on-approve; add terminology + handler tests.
- [x] Implement `POST .../:source/disapprove` with fail-closed state checks
      and comment saving; add terminology + handler tests.
- [x] Add the "Review" dialog (resource details, comments, Approve /
      Disapprove / Cancel) and keep the "Mark approved" button on the page.
- [x] Run go tests/build/vet, svelte-check, and eslint on the new frontend files.
- [x] Update §13.2 and the documentation-impact section; commit with `jj`.

**Live acceptance:** QUDT 3.5.0 was approved through the admin Review dialog
and imported into the staging database as the first live import (1223
quantity-kind entries, 3177 labels, 1342 relations). The `skos:closeMatch`
predicates in the real `qudt-all.ttl` are retained as non-authoritative
`close_match` relations instead of failing conversion. Re-downloading and
re-approving an already imported release with byte-identical content reports
an idempotent replay rather than an immutable-release error. The Approve /
Disapprove / Cancel dialog flows with comments were verified in the browser.

## Task 11: Download sizes, progress, and weekly Wikidata refresh

**Files:**
- Modify: `server/api/ontology/terminology/fetch.go` (+ `fetch_test.go`),
  `server/api/terminologyresourcehandler/handler.go` (+ `handler_test.go`),
  `server/api/kbhandler/scheduler_jobs.go` (+ `scheduler_jobs_test.go`),
  `web/src/lib/services/terminologyResourceService.ts`,
  `web/src/lib/components/home3/external-terminology-resources-view.svelte`
- Modify: `docs/superpowers/specs/2026-08-07-external-terminology-resource-portfolio-design.md`

**Interfaces:**
- The resources API now reports each resource's expected artifact size
  (`expected_size_bytes`, measured for QUDT/UCUM/BIPM SIRP; 0 for Wikidata
  where the snapshot varies with the pinned entity subset, capped at
  `max_bytes`) and the upstream update cadence (`update_cadence`: `weekly`
  for Wikidata, empty when the release is pinned). While a download streams,
  the source's `status.json` carries live progress
  (`downloading`/`downloaded_bytes`/`total_bytes`) that the API surfaces so
  the page can render a progress bar without long-polling the download POST.
- The External Terminology Resources page shows "Expected size" and "Update
  cadence" on every card, and while a download runs renders a spinner plus a
  progress bar (percent + bytes when the server knows the total; an
  indeterminate bar otherwise) by polling the resources API once per second.
- A new scheduler job type `terminology_refresh` ("Refresh External
  Terminology Resources") re-fetches the configured sources (param
  `sources`, comma-separated, default `wikidata`) into the local fetch
  store, writing fresh artifacts plus unapproved draft manifests for
  operator review. It never imports: import still requires approval of the
  new draft. Operators create a weekly schedule row in `kb.scheduled_jobs`
  (interval 604800s, e.g. via the System Admin > Schedules page) so
  Wikidata's weekly dump cadence maps to an automated re-fetch.

- [x] Add expected sizes and cadence to the resource catalog and API view.
- [x] Persist `downloading`/`downloaded_bytes`/`total_bytes` progress in
      `status.json` while fetching and report it in the resources API.
- [x] Render expected size, cadence, spinner, and progress bar on the
      External Terminology Resources page.
- [x] Register the `terminology_refresh` scheduler job and add per-source
      error-path tests.
- [x] Run go tests/build/vet, svelte-check, eslint, and prettier on the
      changed files.
- [x] Update §13.2 and the documentation-impact section; commit with `jj`.

**Live acceptance:** The weekly schedule was created in
`kb.scheduled_jobs` (job_type `terminology_refresh`, interval 604800s,
params `{"sources":"wikidata"}`, enabled, recurring). The first run fired
automatically on the next scheduler tick and succeeded: it fetched the
Wikidata pilot snapshot (`dump-2026-08-08`, 44,417 bytes) into
`/Users/cding/Apps/SemOS/terminology/wikidata/` with a fresh unapproved
`manifest.draft.json`, and advanced `next_run_at` to 2026-08-14. The
External Terminology Resources page now lists the fresh Wikidata draft under
Review External Resources for operator approval.

## Task 12: SIRP Turtle fetch and adapter alignment

**Problem:** The BIPM SIRP `/quantities` endpoint content-negotiates: it
returns a flat JSON array by default and Turtle only when the request sends
`Accept: text/turtle`. The catalog entry declared a `quantities.json` JSON
artifact while the adapter parsed Turtle, so the first live SIRP approval
failed with `sirp turtle parse: ... expected predicate`, and the ontology's
real `si:QuantityKind` class did not match the adapter's `ont#Quantity`.

**Files:**
- Modify: `server/api/ontology/terminology/fetch.go` (+ `fetch_test.go`),
  `server/api/ontology/terminology/sirp.go`,
  `server/api/ontology/terminology/adapter_test.go`,
  `server/api/ontology/terminology/testdata/fixtures/sirp/quantities.ttl`,
  `server/api/ontology/terminology/testdata/fixtures/sirp/manifest.json`,
  `server/api/ontology/terminology/testdata/malformed/sirp/unknown-relation.ttl`

**Interfaces:**
- The SIRP catalog entry now declares `quantities.ttl` (`text/turtle`), and
  the fetch sends a per-resource `Accept` header (`text/turtle` for SIRP) so
  the endpoint serves the Turtle representation the adapter consumes.
- The adapter recognizes subjects typed `si:QuantityKind` as quantities and
  ignores the ontology header, provenance entities, and blank-node unit
  expressions; unknown skos/owl/rdfs predicates on quantity subjects still
  fail closed.

- [x] Reproduce the SIRP parse failure against the real downloaded artifact.
- [x] Request Turtle via `Accept` and align the catalog artifact/media type.
- [x] Adapt the parser to the real `si:QuantityKind` ontology structure.
- [x] Update fixtures/manifest checksums and adapter/fetch tests.
- [x] Re-fetch SIRP and verify the real file converts (149 entries, 447
      labels: en+fr preferred plus code alternative labels; 0 relations —
      the current dump carries no exactMatch links).
- [x] Update this plan; commit with `jj`.

## Task 13: UCUM essence XML charset and root alignment

**Problem:** The official UCUM essence v2.2 declares
`<?xml version="1.0" encoding="ascii"?>` and uses `<root>` as its document
element. Go's `encoding/xml` rejects the ascii declaration unless a
CharsetReader is installed, and the adapter expected a `<units>` root (the
fixture shape), so the first live UCUM approval failed with `xml: encoding
"ascii" declared but Decoder.CharsetReader is nil`, then `expected element
type <units> but have <root>`.

**Files:**
- Modify: `server/api/ontology/terminology/ucum.go`,
  `server/api/ontology/terminology/adapter_test.go`,
  `server/api/ontology/terminology/testdata/fixtures/ucum/ucum-essence.xml`,
  `server/api/ontology/terminology/testdata/fixtures/ucum/manifest.json`,
  `server/api/ontology/terminology/testdata/malformed/ucum/duplicate-code.xml`,
  `server/api/ontology/terminology/testdata/malformed/ucum/missing-code.xml`

**Interfaces:**
- The UCUM adapter installs a CharsetReader that passes
  ascii/us-ascii/utf-8 through and decodes iso-8859-1/windows-1252; unknown
  labels fail closed rather than guessing.
- The document element is now `<root>` (the real essence shape). The real
  file yields 305 unit codes; SI base units are implicit in UCUM (no `<unit>`
  entries), and upstream carries no `dim` attributes, so dimensions are empty.

- [x] Reproduce the charset and root-element failures on the real artifact.
- [x] Add the CharsetReader and switch the document element to `<root>`.
- [x] Update fixtures/manifest checksums and malformed fixtures; add an
      ascii-declared essence regression test.
- [x] Re-fetch UCUM and verify the real file converts (305 unit codes).
- [x] Update this plan; commit with `jj`.

## Task 14: Wikidata snapshot normalization to adapter schema

**Problem:** The first live Wikidata approval failed with
`adapter wikidata: decode wikidata line: json: unknown field "type"` because
`fetchWikidataSnapshot` wrote the raw wbgetentities entity objects (fields
`type`, `claims`, `sitelinks`, and `labels` shaped as `{language,value}`
pairs) while the WikidataAdapter decodes the normalized, revision-pinned
`WikidataLine` schema with `DisallowUnknownFields`. The raw dump also had no
`lastrevid` because `info` was not in the requested props, so no entity
revision could be pinned.

**Files:**
- Modify: `server/api/ontology/terminology/fetch.go` (+ `fetch_test.go`)

**Interfaces:**
- The fetch requests `props=info|labels|aliases|descriptions|claims|sitelinks`
  so `lastrevid` is present, then projects each entity through
  `normalizeWikidataEntity` into the adapter's `WikidataLine` schema before
  writing JSONL: labels/aliases flatten to language->text maps,
  `external-id`/`url` claims become `external_ids`, and the item-relation
  claims P1889 (different from) and P279 (subclass of; the object is the
  broader concept) become proposal `different_from`/`broader` statements.
  External ids and statements are sorted by property/value so identical API
  responses produce byte-identical artifacts with stable SHA-256 checksums.
  The adapter is unchanged: the snapshot is now exactly what it always
  expected.

- [x] Reproduce the unknown-field failure against the real raw artifact.
- [x] Request `info` and normalize each entity into `WikidataLine`.
- [x] Sort external ids and statements for deterministic checksums.
- [x] Update the fetch test to decode normalized lines (revision, flat
      labels, external ids, statements, retrieved_at).
- [x] Re-fetch Wikidata and verify the real file converts (3 entries, 12
      labels, 2 broader relations, 6 different-from decisions).
- [x] Update this plan; commit with `jj`.

## Documentation impact (workspace policy)

- **What knowledge changed?** Tier 6 no longer trusts cosine as merge
  authority; exact identity requires an authoritative, enabled,
  license-approved `exact_equivalent` claim from governed `(source,
  external_id, release)` evidence. Reviewed promotion is the only writer into
  `keyword_external_ids`/`keyword_surface_evidence`/the provenance-linked
  `never_merge` veto; negative evidence is a veto, never a lowered score.
  Four of the five portfolio resources (QUDT, UCUM, BIPM SIRP, Wikidata pilot
  subset) are now downloadable automatically with SHA-256 and an unapproved
  draft manifest; IEC 60050-845 remains permission-gated by design. Downloaded
  sources surface on a Review page until their draft manifest is approved.
  Approval records the operator's review comments and starts the offline
  import of the approved local manifest; disapproval saves the comments,
  marks the draft `disapproved`, and never imports. The QUDT adapter retains
  `skos:closeMatch` as a non-authoritative `close_match` relation, and the
  approve flow replays releases already registered with byte-identical
  content instead of surfacing an immutable-release error. Admin resource
  cards now show expected sizes and update cadence before download, stream
  live download progress (spinner + progress bar) from `status.json`, and
  Wikidata's weekly dump cadence is mapped to an automated
  `terminology_refresh` scheduler job that re-fetches into new unapproved
  draft manifests (import still requires operator approval). Live approvals
  of the UCUM (305 codes), BIPM SIRP (149 entries), and Wikidata (3 pilot
  entries) drafts now succeed: SIRP is fetched as Turtle and parsed as
  `si:QuantityKind`, UCUM's ascii-declared `<root>` essence is decoded, and
  the Wikidata fetch normalizes raw wbgetentities responses into the
  revision-pinned `WikidataLine` schema the adapter consumes.
- **Which docs/specs/ADRs/tests are affected?** The two 2026-08-07 design
  specs, this plan, the governing KnowledgeStore keyword spec (§13.1, R6,
  §21, I2), the Stage-0/1 tooling commits (Tasks 0–10), the live integration
  test `server/api/ontology/keywords/reconcile_identity_integration_test.go`,
  the new fetch/handler/CLI tests, and the External Terminology Resources and
  Review External Resources admin pages.
- **Which docs were updated?** `docs/superpowers/specs/2026-08-07-model-agnostic-tier6-validation-design.md`
  (§7.2 live proof, §9), `docs/superpowers/specs/2026-08-07-external-terminology-resource-portfolio-design.md`
  (status, §12.3, §13 implementation/commands/formats/acceptance, §13.2 fetch
  tool/approve/disapprove API, sizes/progress/weekly refresh), this plan
  (Tasks 9–14), the governing
  `KnowledgeStore/doc-repo/specs/202608/2026080403-spec-keyword-canonicalization-and-reconciliation.md`,
  and the stored coverage report under `docs/superpowers/reports/`.
- **Which docs are now stale?** The keyword spec's §13.1 "embedding may
  auto-accept" wording (revised in place), the §0 "Never validated live" row
  (closed by I2), and §20.1.12 I2 (resolved). No applied migration changed;
  `project_migrations/20260807000001` remains the only governed-source migration.
- **What was intentionally left undocumented?** Production resource files are
  not distributed with the repository (synthetic fixtures stand in for real
  SIRP/QUDT/IEC/Wikidata artifacts); operator approval of the coverage
  backlog, the license review/approval of each fetched draft manifest, and a
  published production seed release remain deliberately outside tooling
  authority. The download tool automates retrieval but cannot approve a
  manifest or import anything; the admin Review dialog is the operator gate
  that writes the approval/rejection into the local draft manifest.

## Completion boundary

This plan implements the validator plus Stage-0/1 coverage, import, governance, promotion, negative-evidence, release-diff, and rollback tooling, and the network-enabled download tool/API plus the admin pages (Tasks 9–10). Task 10 is the operator gate: the Review dialog approves or disapproves each fetched draft manifest (recording comments and reviewer) and starts the offline import on approval; QUDT 3.5.0 is the first live import, and re-approving identical content replays idempotently. Task 11 adds expected sizes and update cadence to the resource cards, live download progress, and the `terminology_refresh` scheduler job so Wikidata's weekly cadence can be automated as scheduled re-fetches that always land in unapproved draft manifests. Tasks 12–14 align the
  live SIRP, UCUM, and Wikidata artifacts to their adapters so approving those fetched drafts
  imports successfully. It does not claim that the production portfolio is bootstrapped: the remaining free source (Wikidata pilot subset) still awaits operator review/approval, and the plan does not redistribute datasets, scrape IEC/CIE/ISO pages, bulk-import licensed IEC content, choose the operator's coverage target, publish a production seed release, or implement Stage-2/3 licensed/clinical adapters. Production activation still requires operators to review/approve each remaining fetched manifest (license, role, scope, relations, checksum), supply the reviewed IEC seed/promotion files, run the corpus/coverage report, approve the acceptance criteria, and publish a versioned seed release before enabling Tier 6.
