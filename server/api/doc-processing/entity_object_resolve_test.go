package docprocessing

import (
	"context"
	"testing"
)

func TestClassificationTierExcludeAboveConfidence(t *testing.T) {
	if tier := classificationTier("exclude", 0.9, 0.85); tier != "exclude" {
		t.Fatalf("tier = %q, want exclude", tier)
	}
}

func TestClassificationTierAssociateAboveConfidence(t *testing.T) {
	if tier := classificationTier("associate", 0.9, 0.85); tier != "associate" {
		t.Fatalf("tier = %q, want associate", tier)
	}
}

func TestClassificationTierBelowConfidenceIsUncertainRegardlessOfDecision(t *testing.T) {
	for _, decision := range []string{"exclude", "associate"} {
		if tier := classificationTier(decision, 0.5, 0.85); tier != "uncertain" {
			t.Fatalf("tier(%q, 0.5) = %q, want uncertain (must not silently force a low-confidence choice)", decision, tier)
		}
	}
}

func TestClassificationTierUncertainDecision(t *testing.T) {
	if tier := classificationTier("uncertain", 0.99, 0.85); tier != "uncertain" {
		t.Fatalf("tier = %q, want uncertain", tier)
	}
}

func TestNextDeferredStatusBelowCap(t *testing.T) {
	if got := nextDeferredStatus(2, 3); got != entityObjectLinkDeferred {
		t.Fatalf("nextDeferredStatus(2,3) = %q, want deferred", got)
	}
}

func TestNextDeferredStatusAtCap(t *testing.T) {
	if got := nextDeferredStatus(3, 3); got != entityObjectLinkExhausted {
		t.Fatalf("nextDeferredStatus(3,3) = %q, want exhausted", got)
	}
}

func TestComputeEntityObjectFingerprintDeterministic(t *testing.T) {
	e := pendingEntityRow{Entity: "Pump A", EntityType: "equipment"}
	f1 := computeEntityObjectFingerprint(e, []string{"obj_2", "obj_1"})
	f2 := computeEntityObjectFingerprint(e, []string{"obj_1", "obj_2"})
	if f1 != f2 {
		t.Fatalf("fingerprint not order-independent: %q != %q", f1, f2)
	}
}

func TestComputeEntityObjectFingerprintChangesWithNewCandidate(t *testing.T) {
	e := pendingEntityRow{Entity: "Pump A", EntityType: "equipment"}
	f1 := computeEntityObjectFingerprint(e, []string{"obj_1"})
	f2 := computeEntityObjectFingerprint(e, []string{"obj_1", "obj_2"})
	if f1 == f2 {
		t.Fatalf("fingerprint unchanged despite a new candidate appearing")
	}
}

func TestComputeEntityObjectFingerprintChangesWithEntityContent(t *testing.T) {
	e1 := pendingEntityRow{Entity: "Pump A", EntityType: "equipment", Desc: "old desc"}
	e2 := pendingEntityRow{Entity: "Pump A", EntityType: "equipment", Desc: "enriched desc after a dedup merge"}
	if computeEntityObjectFingerprint(e1, nil) == computeEntityObjectFingerprint(e2, nil) {
		t.Fatalf("fingerprint unchanged despite entity content changing")
	}
}

// --- orchestration ---

type fakeEntityObjectResolveStore struct {
	rows      []EntityObjectResolveRow
	excluded  []string
	linked    []string
	attempted []attemptedUpdate
}

type attemptedUpdate struct {
	entityID    string
	status      string
	attempts    int
	fingerprint string
}

func (s *fakeEntityObjectResolveStore) LoadResolvable(_ context.Context, limit int) ([]EntityObjectResolveRow, error) {
	if limit > 0 && limit < len(s.rows) {
		return s.rows[:limit], nil
	}
	return s.rows, nil
}

func (s *fakeEntityObjectResolveStore) MarkExcluded(_ context.Context, entityID string) error {
	s.excluded = append(s.excluded, entityID)
	return nil
}

func (s *fakeEntityObjectResolveStore) MarkLinked(_ context.Context, entityID string) error {
	s.linked = append(s.linked, entityID)
	return nil
}

func (s *fakeEntityObjectResolveStore) MarkAttempted(_ context.Context, entityID, status string, attempts int, fingerprint string) error {
	s.attempted = append(s.attempted, attemptedUpdate{entityID: entityID, status: status, attempts: attempts, fingerprint: fingerprint})
	return nil
}

type fakeEntityObjectClassifier struct {
	calls    int
	response EntityObjectClassification
	err      error
}

func (c *fakeEntityObjectClassifier) ClassifyEntityForObjectLink(_ context.Context, _ pendingEntityRow, _ []ObjectNodeCandidate) (EntityObjectClassification, error) {
	c.calls++
	return c.response, c.err
}

