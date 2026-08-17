package assertions

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestMetricAssertionKindTermIDUsesReleasedCatalogVocabulary locks in the
// critical contract: exact_value is not a Go-maintained allowlist member.
// It becomes usable whenever the existing termExists check finds its mea term
// in an included release.
func TestMetricAssertionKindTermIDUsesReleasedCatalogVocabulary(t *testing.T) {
	if got, ok := metricAssertionKindTermID("exact_value"); !ok || got != "mea:exact_value" {
		t.Fatalf("metricAssertionKindTermID(exact_value) = (%q, %t), want (mea:exact_value, true)", got, ok)
	}
	if _, ok := metricAssertionKindTermID("unparsed"); ok {
		t.Fatal("unparsed must not be eligible for governed-term lookup")
	}
}

func TestMetricQualifiersCarryMetricDefinitionTermID(t *testing.T) {
	p := metricCandidatePayload{
		MetricName:             "Organic matter",
		MetricDefinitionTermID: "mea:organic_matter_mass_fraction",
	}
	if got := metricQualifiers(p)["metric_definition_term_id"]; got != p.MetricDefinitionTermID {
		t.Fatalf("metric_definition_term_id qualifier = %#v, want %q", got, p.MetricDefinitionTermID)
	}
}

func TestGovernedTermDependencyFingerprintChangesOnlyWhenAvailabilityChanges(t *testing.T) {
	missing := governedTermDependencyFingerprint("mea:measured_by", false, "mea:exact_value", false)
	stillMissing := governedTermDependencyFingerprint("mea:measured_by", false, "mea:exact_value", false)
	released := governedTermDependencyFingerprint("mea:measured_by", true, "mea:exact_value", true)
	if missing != stillMissing {
		t.Fatalf("identical availability must have the same fingerprint: %q != %q", missing, stillMissing)
	}
	if missing == released {
		t.Fatalf("availability change must change fingerprint: %q", missing)
	}
}

func TestMetricSubjectObjectLinkedRequiresResolvedObjectID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (\n\tSELECT 1 FROM kb.artifact_objects")).
		WithArgs(int64(416), "416_mtc_47").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	linked, err := (AssociateSemantics{DB: db}).metricSubjectObjectLinked(context.Background(), 416, "416_mtc_47")
	if err != nil || linked {
		t.Fatalf("linked=%t err=%v, want false nil", linked, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// decisionCandidateRow builds one full-column kb.semantic_decision_candidates
// row for the given status, carrying a single metric candidate whose
// proposed_payload's value_range_type_lookup tag is lookupTag (ADR 2026081401
// DR3). Column order matches decisionCandidateColumns exactly.
func decisionCandidateRow(id int64, status, lookupTag string) *sqlmock.Rows {
	payload := `{"metric_id":"m1","subject_object_id":"obj1","assertion_kind":"unparsed","value_range_type_lookup":"` + lookupTag + `"}`
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "logical_identity_key", "revision", "payload_fingerprint", "candidate_kind",
		"proposed_payload", "method", "resolution_outcome", "resolution_reason",
		"source_artifact_type", "source_artifact_id", "input_record_id", "source_line_spans", "confidence",
		"status", "decision_reason", "dependency_fingerprint", "superseded_by",
		"resulting_assertion_id", "last_seen", "create_time", "create_by", "modify_time", "modify_by",
	}).AddRow(
		id, "metric:416:m1", 1, "fp1", "assertion",
		[]byte(payload), "explicit_structured", nil, nil,
		"metric", "m1", int64(416), []byte("null"), nil,
		status, nil, nil, nil,
		nil, now, now, nil, now, nil,
	)
}

// expectRunOverOneMetricCandidate sets up the full sqlmock sequence for one
// AssociateSemantics.Run pass over a single 'candidate' metric decision
// candidate whose value_range_type_lookup tag is lookupTag, through
// processOne's candidate->in_review transition and processMetric's
// !supported defer path: recoverDeferredCandidates (empty), the main
// candidate listing, and every GetByID/UPDATE round trip TransitionStatus
// and DeferCandidate perform internally.
func expectRunOverOneMetricCandidate(mock sqlmock.Sqlmock, id int64, lookupTag string) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, source_artifact_type, proposed_payload, dependency_fingerprint")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "source_artifact_type", "proposed_payload", "dependency_fingerprint"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM kb.semantic_decision_candidates\nWHERE status IN ('candidate', 'in_review')")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))

	selectByID := regexp.QuoteMeta("SELECT " + decisionCandidateColumns + "\n" + decisionCandidateFrom + "\nWHERE id = $1")
	// processOne's initial fetch.
	mock.ExpectQuery(selectByID).WillReturnRows(decisionCandidateRow(id, StatusCandidate, lookupTag))
	// TransitionStatus(candidate -> in_review): legality check, UPDATE, re-fetch.
	mock.ExpectQuery(selectByID).WillReturnRows(decisionCandidateRow(id, StatusCandidate, lookupTag))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.semantic_decision_candidates\nSET status = $2")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(selectByID).WillReturnRows(decisionCandidateRow(id, StatusInReview, lookupTag))
	// DeferCandidate(in_review -> deferred): legality check, UPDATE, re-fetch.
	mock.ExpectQuery(selectByID).WillReturnRows(decisionCandidateRow(id, StatusInReview, lookupTag))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.semantic_decision_candidates\nSET status = 'deferred'")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(selectByID).WillReturnRows(decisionCandidateRow(id, StatusDeferred, lookupTag))
}

