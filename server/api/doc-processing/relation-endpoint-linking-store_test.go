package docprocessing

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLoadEntitiesForRecord_CoalescesNullableTextColumns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("CREATE SCHEMA IF NOT EXISTS kb;").
		WillReturnResult(sqlmock.NewResult(0, 0))

	rows := sqlmock.NewRows([]string{
		"entity_id", "entity", "entity_en", "entity_type", "entity_type_en",
		"coalesce", "coalesce", "coalesce", "coalesce", "coalesce",
	}).AddRow("e1", "Acme", "", "", "", []byte(`[]`), []byte(`[]`), []byte(`["10:12"]`), []byte(`[]`), "extracted")
	mock.ExpectQuery("SELECT COALESCE\\(entity_id, ''\\), COALESCE\\(entity, ''\\), COALESCE\\(entity_en, ''\\),").
		WithArgs(int64(415)).
		WillReturnRows(rows)

	store := EntityRelationSQLStore{DB: db}
	got, err := store.LoadEntitiesForRecord(context.Background(), 415)
	if err != nil {
		t.Fatalf("LoadEntitiesForRecord: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	if got[0]["entity_en"] != "" {
		t.Fatalf("entity_en=%v, want empty string", got[0]["entity_en"])
	}
	if got[0]["entity_type"] != "" {
		t.Fatalf("entity_type=%v, want empty string", got[0]["entity_type"])
	}
	if got[0]["entity_type_en"] != "" {
		t.Fatalf("entity_type_en=%v, want empty string", got[0]["entity_type_en"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestLoadRelationsForRecord_CoalescesNullableTextColumns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("CREATE SCHEMA IF NOT EXISTS kb;").
		WillReturnResult(sqlmock.NewResult(0, 0))

	rows := sqlmock.NewRows([]string{
		"relation_id", "subject", "subject_en", "predicate", "predicate_en", "object", "object_en",
		"desc_text", "desc_text_en", "coalesce", "coalesce", "coalesce", "coalesce",
		"coalesce", "coalesce", "coalesce", "coalesce", "coalesce",
	}).AddRow("r1", "Acme", "", "uses", "", "Widget", "", "", "",
		[]byte(`[]`), []byte(`[]`), []byte(`["10:12"]`), []byte(`[]`),
		[]byte(`["10"]`), []byte(`["11"]`), []byte(`["12"]`), 0.75, "en")
	mock.ExpectQuery("SELECT COALESCE\\(relation_id, ''\\), COALESCE\\(subject, ''\\), COALESCE\\(subject_en, ''\\),").
		WithArgs(int64(415)).
		WillReturnRows(rows)

	store := EntityRelationSQLStore{DB: db}
	got, err := store.LoadRelationsForRecord(context.Background(), 415)
	if err != nil {
		t.Fatalf("LoadRelationsForRecord: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got)=%d, want 1", len(got))
	}
	if got[0]["subject_en"] != "" {
		t.Fatalf("subject_en=%v, want empty string", got[0]["subject_en"])
	}
	if got[0]["predicate_en"] != "" {
		t.Fatalf("predicate_en=%v, want empty string", got[0]["predicate_en"])
	}
	if got[0]["object_en"] != "" {
		t.Fatalf("object_en=%v, want empty string", got[0]["object_en"])
	}
	if got[0]["desc"] != "" {
		t.Fatalf("desc=%v, want empty string", got[0]["desc"])
	}
	if got[0]["desc_en"] != "" {
		t.Fatalf("desc_en=%v, want empty string", got[0]["desc_en"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
