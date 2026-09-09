package productreviews

import (
	"encoding/json"
	"reflect"
	"testing"
)

func sn(id int64, kind string, label string, aliases ...string) ScopeNode {
	return ScopeNode{ID: id, Kind: kind, Label: label, NameKeys: normalizedKeys(append([]string{label}, aliases...))}
}

func metricArt(id string, rec int64, subject string) Artifact {
	return Artifact{ArtifactType: "metric", ArtifactID: id, InputRecordID: rec, SourceRowID: rec*100 + 1,
		PrimaryLabel: id, SubjectText: subject, LineSpans: json.RawMessage("[]")}
}

var testBudgets = BudgetsConfig{MaxDepth: 3, MaxNodes: 200, MaxDocuments: 400, MaxResults: 2000, PerDocumentCap: 60}

// Scenario 1.3.1 A + 1.3.3 C: a part metric found by both paths appears once,
// retaining both paths; tier is `part` (spec: Two-path retrieval / dedup).
func TestAssemblePartMetricBothPaths(t *testing.T) {
	nodes := []ScopeNode{sn(1, KindProduct, "Ventilator"), sn(2, KindPart, "Display")}
	scoped := []ScopedDoc{{InputRecordID: 42, Reasons: []string{"standard about Ventilator"}}}
	art := metricArt("42_mtc_1", 42, "Display")
	hits := []pathHit{
		{Artifact: art, Path: "document_first", Score: 0.01},
		{Artifact: art, Path: "direct_hybrid", AnchorNode: 2, Score: 0.5},
	}
	out := assembleResults(nodes, scoped, hits, testBudgets)
	if len(out.Results) != 1 {
		t.Fatalf("got %d results, want exactly 1 (deduped)", len(out.Results))
	}
	r := out.Results[0]
	if r.Tier != TierPart || r.NodeID == nil || *r.NodeID != 2 {
		t.Fatalf("tier/node = %s/%v, want part/2", r.Tier, r.NodeID)
	}
	if !reflect.DeepEqual(r.Paths, []string{"direct", "document_first"}) {
		t.Fatalf("paths = %v, want [direct document_first]", r.Paths)
	}
	if r.InclusionReason == "" {
		t.Fatal("inclusion reason is empty")
	}
}

// Scenario 1.3.2 B: a metric matched by concept equality outside the scope set
// is returned with paths=[direct] only and its row names its own document.
func TestAssembleDirectOutsideScope(t *testing.T) {
	nodes := []ScopeNode{sn(1, KindProduct, "Ventilator"), {ID: 2, Kind: KindPart, Label: "Display",
		NameKeys: normalizedKeys([]string{"Display"}), ConceptID: "kc:display"}}
	art := metricArt("77_mtc_3", 77, "Display Luminance")
	art.SubjectConcept = "kc:display"
	out := assembleResults(nodes, nil /* empty scope set */, []pathHit{
		{Artifact: art, Path: "direct_concept", Score: 1.0},
	}, testBudgets)
	if len(out.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(out.Results))
	}
	r := out.Results[0]
	if !reflect.DeepEqual(r.Paths, []string{"direct"}) {
		t.Fatalf("paths = %v, want [direct]", r.Paths)
	}
	if r.Tier != TierPart || r.InputRecordID != 77 {
		t.Fatalf("tier/record = %s/%d, want part/77", r.Tier, r.InputRecordID)
	}
}

// Scenario 1.4.1 A: tier reflects the matched node even when only the
// document-first path found the artifact.
func TestAssembleTierFromNodeNotPath(t *testing.T) {
	nodes := []ScopeNode{sn(1, KindProduct, "Ventilator"), sn(2, KindPart, "Display")}
	scoped := []ScopedDoc{{InputRecordID: 42}}
	out := assembleResults(nodes, scoped, []pathHit{
		{Artifact: metricArt("42_mtc_9", 42, "Display"), Path: "document_first", Score: 0.01},
	}, testBudgets)
	r := out.Results[0]
	if r.Tier != TierPart {
		t.Fatalf("tier = %s, want part (subject matches the Display node)", r.Tier)
	}
	if !reflect.DeepEqual(r.Paths, []string{"document_first"}) {
		t.Fatalf("paths = %v, want [document_first]", r.Paths)
	}
}

// Scenario 1.4.2 B + 1.4.3 C: an unattributed metric in a scoped document is
// retained with tier document_scope, a null node, and a reason naming the doc.
func TestAssembleUnattributedRetained(t *testing.T) {
	nodes := []ScopeNode{sn(1, KindProduct, "Ventilator"), sn(2, KindPart, "Display")}
	scoped := []ScopedDoc{{InputRecordID: 42, Reasons: []string{"Path A: root node Ventilator, relation scope"}}}
	out := assembleResults(nodes, scoped, []pathHit{
		{Artifact: metricArt("42_mtc_5", 42, "Ambient Temperature"), Path: "document_first", Score: 0.01},
	}, testBudgets)
	r := out.Results[0]
	if r.Tier != TierDocumentScope || r.NodeID != nil {
		t.Fatalf("tier/node = %s/%v, want document_scope/nil", r.Tier, r.NodeID)
	}
	if r.InclusionReason == "" || !contains(r.InclusionReason, "42") || !contains(r.InclusionReason, "relation scope") {
		t.Fatalf("reason = %q, want it to name doc 42 and the scoping reason", r.InclusionReason)
	}
	if out.DocumentScopeCount != 1 || out.AttributedCount != 0 {
		t.Fatalf("counts = attr %d / scope %d, want 0 / 1", out.AttributedCount, out.DocumentScopeCount)
	}
}

