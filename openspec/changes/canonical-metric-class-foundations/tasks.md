## 1. Baseline and reader audit

- [x] 1.1 Produce a corpus baseline for term readers, metric support-link duplicates, class/claim cardinality, and profile storage projections across all metric-bearing records
- [x] 1.2 Inventory every application read of `kb.ontology_terms`; classify it as current-state, historical, or write-path and assign a migration target
- [ ] 1.3 Define and load-test the initial caps for profile examples, redirect depth, canonical-key shadow reports, and same-class Review Document retrieval

## 2. Stable ontology class storage

- [x] 2.1 Add additive migrations for stable term identity headers, append-only term revisions, `kb.ontology_terms_current`, and required indexes
- [x] 2.2 Implement term current/history stores and migrate one representative current-state reader behind the compatibility view
- [x] 2.3 Add a read-audit test/command that blocks base-term reshaping while an application current-state reader remains unmigrated
- [x] 2.4 Add migration tests proving historical term state remains queryable and existing term references remain valid

## 3. Class contracts and capability validation

- [x] 3.1 Add append-only class-contract revision, capability declaration, and capability-validation-result storage with governed vocabulary references
- [x] 3.2 Implement stores that create identity-only classes and append validated contract revisions without changing stable term IDs
- [x] 3.3 Implement capability validation dispatch and persist validator name, version, result, evidence, and failure details per capability
- [x] 3.4 Add tests proving an identity-only class cannot advertise unsupported comparison or validation capability

## 4. Inclusive observed class profiles

- [x] 4.1 Add observed-profile, attribute-observation, distribution, example, contradiction, and outlier storage keyed by stable class and source evidence
- [x] 4.2 Implement profile aggregation that includes represented, malformed, unparsed, missing, nonconforming, and conflicting observations with independent states
- [x] 4.3 Implement a read model/API for observed profiles that labels them non-authoritative and applies configured example/result caps
- [x] 4.4 Add tests proving profile aggregation includes a malformed/outlier observation without writing or changing a class-contract revision

## 5. Class resolution decisions

- [x] 5.1 Add append-only class-resolution decisions, alternatives, evidence, method, confidence, and supersession storage
- [x] 5.2 Implement deterministic class-resolution service that selects an existing safe class or creates a provisional class, never a classless/dropped metric
- [x] 5.3 Add tests for resolved, provisional, ambiguous, and rejected decisions, including same-label/different-quantity negative identity

## 6. Canonical claims and instance references

- [x] 6.1 Add canonical-key-version registry and concurrency-safe `kb.semantic_claim_identities` with stable `claim_id` uniqueness
- [x] 6.2 Add `instance_of_term_id` and normalization-contract revision references to assertions additively, including state-aware constraints and indexes
- [ ] 6.3 Implement canonical key serialization and shadow find-or-create without enabling the new metric writer
- [ ] 6.4 Implement claim-identity store and tests for concurrent convergence, collision reporting, evidence-only changes, and identity-bearing divergence
- [ ] 6.5 Add a compatibility projection proving registry-managed assertion `logical_identity_key` equals its `claim_id`

## 7. Redirects and evidence cardinality

- [ ] 7.1 Add append-only term and assertion redirect stores with one-active-target constraints, supersession, and cycle rejection
- [ ] 7.2 Implement bounded redirect resolution with traversal provenance and explicit unresolved/depth-limit outcomes
- [ ] 7.3 Report and resolve duplicate current metric support links through an auditable history-preserving backfill
- [ ] 7.4 Add `uq_assertion_evidence_current_metric_support` scoped to active supporting metric evidence only
- [ ] 7.5 Add tests for redirect cycles, reversal/supersession, single-target enforcement, metric duplicate rejection, and non-metric evidence fan-out

## 8. Shadow integration and certification

- [ ] 8.1 Wire class, profile, claim, and redirect computations into metric shadow mode with no consumer-visible writer change
- [ ] 8.2 Expose shadow reports for provisional/ambiguous classes, claim convergence/collisions, profile outliers, redirects, and duplicate evidence exceptions
- [ ] 8.3 Build the reader and migration compatibility suite covering legacy terms/assertions, represented states, redirects, and current-term reads
- [ ] 8.4 Run corpus and load tests at active caps; publish capacity, latency, storage, rollback, and exception reports
- [ ] 8.5 Certify the ADR 2026081701 foundations for the dependent `lossless-semantic-processing` change; retain both lossless writer gates disabled

## 9. Documentation and handoff

- [ ] 9.1 Update ChenWeb architecture documentation and the metric semantic-processing manual with stable class, observed profile, contract, claim, and redirect semantics
- [ ] 9.2 Record stale documents, unresolved policy choices, and the precise handoff point for Phase 3 lossless metric writer activation
