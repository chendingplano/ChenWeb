package docprocessing

import (
	"context"
	"regexp"
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

func TestApplyAmbiguousObjectLLMDecisionResolvesHighConfidenceSelection(t *testing.T) {
	obj := ArtifactObject{
		ArtifactID:      "9_prv_1",
		ObjectName:      "收缩压",
		NormalizedNames: []string{"收缩压", "sbp"},
		ExtInfo:         map[string]any{"source": "provision"},
	}
	candidates := []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_sbp", CanonicalName: "收缩压"}},
		{Node: ObjectNode{ObjectID: "obj_htn", CanonicalName: "高血压"}},
	}
	decision := AmbiguousObjectLLMDecision{
		ModelName:            "test-model",
		ResolutionObjectID:   "obj_sbp",
		ResolutionConfidence: 0.91,
		ArtifactUpdates: AmbiguousArtifactObjectLLMUpdate{
			ObjectNameEn: "systolic blood pressure",
			ObjectType:   "vital_sign",
			Description:  "Pressure during cardiac contraction.",
		},
	}

	got, applied, err := ApplyAmbiguousObjectLLMDecision(context.Background(), obj, candidates, decision, nil, 0.85)
	if err != nil {
		t.Fatalf("ApplyAmbiguousObjectLLMDecision: %v", err)
	}
	if !applied {
		t.Fatalf("applied = false, want true")
	}
	if got.ObjectID != "obj_sbp" || got.ReconcileStatus != ObjectReconcileAmbiguousResolved || got.ReconcileConfidence != 0.91 {
		t.Fatalf("resolved object = %+v, want obj_sbp ambiguous_resolved confidence 0.91", got)
	}
	if got.ObjectNameEn != "systolic blood pressure" || got.ObjectType != "vital_sign" || got.Description == "" {
		t.Fatalf("field updates not applied: %+v", got)
	}
	if got.ExtInfo["reconcile_method"] != "llm_ambiguous_resolution" || got.ExtInfo["reconcile_model"] != "test-model" {
		t.Fatalf("ext_info = %+v, want LLM provenance", got.ExtInfo)
	}
	if !containsString(got.NormalizedNames, "systolic blood pressure") {
		t.Fatalf("normalized names = %v, want refreshed English name", got.NormalizedNames)
	}
}

func TestApplyAmbiguousObjectLLMDecisionLeavesLowConfidenceAmbiguous(t *testing.T) {
	obj := ArtifactObject{ArtifactID: "9_prv_1", ObjectName: "收缩压"}
	candidates := []ObjectNodeCandidate{{Node: ObjectNode{ObjectID: "obj_sbp"}}}
	decision := AmbiguousObjectLLMDecision{
		ModelName:            "test-model",
		ResolutionObjectID:   "obj_sbp",
		ResolutionConfidence: 0.7,
	}

	got, applied, err := ApplyAmbiguousObjectLLMDecision(context.Background(), obj, candidates, decision, nil, 0.85)
	if err != nil {
		t.Fatalf("ApplyAmbiguousObjectLLMDecision: %v", err)
	}
	if applied {
		t.Fatalf("applied = true, want false below threshold")
	}
	if got.ObjectID != "" || got.ReconcileStatus != ObjectReconcileAmbiguous {
		t.Fatalf("object = %+v, want unresolved ambiguous", got)
	}
}

func TestApplyAmbiguousObjectLLMDecisionRejectsUnknownSelectedObjectID(t *testing.T) {
	obj := ArtifactObject{ArtifactID: "9_prv_1", ObjectName: "收缩压"}
	candidates := []ObjectNodeCandidate{{Node: ObjectNode{ObjectID: "obj_sbp"}}}
	decision := AmbiguousObjectLLMDecision{
		ModelName:            "test-model",
		ResolutionObjectID:   "obj_missing",
		ResolutionConfidence: 0.95,
	}

	_, _, err := ApplyAmbiguousObjectLLMDecision(context.Background(), obj, candidates, decision, nil, 0.85)
	if err == nil {
		t.Fatalf("ApplyAmbiguousObjectLLMDecision error = nil, want invalid selected object id")
	}
}

