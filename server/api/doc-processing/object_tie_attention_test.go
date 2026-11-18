package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/attention"
)

type fakeAttentionRecorder struct {
	mu      sync.Mutex
	reports []attention.Report
	err     error
}

func (f *fakeAttentionRecorder) Record(_ context.Context, r attention.Report) (attention.RecordResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reports = append(f.reports, r)
	if f.err != nil {
		return attention.RecordResult{}, f.err
	}
	return attention.RecordResult{ID: 41, Created: len(f.reports) == 1}, nil
}

func candidate(id string, score float64, names ...string) ObjectNodeCandidate {
	method := "lexical_name"
	if score >= 1 {
		method = "exact_name"
	}
	return ObjectNodeCandidate{Node: ObjectNode{ObjectID: id, CanonicalName: id, NormalizedNames: names}, Score: score, Method: method}
}

func TestReconcileOneUsesHighestScoreFirst(t *testing.T) {
	store := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{candidate("obj_x", 0.85), candidate("obj_y", 1)}}
	got, err := ObjectReconciler{Store: store}.ReconcileOne(context.Background(), ArtifactObject{ObjectName: "pump"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ObjectReconcileMatched || got.ObjectID != "obj_y" || len(store.created) != 0 {
		t.Fatalf("result = %+v created = %d, want match obj_y and no new node", got, len(store.created))
	}
}

func TestReconcileOneDetectsTieBehindLowerScore(t *testing.T) {
	store := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{candidate("obj_a", 0.85), candidate("obj_b", 1), candidate("obj_c", 1)}}
	got, err := ObjectReconciler{Store: store}.ReconcileOne(context.Background(), ArtifactObject{ObjectName: "pump"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ObjectReconcileAmbiguous || got.Confidence != 1 {
		t.Fatalf("result = %+v, want ambiguous at 1.0", got)
	}
}

func TestTieBreakIgnoresLowerScoredCandidates(t *testing.T) {
	// obj_a shares both names with the object but scores lower than the tie.
	store := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{
		candidate("obj_a", 0.85, "pump", "feed pump"),
		candidate("obj_c", 1),
		candidate("obj_b", 1),
	}}
	rec := &fakeAttentionRecorder{}
	objects := []ArtifactObject{{InputRecordID: 3, ArtifactType: searchArtifactMetric, ArtifactID: "3_mtc_1", ObjectName: "pump", NormalizedNames: []string{"pump", "feed pump"}}}
	got, err := reconcileArtifactObjects(context.Background(), objects, ObjectReconciler{Store: store, Attention: rec}, &fakeLogger{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ObjectID != "obj_b" || got[0].ReconcileStatus != ObjectReconcileAmbiguousResolved {
		t.Fatalf("object = %+v, want tie-break among the 1.0 candidates (obj_b)", got[0])
	}
	if len(rec.reports) != 1 || rec.reports[0].DedupeKey != "obj_b|obj_c" {
		t.Fatalf("reports = %+v, want tied set obj_b|obj_c", rec.reports)
	}
}

func TestReconcileTieAfterLLM(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resolver fakeAmbiguousObjectLLMResolver
		want     string
	}{
		{"declines", fakeAmbiguousObjectLLMResolver{decision: AmbiguousObjectLLMDecision{ResolutionObjectID: "obj_a", ResolutionConfidence: 0.5}}, tieLLMUnresolved},
		{"fails", fakeAmbiguousObjectLLMResolver{err: errors.New("model unavailable")}, tieLLMFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{candidate("obj_a", 0.85), candidate("obj_b", 0.85)}}
			rec := &fakeAttentionRecorder{}
			objects := []ArtifactObject{{InputRecordID: 5, ArtifactType: searchArtifactMetric, ArtifactID: "5_mtc_2", ObjectName: "钢印"}}
			got, err := reconcileArtifactObjectsWithLLM(context.Background(), objects, ObjectReconciler{Store: store, Attention: rec},
				&fakeLogger{}, tc.resolver, 0.85, objectReconcileLogSink{})
			if err != nil {
				t.Fatal(err)
			}
			if got[0].ObjectID != "obj_a" || got[0].ReconcileStatus != ObjectReconcileAmbiguousResolved {
				t.Fatalf("object = %+v, want tie-break link", got[0])
			}
			if len(rec.reports) != 1 || rec.reports[0].Details["llm_outcome"] != tc.want {
				t.Fatalf("reports = %+v, want llm_outcome %s", rec.reports, tc.want)
			}
		})
	}
}

func TestReconcileTieRecorderFailureStillLinks(t *testing.T) {
	store := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{candidate("obj_a", 1), candidate("obj_b", 1)}}
	rec := &fakeAttentionRecorder{err: errors.New("db down")}
	logger := &fakeLogger{}
	objects := []ArtifactObject{{InputRecordID: 7, ArtifactType: searchArtifactProvision, ArtifactID: "7_prv_1", ObjectName: "管路"}}
	got, err := reconcileArtifactObjects(context.Background(), objects, ObjectReconciler{Store: store, Attention: rec}, logger)
	if err != nil {
		t.Fatalf("reconcile must not fail when recording fails: %v", err)
	}
	if got[0].ObjectID != "obj_a" || got[0].ReconcileStatus != ObjectReconcileAmbiguousResolved {
		t.Fatalf("object = %+v, want tie-break link", got[0])
	}
	if len(logger.errors) != 1 || logger.errors[0].message != "object reconciliation: recording attention item failed (ChenWeb#18)" {
		t.Fatalf("errors = %+v, want one recording failure", logger.errors)
	}
}

func TestReconcileSameTieTwiceUsesOneKey(t *testing.T) {
	store := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{candidate("obj_285_x", 1), candidate("obj_279_y", 1)}}
	rec := &fakeAttentionRecorder{}
	objects := []ArtifactObject{
		{InputRecordID: 279, ArtifactType: searchArtifactMetric, ArtifactID: "279_mtc_1", ObjectName: "管路"},
		{InputRecordID: 279, ArtifactType: searchArtifactProvision, ArtifactID: "279_prv_4", ObjectName: "管路系统"},
	}
	if _, err := reconcileArtifactObjects(context.Background(), objects, ObjectReconciler{Store: store, Attention: rec}, &fakeLogger{}); err != nil {
		t.Fatal(err)
	}
	if len(rec.reports) != 2 || rec.reports[0].DedupeKey != "obj_279_y|obj_285_x" || rec.reports[1].DedupeKey != rec.reports[0].DedupeKey {
		t.Fatalf("reports = %+v, want both on obj_279_y|obj_285_x", rec.reports)
	}
}

func TestFindTieOccurrenceRowSkipsMissingOccurrences(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	q := regexp.QuoteMeta("SELECT id FROM kb.artifact_objects")
	mock.ExpectQuery(q).WithArgs(int64(400), "metric", "400_mtc_1", "管路").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(q).WithArgs(int64(279), "metric", "279_mtc_1", "管路").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(77)))

	id, found, err := ArtifactObjectSQLStore{DB: db}.FindTieOccurrenceRow(context.Background(), []TieOccurrence{
		{InputRecordID: 400, ArtifactType: "metric", ArtifactID: "400_mtc_1", ObjectName: "管路"},
		{InputRecordID: 279, ArtifactType: "metric", ArtifactID: "279_mtc_1", ObjectName: "管路"},
	})
	if err != nil || !found || id != 77 {
		t.Fatalf("id = %d found = %v err = %v", id, found, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