func TestResolveEntityObjectsSkipsUnchangedFingerprintWithoutClassifying(t *testing.T) {
	store := &fakeEntityObjectResolveStore{rows: []EntityObjectResolveRow{
		{
			Entity:      pendingEntityRow{EntityID: "1_ent_1", Entity: "Pump A", EntityType: "equipment"},
			Attempts:    1,
			Fingerprint: computeEntityObjectFingerprint(pendingEntityRow{Entity: "Pump A", EntityType: "equipment"}, nil),
		},
	}}
	classifier := &fakeEntityObjectClassifier{}
	objectStore := &fakeArtifactObjectPersister{}
	reconciler := ObjectReconciler{Store: &stubObjectNodeStore{}}

	res, err := ResolveEntityObjects(context.Background(), store, objectStore, reconciler, classifier, EntityObjectResolveConfig{}, 200, nil)
	if err != nil {
		t.Fatalf("ResolveEntityObjects: %v", err)
	}
	if classifier.calls != 0 {
		t.Fatalf("classifier.calls = %d, want 0 (unchanged fingerprint must not spend an LLM call)", classifier.calls)
	}
	if res.SkippedUnchanged != 1 || len(store.attempted) != 0 {
		t.Fatalf("res = %+v, attempted = %+v, want one skipped-unchanged and no writes", res, store.attempted)
	}
}

func TestResolveEntityObjectsExcludesOnExcludeDecision(t *testing.T) {
	store := &fakeEntityObjectResolveStore{rows: []EntityObjectResolveRow{
		{Entity: pendingEntityRow{EntityID: "1_ent_1", Entity: "Widget Co", EntityType: "organization"}},
	}}
	classifier := &fakeEntityObjectClassifier{response: EntityObjectClassification{Decision: "exclude", Confidence: 0.95}}
	objectStore := &fakeArtifactObjectPersister{}
	reconciler := ObjectReconciler{Store: &stubObjectNodeStore{}}

	res, err := ResolveEntityObjects(context.Background(), store, objectStore, reconciler, classifier, EntityObjectResolveConfig{}, 200, nil)
	if err != nil {
		t.Fatalf("ResolveEntityObjects: %v", err)
	}
	if res.Excluded != 1 || len(store.excluded) != 1 || store.excluded[0] != "1_ent_1" {
		t.Fatalf("res = %+v, excluded = %+v", res, store.excluded)
	}
}

func TestResolveEntityObjectsAssociatesWithExistingCandidate(t *testing.T) {
	store := &fakeEntityObjectResolveStore{rows: []EntityObjectResolveRow{
		{Entity: pendingEntityRow{EntityID: "1_ent_1", Entity: "Pump A", EntityType: "equipment"}},
	}}
	classifier := &fakeEntityObjectClassifier{response: EntityObjectClassification{
		Decision: "associate", Confidence: 0.9, SelectedObjectID: "obj_pump",
	}}
	objectStore := &fakeArtifactObjectPersister{}
	nodeStore := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_pump"}, Score: 0.85, Method: "lexical_name"},
	}}
	reconciler := ObjectReconciler{Store: nodeStore}

	res, err := ResolveEntityObjects(context.Background(), store, objectStore, reconciler, classifier, EntityObjectResolveConfig{}, 200, nil)
	if err != nil {
		t.Fatalf("ResolveEntityObjects: %v", err)
	}
	if res.Linked != 1 || len(store.linked) != 1 {
		t.Fatalf("res = %+v, linked = %+v", res, store.linked)
	}
	if len(nodeStore.created) != 0 {
		t.Fatalf("CreateNode called = %+v, want none (an existing candidate was selected)", nodeStore.created)
	}
	if len(objectStore.calls) != 0 {
		t.Fatalf("ReplaceObjectsForRecord must not be used by Phase 4 (cross-record); got %+v", objectStore.calls)
	}
}

func TestResolveEntityObjectsCreatesNodeWhenAssociateHasNoCandidates(t *testing.T) {
	store := &fakeEntityObjectResolveStore{rows: []EntityObjectResolveRow{
		{Entity: pendingEntityRow{EntityID: "1_ent_1", Entity: "Brand New System", EntityType: "system"}},
	}}
	classifier := &fakeEntityObjectClassifier{response: EntityObjectClassification{
		Decision: "associate", Confidence: 0.9, ObjectType: "system",
	}}
	objectStore := &fakeArtifactObjectPersister{}
	nodeStore := &stubObjectNodeStore{} // no candidates
	reconciler := ObjectReconciler{Store: nodeStore}

	res, err := ResolveEntityObjects(context.Background(), store, objectStore, reconciler, classifier, EntityObjectResolveConfig{}, 200, nil)
	if err != nil {
		t.Fatalf("ResolveEntityObjects: %v", err)
	}
	if res.Linked != 1 || len(nodeStore.created) != 1 {
		t.Fatalf("res = %+v, created = %+v, want one linked via a newly created node", res, nodeStore.created)
	}
}

