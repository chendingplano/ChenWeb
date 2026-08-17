package semantic

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// Task 4.2 / DR6: the value-state-aware payload constraint. The migration
// replaces chk_semantic_assertions_object_ref_or_literal, whose whole problem
// was that a genuine missing-value claim could not be stored at all.
func TestIntegrationValueStatePayloadConstraint(t *testing.T) {
	db := freshSemanticTestDB(t)

	insert := func(valueState string, objectLiteral, rawText, qualifiers any) error {
		_, err := db.Exec(`
INSERT INTO kb.semantic_assertions
  (logical_identity_key, subject_ref_kind, subject_ref_id, predicate_term_id,
   object_ref_kind, object_literal, raw_text, qualifiers, status, value_state_term_id)
VALUES ($1,'object_node','obj-1','mea:measured_by',
        CASE WHEN $2::jsonb IS NULL THEN NULL ELSE 'literal' END,
        $2::jsonb, $3, $4::jsonb, 'represented', $5)`,
			"test:"+valueState+randSuffix(), objectLiteral, rawText, qualifiers, valueState)
		return err
	}

	// present requires a normalized literal or reference.
	if err := insert(ValuePresent, `{"value":300}`, nil, nil); err != nil {
		t.Errorf("present with a literal should be accepted: %v", err)
	}
	if err := insert(ValuePresent, nil, nil, nil); err == nil {
		t.Error("present with neither literal nor reference must be rejected")
	}

	// The case the old constraint made impossible: a source names a metric but
	// supplies no value. No fabricated object, but subject is required.
	if err := insert(ValueMissing, nil, nil, nil); err != nil {
		t.Errorf("a genuine missing-value instance must be storable (DR6): %v", err)
	}
	if err := insert(ValueMissing, `{"value":0}`, nil, nil); err == nil {
		t.Error("a missing-value instance must not carry a fabricated object")
	}

	// unparsed / datatype_mismatch / unknown carry the claim in the raw payload.
	for _, state := range []string{ValueUnparsed, ValueDatatypeMismatch, ValueUnknown} {
		if err := insert(state, nil, "3 to 5 or as specified", nil); err != nil {
			t.Errorf("%s with raw text should be accepted: %v", state, err)
		}
		if err := insert(state, nil, nil, nil); err == nil {
			t.Errorf("%s with no raw payload must be rejected: the exact offending value must be preserved (DR2)", state)
		}
	}

	// not_applicable requires explicit non-applicability context.
	if err := insert(ValueNotApplicable, nil, nil, `{"applicability":"not applicable to portable units"}`); err != nil {
		t.Errorf("not_applicable with context should be accepted: %v", err)
	}
	if err := insert(ValueNotApplicable, nil, nil, nil); err == nil {
		t.Error("not_applicable with no context must be rejected")
	}

	// Legacy rows written by the existing writer carry no state at all; Phase 1
	// is additive and must not break the running writer.
	if _, err := db.Exec(`
INSERT INTO kb.semantic_assertions
  (logical_identity_key, subject_ref_kind, subject_ref_id, predicate_term_id,
   object_ref_kind, object_literal, status)
VALUES ('test:legacy','object_node','obj-1','mea:measured_by','literal','{"value":1}'::jsonb,'candidate')`); err != nil {
		t.Errorf("a legacy row with no value state must still be insertable: %v", err)
	}
}

// Task 4.2 / DR6: unsupported_prior_status is permitted only on unsupported
// rows and only with a legal prior status.
func TestIntegrationUnsupportedPriorStatusConstraint(t *testing.T) {
	db := freshSemanticTestDB(t)

	insert := func(status, prior string) error {
		var priorArg any
		if prior != "" {
			priorArg = prior
		}
		_, err := db.Exec(`
INSERT INTO kb.semantic_assertions
  (logical_identity_key, subject_ref_kind, subject_ref_id, predicate_term_id,
   object_ref_kind, object_literal, status, unsupported_prior_status, value_state_term_id)
VALUES ($1,'object_node','obj-1','mea:measured_by','literal','{"value":1}'::jsonb,$2,$3,$4)`,
			"test:prior:"+status+prior+randSuffix(), status, priorArg, ValuePresent)
		return err
	}

	for _, prior := range []string{"represented", "candidate", "in_review", "deferred", "accepted"} {
		if err := insert("unsupported", prior); err != nil {
			t.Errorf("unsupported with prior %q should be accepted: %v", prior, err)
		}
	}
	// rejected and superseded are historical decision states, never restoration
	// targets (DR6).
	for _, prior := range []string{"rejected", "superseded"} {
		if err := insert("unsupported", prior); err == nil {
			t.Errorf("unsupported with prior %q must be rejected", prior)
		}
	}
	// A prior status on a row that is not unsupported is meaningless.
	if err := insert("accepted", "represented"); err == nil {
		t.Error("a non-unsupported row must not carry unsupported_prior_status")
	}
	if err := insert("unsupported", ""); err != nil {
		t.Errorf("unsupported without a prior status must remain insertable for legacy rows: %v", err)
	}
}

