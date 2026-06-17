package docprocessing

import (
	"context"
	"testing"
)

// Orchestration test for Phase C linking, using an in-memory store (no DB).
// Verifies the load -> link -> replace-relations -> save-provisionals glue:
// matched endpoints get entity_ids, the unmatched one is minted provisional and
// saved, and the record's relations are replaced exactly once.

type fakeLinkStore struct {
	entities  []map[string]any
	relations []map[string]any

	deletedRecord  int64
	savedRelations []map[string]any
	savedEntities  []map[string]any
}

func (f *fakeLinkStore) LoadEntitiesForRecord(_ context.Context, _ int64) ([]map[string]any, error) {
	return f.entities, nil
}
func (f *fakeLinkStore) LoadRelationsForRecord(_ context.Context, _ int64) ([]map[string]any, error) {
	return f.relations, nil
}
func (f *fakeLinkStore) DeleteRelationsByInputRecordID(_ context.Context, recordID int64) (int64, error) {
	f.deletedRecord = recordID
	return int64(len(f.relations)), nil
}
func (f *fakeLinkStore) SaveRelations(_ context.Context, req SaveRelationsRequest) (int64, error) {
	f.savedRelations = req.Relations
	return int64(len(req.Relations)), nil
}
func (f *fakeLinkStore) SaveEntities(_ context.Context, req SaveEntitiesRequest) (int64, error) {
	f.savedEntities = req.Entities
	return int64(len(req.Entities)), nil
}

func TestLinkRecordEndpoints_LinksReplacesAndMintsProvisional(t *testing.T) {
	store := &fakeLinkStore{
		entities: []map[string]any{
			{"entity_id": "r_ent_1", "entity": "Acme", "line_spans": []string{"10"}},
			{"entity_id": "r_ent_2", "entity": "Globex", "line_spans": []string{"20"}},
		},
		relations: []map[string]any{
			relForLink("Acme", "supplies", "Globex", []string{"10"}, []string{"12"}, []string{"20"}),
			relForLink("Acme", "competes_with", "Initech", []string{"10"}, []string{"30"}, []string{"31"}),
		},
	}

	stats, err := linkRecordEndpoints(context.Background(), store, 42, "evt-1", "en", "m", "p", "t0")
	if err != nil {
		t.Fatalf("linkRecordEndpoints error: %v", err)
	}

	if stats.Relations != 2 || stats.Linked != 2 || stats.Provisional != 1 {
		t.Errorf("stats = %+v, want {Relations:2 Linked:2 Provisional:1}", stats)
	}
	if store.deletedRecord != 42 {
		t.Errorf("relations not replaced for record 42 (deletedRecord=%d)", store.deletedRecord)
	}
	if len(store.savedRelations) != 2 {
		t.Fatalf("saved %d relations, want 2", len(store.savedRelations))
	}
	// First relation links to both real entities.
	r0 := store.savedRelations[0]
	if r0["subject_entity_id"] != "r_ent_1" || r0["object_entity_id"] != "r_ent_2" {
		t.Errorf("relation 0 links = (%v,%v), want (r_ent_1,r_ent_2)", r0["subject_entity_id"], r0["object_entity_id"])
	}
	// Second relation's object "Initech" is provisional and was saved.
	if len(store.savedEntities) != 1 {
		t.Fatalf("saved %d provisional entities, want 1", len(store.savedEntities))
	}
	prov := store.savedEntities[0]
	if prov["entity"] != "Initech" || prov["entity_status"] != entityStatusProvisional {
		t.Errorf("provisional = %v (status %v), want Initech/provisional", prov["entity"], prov["entity_status"])
	}
	if store.savedRelations[1]["object_entity_id"] != prov["entity_id"] {
		t.Errorf("relation 1 object should link to provisional id %v, got %v",
			prov["entity_id"], store.savedRelations[1]["object_entity_id"])
	}
}

func TestLinkRecordEndpoints_NoRelationsNoop(t *testing.T) {
	store := &fakeLinkStore{
		entities:  []map[string]any{{"entity_id": "r_ent_1", "entity": "Acme"}},
		relations: nil,
	}
	stats, err := linkRecordEndpoints(context.Background(), store, 7, "", "", "", "", "t0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if stats.Relations != 0 {
		t.Errorf("stats.Relations = %d, want 0", stats.Relations)
	}
	if store.deletedRecord != 0 || store.savedRelations != nil {
		t.Errorf("no relations => must not delete/save (deleted=%d, saved=%v)", store.deletedRecord, store.savedRelations)
	}
}
