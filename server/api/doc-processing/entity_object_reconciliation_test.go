package docprocessing

import (
	"context"
	"testing"
)

func TestEntityObjectTypeCandidateMapsKnownTypes(t *testing.T) {
	cases := map[string]string{
		"software_system": "system",
		"equipment":       "equipment",
		"material":        "material",
		"organization":    "organization",
		"place":           "place",
	}
	for entityType, wantObjType := range cases {
		objType, ok := entityObjectTypeCandidate(entityType, "")
		if !ok {
			t.Fatalf("entityObjectTypeCandidate(%q) = not eligible, want eligible", entityType)
		}
		if objType != wantObjType {
			t.Fatalf("entityObjectTypeCandidate(%q) = %q, want %q", entityType, objType, wantObjType)
		}
	}
}

func TestEntityObjectTypeCandidateRejectsConceptAndUnknownTypes(t *testing.T) {
	for _, entityType := range []string{"concept", "standard", "person", "document", "generic_term", ""} {
		if _, ok := entityObjectTypeCandidate(entityType, ""); ok {
			t.Fatalf("entityObjectTypeCandidate(%q) = eligible, want not eligible", entityType)
		}
	}
}

func TestEntityObjectTypeCandidateFallsBackToNonEnglishType(t *testing.T) {
	objType, ok := entityObjectTypeCandidate("", "equipment")
	if !ok || objType != "equipment" {
		t.Fatalf("entityObjectTypeCandidate(empty_en, %q) = (%q, %v), want (equipment, true)", "equipment", objType, ok)
	}
}

func TestMatchEntityToExistingObjectAcceptsSingleExactMatch(t *testing.T) {
	store := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_1"}, Score: 1, Method: "exact_name"},
	}}
	candidate, ok, err := matchEntityToExistingObject(context.Background(), store, ObjectReconcileOptions{}, ArtifactObject{})
	if err != nil {
		t.Fatalf("matchEntityToExistingObject: %v", err)
	}
	if !ok || candidate.Node.ObjectID != "obj_1" {
		t.Fatalf("got (%+v, %v), want matched obj_1", candidate, ok)
	}
}

func TestMatchEntityToExistingObjectAcceptsSingleBestAboveThreshold(t *testing.T) {
	store := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_1"}, Score: 0.95, Method: "lexical_name"},
	}}
	_, ok, err := matchEntityToExistingObject(context.Background(), store, ObjectReconcileOptions{}, ArtifactObject{})
	if err != nil {
		t.Fatalf("matchEntityToExistingObject: %v", err)
	}
	if !ok {
		t.Fatalf("got not matched, want matched at score 0.95")
	}
}

func TestMatchEntityToExistingObjectRejectsTie(t *testing.T) {
	store := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_1"}, Score: 0.85, Method: "lexical_name"},
		{Node: ObjectNode{ObjectID: "obj_2"}, Score: 0.85, Method: "lexical_name"},
	}}
	_, ok, err := matchEntityToExistingObject(context.Background(), store, ObjectReconcileOptions{}, ArtifactObject{})
	if err != nil {
		t.Fatalf("matchEntityToExistingObject: %v", err)
	}
	if ok {
		t.Fatalf("got matched on a tie, want not matched (Phase 3 does not resolve ties)")
	}
}

func TestMatchEntityToExistingObjectRejectsBelowThreshold(t *testing.T) {
	store := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_1"}, Score: 0.85, Method: "lexical_name"},
	}}
	_, ok, err := matchEntityToExistingObject(context.Background(), store, ObjectReconcileOptions{}, ArtifactObject{})
	if err != nil {
		t.Fatalf("matchEntityToExistingObject: %v", err)
	}
	if ok {
		t.Fatalf("got matched below 0.95, want not matched")
	}
}

func TestMatchEntityToExistingObjectNeverCreatesNode(t *testing.T) {
	store := &neverCreateObjectNodeStore{t: t}
	_, ok, err := matchEntityToExistingObject(context.Background(), store, ObjectReconcileOptions{}, ArtifactObject{ObjectName: "widget"})
	if err != nil {
		t.Fatalf("matchEntityToExistingObject: %v", err)
	}
	if ok {
		t.Fatalf("got matched with zero candidates, want not matched")
	}
}

// neverCreateObjectNodeStore fails the test if CreateNode is ever called,
// enforcing Phase 3's "never CreateNode" restraint (ADR 2026070101 Phase 3
// §Scope) at the test level, not just by convention.
type neverCreateObjectNodeStore struct {
	t *testing.T
}

func (s *neverCreateObjectNodeStore) FindCandidates(context.Context, ArtifactObject, ObjectReconcileOptions) ([]ObjectNodeCandidate, error) {
	return nil, nil
}

func (s *neverCreateObjectNodeStore) CreateNode(context.Context, ArtifactObject) (ObjectNode, error) {
	s.t.Fatal("CreateNode must never be called by Phase 3 entity matching")
	return ObjectNode{}, nil
}

type fakeEntityObjectStore struct {
	entities       []pendingEntityRow
	statusUpdates  []entityObjectLinkStatusUpdate
}

type entityObjectLinkStatusUpdate struct {
	entityID string
	status   string
}

func (s *fakeEntityObjectStore) LoadEntitiesForRecord(_ context.Context, _ int64) ([]pendingEntityRow, error) {
	return s.entities, nil
}

func (s *fakeEntityObjectStore) SetEntityObjectLinkStatus(_ context.Context, entityID, status string) error {
	s.statusUpdates = append(s.statusUpdates, entityObjectLinkStatusUpdate{entityID: entityID, status: status})
	return nil
}