// The represented status must be accepted by the widened CHECK constraint.
func TestIntegrationRepresentedStatusAccepted(t *testing.T) {
	db := freshSemanticTestDB(t)
	if _, err := db.Exec(`
INSERT INTO kb.semantic_assertions
  (logical_identity_key, subject_ref_kind, subject_ref_id, predicate_term_id,
   object_ref_kind, object_literal, status, value_state_term_id)
VALUES ('test:represented','object_node','obj-1','mea:measured_by','literal','{"value":1}'::jsonb,'represented',$1)`,
		ValuePresent); err != nil {
		t.Fatalf("represented must be a legal persisted status (DR6): %v", err)
	}
	// And an invented status must still be rejected: widening the constraint
	// must not have removed it.
	if _, err := db.Exec(`
INSERT INTO kb.semantic_assertions
  (logical_identity_key, subject_ref_kind, subject_ref_id, predicate_term_id,
   object_ref_kind, object_literal, status)
VALUES ('test:invented','object_node','obj-1','mea:measured_by','literal','{"value":1}'::jsonb,'probably_fine')`); err == nil {
		t.Fatal("an invented status must still be rejected")
	}
}

// Task 4.8: the completeness projection reports missing required stage
// outcomes and blocks activation. This is the mechanism the ADR relies on
// because a unique index can only enforce "at most one", never existence.
func TestIntegrationCompletenessDetectsMissingStageOutcomes(t *testing.T) {
	db := freshSemanticTestDB(t)
	ctx := context.Background()
	adapter := MetricAdapter{}

	seedMetric(t, db, 1, "m-1")
	seedMetric(t, db, 1, "m-2")

	checker := CompletenessChecker{DB: db, ArtifactSourceSQL: MetricArtifactSourceSQL}
	rep, err := checker.Run(ctx, adapter)
	if err != nil {
		t.Fatalf("run completeness: %v", err)
	}
	if rep.CurrentArtifacts != 2 {
		t.Fatalf("current artifacts = %d, want 2", rep.CurrentArtifacts)
	}
	// 2 artifacts x 3 required stages, none recorded.
	if rep.MissingStageOutcomes != 6 {
		t.Errorf("missing stage outcomes = %d, want 6", rep.MissingStageOutcomes)
	}
	if rep.ArtifactsMissingAnyStage != 2 {
		t.Errorf("artifacts missing a stage = %d, want 2", rep.ArtifactsMissingAnyStage)
	}
	// Neither an assertion path nor an unresolved occurrence: the failed
	// losslessness invariant.
	if rep.ArtifactsWithNeitherPath != 2 {
		t.Errorf("artifacts with neither path = %d, want 2", rep.ArtifactsWithNeitherPath)
	}
	if rep.Complete() {
		t.Fatal("an incomplete corpus must not report complete")
	}
	if len(rep.BlockingReasons()) == 0 {
		t.Fatal("an incomplete report must explain what blocks cutover")
	}

	// Record every required stage for one metric; its stage gap must close.
	store := OutcomeStore{DB: db}
	recordID := int64(1)
	for _, st := range adapter.RequiredStages() {
		out := Outcome{
			OutcomeKey:            OutcomeKey(1, MetricArtifactType, "m-1", st.StageTermID),
			InputRecordID:         &recordID,
			ArtifactType:          MetricArtifactType,
			ArtifactID:            "m-1",
			StageTermID:           st.StageTermID,
			DispositionTermID:     DispositionNormalized,
			ExecutionStatus:       ExecutionCompleted,
			OutcomeCategory:       CategorySemanticSuccess,
			DependencyFingerprint: Dependencies{ParserVersion: "p-1"}.Fingerprint(),
			InputFingerprint:      "v1:in",
		}
		if _, err := store.Record(ctx, out, nil); err != nil {
			t.Fatalf("record %s: %v", st.StageTermID, err)
		}
	}
	rep, err = checker.Run(ctx, adapter)
	if err != nil {
		t.Fatalf("re-run completeness: %v", err)
	}
	if rep.MissingStageOutcomes != 3 {
		t.Errorf("after recording m-1's stages, missing = %d, want 3", rep.MissingStageOutcomes)
	}
	if rep.ArtifactsMissingAnyStage != 1 {
		t.Errorf("artifacts missing a stage = %d, want 1", rep.ArtifactsMissingAnyStage)
	}
}

