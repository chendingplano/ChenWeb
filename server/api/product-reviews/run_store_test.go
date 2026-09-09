package productreviews

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Re-run creates the next run_number under the same request (spec: Re-run and diff).
func TestCreateRunNextNumber(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(rx("INSERT INTO kb.product_review_runs")).
		WithArgs(int64(100)).
		WillReturnRows(runReturnRow(501, 100, 2, "pending"))

	run, err := (RunStore{DB: db}).CreateRun(context.Background(), 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.RunNumber != 2 || run.Status != RunPending {
		t.Fatalf("run = %+v, want run_number 2 / pending", run)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// SaveResults replaces the run's rows (spec: Result persistence with provenance).
func TestSaveResultsReplaces(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(rx("DELETE FROM kb.product_review_results")).WithArgs(int64(500)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(rx("INSERT INTO kb.product_review_results")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	n2 := int64(2)
	err = (RunStore{DB: db}).SaveResults(context.Background(), 500, []Result{
		{ArtifactType: "metric", ArtifactID: "42_mtc_1", SourceRowID: 900, InputRecordID: 42,
			NodeID: &n2, Tier: TierPart, Score: 1.2, Paths: []string{"direct"}, InclusionReason: "x"},
	})
	if err != nil {
		t.Fatalf("SaveResults: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// Scenario: Reviewer inspects why a document was in scope (spec: Scoped document
// set persistence) — a doc scoped only via a storage_requirement product record
// shows the storage aspect node as its matching node.
func TestSaveAndLoadScopedDocs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	store := RunStore{DB: db}

	mock.ExpectBegin()
	mock.ExpectExec(rx("DELETE FROM kb.product_review_run_documents")).WithArgs(int64(500)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(rx("INSERT INTO kb.product_review_run_documents")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := store.SaveScopedDocs(context.Background(), 500, []ScopedDoc{{
		InputRecordID: 60, FusedScore: 0.6, NodeIDs: []int64{9},
		Paths: []string{"product_record"}, Reasons: []string{"storage aspect: relation storage_requirement"},
	}}); err != nil {
		t.Fatalf("SaveScopedDocs: %v", err)
	}

	mock.ExpectQuery(rx("FROM kb.product_review_run_documents")).WithArgs(int64(500)).
		WillReturnRows(sqlmock.NewRows([]string{
			"input_record_id", "fused_score", "matching_node_ids", "matching_paths", "match_reasons", "doc_kind",
		}).AddRow(int64(60), 0.6, []byte(`[9]`), []byte(`["product_record"]`),
			[]byte(`["storage aspect: relation storage_requirement"]`), ""))
	docs, err := store.LoadScopedDocs(context.Background(), 500)
	if err != nil {
		t.Fatalf("LoadScopedDocs: %v", err)
	}
	if len(docs) != 1 || !containsInt(docs[0].NodeIDs, 9) {
		t.Fatalf("scoped doc = %+v, want NodeIDs to include the storage aspect node (9)", docs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// Results are readable with the document_scope tier excluded (spec: Results
// filtered by tier).
func TestLoadResultsExcludeTier(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(rx("tier <> $2")).
		WithArgs(int64(500), TierDocumentScope).
		WillReturnRows(sqlmock.NewRows([]string{
			"artifact_type", "artifact_id", "source_row_id", "input_record_id",
			"node_id", "tier", "score", "paths", "inclusion_reason", "source_line_spans",
		}).AddRow("metric", "42_mtc_1", int64(900), int64(42), int64(2), TierPart, 1.2, []byte(`["direct"]`), "x", []byte(`[]`)))

	got, err := (RunStore{DB: db}).LoadResults(context.Background(), 500, ResultFilters{
		ExcludeTiers: []string{TierDocumentScope},
	})
	if err != nil {
		t.Fatalf("LoadResults: %v", err)
	}
	if len(got) != 1 || got[0].Tier != TierPart || got[0].NodeID == nil {
		t.Fatalf("got %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
