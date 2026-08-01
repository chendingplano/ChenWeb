package assertions

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestProjectSemanticsRunRequiresDB(t *testing.T) {
	p := ProjectSemantics{}
	if _, err := p.Run(context.Background(), 1); err == nil {
		t.Fatal("expected error when db is nil")
	}
}

// TestProjectSemanticsRunMarksTargetStaleOnBuildFailure locks in the P3-
// review fix for finding 2d: a build failure during Run must mark the
// target's projection stale, not merely increment an error counter that
// nothing else observes. Exercises the real registered classification
// projection kind (registered via classification_projection.go's init) end
// to end through ProjectSemantics.Run's registry-driven loop.
func TestProjectSemanticsRunMarksTargetStaleOnBuildFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// classificationTargetsForRecord: one object touched by this record.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT DISTINCT a.subject_object_id\nFROM kb.semantic_assertions a")).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"subject_object_id"}).AddRow("obj-1"))

	// buildPrimaryClassProjection("obj-1") -> primaryClassificationFor finds
	// a classification, then the materializing UPDATE fails.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT object_ref_id, id, revision\nFROM kb.semantic_assertions")).
		WithArgs("obj-1").
		WillReturnRows(sqlmock.NewRows([]string{"object_ref_id", "id", "revision"}).AddRow("mea:some_class", int64(5), 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.object_nodes SET primary_class_term_id = $2 WHERE object_id = $1")).
		WithArgs("obj-1", "mea:some_class").
		WillReturnError(sqlmock.ErrCancelled)

	// Run must mark the target stale on that failure.
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.projection_state\nSET stale = TRUE")).
		WithArgs(ProjectionKindObjectPrimaryClass, "kb.object_nodes", "obj-1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	p := ProjectSemantics{DB: db}
	report, err := p.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.TargetsExamined != 1 || report.Errors != 1 || report.Built != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v (MarkStale was not called on build failure)", err)
	}
}
