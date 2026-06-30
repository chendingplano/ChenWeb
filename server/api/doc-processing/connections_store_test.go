package docprocessing

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLoadConnectionsBySource(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	cols := []string{
		"source_record_id", "target_record_id", "source_type", "source_id",
		"target_type", "target_id", "relation_name", "relation_method",
		"confidence", "source_desc", "target_desc",
	}
	rows := sqlmock.NewRows(cols).
		AddRow(int64(1), int64(2), "metric", "1_m_1", "metric", "2_m_9",
			RelationSemanticallyRelated, RelationMethodHybridSearch, 0.42, "", "metric:2_m_9")

	mock.ExpectQuery("FROM kb.artifact_connections").
		WithArgs("metric", int64(1), RelationMethodHybridSearch, "metric").
		WillReturnRows(rows)

	store := &ConnectionSQLStore{DB: db}
	got, err := store.LoadConnectionsBySource(context.Background(), 1, "metric", RelationMethodHybridSearch, "metric")
	if err != nil {
		t.Fatalf("LoadConnectionsBySource: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d connections, want 1", len(got))
	}
	c := got[0]
	if c.SourceID != "1_m_1" || c.TargetID != "2_m_9" || c.TargetRecordID != 2 {
		t.Errorf("unexpected endpoints: %+v", c)
	}
	if c.RelationName != RelationSemanticallyRelated || c.RelationMethod != RelationMethodHybridSearch {
		t.Errorf("unexpected relation: %+v", c)
	}
	if c.Confidence != 0.42 {
		t.Errorf("confidence = %v, want 0.42", c.Confidence)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestLoadConnectionsBySource_OptionalFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	// relationMethod and targetType empty -> only source_type + source_record_id args.
	mock.ExpectQuery("FROM kb.artifact_connections").
		WithArgs("entity", int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{
			"source_record_id", "target_record_id", "source_type", "source_id",
			"target_type", "target_id", "relation_name", "relation_method",
			"confidence", "source_desc", "target_desc",
		}))

	store := &ConnectionSQLStore{DB: db}
	got, err := store.LoadConnectionsBySource(context.Background(), 5, "entity", "", "")
	if err != nil {
		t.Fatalf("LoadConnectionsBySource: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d, want 0", len(got))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestLoadConnectionsBySource_RejectsEmptySourceType(t *testing.T) {
	store := &ConnectionSQLStore{DB: nil}
	if _, err := store.LoadConnectionsBySource(context.Background(), 1, "", "", ""); err == nil {
		t.Fatal("expected error for nil db / empty source type")
	}
}