func TestApplyAmbiguousObjectLLMDecisionAppliesNodeUpdatesAndMerges(t *testing.T) {
	obj := ArtifactObject{ArtifactID: "9_prv_1", ObjectName: "SBP"}
	candidates := []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_sbp", CanonicalName: "收缩压"}},
		{Node: ObjectNode{ObjectID: "obj_sbp_dup", CanonicalName: "Systolic BP"}},
	}
	store := &fakeAmbiguousObjectLLMApplyStore{}
	decision := AmbiguousObjectLLMDecision{
		ModelName:            "test-model",
		ResolutionObjectID:   "obj_sbp_dup",
		ResolutionConfidence: 0.93,
		NodeUpdates: []AmbiguousObjectNodeLLMUpdate{{
			ObjectID:        "obj_sbp",
			CanonicalNameEn: "systolic blood pressure",
			ObjectType:      "vital_sign",
			Description:     "Pressure during cardiac contraction.",
		}},
		Merges: []AmbiguousObjectNodeLLMMerge{{SurvivorObjectID: "obj_sbp", LoserObjectIDs: []string{"obj_sbp_dup"}, Confidence: 0.94}},
	}

	got, applied, err := ApplyAmbiguousObjectLLMDecision(context.Background(), obj, candidates, decision, store, 0.85)
	if err != nil {
		t.Fatalf("ApplyAmbiguousObjectLLMDecision: %v", err)
	}
	if !applied || got.ObjectID != "obj_sbp" {
		t.Fatalf("resolved object = %+v applied=%v, want survivor obj_sbp", got, applied)
	}
	if len(store.nodeUpdates) != 1 || store.nodeUpdates[0].ObjectID != "obj_sbp" {
		t.Fatalf("node updates = %+v, want obj_sbp update", store.nodeUpdates)
	}
	if len(store.merges) != 1 || store.merges[0].SurvivorObjectID != "obj_sbp" || store.merges[0].LoserObjectIDs[0] != "obj_sbp_dup" {
		t.Fatalf("merges = %+v, want obj_sbp_dup -> obj_sbp", store.merges)
	}
}

func TestParseAmbiguousObjectLLMDecisionReadsExpectedShape(t *testing.T) {
	payload := map[string]any{
		"artifact_object": map[string]any{
			"object_name_en": "systolic blood pressure",
			"acronyms":       []any{"SBP"},
			"object_type":    "vital sign",
			"description":    "Pressure during cardiac contraction.",
		},
		"object_nodes": []any{
			map[string]any{
				"object_id":         "obj_sbp",
				"canonical_name_en": "systolic blood pressure",
				"object_type":       "vital sign",
				"description":       "Pressure during cardiac contraction.",
			},
		},
		"same_object_groups": []any{
			map[string]any{
				"survivor_object_id": "obj_sbp",
				"loser_object_ids":   []any{"obj_sbp_dup"},
				"confidence":         0.94,
			},
		},
		"resolution": map[string]any{
			"object_id":  "obj_sbp",
			"confidence": 0.93,
		},
		"rationale": "The artifact and node both refer to systolic blood pressure.",
	}

	got, err := parseAmbiguousObjectLLMDecision(payload, "test-model")
	if err != nil {
		t.Fatalf("parseAmbiguousObjectLLMDecision: %v", err)
	}
	if got.ModelName != "test-model" || got.ResolutionObjectID != "obj_sbp" || got.ResolutionConfidence != 0.93 {
		t.Fatalf("decision = %+v, unexpected resolution", got)
	}
	if got.ArtifactUpdates.ObjectNameEn != "systolic blood pressure" || got.ArtifactUpdates.ObjectType != "vital_sign" {
		t.Fatalf("artifact updates = %+v, unexpected", got.ArtifactUpdates)
	}
	if len(got.NodeUpdates) != 1 || got.NodeUpdates[0].ObjectID != "obj_sbp" || got.NodeUpdates[0].ObjectType != "vital_sign" {
		t.Fatalf("node updates = %+v, unexpected", got.NodeUpdates)
	}
	if len(got.Merges) != 1 || got.Merges[0].SurvivorObjectID != "obj_sbp" || got.Merges[0].LoserObjectIDs[0] != "obj_sbp_dup" {
		t.Fatalf("merges = %+v, unexpected", got.Merges)
	}
}

