package docprocessing

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPickTieBreakCandidatePrefersMoreNormalizedNameOverlap(t *testing.T) {
	obj := ArtifactObject{NormalizedNames: []string{"pressure regulator", "reg"}}
	candidates := []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_a", NormalizedNames: []string{"pressure regulator"}}, Score: 0.85},
		{Node: ObjectNode{ObjectID: "obj_b", NormalizedNames: []string{"pressure regulator", "reg"}}, Score: 0.85},
	}

	got := pickTieBreakCandidate(obj, candidates)
	if got.Node.ObjectID != "obj_b" {
		t.Fatalf("picked %q, want obj_b (more normalized-name overlap)", got.Node.ObjectID)
	}
}

func TestPickTieBreakCandidateFallsBackToLexicographicObjectID(t *testing.T) {
	obj := ArtifactObject{NormalizedNames: []string{"pressure regulator"}}
	candidates := []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_z", NormalizedNames: []string{"other"}}, Score: 0.85},
		{Node: ObjectNode{ObjectID: "obj_a", NormalizedNames: []string{"other"}}, Score: 0.85},
	}

	got := pickTieBreakCandidate(obj, candidates)
	if got.Node.ObjectID != "obj_a" {
		t.Fatalf("picked %q, want obj_a (lexicographically smallest on equal overlap)", got.Node.ObjectID)
	}
}

func TestResolveAmbiguousArtifactObjectsAppliesTieBreakWhenStillTied(t *testing.T) {
	store := &fakeAmbiguousObjectStore{rows: []AmbiguousObjectRow{{
		RowID: 1,
		Object: ArtifactObject{
			ArtifactType:    searchArtifactProvision,
			ArtifactID:      "9_prv_1",
			NormalizedNames: []string{"pressure regulator", "reg"},
		},
	}}}
	nodes := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_a", NormalizedNames: []string{"pressure regulator"}}, Score: 0.85, Method: "lexical_name"},
		{Node: ObjectNode{ObjectID: "obj_b", NormalizedNames: []string{"pressure regulator", "reg"}}, Score: 0.85, Method: "lexical_name"},
	}}
	reconciler := ObjectReconciler{Store: nodes}

	res, err := ResolveAmbiguousArtifactObjects(context.Background(), store, reconciler, nil, 10)
	if err != nil {
		t.Fatalf("ResolveAmbiguousArtifactObjects: %v", err)
	}
	if res.Scanned != 1 || res.TieBroken != 1 || res.Matched != 0 || res.Created != 0 || res.Failed != 0 {
		t.Fatalf("result = %+v, want 1 scanned/tie-broken", res)
	}
	if len(store.updates) != 1 {
		t.Fatalf("updates = %+v, want 1", store.updates)
	}
	got := store.updates[0]
	if got.rowID != 1 || got.objectID != "obj_b" || got.status != ObjectReconcileAmbiguousResolved {
		t.Fatalf("update = %+v, want rowID=1 objectID=obj_b status=ambiguous_resolved", got)
	}
}

func TestResolveAmbiguousArtifactObjectsMatchesWhenTieResolvedNaturally(t *testing.T) {
	store := &fakeAmbiguousObjectStore{rows: []AmbiguousObjectRow{{
		RowID:  2,
		Object: ArtifactObject{ArtifactType: searchArtifactProvision, ArtifactID: "9_prv_2"},
	}}}
	nodes := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_exact"}, Score: 1, Method: "exact_name"},
		{Node: ObjectNode{ObjectID: "obj_lex"}, Score: 0.85, Method: "lexical_name"},
	}}
	reconciler := ObjectReconciler{Store: nodes}

	res, err := ResolveAmbiguousArtifactObjects(context.Background(), store, reconciler, nil, 10)
	if err != nil {
		t.Fatalf("ResolveAmbiguousArtifactObjects: %v", err)
	}
	if res.Matched != 1 || res.TieBroken != 0 || res.Created != 0 {
		t.Fatalf("result = %+v, want 1 matched", res)
	}
	got := store.updates[0]
	if got.objectID != "obj_exact" || got.status != ObjectReconcileMatched {
		t.Fatalf("update = %+v, want objectID=obj_exact status=matched", got)
	}
}

