package searchengine

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLoadKeepsOrderAndSkipsMissingRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cols := []string{"artifact_type", "artifact_id", "input_record_id", "primary_label", "secondary_label",
		"source_title", "source_filename", "source_line_spans", "semantic_payload", "keywords", "snippet"}
	// The database returns the rows in its own order; chunk/9 has no row.
	mock.ExpectQuery(`FROM kb.search_artifacts sa`).WillReturnRows(sqlmock.NewRows(cols).
		AddRow("provision", "p1", int64(7), "", "", "Std B", "", []byte("[]"), []byte("{}"), "{}", "provision text").
		AddRow("chunk", "3", int64(5), "", "", "Std A", "", []byte("[]"), []byte("{}"), "{}", "chunk text"))

	got, err := Load(context.Background(), db, Config{Backend: BackendParadeDB},
		[]ArtifactRef{{Type: "chunk", ID: "3"}, {Type: "chunk", ID: "9"}, {Type: "provision", ID: "p1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ArtifactID != "3" || got[1].ArtifactID != "p1" {
		t.Fatalf("want chunk/3 then provision/p1, got %+v", got)
	}
	if got[0].Rank != 0 || got[0].Snippet != "chunk text" || got[0].InputRecordID != 5 || got[0].SourceTitle != "Std A" {
		t.Fatalf("display fields not loaded or rank set: %+v", got[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
