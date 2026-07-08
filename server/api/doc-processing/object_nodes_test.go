package docprocessing

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestObjectNodeCreateNodeStoresEmptyArraysForNilSlices(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := ObjectNodeSQLStore{DB: db}
	mock.ExpectQuery("INSERT INTO kb.object_nodes").
		WithArgs(
			sqlmock.AnyArg(),
			"pump",
			nil,
			nil,
			nil,
			"equipment",
			"[]",
			"[]",
			"[]",
			nil,
			"pump equipment self",
			"active",
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"object_id"}).AddRow("obj_1"))

	_, err = store.CreateNode(context.Background(), ArtifactObject{
		InputRecordID: 1,
		ObjectName:    "pump",
		ObjectType:    "equipment",
		ObjectRole:    "self",
	})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestFindCandidatesExcludesRejectedAndMergedNodes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := ObjectNodeSQLStore{DB: db}
	cols := []string{
		"id", "object_id", "canonical_object_id", "canonical_name", "canonical_name_en",
		"canonical_name_zh", "primary_language", "object_type", "aliases", "acronyms",
		"normalized_names", "description", "search_document", "reconcile_status", "ext_info",
	}
	mock.ExpectQuery(regexp.QuoteMeta("reconcile_status NOT IN ('rejected', 'merged')")).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			int64(1), "obj_1", "", "pump", "", "", "", "equipment",
			[]byte("[]"), []byte("[]"), []byte(`["pump"]`), "", "", "active", []byte("{}"),
		))

	candidates, err := store.FindCandidates(context.Background(), ArtifactObject{
		ObjectName:      "pump",
		ObjectType:      "equipment",
		NormalizedNames: []string{"pump"},
	}, ObjectReconcileOptions{})
	if err != nil {
		t.Fatalf("FindCandidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Node.ObjectID != "obj_1" {
		t.Fatalf("candidates = %+v, want one candidate obj_1", candidates)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestReconcileArtifactObjectsLogsWarnOnAmbiguousTie(t *testing.T) {
	store := &tiedCandidateStore{
		nodes: []ObjectNode{
			{ObjectID: "obj_a", CanonicalName: "Pressure Regulator A"},
			{ObjectID: "obj_b", CanonicalName: "Pressure Regulator B"},
		},
	}
	reconciler := ObjectReconciler{Store: store}
	logger := &fakeLogger{}

	objects := []ArtifactObject{{
		InputRecordID: 9,
		ArtifactType:  searchArtifactProvision,
		ArtifactID:    "9_prv_1",
		ObjectName:    "pressure regulator",
	}}

	reconciled, err := reconcileArtifactObjects(context.Background(), objects, reconciler, logger)
	if err != nil {
		t.Fatalf("reconcileArtifactObjects: %v", err)
	}
	if len(reconciled) != 1 || reconciled[0].ObjectID != "" || reconciled[0].ReconcileStatus != ObjectReconcileAmbiguous {
		t.Fatalf("reconciled = %+v, want ambiguous with empty object id", reconciled)
	}
	if len(logger.warns) != 1 {
		t.Fatalf("warns = %+v, want exactly one warning", logger.warns)
	}
	warn := logger.warns[0]
	if warn.message != "object reconciliation ambiguous" {
		t.Fatalf("warn message = %q", warn.message)
	}
	if v, ok := logValue(warn.args, "artifact_id"); !ok || v != "9_prv_1" {
		t.Fatalf("warn args = %v, want artifact_id=9_prv_1", warn.args)
	}
	if v, ok := logValue(warn.args, "candidates"); !ok {
		t.Fatalf("warn args = %v, want candidates key present", warn.args)
	} else if names, ok := v.([]string); !ok || len(names) != 2 {
		t.Fatalf("candidates arg = %v, want 2 candidate names", names)
	}
}

func TestReconcileArtifactObjectsUsesLLMForAmbiguousTie(t *testing.T) {
	objects := []ArtifactObject{{
		ArtifactType:    searchArtifactProvision,
		ArtifactID:      "9_prv_1",
		ObjectName:      "SBP",
		NormalizedNames: []string{"sbp"},
		ExtInfo:         map[string]any{"source": "provision"},
	}}
	nodes := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_sbp", CanonicalName: "收缩压"}, Score: 0.85, Method: "lexical_name"},
		{Node: ObjectNode{ObjectID: "obj_htn", CanonicalName: "高血压"}, Score: 0.85, Method: "lexical_name"},
	}}
	resolver := fakeAmbiguousObjectLLMResolver{decision: AmbiguousObjectLLMDecision{
		ModelName:            "test-model",
		ResolutionObjectID:   "obj_sbp",
		ResolutionConfidence: 0.92,
		ArtifactUpdates:      AmbiguousArtifactObjectLLMUpdate{ObjectNameEn: "systolic blood pressure"},
	}}
	logger := &fakeLogger{}

	got, err := reconcileArtifactObjectsWithLLM(context.Background(), objects, ObjectReconciler{Store: nodes}, logger, resolver, 0.85)
	if err != nil {
		t.Fatalf("reconcileArtifactObjectsWithLLM: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("objects = %+v, want 1", got)
	}
	obj := got[0]
	if obj.ObjectID != "obj_sbp" || obj.ReconcileStatus != ObjectReconcileAmbiguousResolved || obj.ReconcileConfidence != 0.92 {
		t.Fatalf("object = %+v, want LLM ambiguous_resolved obj_sbp", obj)
	}
	if obj.ObjectNameEn != "systolic blood pressure" {
		t.Fatalf("object_name_en = %q, want LLM completion", obj.ObjectNameEn)
	}
	if obj.ExtInfo["reconcile_method"] != ObjectReconcileMethodLLMAmbiguous {
		t.Fatalf("ext_info = %+v, want LLM reconcile method", obj.ExtInfo)
	}
	if len(logger.warns) != 0 {
		t.Fatalf("warns = %+v, want no ambiguous warning after LLM resolution", logger.warns)
	}
}

type fakeAmbiguousObjectLLMResolver struct {
	decision AmbiguousObjectLLMDecision
	err      error
}

func (r fakeAmbiguousObjectLLMResolver) ResolveAmbiguousObject(_ context.Context, _ ArtifactObject, _ []ObjectNodeCandidate) (AmbiguousObjectLLMDecision, error) {
	return r.decision, r.err
}

type tiedCandidateStore struct {
	nodes []ObjectNode
}

func (s *tiedCandidateStore) FindCandidates(_ context.Context, _ ArtifactObject, _ ObjectReconcileOptions) ([]ObjectNodeCandidate, error) {
	var out []ObjectNodeCandidate
	for _, n := range s.nodes {
		out = append(out, ObjectNodeCandidate{Node: n, Score: 0.85, Method: "lexical_name"})
	}
	return out, nil
}

func (s *tiedCandidateStore) CreateNode(_ context.Context, obj ArtifactObject) (ObjectNode, error) {
	node := ObjectNode{ObjectID: "new_node", CanonicalName: obj.ObjectName}
	s.nodes = append(s.nodes, node)
	return node, nil
}

func TestObjectNodeCreateNodeKeepsEmptyArraysOnConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := ObjectNodeSQLStore{DB: db}
	mock.ExpectQuery(regexp.QuoteMeta("ON CONFLICT (object_id) DO UPDATE SET")+`[\s\S]*`+regexp.QuoteMeta("aliases = (\n\t\tCOALESCE(")+`[\s\S]*`+regexp.QuoteMeta("'[]'::jsonb")).
		WithArgs(
			sqlmock.AnyArg(),
			"pump",
			nil,
			nil,
			nil,
			"equipment",
			"[]",
			"[]",
			"[]",
			nil,
			"pump equipment self",
			"active",
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"object_id"}).AddRow("obj_1"))

	_, err = store.CreateNode(context.Background(), ArtifactObject{
		InputRecordID: 1,
		ObjectName:    "pump",
		ObjectType:    "equipment",
		ObjectRole:    "self",
	})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
