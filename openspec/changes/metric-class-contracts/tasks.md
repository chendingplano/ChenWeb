## 1. Contract store: idempotent header/revision access

- [x] 1.1 Add `ContractStore.EnsureHeader(ctx, db DBX, identity ClassIdentity) (ContractRevision, error)` in `server/api/ontology/classfoundation/contracts_store.go`: upsert the header row (`ON CONFLICT (term_id) DO NOTHING`), then return the existing current revision if `current_contract_revision_id` is set, else call the existing `AppendContractRevision` with `DefinitionIdentityOnly` and return that.
- [x] 1.2 Add `ContractStore.Current(ctx, db DBX, termID string) (ContractRevision, bool, error)`: read the header's `current_contract_revision_id` and join to `kb.ontology_class_contract_revisions`; return `false` (not an error) when no header exists yet for `termID`.
- [x] 1.3 Unit tests (extend `contracts_store_test.go`): `EnsureHeader` creates header+revision on first call; second call on the same term returns the same revision without appending a new one; `Current` returns `false` for an unknown term and the right revision for a known one.

## 2. Reconcile the live class-resolution path with the header table

- [x] 2.1 In `server/api/ontology/assertions/metric_lossless_writer.go`, call `classfoundation.ContractStore{DB: tx}.EnsureHeader` from `resolveOrCreateMetricClass` immediately after `selectedClassTermID`/`classIdentityState` are settled (all three branches: signature match, existing-term reuse, fresh synthesis), using the class's `module_id` (`measurement`) and `by = "metric_lossless_writer"`.
- [x] 2.2 Integration test (extend `metric_lossless_writer_integration_test.go`): after writing a metric, assert `kb.ontology_term_headers` has a row for the resulting `instance_of_term_id` and `kb.ontology_class_contract_revisions` has exactly one `identity_only` row.
- [x] 2.3 Integration test: write two metrics that resolve to the same class term (same signature); assert only one `identity_only` revision exists (no duplicate on the second write).
- [x] 2.4 Integration test: manually insert a `kb.ontology_terms`/`metric_definition` row the old way (no header), then run a metric write that resolves to it; assert the header and first revision are backfilled on that write.

## 3. Record observed-profile evidence from the write path

- [x] 3.1 In `writeMetricLossless`, after class resolution, call `classfoundation.ObservedProfileStore{DB: tx}.Record` with `AttributeKey: "value"`, `LogicalDatatype` derived from `p.ValueDataType`/`p.ValueForm`, `UnitTermID: unitTermID`, `ObservationState` derived from `valueStateTermID` (`present` when `semantic.ValuePresent`, else the matching non-present state), `DocumentKey: strconv.FormatInt(inputRecordID, 10)`, `AggregationMethod: "metric_lossless_writer"`, `MethodVersion: MetricLosslessWriterVersion`, `AssertionID: &created.ID` (recorded after the assertion is persisted, so this call moves to after `persistAssertionTx`, still inside the same transaction).
- [x] 3.2 Integration test: writing a metric with a parseable numeric value and resolved unit results in one `kb.ontology_observed_class_profiles` row and one attribute-observation row with `observed_count = 1`, `document_count = 1`.
- [x] 3.3 Integration test: writing a second metric for the same class from a different `input_record_id` with the same datatype/unit increments the existing attribute-observation row's counts rather than creating a duplicate.

## 4. Governed capability vocabulary

- [x] 4.1 Add `semantic:can_instantiate` and `semantic:can_validate_value` term entries (`Kind: "concept"`, module `semantic`) to `server/api/ontology/seed/content.go`, alongside the existing `semantic:not_evaluated`/`semantic:conforms` entries.
- [x] 4.2 Confirm (existing `ontology-seed` mechanism, no new command) the two terms release cleanly on a scratch database via the existing seed test suite.

## 5. Capability validators