func TestParseAmbiguousObjectLLMDecisionAcceptsTopLevelResolutionShape(t *testing.T) {
	payload := map[string]any{
		"same_object_groups": []any{
			map[string]any{
				"survivor_object_id": "obj_244_3092b9548ab7",
				"loser_object_ids":   []any{"obj_244_af72f9a123dc"},
				"confidence":         0.95,
			},
		},
		"selected_resolution_object_id": "obj_244_3092b9548ab7",
		"confidence":                    0.95,
	}

	got, err := parseAmbiguousObjectLLMDecision(payload, "test-model")
	if err != nil {
		t.Fatalf("parseAmbiguousObjectLLMDecision: %v", err)
	}
	if got.ResolutionObjectID != "obj_244_3092b9548ab7" || got.ResolutionConfidence != 0.95 {
		t.Fatalf("decision = %+v, want top-level selected resolution", got)
	}
	if len(got.Merges) != 1 || got.Merges[0].SurvivorObjectID != "obj_244_3092b9548ab7" || got.Merges[0].LoserObjectIDs[0] != "obj_244_af72f9a123dc" {
		t.Fatalf("merges = %+v, unexpected", got.Merges)
	}
}

type fakeAmbiguousObjectLLMApplyStore struct {
	nodeUpdates []AmbiguousObjectNodeLLMUpdate
	merges      []AmbiguousObjectNodeLLMMerge
}

func (s *fakeAmbiguousObjectLLMApplyStore) ApplyAmbiguousObjectLLMNodeChanges(_ context.Context, _ ArtifactObject, updates []AmbiguousObjectNodeLLMUpdate, merges []AmbiguousObjectNodeLLMMerge, _ AmbiguousObjectLLMAudit) error {
	s.nodeUpdates = append(s.nodeUpdates, updates...)
	s.merges = append(s.merges, merges...)
	return nil
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

func TestArtifactObjectSQLStoreListAmbiguousSummariesReadsRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM kb.artifact_objects").
		WithArgs(ObjectReconcileAmbiguous).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "artifact_type", "artifact_id", "object_name", "object_name_en", "confidence",
		}).AddRow(
			int64(7), searchArtifactProvision, "9_prv_1", "pressure regulator", "", 0.85,
		))

	store := ArtifactObjectSQLStore{DB: db}
	rows, err := store.ListAmbiguousSummaries(context.Background())
	if err != nil {
		t.Fatalf("ListAmbiguousSummaries: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != 7 || rows[0].ObjectName != "pressure regulator" {
		t.Fatalf("rows = %+v, unexpected", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestArtifactObjectSQLStoreLoadByIDReadsRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM kb.artifact_objects").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_record_id", "input_record_id", "artifact_type", "artifact_id",
			"object_name", "object_name_en", "object_name_zh",
			"language", "object_type", "object_role",
			"aliases", "acronyms", "normalized_names",
			"description", "evidence_quote", "source_line_spans", "confidence",
			"object_id", "reconcile_status", "reconcile_confidence",
		}).AddRow(
			int64(7), int64(9), int64(9), searchArtifactProvision, "9_prv_1",
			"pressure regulator", "", "",
			"", "equipment", "regulated_object",
			[]byte(`["reg"]`), []byte(`[]`), []byte(`["pressure regulator"]`),
			"", "", []byte(`["8"]`), 0.85,
			"", ObjectReconcileAmbiguous, 0.0,
		))

	store := ArtifactObjectSQLStore{DB: db}
	obj, found, err := store.LoadByID(context.Background(), 7)
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if !found {
		t.Fatalf("found = false, want true")
	}
	if obj.ID != 7 || obj.ObjectName != "pressure regulator" || obj.ReconcileStatus != ObjectReconcileAmbiguous {
		t.Fatalf("obj = %+v, unexpected", obj)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestArtifactObjectSQLStoreLoadByIDReturnsNotFoundForMissingRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM kb.artifact_objects").
		WithArgs(int64(999)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "source_record_id", "input_record_id", "artifact_type", "artifact_id",
			"object_name", "object_name_en", "object_name_zh",
			"language", "object_type", "object_role",
			"aliases", "acronyms", "normalized_names",
			"description", "evidence_quote", "source_line_spans", "confidence",
			"object_id", "reconcile_status", "reconcile_confidence",
		}))

	store := ArtifactObjectSQLStore{DB: db}
	_, found, err := store.LoadByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if found {
		t.Fatalf("found = true, want false")
	}
}

