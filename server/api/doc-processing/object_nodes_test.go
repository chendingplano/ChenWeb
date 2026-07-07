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
