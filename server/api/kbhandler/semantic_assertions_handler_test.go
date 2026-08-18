package kbhandler

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestListSemanticAssertionsFiltersByInputRecordID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.semantic_assertions WHERE 1=1 AND EXISTS")).
		WithArgs(int64(17)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT ").
		WithArgs(int64(17), 50, 0).
		WillReturnRows(sqlmock.NewRows(assertionListColumnNames()))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/semantic-assertions?input_record_id=17", nil)
	rec := httptest.NewRecorder()
	if err := ListSemanticAssertions(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ListSemanticAssertions: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func assertionListColumnNames() []string {
	return []string{
		"id", "logical_identity_key", "revision", "subject_ref_kind", "subject_ref_id",
		"subject_object_id", "predicate_term_id", "object_ref_kind", "object_ref_id",
		"object_object_id", "object_literal", "assertion_kind_term_id", "polarity",
		"modality", "qualifiers", "confidence", "value_form", "numeric_value", "lower_value",
		"upper_value", "lower_inclusive", "upper_inclusive", "comparator", "unit_term_id",
		"quantity_kind_term_id", "raw_text", "status", "unsupported_prior_status",
		"class_identity_state_term_id", "mapping_resolution_state_term_id",
		"value_state_term_id", "conformance_state_term_id", "raw_payload",
		"raw_snapshot_fingerprint", "processing_error_details",
		"normalized_against_contract_revision_id", "decision_reason",
		"dependency_fingerprint", "superseded_by", "valid_time_start", "valid_time_end",
		"transaction_time", "create_time", "create_by", "modify_time", "modify_by",
	}
}
