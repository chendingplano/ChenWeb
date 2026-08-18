package classfoundation

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestClassResolutionDecisionStoreRecordIfChangedInsertsWhenNoPriorDecision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_class_resolution_decisions")).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_class_resolution_decisions")).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_artifact_type", "source_artifact_id", "source_input_record_id", "source_assertion_id",
			"selected_class_term_id", "identity_state", "method", "confidence", "evidence", "supersedes_decision_id", "create_by",
		}).AddRow(int64(1), "metric", "m1", nil, nil, "measurement:auto:kwc_1", ResolutionResolvedExisting, "deterministic", nil, "{}", nil, "writer"))

	store := ClassResolutionDecisionStore{DB: db}
	decision, changed, err := store.RecordIfChanged(context.Background(), ClassResolutionDecision{
		SourceArtifactType:  "metric",
		SourceArtifactID:    "m1",
		SelectedClassTermID: "measurement:auto:kwc_1",
		IdentityState:       ResolutionResolvedExisting,
		Method:              "deterministic",
		CreateBy:            "writer",
	}, nil)
	if err != nil {
		t.Fatalf("RecordIfChanged: %v", err)
	}
	if !changed || decision.ID != 1 {
		t.Fatalf("decision=%#v changed=%v", decision, changed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClassResolutionDecisionStoreRecordIfChangedSkipsUnchangedSelection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_class_resolution_decisions")).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_artifact_type", "source_artifact_id", "source_input_record_id", "source_assertion_id",
			"selected_class_term_id", "identity_state", "method", "confidence", "evidence", "supersedes_decision_id", "create_by",
		}).AddRow(int64(7), "metric", "m1", nil, nil, "measurement:auto:kwc_1", ResolutionResolvedExisting, "deterministic", nil, "{}", nil, "writer"))

	store := ClassResolutionDecisionStore{DB: db}
	decision, changed, err := store.RecordIfChanged(context.Background(), ClassResolutionDecision{
		SourceArtifactType:  "metric",
		SourceArtifactID:    "m1",
		SelectedClassTermID: "measurement:auto:kwc_1",
		IdentityState:       ResolutionResolvedExisting,
		Method:              "deterministic",
	}, nil)
	if err != nil {
		t.Fatalf("RecordIfChanged: %v", err)
	}
	if changed || decision.ID != 7 {
		t.Fatalf("decision=%#v changed=%v, want unchanged reuse of id 7", decision, changed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClassResolutionDecisionStoreRecordIfChangedSupersedesOnDifferentSelection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_class_resolution_decisions")).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_artifact_type", "source_artifact_id", "source_input_record_id", "source_assertion_id",
			"selected_class_term_id", "identity_state", "method", "confidence", "evidence", "supersedes_decision_id", "create_by",
		}).AddRow(int64(7), "metric", "m1", nil, nil, "measurement:auto:kwc_1", ResolutionResolvedExisting, "deterministic", nil, "{}", nil, "writer"))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_class_resolution_decisions")).
		WithArgs("metric", "m1", nil, nil, "measurement:auto:kwc_2", ResolutionProvisionalNew, "deterministic", nil, "{}", int64(7), nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_artifact_type", "source_artifact_id", "source_input_record_id", "source_assertion_id",
			"selected_class_term_id", "identity_state", "method", "confidence", "evidence", "supersedes_decision_id", "create_by",
		}).AddRow(int64(8), "metric", "m1", nil, nil, "measurement:auto:kwc_2", ResolutionProvisionalNew, "deterministic", nil, "{}", int64(7), ""))

	store := ClassResolutionDecisionStore{DB: db}
	decision, changed, err := store.RecordIfChanged(context.Background(), ClassResolutionDecision{
		SourceArtifactType:  "metric",
		SourceArtifactID:    "m1",
		SelectedClassTermID: "measurement:auto:kwc_2",
		IdentityState:       ResolutionProvisionalNew,
		Method:              "deterministic",
	}, nil)
	if err != nil {
		t.Fatalf("RecordIfChanged: %v", err)
	}
	if !changed || decision.ID != 8 || decision.SupersedesDecisionID == nil || *decision.SupersedesDecisionID != 7 {
		t.Fatalf("decision=%#v changed=%v", decision, changed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