func TestRankAmbiguousCandidatesMarksRecommended(t *testing.T) {
	obj := ArtifactObject{NormalizedNames: []string{"pressure regulator", "reg"}}
	nodes := &stubObjectNodeStore{candidates: []ObjectNodeCandidate{
		{Node: ObjectNode{ObjectID: "obj_a", NormalizedNames: []string{"pressure regulator"}}, Score: 0.85, Method: "lexical_name"},
		{Node: ObjectNode{ObjectID: "obj_b", NormalizedNames: []string{"pressure regulator", "reg"}}, Score: 0.85, Method: "lexical_name"},
	}}
	reconciler := ObjectReconciler{Store: nodes}

	candidates, recommended, err := RankAmbiguousCandidates(context.Background(), reconciler, obj)
	if err != nil {
		t.Fatalf("RankAmbiguousCandidates: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %+v, want 2", candidates)
	}
	if recommended != "obj_b" {
		t.Fatalf("recommended = %q, want obj_b (more normalized-name overlap)", recommended)
	}
}

func TestRankAmbiguousCandidatesReturnsEmptyRecommendedWhenNoCandidates(t *testing.T) {
	nodes := &stubObjectNodeStore{}
	reconciler := ObjectReconciler{Store: nodes}

	candidates, recommended, err := RankAmbiguousCandidates(context.Background(), reconciler, ArtifactObject{})
	if err != nil {
		t.Fatalf("RankAmbiguousCandidates: %v", err)
	}
	if len(candidates) != 0 || recommended != "" {
		t.Fatalf("candidates = %+v, recommended = %q, want empty", candidates, recommended)
	}
}

func TestObjectNodeSQLStoreApplyAmbiguousObjectLLMNodeChangesUpdatesAndMerges(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.object_nodes SET")).
		WithArgs(
			"systolic blood pressure",
			"",
			"vital_sign",
			"Pressure during cardiac contraction.",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"obj_sbp",
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO kb.object_audit_log").
		WithArgs("kb.object_nodes", "obj_sbp", "edit_fields", sqlmock.AnyArg(), "llm").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE kb.artifact_objects SET object_id").
		WithArgs("obj_sbp", "obj_sbp_dup").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE kb.object_nodes SET canonical_object_id").
		WithArgs("obj_sbp", sqlmock.AnyArg(), "obj_sbp_dup").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO kb.object_audit_log").
		WithArgs("kb.object_nodes", "obj_sbp_dup", "merge_nodes", sqlmock.AnyArg(), "llm").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	store := ObjectNodeSQLStore{DB: db}
	err = store.ApplyAmbiguousObjectLLMNodeChanges(
		context.Background(),
		ArtifactObject{ArtifactID: "9_prv_1", ObjectName: "SBP"},
		[]AmbiguousObjectNodeLLMUpdate{{
			ObjectID:        "obj_sbp",
			CanonicalNameEn: "systolic blood pressure",
			ObjectType:      "vital_sign",
			Description:     "Pressure during cardiac contraction.",
		}},
		[]AmbiguousObjectNodeLLMMerge{{
			SurvivorObjectID: "obj_sbp",
			LoserObjectIDs:   []string{"obj_sbp_dup"},
			Confidence:       0.94,
		}},
		AmbiguousObjectLLMAudit{ModelName: "test-model", Rationale: "same object"},
	)
	if err != nil {
		t.Fatalf("ApplyAmbiguousObjectLLMNodeChanges: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
