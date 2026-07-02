package docprocessing

import (
	"context"
	"reflect"
	"testing"
)

func TestNormalizeArtifactObjectsSynthesizesMetricSubject(t *testing.T) {
	metric := map[string]any{
		"metric_id":         "42_mtc_1",
		"subject":           "LPG storage tank",
		"subject_en":        "LPG storage tank",
		"source_line_spans": []string{"12", "13:14"},
	}

	got := normalizeArtifactObjectsForArtifact(42, searchArtifactMetric, "42_mtc_1", metric)
	if len(got) != 1 {
		t.Fatalf("got %d objects, want 1: %+v", len(got), got)
	}
	obj := got[0]
	if obj.InputRecordID != 42 || obj.ArtifactType != searchArtifactMetric || obj.ArtifactID != "42_mtc_1" {
		t.Fatalf("unexpected artifact link: %+v", obj)
	}
	if obj.ObjectName != "LPG storage tank" || obj.ObjectNameEn != "LPG storage tank" {
		t.Fatalf("unexpected object names: %+v", obj)
	}
	if obj.ObjectRole != "measured_object" {
		t.Fatalf("object role = %q, want measured_object", obj.ObjectRole)
	}
	if !reflect.DeepEqual(obj.SourceLineSpans, []string{"12:14"}) {
		t.Fatalf("source spans = %v, want [12:14]", obj.SourceLineSpans)
	}
	if !containsString(obj.NormalizedNames, "lpg storage tank") {
		t.Fatalf("normalized names = %v, want lpg storage tank", obj.NormalizedNames)
	}
}

func TestNormalizeArtifactObjectsUsesExplicitObjectsAndDedupes(t *testing.T) {
	metric := map[string]any{
		"subject": "fallback subject",
		"objects": []any{
			map[string]any{
				"object_name":       "液化气储罐",
				"object_name_en":    "LPG storage tank",
				"object_type":       "equipment",
				"object_role":       "measured_object",
				"aliases":           []any{"LPG tank"},
				"acronyms":          []any{"LPG"},
				"source_line_spans": []any{"3", "4"},
				"confidence":        0.82,
			},
			map[string]any{
				"object_name": " 液化气储罐 ",
				"object_role": "measured_object",
			},
		},
	}

	got := normalizeArtifactObjectsForArtifact(7, searchArtifactMetric, "7_mtc_1", metric)
	if len(got) != 1 {
		t.Fatalf("got %d objects, want 1: %+v", len(got), got)
	}
	obj := got[0]
	if obj.ObjectName != "液化气储罐" || obj.ObjectNameEn != "LPG storage tank" {
		t.Fatalf("unexpected object names: %+v", obj)
	}
	if obj.ObjectType != "equipment" || obj.Confidence != 0.82 {
		t.Fatalf("unexpected object metadata: %+v", obj)
	}
	for _, want := range []string{"液化气储罐", "lpg storage tank", "lpg tank", "lpg"} {
		if !containsString(obj.NormalizedNames, want) {
			t.Fatalf("normalized names = %v, want %q", obj.NormalizedNames, want)
		}
	}
}

func TestReconcileArtifactObjectExactAliasMatch(t *testing.T) {
	store := &memoryObjectNodeStore{
		nodes: []ObjectNode{{
			ObjectID:        "obj_1",
			CanonicalName:   "Liquefied gas storage tank",
			ObjectType:      "equipment",
			Aliases:         []string{"LPG tank"},
			NormalizedNames: []string{"liquefied gas storage tank", "lpg tank"},
		}},
	}
	reconciler := ObjectReconciler{Store: store}

	obj := ArtifactObject{
		ObjectName:      "LPG tank",
		ObjectType:      "equipment",
		ObjectRole:      "measured_object",
		NormalizedNames: []string{"lpg tank"},
	}
	got, err := reconciler.ReconcileOne(context.Background(), obj)
	if err != nil {
		t.Fatalf("ReconcileOne: %v", err)
	}
	if got.ObjectID != "obj_1" || got.Status != ObjectReconcileMatched {
		t.Fatalf("result = %+v, want matched obj_1", got)
	}
	if got.Confidence != 1 {
		t.Fatalf("confidence = %v, want 1", got.Confidence)
	}
}

func TestReconcileArtifactObjectCreatesNodeWhenNoMatch(t *testing.T) {
	store := &memoryObjectNodeStore{}
	reconciler := ObjectReconciler{Store: store}

	obj := ArtifactObject{
		InputRecordID:    9,
		ObjectName:       "pressure regulator",
		ObjectType:       "equipment",
		ObjectRole:       "measured_object",
		NormalizedNames:  []string{"pressure regulator"},
		SourceLineSpans:  []string{"8"},
		ArtifactType:     searchArtifactMetric,
		ArtifactID:       "9_mtc_1",
		ReconcileStatus:  ObjectReconcilePending,
		ReconcileConfidence: 0,
	}
	got, err := reconciler.ReconcileOne(context.Background(), obj)
	if err != nil {
		t.Fatalf("ReconcileOne: %v", err)
	}
	if got.Status != ObjectReconcileNew || got.ObjectID == "" {
		t.Fatalf("result = %+v, want new object id", got)
	}
	if len(store.nodes) != 1 || store.nodes[0].CanonicalName != "pressure regulator" {
		t.Fatalf("created nodes = %+v", store.nodes)
	}
}

type memoryObjectNodeStore struct {
	nodes []ObjectNode
}

func (s *memoryObjectNodeStore) FindCandidates(_ context.Context, obj ArtifactObject, _ ObjectReconcileOptions) ([]ObjectNodeCandidate, error) {
	var out []ObjectNodeCandidate
	for _, node := range s.nodes {
		if objectTypesCompatible(obj.ObjectType, node.ObjectType) && objectNameBundlesOverlap(obj.NormalizedNames, node.NormalizedNames) {
			out = append(out, ObjectNodeCandidate{Node: node, Score: 1, Method: "exact_name"})
		}
	}
	return out, nil
}

func (s *memoryObjectNodeStore) CreateNode(_ context.Context, obj ArtifactObject) (ObjectNode, error) {
	node := ObjectNode{
		ObjectID:        buildObjectID(obj.InputRecordID, obj.ObjectName, len(s.nodes)+1),
		CanonicalName:   obj.ObjectName,
		CanonicalNameEn: obj.ObjectNameEn,
		ObjectType:      obj.ObjectType,
		Aliases:         obj.Aliases,
		Acronyms:        obj.Acronyms,
		NormalizedNames: obj.NormalizedNames,
	}
	s.nodes = append(s.nodes, node)
	return node, nil
}