func TestResolveAmbiguousArtifactObjectsCreatesNodeWhenNoCandidatesRemain(t *testing.T) {
	store := &fakeAmbiguousObjectStore{rows: []AmbiguousObjectRow{{
		RowID:  3,
		Object: ArtifactObject{ArtifactType: searchArtifactProvision, ArtifactID: "9_prv_3", ObjectName: "flow meter"},
	}}}
	nodes := &stubObjectNodeStore{}
	reconciler := ObjectReconciler{Store: nodes}

	res, err := ResolveAmbiguousArtifactObjects(context.Background(), store, reconciler, nil, 10)
	if err != nil {
		t.Fatalf("ResolveAmbiguousArtifactObjects: %v", err)
	}
	if res.Created != 1 || res.Matched != 0 || res.TieBroken != 0 {
		t.Fatalf("result = %+v, want 1 created", res)
	}
	if len(nodes.created) != 1 || nodes.created[0].ObjectName != "flow meter" {
		t.Fatalf("created = %+v, want flow meter", nodes.created)
	}
	got := store.updates[0]
	if got.objectID != "obj_new" || got.status != ObjectReconcileNew {
		t.Fatalf("update = %+v, want objectID=obj_new status=new", got)
	}
}

type fakeAmbiguousObjectStore struct {
	rows    []AmbiguousObjectRow
	updates []ambiguousUpdate
}

type ambiguousUpdate struct {
	rowID      int64
	objectID   string
	status     string
	confidence float64
}

func (s *fakeAmbiguousObjectStore) LoadAmbiguous(_ context.Context, limit int) ([]AmbiguousObjectRow, error) {
	if limit > 0 && limit < len(s.rows) {
		return s.rows[:limit], nil
	}
	return s.rows, nil
}

func (s *fakeAmbiguousObjectStore) UpdateResolution(_ context.Context, rowID int64, objectID, status string, confidence float64, _ map[string]any) error {
	s.updates = append(s.updates, ambiguousUpdate{rowID: rowID, objectID: objectID, status: status, confidence: confidence})
	return nil
}

type stubObjectNodeStore struct {
	candidates []ObjectNodeCandidate
	created    []ArtifactObject
}

func (s *stubObjectNodeStore) FindCandidates(_ context.Context, _ ArtifactObject, _ ObjectReconcileOptions) ([]ObjectNodeCandidate, error) {
	return s.candidates, nil
}

func (s *stubObjectNodeStore) CreateNode(_ context.Context, obj ArtifactObject) (ObjectNode, error) {
	s.created = append(s.created, obj)
	return ObjectNode{ObjectID: "obj_new", CanonicalName: obj.ObjectName}, nil
}

func TestArtifactObjectSQLStoreLoadAmbiguousReadsRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM kb.artifact_objects").
		WithArgs(ObjectReconcileAmbiguous, 50).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_record_id", "input_record_id", "artifact_type", "artifact_id",
			"object_name", "object_name_en", "object_name_zh",
			"language", "object_type", "object_role",
			"aliases", "acronyms", "normalized_names",
			"description", "evidence_quote", "source_line_spans", "confidence",
		}).AddRow(
			int64(7), int64(9), int64(9), searchArtifactProvision, "9_prv_1",
			"pressure regulator", "", "",
			"", "equipment", "regulated_object",
			[]byte(`["reg"]`), []byte(`[]`), []byte(`["pressure regulator"]`),
			"", "", []byte(`["8"]`), 0.85,
		))

	store := ArtifactObjectSQLStore{DB: db}
	rows, err := store.LoadAmbiguous(context.Background(), 50)
	if err != nil {
		t.Fatalf("LoadAmbiguous: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want 1", rows)
	}
	got := rows[0]
	if got.RowID != 7 || got.Object.ArtifactID != "9_prv_1" || got.Object.ObjectName != "pressure regulator" {
		t.Fatalf("row = %+v, unexpected", got)
	}
	if !containsString(got.Object.NormalizedNames, "pressure regulator") {
		t.Fatalf("normalized names = %v, want pressure regulator", got.Object.NormalizedNames)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestArtifactObjectSQLStoreUpdateResolutionWritesRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("UPDATE kb.artifact_objects").
		WithArgs("obj_b", ObjectReconcileAmbiguousResolved, 0.85, sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := ArtifactObjectSQLStore{DB: db}
	err = store.UpdateResolution(context.Background(), 7, "obj_b", ObjectReconcileAmbiguousResolved, 0.85, map[string]any{"reconcile_method": "tie_break_deterministic"})
	if err != nil {
		t.Fatalf("UpdateResolution: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
