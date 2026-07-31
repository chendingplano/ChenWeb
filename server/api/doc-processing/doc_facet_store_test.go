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
