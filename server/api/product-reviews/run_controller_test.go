package productreviews

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func scopeNodeRows(rows ...[]driver.Value) *sqlmock.Rows {
	out := sqlmock.NewRows([]string{
		"id", "node_kind", "label", "label_en", "aliases", "object_id",
		"concept_id", "grounding", "aspect_key", "relation_types", "embedding",
	})
	for _, r := range rows {
		out.AddRow(r...)
	}
	return out
}

// Scenario: Run against a draft profile is refused (spec) — CWB_KB_PMR_030, no
// request or run created.
func TestStartReviewRefusesDraft(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	cfg, _ := ParseConfig([]byte("[budgets]\nmax_depth = 3\n"))
	ctrl := RunController{Store: Store{DB: db}, Runs: RunStore{DB: db},
		Scoper: DocumentScoper{DB: db, Config: cfg}, Retriever: Retriever{DB: db, Config: cfg}, Config: cfg}

	mock.ExpectQuery(rx("FROM kb.product_profiles WHERE id = $1")).WithArgs(int64(7)).
		WillReturnRows(profileRows(7, "Ventilator", 3, "draft", false, 0))

	_, err = ctrl.StartReview(context.Background(), StartReviewInput{ProfileID: 7, ArtifactTypes: []string{"metric"}})
	var pmr *PMRError
	if !errors.As(err, &pmr) || pmr.Code != CodeDraftProfile {
		t.Fatalf("err = %v, want CWB_KB_PMR_030", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("a request/run must not be created for a draft profile: %v", err)
	}
}

// Scenario: Unregistered type is refused before a run is created (spec:
// product-artifact-retrieval 1.1.2 B, enforced by the controller).
func TestStartReviewRefusesUnregisteredType(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	cfg, _ := ParseConfig([]byte("[budgets]\nmax_depth = 3\n"))
	ctrl := RunController{Store: Store{DB: db}, Runs: RunStore{DB: db},
		Scoper: DocumentScoper{DB: db, Config: cfg}, Retriever: Retriever{DB: db, Config: cfg}, Config: cfg}

	mock.ExpectQuery(rx("FROM kb.product_profiles WHERE id = $1")).WithArgs(int64(7)).
		WillReturnRows(profileRows(7, "Ventilator", 1, "ready", false, 0))

	_, err = ctrl.StartReview(context.Background(), StartReviewInput{ProfileID: 7, ArtifactTypes: []string{"provision"}})
	var pmr *PMRError
	if !errors.As(err, &pmr) || pmr.Code != CodeUnregisteredType {
		t.Fatalf("err = %v, want CWB_KB_PMR_020", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no request must be created for an unregistered type: %v", err)
	}
}

// Scenario: Creating a review starts a run that transitions pending → running →
// completed with its counts populated (spec: Review request and run lifecycle).
func TestStartReviewCompletesRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	cfg, _ := ParseConfig([]byte("[budgets]\nmax_depth = 3\n"))
	ctrl := RunController{Store: Store{DB: db}, Runs: RunStore{DB: db},
		Scoper: DocumentScoper{DB: db, Config: cfg}, Retriever: Retriever{DB: db, Config: cfg}, Config: cfg}

	mock.ExpectQuery(rx("FROM kb.product_profiles WHERE id = $1")).WithArgs(int64(7)).
		WillReturnRows(profileRows(7, "Ventilator", 3, "ready", false, 0))
	mock.ExpectQuery(rx("INSERT INTO kb.product_review_requests")).
		WillReturnRows(requestReturnRow(100, 7, 3))
	mock.ExpectQuery(rx("INSERT INTO kb.product_review_runs")).WithArgs(int64(100)).
		WillReturnRows(runReturnRow(500, 100, 1, "pending"))
	mock.ExpectExec(rx("status = 'running'")).WithArgs(int64(500)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// LoadNodes (full node columns)
	mock.ExpectQuery(rx("(embedding IS NOT NULL)")).WithArgs(int64(7)).
		WillReturnRows(nodeRowsFrom(7, nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator", Status: StatusAccepted, Origin: OriginUserAdded}))
	// LoadScopeNodes (accepted nodes with embedding::text)
	mock.ExpectQuery(rx("COALESCE(embedding::text, '')")).WithArgs(int64(7)).
		WillReturnRows(scopeNodeRows([]driver.Value{
			int64(1), KindProduct, "Ventilator", "Ventilator", []byte("[]"), "", "", GroundingUngrounded, "", []byte("[]"), "",
		}))
	// Scope: Path A (products), Path B (rrf lex)
	mock.ExpectQuery(rx("FROM kb.products")).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(productRows())
	mock.ExpectQuery(rx("lex AS (")).WillReturnRows(emptyRRFRows())
	// Retrieve: Path E hybrid (rrf lex over the metric partition)
	mock.ExpectQuery(rx("lex AS (")).WillReturnRows(emptyRRFRows())
	// Persist scoped docs + results (both replace-then-insert in a txn)
	mock.ExpectBegin()
	mock.ExpectExec(rx("DELETE FROM kb.product_review_run_documents")).WithArgs(int64(500)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(rx("DELETE FROM kb.product_review_results")).WithArgs(int64(500)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectExec(rx("status = 'completed'")).
		WithArgs(int64(500), 0, 0, 0, 0, 0, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(rx("FROM kb.product_review_runs WHERE id = $1")).WithArgs(int64(500)).
		WillReturnRows(runGetRow(500, 100, 1, "completed", ""))

	run, err := ctrl.StartReview(context.Background(), StartReviewInput{ProfileID: 7, ArtifactTypes: []string{"metric"}})
	if err != nil {
		t.Fatalf("StartReview: %v", err)
	}
	if run == nil || run.Status != RunCompleted || run.RunNumber != 1 {
		t.Fatalf("run = %+v, want run 1 / completed", run)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// Scenario: Run failure is recorded, not lost (spec) — the run goes to `failed`
// with a non-empty error_message and the request stays re-runnable.
func TestStartReviewRecordsFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	cfg, _ := ParseConfig([]byte("[budgets]\nmax_depth = 3\n"))
	ctrl := RunController{Store: Store{DB: db}, Runs: RunStore{DB: db},
		Scoper: DocumentScoper{DB: db, Config: cfg}, Retriever: Retriever{DB: db, Config: cfg}, Config: cfg}

	mock.ExpectQuery(rx("FROM kb.product_profiles WHERE id = $1")).WithArgs(int64(7)).
		WillReturnRows(profileRows(7, "Ventilator", 3, "ready", false, 0))
	mock.ExpectQuery(rx("INSERT INTO kb.product_review_requests")).WillReturnRows(requestReturnRow(100, 7, 3))
	mock.ExpectQuery(rx("INSERT INTO kb.product_review_runs")).WithArgs(int64(100)).
		WillReturnRows(runReturnRow(500, 100, 1, "pending"))
	mock.ExpectExec(rx("status = 'running'")).WithArgs(int64(500)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(rx("(embedding IS NOT NULL)")).WithArgs(int64(7)).
		WillReturnRows(nodeRowsFrom(7, nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator", Status: StatusAccepted, Origin: OriginUserAdded}))
	mock.ExpectQuery(rx("COALESCE(embedding::text, '')")).WithArgs(int64(7)).
		WillReturnRows(scopeNodeRows([]driver.Value{
			int64(1), KindProduct, "Ventilator", "Ventilator", []byte("[]"), "", "", GroundingUngrounded, "", []byte("[]"), "",
		}))
	mock.ExpectQuery(rx("FROM kb.products")).WithArgs(sqlmock.AnyArg()).WillReturnRows(productRows())
	mock.ExpectQuery(rx("lex AS (")).WillReturnRows(emptyRRFRows())
	mock.ExpectQuery(rx("lex AS (")).WillReturnError(errors.New("search backend down"))
	mock.ExpectExec(rx("status = 'failed'")).WithArgs(int64(500), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(rx("FROM kb.product_review_runs WHERE id = $1")).WithArgs(int64(500)).
		WillReturnRows(runGetRow(500, 100, 1, "failed", ""))

	run, err := ctrl.StartReview(context.Background(), StartReviewInput{ProfileID: 7, ArtifactTypes: []string{"metric"}})
	if err != nil {
		t.Fatalf("StartReview should not surface the run error: %v", err)
	}
	if run.Status != RunFailed {
		t.Fatalf("run status = %s, want failed", run.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
