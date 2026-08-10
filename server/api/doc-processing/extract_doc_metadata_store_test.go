package docprocessing

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDocMetadataSQLStoreGetInputRecordLoadsKnowledgeStoreID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	query := regexp.QuoteMeta(`
SELECT i.id,
       COALESCE(i.ks_store_id, 0),
       COALESCE(i.requested_pipeline, ''),
       COALESCE(bp.name, ''),
       COALESCE(i.type, ''),
       COALESCE(i.doc_metadata->'metadata'->>'language', i.doc_metadata->>'language', ''),
       COALESCE(NULLIF(BTRIM(i.doc_metadata->>'doc_no'), ''), NULLIF(BTRIM(i.doc_no), ''), ''),
       COALESCE(i.parser_name, ''),
       COALESCE(i.result_filename, ''),
       COALESCE(i.staging_filename, ''),
       COALESCE(i.status::text, '[]'),
       COALESCE(i.file_name, ''),
       COALESCE(i.title, ''),
       COALESCE(i.doc_metadata::text, ''),
       COALESCE(i.publish_date::text, '')
FROM kb.inputs i
LEFT JOIN kb.pipeline_bindings pb ON pb.ks_store_id = i.ks_store_id
    AND pb.active
LEFT JOIN kb.pipelines bp ON bp.id = pb.pipeline_id
WHERE i.id = $1`)

	mock.ExpectQuery(query).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "ks_store_id", "requested_pipeline", "bound_pipeline_name", "type", "source_language", "document_number", "parser_name", "result_filename", "staging_filename", "status", "file_name", "title", "doc_metadata", "publish_date",
		}).AddRow(
			int64(91),
			int64(42),
			"narrative_default",
			"store_default",
			"pdf",
			"en",
			"YY 9706.252-2021",
			"benchmark",
			"result.json",
			"staging.pdf",
			"[]",
			"input.pdf",
			"Ventilator display module",
			`{"title":"Ventilator display module"}`,
			"2021-06-01",
		))

	store := DocMetadataSQLStore{DB: db}
	rec, err := store.GetInputRecord(context.Background(), 91)
	if err != nil {
		t.Fatalf("GetInputRecord: %v", err)
	}

	if got, want := rec.KSStoreID, int64(42); got != want {
		t.Fatalf("KSStoreID=%d want=%d", got, want)
	}
	if got, want := rec.RequestedPipeline, "narrative_default"; got != want {
		t.Fatalf("RequestedPipeline=%q want=%q", got, want)
	}
	if got, want := rec.StoreBoundPipeline, "store_default"; got != want {
		t.Fatalf("StoreBoundPipeline=%q want=%q", got, want)
	}
	if got, want := rec.InputDocType, "pdf"; got != want {
		t.Fatalf("InputDocType=%q want=%q", got, want)
	}
	if got, want := rec.SourceLanguage, "en"; got != want {
		t.Fatalf("SourceLanguage=%q want=%q", got, want)
	}
	if got, want := rec.DocumentNumber, "YY 9706.252-2021"; got != want {
		t.Fatalf("DocumentNumber=%q want=%q", got, want)
	}
	if got, want := rec.ParserName, "benchmark"; got != want {
		t.Fatalf("ParserName=%q want=%q", got, want)
	}
	if got, want := rec.Title, "Ventilator display module"; got != want {
		t.Fatalf("Title=%q want=%q", got, want)
	}
	if got, want := rec.PublishDate, "2021-06-01"; got != want {
		t.Fatalf("PublishDate=%q want=%q", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}
