package assertions

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestEvidenceStoreAddEvidenceRequiresAssertionID(t *testing.T) {
	store := EvidenceStore{DB: nil}
	_, err := store.AddEvidence(context.Background(), Evidence{
		ArtifactType: "metric",
		ArtifactID:   "m1",
	})
	if err == nil {
		t.Fatal("expected error when assertion_id is zero")
	}
}

func TestEvidenceStoreAddEvidenceRejectsBadRole(t *testing.T) {
	store := EvidenceStore{DB: nil}
	_, err := store.AddEvidence(context.Background(), Evidence{
		AssertionID:  1,
		ArtifactType: "metric",
		ArtifactID:   "m1",
		EvidenceRole: "maybe",
	})
	if err == nil {
		t.Fatal("expected error for invalid evidence_role")
	}
}

func TestEvidenceStoreAddEvidenceRequiresArtifactFields(t *testing.T) {
	store := EvidenceStore{DB: nil}
	_, err := store.AddEvidence(context.Background(), Evidence{AssertionID: 1})
	if err == nil {
		t.Fatal("expected error when artifact_type/artifact_id are missing")
	}
}

func TestEvidenceStoreDeleteLastSupportRecordsRepresentedPriorStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT assertion_id, evidence_role FROM kb.assertion_evidence WHERE id = $1 AND NOT deleted")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"assertion_id", "evidence_role"}).AddRow(int64(1), "supports"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.assertion_evidence\nSET deleted = TRUE, deleted_reason = $2, deleted_time = NOW()\nWHERE id = $1")).
		WithArgs(int64(7), "source withdrawn").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.assertion_evidence\nWHERE assertion_id = $1 AND evidence_role = 'supports' AND NOT deleted")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusRepresented))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusRepresented))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.semantic_assertions\nSET status = $2, unsupported_prior_status = $3, decision_reason = $4, modify_time = NOW(), modify_by = $5\nWHERE id = $1")).
		WithArgs(int64(1), StatusUnsupported, StatusRepresented, "last qualifying evidence removed (evidence id 7): source withdrawn", "tester").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusUnsupported))

	store := EvidenceStore{DB: db, Assertions: AssertionStore{DB: db}}
	if err := store.DeleteEvidence(context.Background(), 7, "tester", "source withdrawn"); err != nil {
		t.Fatalf("DeleteEvidence: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceStoreRestoreReturnsRecordedPriorStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(assertionRowWithStatusAndPrior(assertionColumnNames(), StatusUnsupported, StatusRepresented))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(assertionRowWithStatusAndPrior(assertionColumnNames(), StatusUnsupported, StatusRepresented))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.semantic_assertions\nSET status = $2, unsupported_prior_status = NULL, decision_reason = $3, modify_time = NOW(), modify_by = $4\nWHERE id = $1")).
		WithArgs(int64(1), StatusRepresented, "qualifying evidence restored", nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusRepresented))

	store := EvidenceStore{DB: db, Assertions: AssertionStore{DB: db}}
	got, err := store.restoreIfUnsupported(context.Background(), 1)
	if err != nil {
		t.Fatalf("restoreIfUnsupported: %v", err)
	}
	if got.Status != StatusRepresented {
		t.Fatalf("restored status = %q, want %q", got.Status, StatusRepresented)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