// An artifact covered by an active unresolved occurrence satisfies the
// losslessness invariant even without an assertion (DR13's option 3).
func TestIntegrationCompletenessAcceptsUnresolvedOccurrencePath(t *testing.T) {
	db := freshSemanticTestDB(t)
	ctx := context.Background()
	seedMetric(t, db, 1, "m-3")

	recordID := int64(1)
	if _, _, err := (OccurrenceStore{DB: db}).Upsert(ctx, UnresolvedOccurrence{
		OccurrenceKey:         OccurrenceKey(MetricOccurrenceScope, 1, MetricArtifactType, "m-3"),
		InputRecordID:         &recordID,
		ArtifactType:          MetricArtifactType,
		ArtifactID:            "m-3",
		InputFingerprint:      "v1:in",
		DependencyFingerprint: "v1:dep",
	}); err != nil {
		t.Fatalf("upsert occurrence: %v", err)
	}

	rep, err := CompletenessChecker{DB: db, ArtifactSourceSQL: MetricArtifactSourceSQL}.Run(ctx, MetricAdapter{})
	if err != nil {
		t.Fatalf("run completeness: %v", err)
	}
	if rep.ArtifactsWithNeitherPath != 0 {
		t.Fatalf("artifacts with neither path = %d; an active unresolved occurrence is a valid path (DR13)",
			rep.ArtifactsWithNeitherPath)
	}
}

// Task 4.8 / DR13: activation is refused unless the registered adapter has
// passed the CURRENT conformance suite.
func TestIntegrationWriterActivationRefusal(t *testing.T) {
	db := freshSemanticTestDB(t)
	ctx := context.Background()
	resetAdaptersForTest()
	t.Cleanup(resetAdaptersForTest)
	RegisterAdapter(MetricAdapter{})

	// Gate off: refused regardless of compliance.
	if err := AuthorizeWriterActivation(ctx, db, MetricArtifactType, false); err == nil {
		t.Fatal("activation with the gate disabled must be refused")
	}

	// Gate on but no compliance record.
	err := AuthorizeWriterActivation(ctx, db, MetricArtifactType, true)
	if err == nil || !strings.Contains(err.Error(), "no compliance record") {
		t.Fatalf("expected a no-compliance-record refusal, got %v", err)
	}

	// Record a pass, then activation succeeds.
	res, err := VerifyAndRecord(ctx, db, MetricAdapter{}, WriterShadow)
	if err != nil {
		t.Fatalf("verify and record: %v", err)
	}
	if !res.Passed {
		t.Fatalf("metric adapter failed conformance: %v", res.Failures)
	}
	if err := AuthorizeWriterActivation(ctx, db, MetricArtifactType, true); err != nil {
		t.Fatalf("activation should be authorized after a current-suite pass: %v", err)
	}

	// A pass recorded against an older suite version is not a pass: bumping the
	// suite must de-certify every adapter until it is re-verified.
	if _, err := db.Exec(`UPDATE kb.semantic_adapter_compliance
		SET conformance_suite_version = '0.0.1' WHERE artifact_type = $1`, MetricArtifactType); err != nil {
		t.Fatal(err)
	}
	err = AuthorizeWriterActivation(ctx, db, MetricArtifactType, true)
	if err == nil || !strings.Contains(err.Error(), "current suite") {
		t.Fatalf("expected a stale-suite refusal, got %v", err)
	}

	// An unregistered family must fall back, never activate.
	if err := AuthorizeWriterActivation(ctx, db, "provision", true); err == nil {
		t.Fatal("an unregistered family must not be authorized to activate a lossless writer")
	}
}

