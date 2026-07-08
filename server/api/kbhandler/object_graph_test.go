package kbhandler

import (
	"context"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

// fakeGraphSource is an in-memory objectGraphSource for unit-testing the
// bounded BFS in BuildObjectGraph without a database.
type fakeGraphSource struct {
	byID       map[int64]docprocessing.ArtifactObject
	byObjectID map[string][]docprocessing.ArtifactObject
	byArtifact map[string][]docprocessing.ArtifactObject
	nodes      map[string]docprocessing.ObjectNode
	similar    map[string][]docprocessing.OnTheFlySemanticMatch
}

func artKeyTest(t, id string) string { return t + "/" + id }

func (f fakeGraphSource) LoadArtifactObjectByID(_ context.Context, id int64) (docprocessing.ArtifactObject, bool, error) {
	o, ok := f.byID[id]
	return o, ok, nil
}
func (f fakeGraphSource) ObjectsByObjectID(_ context.Context, objectID string) ([]docprocessing.ArtifactObject, error) {
	return f.byObjectID[objectID], nil
}
func (f fakeGraphSource) ObjectsByArtifact(_ context.Context, at, aid string) ([]docprocessing.ArtifactObject, error) {
	return f.byArtifact[artKeyTest(at, aid)], nil
}
func (f fakeGraphSource) ObjectNodeByID(_ context.Context, objectID string) (docprocessing.ObjectNode, bool, error) {
	n, ok := f.nodes[objectID]
	return n, ok, nil
}
func (f fakeGraphSource) SimilarArtifacts(_ context.Context, at, aid string, _ int) ([]docprocessing.OnTheFlySemanticMatch, error) {
	return f.similar[artKeyTest(at, aid)], nil
}

func hasNode(g ObjectGraph, key string) bool {
	for _, n := range g.Nodes {
		if n.Key == key {
			return true
		}
	}
	return false
}

func hasEdge(g ObjectGraph, from, to, typ string) bool {
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.Type == typ {
			return true
		}
	}
	return false
}

func countNode(g ObjectGraph, key string) int {
	n := 0
	for _, node := range g.Nodes {
		if node.Key == key {
			n++
		}
	}
	return n
}

// chainSource builds O1-(m1,M1) ~similar~ M2 -(m2,M2)-> O2 ~similar~ M3 -(m3,M3)-> O3.
func chainSource() fakeGraphSource {
	return fakeGraphSource{
		byID: map[int64]docprocessing.ArtifactObject{
			1: {ID: 1, ArtifactType: "metric", ArtifactID: "M1", ObjectID: "O1", ObjectName: "Pump"},
		},
		byObjectID: map[string][]docprocessing.ArtifactObject{
			"O1": {{ID: 1, ArtifactType: "metric", ArtifactID: "M1", ObjectID: "O1", ObjectName: "Pump"}},
			"O2": {{ID: 2, ArtifactType: "metric", ArtifactID: "M2", ObjectID: "O2", ObjectName: "Valve"}},
			"O3": {{ID: 3, ArtifactType: "metric", ArtifactID: "M3", ObjectID: "O3", ObjectName: "Tank"}},
		},
		byArtifact: map[string][]docprocessing.ArtifactObject{
			"metric/M2": {{ID: 2, ArtifactType: "metric", ArtifactID: "M2", ObjectID: "O2", ObjectName: "Valve"}},
			"metric/M3": {{ID: 3, ArtifactType: "metric", ArtifactID: "M3", ObjectID: "O3", ObjectName: "Tank"}},
		},
		nodes: map[string]docprocessing.ObjectNode{
			"O1": {ObjectID: "O1", CanonicalName: "Pump"},
			"O2": {ObjectID: "O2", CanonicalName: "Valve"},
			"O3": {ObjectID: "O3", CanonicalName: "Tank"},
		},
		similar: map[string][]docprocessing.OnTheFlySemanticMatch{
			"metric/M1": {{ArtifactType: "metric", ArtifactID: "M2"}},
			"metric/M2": {{ArtifactType: "metric", ArtifactID: "M3"}},
		},
	}
}

func TestBuildObjectGraphWiresObjectMentionArtifactAndSimilar(t *testing.T) {
	g, err := BuildObjectGraph(context.Background(), chainSource(),
		objectGraphSeed{ObjectID: "O1"},
		objectGraphOptions{SimilarTopN: 10, RecursiveLevel: 1, MaxNodes: 100})
	if err != nil {
		t.Fatalf("BuildObjectGraph: %v", err)
	}
	if g.SeedObjectID != "O1" {
		t.Errorf("seed_object_id = %q, want O1", g.SeedObjectID)
	}
	if g.Truncated {
		t.Errorf("truncated = true, want false")
	}
	for _, key := range []string{"object:O1", "ao:1", "artifact:metric:M1", "artifact:metric:M2", "object:O2", "ao:2"} {
		if !hasNode(g, key) {
			t.Errorf("missing node %q", key)
		}
	}
	if !hasEdge(g, "ao:1", "artifact:metric:M1", "about") {
		t.Errorf("missing 'about' edge ao:1 -> artifact:metric:M1")
	}
	if !hasEdge(g, "artifact:metric:M1", "artifact:metric:M2", "similar") {
		t.Errorf("missing 'similar' edge M1 -> M2")
	}
	if !hasEdge(g, "ao:1", "object:O1", "same_object") {
		t.Errorf("missing 'same_object' edge ao:1 -> object:O1")
	}
}