// Scenario 1.5.1 A: the per-document cap keeps the highest-scoring hits and the
// run records the truncated count for that document.
func TestAssemblePerDocumentCap(t *testing.T) {
	nodes := []ScopeNode{sn(1, KindProduct, "Ventilator"), sn(2, KindPart, "Display")}
	scoped := []ScopedDoc{{InputRecordID: 42}}
	mk := func(id string, score float64) pathHit {
		return pathHit{Artifact: metricArt(id, 42, "Display"), Path: "direct_hybrid", AnchorNode: 2, Score: score}
	}
	b := testBudgets
	b.PerDocumentCap = 2
	out := assembleResults(nodes, scoped, []pathHit{
		mk("42_mtc_a", 0.9), mk("42_mtc_b", 0.5), mk("42_mtc_c", 0.1),
	}, b)
	if len(out.Results) != 2 {
		t.Fatalf("got %d results, want 2 (per-document cap)", len(out.Results))
	}
	if out.PerDocumentTruncated[42] != 1 || out.TruncatedCount != 1 {
		t.Fatalf("truncation = perdoc %v / total %d, want 1 / 1", out.PerDocumentTruncated, out.TruncatedCount)
	}
	kept := map[string]bool{}
	for _, r := range out.Results {
		kept[r.ArtifactID] = true
	}
	if !kept["42_mtc_a"] || !kept["42_mtc_b"] || kept["42_mtc_c"] {
		t.Fatalf("kept the wrong hits: %v (want a,b — the highest scores)", kept)
	}
}

// Scenario 1.5.2 B: at the run cap, document_scope results are removed before
// attributed ones.
func TestAssembleAttributedSurviveRunCap(t *testing.T) {
	nodes := []ScopeNode{sn(1, KindProduct, "Ventilator"), sn(2, KindPart, "Display")}
	scoped := []ScopedDoc{{InputRecordID: 42}, {InputRecordID: 43}}
	b := testBudgets
	b.MaxResults = 2
	out := assembleResults(nodes, scoped, []pathHit{
		{Artifact: metricArt("42_mtc_1", 42, "Display"), Path: "direct_hybrid", AnchorNode: 2, Score: 0.8},
		{Artifact: metricArt("43_mtc_1", 43, "Airflow"), Path: "document_first", Score: 0.01},
		{Artifact: metricArt("43_mtc_2", 43, "Noise"), Path: "document_first", Score: 0.01},
	}, b)
	if len(out.Results) != 2 {
		t.Fatalf("got %d results, want 2 (run cap)", len(out.Results))
	}
	if out.AttributedCount != 1 {
		t.Fatalf("attributed count = %d, want 1 (the part-tier hit must survive)", out.AttributedCount)
	}
	var partSurvived bool
	for _, r := range out.Results {
		if r.ArtifactID == "42_mtc_1" && r.Tier == TierPart {
			partSurvived = true
		}
	}
	if !partSurvived {
		t.Fatal("the part-tier result was dropped in favour of a document_scope result")
	}
	if out.TruncatedCount != 1 {
		t.Fatalf("truncated count = %d, want 1", out.TruncatedCount)
	}
}

// Scenario 1.6.1 A: two assemblies over identical inputs produce identical
// result sets, tiers, and ordering (spec: Retrieval determinism).
func TestAssembleDeterministic(t *testing.T) {
	nodes := []ScopeNode{
		sn(1, KindProduct, "Ventilator"), sn(2, KindPart, "Display"),
		sn(3, KindPart, "Battery"), {ID: 4, Kind: KindAspect, Label: "Storage", AspectKey: "storage", RelationTypes: []string{"storage_requirement"}},
	}
	scoped := []ScopedDoc{{InputRecordID: 42, Reasons: []string{"r1"}}, {InputRecordID: 51, Reasons: []string{"r2"}}}
	hits := []pathHit{
		{Artifact: metricArt("42_mtc_1", 42, "Display"), Path: "document_first", Score: 0.01},
		{Artifact: metricArt("42_mtc_1", 42, "Display"), Path: "direct_hybrid", AnchorNode: 2, Score: 0.44},
		{Artifact: metricArt("42_mtc_2", 42, "Battery"), Path: "direct_hybrid", AnchorNode: 3, Score: 0.44},
		{Artifact: metricArt("42_mtc_7", 42, "Unmapped"), Path: "document_first", Score: 0.01},
		{Artifact: metricArt("51_mtc_3", 51, "Battery"), Path: "direct_concept", Score: 1.0},
	}
	a := assembleResults(nodes, scoped, hits, testBudgets)
	b := assembleResults(nodes, scoped, hits, testBudgets)
	if !reflect.DeepEqual(a.Results, b.Results) {
		t.Fatalf("non-deterministic result set:\n a=%+v\n b=%+v", a.Results, b.Results)
	}
	if a.AttributedCount != b.AttributedCount || a.DocumentScopeCount != b.DocumentScopeCount {
		t.Fatal("non-deterministic counts")
	}
	// 42_mtc_2 (Battery) matched the Battery node even though only in doc 42.
	for _, r := range a.Results {
		if r.ArtifactID == "42_mtc_2" && r.Tier != TierPart {
			t.Fatalf("42_mtc_2 tier = %s, want part", r.Tier)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
