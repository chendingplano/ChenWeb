package docprocessing

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSQLStoreUpsertDocFacetsInsertsOnConflictUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	stmt := regexp.QuoteMeta(`
INSERT INTO kb.doc_facets (record_id, ks_store_id, knowledge_store_binding, input_doc_type, source_language, has_document_number)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (record_id) DO UPDATE SET
    ks_store_id = EXCLUDED.ks_store_id,
    knowledge_store_binding = EXCLUDED.knowledge_store_binding,
    input_doc_type = EXCLUDED.input_doc_type,
    source_language = EXCLUDED.source_language,
    has_document_number = EXCLUDED.has_document_number,
    modify_time = NOW()`)

	mock.ExpectExec(stmt).
		WithArgs(int64(91), int64(42), "bound", "pdf", "en", true).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := SQLStore{DB: db}
	err = store.UpsertDocFacets(context.Background(), DocFacetRecord{
		RecordID:              91,
		KSStoreID:             42,
		KnowledgeStoreBinding: "bound",
		InputDocType:          "pdf",
		SourceLanguage:        "en",
		HasDocumentNumber:     true,
	})
	if err != nil {
		t.Fatalf("UpsertDocFacets: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSQLStoreUpsertDocFacetsRequiresRecordID(t *testing.T) {
	store := SQLStore{DB: nil}
	if err := store.UpsertDocFacets(context.Background(), DocFacetRecord{}); err == nil {
		t.Fatal("expected error for db nil / missing record id")
	}
}

func TestSQLStoreGetDocFacetsReadsRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	stmt := regexp.QuoteMeta(`
SELECT record_id, ks_store_id, knowledge_store_binding, input_doc_type, source_language, has_document_number
FROM kb.doc_facets
WHERE record_id = $1`)
	mock.ExpectQuery(stmt).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{
			"record_id", "ks_store_id", "knowledge_store_binding", "input_doc_type", "source_language", "has_document_number",
		}).AddRow(int64(91), int64(42), "bound", "pdf", "en", true))

	store := SQLStore{DB: db}
	got, err := store.GetDocFacets(context.Background(), 91)
	if err != nil {
		t.Fatalf("GetDocFacets: %v", err)
	}
	want := DocFacetRecord{RecordID: 91, KSStoreID: 42, KnowledgeStoreBinding: "bound", InputDocType: "pdf", SourceLanguage: "en", HasDocumentNumber: true}
	if got != want {
		t.Fatalf("got=%#v want=%#v", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSQLStoreGetDocFacetsReturnsNoRowsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	stmt := regexp.QuoteMeta(`
SELECT record_id, ks_store_id, knowledge_store_binding, input_doc_type, source_language, has_document_number
FROM kb.doc_facets
WHERE record_id = $1`)
	mock.ExpectQuery(stmt).WithArgs(int64(404)).WillReturnError(sql.ErrNoRows)

	store := SQLStore{DB: db}
	if _, err := store.GetDocFacets(context.Background(), 404); err != sql.ErrNoRows {
		t.Fatalf("err=%v want=%v", err, sql.ErrNoRows)
	}
}

func TestSQLStoreInsertFacetObservationIsIdempotentByInvocation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	stmt := regexp.QuoteMeta(`
WITH inserted AS (
    INSERT INTO kb.doc_facet_values (
        record_id, path, value, state, method, confidence, evidence, source_fingerprint,
        decision_attempt_id, invocation_id, vocabulary_release_id
    )
    VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7::jsonb, $8, $9, $10, $11)
    ON CONFLICT (record_id, path, decision_attempt_id, invocation_id) DO NOTHING
    RETURNING id, record_id, path, value, state, method, confidence, evidence, source_fingerprint,
              decision_attempt_id, invocation_id, vocabulary_release_id
)
SELECT id, record_id, path, value, state, method, confidence, evidence, source_fingerprint,
       decision_attempt_id, invocation_id, vocabulary_release_id
FROM inserted
UNION ALL
SELECT id, record_id, path, value, state, method, confidence, evidence, source_fingerprint,
       decision_attempt_id, invocation_id, vocabulary_release_id
FROM kb.doc_facet_values
WHERE record_id = $1 AND path = $2 AND decision_attempt_id = $9 AND invocation_id = $10
  AND NOT EXISTS (SELECT 1 FROM inserted)`)
	mock.ExpectQuery(stmt).
		WithArgs(int64(91), "document.doc_kind", `"standard"`, "known", "classifier", 0.8, `{"line":7}`, "source-sha", "attempt-1", "invoke-1", int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "record_id", "path", "value", "state", "method", "confidence", "evidence", "source_fingerprint",
			"decision_attempt_id", "invocation_id", "vocabulary_release_id",
		}).AddRow(int64(7), int64(91), "document.doc_kind", []byte(`"standard"`), "known", "classifier", 0.8, []byte(`{"line":7}`), "source-sha", "attempt-1", "invoke-1", int64(42)))

	got, err := (SQLStore{DB: db}).InsertFacetObservation(context.Background(), FacetObservation{
		RecordID:            91,
		Path:                "document.doc_kind",
		Value:               "standard",
		State:               "known",
		Method:              FacetMethodClassifier,
		Confidence:          ptrFloat64(0.8),
		Evidence:            map[string]any{"line": float64(7)},
		SourceFingerprint:   "source-sha",
		DecisionAttemptID:   "attempt-1",
		InvocationID:        "invoke-1",
		VocabularyReleaseID: 42,
	})
	if err != nil {
		t.Fatalf("InsertFacetObservation: %v", err)
	}
	if got.ID != 7 || got.Value != "standard" || got.State != "known" || got.VocabularyReleaseID != 42 {
		t.Fatalf("got=%+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSQLStoreListFacetObservationsByRecordAndRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	stmt := regexp.QuoteMeta(`
SELECT id, record_id, path, value, state, method, confidence, evidence, source_fingerprint,
       decision_attempt_id, invocation_id, vocabulary_release_id
FROM kb.doc_facet_values
WHERE record_id = $1 AND vocabulary_release_id = $2
ORDER BY path, method, decision_attempt_id, invocation_id, id`)
	mock.ExpectQuery(stmt).WithArgs(int64(91), int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "record_id", "path", "value", "state", "method", "confidence", "evidence", "source_fingerprint",
			"decision_attempt_id", "invocation_id", "vocabulary_release_id",
		}).AddRow(int64(7), int64(91), "document.doc_kind", []byte(`"standard"`), "known", "classifier", 0.8, []byte(`{"line":7}`), "source-sha", "attempt-1", "invoke-1", int64(42)))

	got, err := (SQLStore{DB: db}).ListFacetObservations(context.Background(), 91, 42)
	if err != nil {
		t.Fatalf("ListFacetObservations: %v", err)
	}
	if len(got) != 1 || got[0].Path != "document.doc_kind" || got[0].ReleaseIDString() != "42" {
		t.Fatalf("got=%+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