func TestBuildObjectGraphTerminatesOnCycle(t *testing.T) {
	src := chainSource()
	// Make it a cycle: M3's object points back to O1, and M3 is similar to M1.
	src.byArtifact["metric/M3"] = []docprocessing.ArtifactObject{
		{ID: 3, ArtifactType: "metric", ArtifactID: "M3", ObjectID: "O1", ObjectName: "Pump"},
	}
	src.similar["metric/M3"] = []docprocessing.OnTheFlySemanticMatch{{ArtifactType: "metric", ArtifactID: "M1"}}

	g, err := BuildObjectGraph(context.Background(), src,
		objectGraphSeed{ObjectID: "O1"},
		objectGraphOptions{SimilarTopN: 10, RecursiveLevel: 5, MaxNodes: 100})
	if err != nil {
		t.Fatalf("BuildObjectGraph: %v", err)
	}
	if countNode(g, "object:O1") != 1 {
		t.Errorf("object:O1 appears %d times, want exactly 1 (dedup)", countNode(g, "object:O1"))
	}
}

func TestBuildObjectGraphRespectsRecursiveLevel(t *testing.T) {
	g, err := BuildObjectGraph(context.Background(), chainSource(),
		objectGraphSeed{ObjectID: "O1"},
		objectGraphOptions{SimilarTopN: 10, RecursiveLevel: 1, MaxNodes: 100})
	if err != nil {
		t.Fatalf("BuildObjectGraph: %v", err)
	}
	if !hasNode(g, "object:O2") {
		t.Errorf("object:O2 should be expanded at depth 1")
	}
	if hasNode(g, "object:O3") {
		t.Errorf("object:O3 should NOT be expanded beyond RecursiveLevel=1")
	}
}

func TestBuildObjectGraphTruncatesAtMaxNodes(t *testing.T) {
	g, err := BuildObjectGraph(context.Background(), chainSource(),
		objectGraphSeed{ObjectID: "O1"},
		objectGraphOptions{SimilarTopN: 10, RecursiveLevel: 5, MaxNodes: 3})
	if err != nil {
		t.Fatalf("BuildObjectGraph: %v", err)
	}
	if !g.Truncated {
		t.Errorf("truncated = false, want true at MaxNodes=3")
	}
	if len(g.Nodes) > 3 {
		t.Errorf("len(nodes) = %d, want <= 3", len(g.Nodes))
	}
}

func TestBuildObjectGraphUnresolvedMentionSeedIsTerminal(t *testing.T) {
	src := fakeGraphSource{
		byID: map[int64]docprocessing.ArtifactObject{
			9: {ID: 9, ArtifactType: "metric", ArtifactID: "M9", ObjectID: "", ReconcileStatus: "ambiguous", ObjectName: "Mystery"},
		},
		byArtifact: map[string][]docprocessing.ArtifactObject{
			"metric/M10": {{ID: 10, ArtifactType: "metric", ArtifactID: "M10", ObjectID: "O10", ObjectName: "Pump"}},
		},
		byObjectID: map[string][]docprocessing.ArtifactObject{
			"O10": {{ID: 10, ArtifactType: "metric", ArtifactID: "M10", ObjectID: "O10", ObjectName: "Pump"}},
		},
		nodes:   map[string]docprocessing.ObjectNode{"O10": {ObjectID: "O10", CanonicalName: "Pump"}},
		similar: map[string][]docprocessing.OnTheFlySemanticMatch{"metric/M9": {{ArtifactType: "metric", ArtifactID: "M10"}}},
	}
	seed := src.byID[9]
	g, err := BuildObjectGraph(context.Background(), src,
		objectGraphSeed{Mention: &seed},
		objectGraphOptions{SimilarTopN: 10, RecursiveLevel: 1, MaxNodes: 100})
	if err != nil {
		t.Fatalf("BuildObjectGraph: %v", err)
	}
	if g.SeedObjectID != "" {
		t.Errorf("seed_object_id = %q, want empty for unresolved mention", g.SeedObjectID)
	}
	if !hasNode(g, "ao:9") {
		t.Fatalf("missing terminal mention node ao:9")
	}
	if hasNode(g, "object:") || hasNode(g, "object:O9") {
		t.Errorf("unresolved seed must not create an object node for itself")
	}
	if !hasNode(g, "object:O10") {
		t.Errorf("similar artifact's object O10 should be reachable/expanded")
	}
}

func TestBuildObjectGraphDedupesSharedArtifact(t *testing.T) {
	src := fakeGraphSource{
		byObjectID: map[string][]docprocessing.ArtifactObject{
			"O1": {
				{ID: 1, ArtifactType: "metric", ArtifactID: "M1", ObjectID: "O1", ObjectName: "Pump"},
				{ID: 2, ArtifactType: "metric", ArtifactID: "M1", ObjectID: "O1", ObjectName: "Pump"},
			},
		},
		nodes:   map[string]docprocessing.ObjectNode{"O1": {ObjectID: "O1", CanonicalName: "Pump"}},
		similar: map[string][]docprocessing.OnTheFlySemanticMatch{},
	}
	g, err := BuildObjectGraph(context.Background(), src,
		objectGraphSeed{ObjectID: "O1"},
		objectGraphOptions{SimilarTopN: 10, RecursiveLevel: 1, MaxNodes: 100})
	if err != nil {
		t.Fatalf("BuildObjectGraph: %v", err)
	}
	if countNode(g, "artifact:metric:M1") != 1 {
		t.Errorf("artifact:metric:M1 appears %d times, want 1", countNode(g, "artifact:metric:M1"))
	}
	if !hasNode(g, "ao:1") || !hasNode(g, "ao:2") {
		t.Errorf("both mention nodes ao:1 and ao:2 should be present")
	}
}