// TestRunFailsWhenCandidateBlockedOnProposedValueRangeType locks in ADR
// 2026081401 DR3: a candidate deferred specifically because its
// value_range_type_lookup tag is "proposed" makes Run report a MappingMiss
// and return a non-nil aggregate error -- the candidate itself still ends up
// deferred exactly as an ordinary deferral would.
func TestRunFailsWhenCandidateBlockedOnProposedValueRangeType(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectRunOverOneMetricCandidate(mock, 101, "proposed")

	report, err := (AssociateSemantics{DB: db}).Run(context.Background(), 416)
	if err == nil {
		t.Fatal("expected a non-nil error when a candidate is blocked on ungoverned vocabulary")
	}
	if report.MappingMisses != 1 {
		t.Fatalf("MappingMisses = %d, want 1", report.MappingMisses)
	}
	if report.Deferred != 1 {
		t.Fatalf("Deferred = %d, want 1 (a mapping miss is still a deferral)", report.Deferred)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRunSucceedsWhenCandidateDeferredForAmbiguousValueRangeType locks in DR3:
// a candidate deferred for a reason OTHER than "proposed" -- here the lookup
// tag is "ambiguous" -- must not count as a MappingMiss and must not fail
// the run, matching today's behavior for ordinary deferrals.
func TestRunSucceedsWhenCandidateDeferredForAmbiguousValueRangeType(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectRunOverOneMetricCandidate(mock, 102, "ambiguous")

	report, err := (AssociateSemantics{DB: db}).Run(context.Background(), 416)
	if err != nil {
		t.Fatalf("unexpected error for an ambiguous (non-proposed) deferral: %v", err)
	}
	if report.MappingMisses != 0 {
		t.Fatalf("MappingMisses = %d, want 0", report.MappingMisses)
	}
	if report.Deferred != 1 {
		t.Fatalf("Deferred = %d, want 1", report.Deferred)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// evidenceRow builds one full-column kb.assertion_evidence RETURNING row,
// column order matching evidenceColumns exactly.
func evidenceRow(id, assertionID int64) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "assertion_id", "input_record_id", "artifact_type", "artifact_id",
		"artifact_object_id", "evidence_quote", "source_line_spans", "extraction_run",
		"model", "prompt_version", "confidence", "evidence_role", "actor_kind", "deleted",
		"deleted_reason", "deleted_time", "create_time", "create_by",
	}).AddRow(
		id, assertionID, int64(416), "metric", "m1",
		"obj_1", "250", []byte(`[{"start":10,"end":12}]`), "evt-abc123",
		"deepseek-chat", "prompt-extract-metrics-v3", 0.87, "supports", "processor", false,
		nil, nil, now, "associate_semantics",
	)
}

// TestProcessMetricAcceptedPathPopulatesEvidenceProvenanceFields locks in the
// associate_semantics fix for kb.assertion_evidence's previously-blank
// provenance columns (spec 2026072702 §8.3): artifact_object_id,
// source_line_spans, extraction_run, model, prompt_version, and the evidence
// row's own confidence must all carry through from the decision candidate/
// payload into the evidence INSERT, not just onto the assertion.
func TestProcessMetricAcceptedPathPopulatesEvidenceProvenanceFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	confidence := 0.87
	payload := `{"metric_id":"m1","subject_object_id":"obj_1","assertion_kind":"exact_value","raw_text":"250","extraction_run":"evt-abc123","model":"deepseek-chat","prompt_version":"prompt-extract-metrics-v3"}`
	dc := DecisionCandidate{
		ID:                 42,
		LogicalIdentityKey: "metric:416:m1",
		SourceArtifactType: "metric",
		SourceArtifactID:   "m1",
		ProposedPayload:    []byte(payload),
		SourceLineSpans:    []byte(`[{"start":10,"end":12}]`),
		Confidence:         &confidence,
	}

	// termExists(mea:measured_by), termExists(mea:exact_value)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (\n\tSELECT 1 FROM kb.ontology_terms")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (\n\tSELECT 1 FROM kb.ontology_terms")).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// dcStore.SetResolution: UPDATE + refetch.
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.semantic_decision_candidates\nSET resolution_outcome")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT " + decisionCandidateColumns + "\n" + decisionCandidateFrom + "\nWHERE id = $1")).
		WillReturnRows(decisionCandidateRow(42, StatusInReview, "approved"))

	// persistAssertion: GetLatest finds nothing -> CreateAssertion.
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE logical_identity_key = $1")).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semantic_assertions")).
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusCandidate))

	// evStore.AddEvidence: INSERT ... RETURNING, args pinned to prove every
	// provenance field reaches the statement.
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.assertion_evidence")).
		WithArgs(
			int64(1), int64(416), "metric", "m1",
			"obj_1", "250", `[{"start":10,"end":12}]`,
			"evt-abc123", "deepseek-chat", "prompt-extract-metrics-v3",
			0.87, "supports", "processor", "associate_semantics",
		).
		WillReturnRows(evidenceRow(1, 1))
	// AddEvidence's restoreIfUnsupported check.
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusCandidate))

	// asStore.TransitionStatus(1, InReview): fetch, update, refetch.
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusCandidate))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.semantic_assertions")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusInReview))

	// asStore.TransitionStatus(1, Accepted): fetch, update, refetch.
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusInReview))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.semantic_assertions")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusAccepted))

	// dcStore.SetResultingAssertion: UPDATE + refetch.
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.semantic_decision_candidates\nSET resulting_assertion_id")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT " + decisionCandidateColumns + "\n" + decisionCandidateFrom + "\nWHERE id = $1")).
		WillReturnRows(decisionCandidateRow(42, StatusInReview, "approved"))

	// dcStore.TransitionStatus(42, Accepted): fetch, update, refetch.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT " + decisionCandidateColumns + "\n" + decisionCandidateFrom + "\nWHERE id = $1")).
		WillReturnRows(decisionCandidateRow(42, StatusInReview, "approved"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.semantic_decision_candidates\nSET status")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT " + decisionCandidateColumns + "\n" + decisionCandidateFrom + "\nWHERE id = $1")).
		WillReturnRows(decisionCandidateRow(42, StatusAccepted, "approved"))

	a := AssociateSemantics{DB: db}
	dcStore := DecisionCandidateStore{DB: db}
	asStore := AssertionStore{DB: db}
	evStore := EvidenceStore{DB: db, Assertions: asStore}

	outcome, err := a.processMetric(context.Background(), dcStore, asStore, evStore, dc, 416)
	if err != nil {
		t.Fatalf("processMetric: %v", err)
	}
	if outcome != "accepted" {
		t.Fatalf("outcome = %q, want accepted", outcome)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