- [x] 5.1 Add `server/api/ontology/classfoundation/metric_capability_validators.go` with `CanInstantiateValidator` (`ID() == "semantic:can_instantiate"`; `Validate` always returns `ValidationPass` with evidence `{"definition_state": "<state>"}` for any contract) and `CanValidateValueValidator` (`ID() == "semantic:can_validate_value"`; `Validate` parses `input.ContractPayload` and returns `ValidationPass` when it declares a non-empty `value_type` and at least one `permitted_unit_term_ids` entry, `ValidationFail` otherwise, both with evidence naming what was checked).
- [x] 5.2 Unit tests for both validators covering pass/fail/malformed-payload cases (`ErrCapabilityValidatorNotFound`/`ErrIdentityOnlyCapability` paths already covered by existing `capability_validation_test.go`; add only the two new validators' own behavior).
- [x] 5.3 Wire a `CapabilityValidationDispatcher{DB: tx, Validators: []CapabilityValidator{CanInstantiateValidator{}, CanValidateValueValidator{}}}` call site: declare `semantic:can_instantiate` once per `EnsureHeader` call (task 2.1); declare `semantic:can_validate_value` once per contract right after a promotion (task 6) or when it's already `partially_defined`+ and hasn't been declared yet (check `kb.ontology_class_contract_capabilities` first to avoid redundant validator runs on every write).

## 6. Deterministic contract synthesis

- [x] 6.1 Add `classfoundation.SynthesizeContractFromObservations(ctx, db DBX, classTermID string) (revision ContractRevision, promoted bool, err error)`: no-op (not promoted) if the class's current contract is already past `identity_only`; otherwise query `kb.ontology_observed_class_attribute_observations` for `attribute_key = 'value'`, `observation_state = 'present'` rows for this class, group by `(logical_datatype, unit_term_id)`, and promote via `AppendContractRevision` (`DefinitionPartiallyDefined`, payload `{"value_type": ..., "permitted_unit_term_ids": [...]}`, `SynthesisMethod: "deterministic_unambiguous_observation_agreement"`, `Provenance` recording the counts) only when exactly one group exists with summed `document_count >= 2`.
- [x] 6.2 Call `SynthesizeContractFromObservations` from `writeMetricLossless` right after the observed-profile record (task 3.1), inside the same transaction; on `promoted == true`, run the `can_validate_value` capability declaration from task 5.3 against the new revision.
- [x] 6.3 Integration tests (real Postgres, more reliable than mocking multi-query SQL): two documents agreeing → promotion; one document → no promotion; two documents disagreeing on unit → no promotion; non-`present` observations excluded from the agreement check; a contract already `partially_defined` is left untouched by a later contradicting observation (no exception row -- the contradiction surfaces via the specific instance's conformance state from task 7 instead, a simplification over the original design).

## 7. Per-instance conformance

- [x] 7.1 In `writeMetricLossless`, after class resolution (and after any synthesis attempt from task 6.2), replace the hardcoded `ConformanceStateTermID: semantic.ConformanceNotEvaluated` with a lookup: read the class's current contract (`ContractStore.Current`); if `identity_only` or missing, use `semantic.ConformanceNotEvaluated` unchanged; if `partially_defined`/`validated`, compare this instance's `unitTermID` against the contract payload's `permitted_unit_term_ids` (and datatype compatibility) and set `semantic.Conforms` or `semantic.ConformanceContractViolation`.
- [x] 7.2 Integration tests: identity-only class → assertion is `not_evaluated` (existing behavior, regression-covered); partially-defined class with matching unit → `conforms`; partially-defined class with a different unit → `conformance_contract_violation`.
- [x] 7.3 Confirm (test) that setting `conforms`/`conformance_contract_violation` does not change the assertion's `Status` field (`represented` stays `represented`), per the `semantic-assertion-lifecycle` spec delta.

## 8. Backfill command

- [x] 8.1 Add `server/cmd/metric-contract-backfill/main.go`, modeled on `server/cmd/metric-support-cleanup/main.go`: select `kb.semantic_assertions` rows with `conformance_state_term_id = 'semantic:not_evaluated'` and an `instance_of_term_id` whose current contract is no longer `identity_only`; re-run the task 7.1 comparison for each and update `conformance_state_term_id` in place.
- [x] 8.2 Dry-run flag (report counts without writing), matching the existing admin-command convention in this package.
- [x] 8.3 Test against a scratch database: seed a `not_evaluated` assertion, promote its class's contract directly, run the command, assert the assertion's conformance state updates and no other field changes.

## 9. Verification

- [x] 9.1 `go build ./...` and `go vet ./...` clean workspace-wide. Verified clean.
- [x] 9.2 `go test ./server/api/ontology/classfoundation/... ./server/api/ontology/assertions/... ./server/cmd/metric-contract-backfill/...` clean, including new integration tests against `TEST_DATABASE_URL=chenweb_test` (never `miner`). All pass. A workspace-wide `go test ./...` also run: `seed`, `terminologyresourcehandler`, and `qudt-import` fail on pre-existing sqlmock/fixture staleness unrelated to this change (the first two confirmed identical on unmodified code; `qudt-import` was already named in ADR `2026082203`'s own pre-existing-failure catalogue); `semantic`'s `TestIntegrationPhase1MigrationsRollBackCleanly` fails on unrelated pre-existing tables and this change adds zero migrations, so it cannot be implicated.
- [x] 9.3 Ran `metric-contract-backfill` (report-only, no `--apply` -- this command's actual flag convention, matching the sibling `metric-support-cleanup` command) against `miner`: reported 0 stale assertions, as expected before any write has gone through the new code path.
- [~] 9.4 Not run via the full HTTP-triggered pipeline (would need live auth/session setup, out of proportion to what it would additionally prove). Verified instead via `TestIntegrationWriteMetricLosslessObservedProfileEvidenceAndConformance` (task 3/6/7's test), which drives four real writes through the actual production `writeMetricLossless` function -- the same function `associate_semantics` calls in production, unmodified orchestration around it -- against a real, fully-migrated Postgres database, and confirms: two agreeing documents promote the contract to `partially_defined`, `can_validate_value` records `pass`, a third write with a matching unit shows `semantic:conforms`, and a fourth with a different unit shows `semantic:conformance_contract_violation` without reverting the contract. This is the same underlying evidence 9.4 asked for, gathered through the test harness rather than the HTTP layer -- flagged here rather than silently checked off.