// The database refuses to store 'lossless' without a recorded pass, so a
// direct UPDATE cannot bypass the activation check.
func TestIntegrationLosslessWriterModeRequiresRecordedPass(t *testing.T) {
	db := freshSemanticTestDB(t)
	if _, err := db.Exec(`
INSERT INTO kb.semantic_adapter_compliance (artifact_type, adapter_name, adapter_version, writer_mode)
VALUES ('metric','metric_lossless_adapter','0.1.0','lossless')`); err == nil {
		t.Fatal("writer_mode 'lossless' without a recorded conformance pass must be rejected by the database (DR13)")
	}
}

// Task 4.9: shadow mode performs no consumer-visible semantic writes.
func TestIntegrationShadowModeWritesNothing(t *testing.T) {
	db := freshSemanticTestDB(t)
	ctx := context.Background()
	seedMetricWithRangeType(t, db, 1, "m-mapped", "min", "≥ 300")
	seedMetricWithRangeType(t, db, 1, "m-unmapped", "totally_novel_type", "≥ 5")
	seedMetricWithRangeType(t, db, 1, "m-novalue", "min", "")

	before := tableCounts(t, db)
	cmp, err := MetricAdapter{}.RunShadow(ctx, db, 1)
	if err != nil {
		t.Fatalf("run shadow: %v", err)
	}
	after := tableCounts(t, db)
	for table, n := range before {
		if after[table] != n {
			t.Errorf("shadow mode wrote to %s: %d -> %d (Phase 1 item 7 forbids consumer-visible writes)",
				table, n, after[table])
		}
	}

	if cmp.MetricsExamined != 3 {
		t.Fatalf("metrics examined = %d, want 3", cmp.MetricsExamined)
	}
	// All three are unreachable today: none has a supporting evidence link.
	if cmp.ExistingUnreachableMetrics != 3 {
		t.Errorf("existing unreachable = %d, want 3", cmp.ExistingUnreachableMetrics)
	}
	// 'min' is an approved mapping seeded by migration 20260814000002.
	if cmp.WouldBeNormalized != 1 {
		t.Errorf("would-be-normalized = %d, want 1 (the approved-mapping metric)", cmp.WouldBeNormalized)
	}
	if cmp.WouldBecomeRawPreserved != 2 {
		t.Errorf("would-become-raw-preserved = %d, want 2", cmp.WouldBecomeRawPreserved)
	}
	if cmp.IntendedFindingsByTerm[FindingMappingUnresolved] != 1 {
		t.Errorf("mapping_unresolved findings = %d, want 1", cmp.IntendedFindingsByTerm[FindingMappingUnresolved])
	}
	if cmp.IntendedFindingsByTerm[FindingValueMissing] != 1 {
		t.Errorf("value_missing findings = %d, want 1", cmp.IntendedFindingsByTerm[FindingValueMissing])
	}
	if cmp.IntendedOutcomeEnvelopes != 9 {
		t.Errorf("intended envelopes = %d, want 9 (3 metrics x 3 required stages)", cmp.IntendedOutcomeEnvelopes)
	}
}

func tableCounts(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range []string{
		"kb.semantic_assertions", "kb.assertion_evidence", "kb.semantic_processing_outcomes",
		"kb.semantic_processing_findings", "kb.unresolved_semantic_occurrences",
		"kb.semantic_decision_candidates", "kb.semantic_retry_queue",
	} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		out[table] = n
	}
	return out
}

func seedMetric(t *testing.T, db *sql.DB, recordID int64, metricID string) {
	t.Helper()
	seedMetricWithRangeType(t, db, recordID, metricID, "min", "≥ 300")
}

func seedMetricWithRangeType(t *testing.T, db *sql.DB, recordID int64, metricID, rangeType, threshold string) {
	t.Helper()
	if _, err := db.Exec(`
INSERT INTO kb.metrics (input_record_id, metric_id, metric_name, value_range_type, threshold_or_target)
VALUES ($1,$2,$3,$4,$5)`, recordID, metricID, "test metric "+metricID, rangeType, threshold); err != nil {
		t.Fatalf("seed metric %s: %v", metricID, err)
	}
}

var randCounter int

func randSuffix() string {
	randCounter++
	return string(rune('a'+randCounter%26)) + string(rune('a'+(randCounter/26)%26)) + string(rune('a'+(randCounter/676)%26))
}