type fakeArtifactObjectPersister struct {
	calls    []fakeReplaceCall
	inserted []ArtifactObject
}

type fakeReplaceCall struct {
	recordID     int64
	artifactType string
	objects      []ArtifactObject
}

func (s *fakeArtifactObjectPersister) ReplaceObjectsForRecord(_ context.Context, recordID int64, artifactType string, objects []ArtifactObject) error {
	s.calls = append(s.calls, fakeReplaceCall{recordID: recordID, artifactType: artifactType, objects: objects})
	return nil
}

// InsertOne implements ArtifactObjectSingleInserter so this fake can double
// as the Phase 4 persistence seam in entity_object_resolve_test.go.
func (s *fakeArtifactObjectPersister) InsertOne(_ context.Context, obj ArtifactObject) error {
	s.inserted = append(s.inserted, obj)
	return nil
}

func TestReconcileEntityObjectsForRecordExcludesTypeFilteredEntity(t *testing.T) {
	entityStore := &fakeEntityObjectStore{entities: []pendingEntityRow{
		{EntityID: "1_ent_1", InputRecordID: 1, Entity: "quality management", EntityType: "concept"},
	}}
	objectStore := &fakeArtifactObjectPersister{}
	reconciler := ObjectReconciler{Store: &stubObjectNodeStore{}}

	if err := ReconcileEntityObjectsForRecord(context.Background(), entityStore, objectStore, reconciler, 1, nil); err != nil {
		t.Fatalf("ReconcileEntityObjectsForRecord: %v", err)
	}

	if len(entityStore.statusUpdates) != 1 || entityStore.statusUpdates[0] != (entityObjectLinkStatusUpdate{entityID: "1_ent_1", status: entityObjectLinkExcluded}) {
		t.Fatalf("statusUpdates = %+v, want one excluded update for 1_ent_1", entityStore.statusUpdates)
	}
	if len(objectStore.calls) != 1 || len(objectStore.calls[0].objects) != 0 {
		t.Fatalf("objectStore.calls = %+v, want one call with zero objects", objectStore.calls)
	}
}

func TestReconcileEntityObjectsForRecordLinksConfidentMatch(t *testing.T) {
	entityStore := &fakeEntityObjectStore{entities: []pendingEntityRow{
		{EntityID: "1_ent_1", InputRecordID: 1, Entity: "Pump A", EntityType: "equipment"},
	}}
	objectStore := &fakeArtifactObjectPersister{}
	reconciler := ObjectReconciler{Store: &stubObjectNodeStore{candidates: []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_pump"}, Score: 1, Method: "exact_name"},
	}}}

	if err := ReconcileEntityObjectsForRecord(context.Background(), entityStore, objectStore, reconciler, 1, nil); err != nil {
		t.Fatalf("ReconcileEntityObjectsForRecord: %v", err)
	}

	if len(objectStore.calls) != 1 || len(objectStore.calls[0].objects) != 1 {
		t.Fatalf("objectStore.calls = %+v, want one call with one object", objectStore.calls)
	}
	obj := objectStore.calls[0].objects[0]
	if obj.ArtifactType != "entity" || obj.ArtifactID != "1_ent_1" || obj.ObjectID != "obj_pump" || obj.ObjectRole != "represented_entity" {
		t.Fatalf("persisted object = %+v, unexpected shape", obj)
	}
	if len(entityStore.statusUpdates) != 1 || entityStore.statusUpdates[0] != (entityObjectLinkStatusUpdate{entityID: "1_ent_1", status: entityObjectLinkLinked}) {
		t.Fatalf("statusUpdates = %+v, want one linked update for 1_ent_1", entityStore.statusUpdates)
	}
}

func TestReconcileEntityObjectsForRecordLeavesNoMatchEntityPending(t *testing.T) {
	entityStore := &fakeEntityObjectStore{entities: []pendingEntityRow{
		{EntityID: "1_ent_1", InputRecordID: 1, Entity: "Widget", EntityType: "equipment"},
	}}
	objectStore := &fakeArtifactObjectPersister{}
	reconciler := ObjectReconciler{Store: &stubObjectNodeStore{}} // no candidates

	if err := ReconcileEntityObjectsForRecord(context.Background(), entityStore, objectStore, reconciler, 1, nil); err != nil {
		t.Fatalf("ReconcileEntityObjectsForRecord: %v", err)
	}

	if len(entityStore.statusUpdates) != 0 {
		t.Fatalf("statusUpdates = %+v, want none — no-match entities stay at the pending default", entityStore.statusUpdates)
	}
	if len(objectStore.calls) != 1 || len(objectStore.calls[0].objects) != 0 {
		t.Fatalf("objectStore.calls = %+v, want one call with zero objects", objectStore.calls)
	}
}

func TestReconcileEntityObjectsForRecordAlwaysReplacesForIdempotency(t *testing.T) {
	entityStore := &fakeEntityObjectStore{entities: nil}
	objectStore := &fakeArtifactObjectPersister{}
	reconciler := ObjectReconciler{Store: &stubObjectNodeStore{}}

	if err := ReconcileEntityObjectsForRecord(context.Background(), entityStore, objectStore, reconciler, 42, nil); err != nil {
		t.Fatalf("ReconcileEntityObjectsForRecord: %v", err)
	}

	if len(objectStore.calls) != 1 || objectStore.calls[0].recordID != 42 || objectStore.calls[0].artifactType != "entity" {
		t.Fatalf("objectStore.calls = %+v, want one call clearing record 42's entity objects even with no entities", objectStore.calls)
	}
}