func TestResolveEntityObjectsDefersUncertainBelowAttemptCap(t *testing.T) {
	store := &fakeEntityObjectResolveStore{rows: []EntityObjectResolveRow{
		{Entity: pendingEntityRow{EntityID: "1_ent_1", Entity: "Ambiguous Thing", EntityType: "equipment"}, Attempts: 0},
	}}
	classifier := &fakeEntityObjectClassifier{response: EntityObjectClassification{Decision: "uncertain", Confidence: 0.5}}
	objectStore := &fakeArtifactObjectPersister{}
	reconciler := ObjectReconciler{Store: &stubObjectNodeStore{}}

	res, err := ResolveEntityObjects(context.Background(), store, objectStore, reconciler, classifier, EntityObjectResolveConfig{MaxAttempts: 3}, 200, nil)
	if err != nil {
		t.Fatalf("ResolveEntityObjects: %v", err)
	}
	if res.Deferred != 1 || len(store.attempted) != 1 || store.attempted[0].status != entityObjectLinkDeferred || store.attempted[0].attempts != 1 {
		t.Fatalf("res = %+v, attempted = %+v", res, store.attempted)
	}
}

func TestResolveEntityObjectsExhaustsUncertainAtAttemptCap(t *testing.T) {
	store := &fakeEntityObjectResolveStore{rows: []EntityObjectResolveRow{
		{Entity: pendingEntityRow{EntityID: "1_ent_1", Entity: "Ambiguous Thing", EntityType: "equipment"}, Attempts: 2},
	}}
	classifier := &fakeEntityObjectClassifier{response: EntityObjectClassification{Decision: "uncertain", Confidence: 0.5}}
	objectStore := &fakeArtifactObjectPersister{}
	reconciler := ObjectReconciler{Store: &stubObjectNodeStore{}}

	res, err := ResolveEntityObjects(context.Background(), store, objectStore, reconciler, classifier, EntityObjectResolveConfig{MaxAttempts: 3}, 200, nil)
	if err != nil {
		t.Fatalf("ResolveEntityObjects: %v", err)
	}
	if res.Exhausted != 1 || len(store.attempted) != 1 || store.attempted[0].status != entityObjectLinkExhausted || store.attempted[0].attempts != 3 {
		t.Fatalf("res = %+v, attempted = %+v", res, store.attempted)
	}
}

func TestResolveEntityObjectsClassifierFailureDoesNotAbortBatch(t *testing.T) {
	store := &fakeEntityObjectResolveStore{rows: []EntityObjectResolveRow{
		{Entity: pendingEntityRow{EntityID: "1_ent_1", Entity: "First", EntityType: "equipment"}},
		{Entity: pendingEntityRow{EntityID: "1_ent_2", Entity: "Second", EntityType: "equipment"}},
	}}
	classifier := &sequencedClassifier{
		responses: []classifyResponse{
			{err: errClassifierBoom},
			{result: EntityObjectClassification{Decision: "exclude", Confidence: 0.95}},
		},
	}
	objectStore := &fakeArtifactObjectPersister{}
	reconciler := ObjectReconciler{Store: &stubObjectNodeStore{}}

	res, err := ResolveEntityObjects(context.Background(), store, objectStore, reconciler, classifier, EntityObjectResolveConfig{MaxAttempts: 3}, 200, nil)
	if err != nil {
		t.Fatalf("ResolveEntityObjects: %v", err)
	}
	if res.Excluded != 1 {
		t.Fatalf("res = %+v, want the second entity still excluded despite the first classifier call failing", res)
	}
	if len(store.attempted) != 1 || store.attempted[0].entityID != "1_ent_1" || store.attempted[0].status != entityObjectLinkDeferred {
		t.Fatalf("attempted = %+v, want the failed entity deferred with attempts incremented", store.attempted)
	}
}

type classifyResponse struct {
	result EntityObjectClassification
	err    error
}

type sequencedClassifier struct {
	responses []classifyResponse
	i         int
}

var errClassifierBoom = &classifierError{"boom"}

type classifierError struct{ msg string }

func (e *classifierError) Error() string { return e.msg }

func (c *sequencedClassifier) ClassifyEntityForObjectLink(_ context.Context, _ pendingEntityRow, _ []ObjectNodeCandidate) (EntityObjectClassification, error) {
	r := c.responses[c.i]
	c.i++
	return r.result, r.err
}
