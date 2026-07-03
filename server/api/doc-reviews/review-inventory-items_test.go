package docreviews

import (
	"context"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func di(itemID string, cats ...string) docInventoryItem {
	return docInventoryItem{view: inventoryItemView{ItemID: itemID, Categories: cats}}
}

func TestAssembleInventoryMatches_Branches_DedupExclusionCap(t *testing.T) {
	const recordID = int64(1)
	docItems := []docInventoryItem{
		di("1_inv_1", "valve"), // index 0
		di("1_inv_2", "pump"),  // index 1
	}

	// Branch A: live hybrid-search hits for I1 (index 0), keyed by doc item index.
	hybridMatches := map[int][]docprocessing.OnTheFlySemanticMatch{
		0: {
			{ArtifactID: "2_inv_9", RecordID: 2, RRFScore: 0.9},
			// Also reached via the entity branch below -> must dedup.
			{ArtifactID: "3_inv_7", RecordID: 3, RRFScore: 0.2},
			// Same-document hit -> excluded by add().
			{ArtifactID: "1_inv_5", RecordID: recordID, RRFScore: 0.5},
		},
	}
	entEdges := []docprocessing.Connection{
		// Branch C: entity -> item 3_inv_7 (shares "valve" with I1).
		{TargetID: "3_inv_7", Confidence: 0.3},
	}
	resolved := map[string]resolvedInventoryItem{
		"2_inv_9": {view: inventoryItemView{ItemID: "2_inv_9", Categories: []string{"valve"}}, recordID: 2},
		"3_inv_7": {view: inventoryItemView{ItemID: "3_inv_7", Categories: []string{"valve"}}, recordID: 3},
		"1_inv_5": {view: inventoryItemView{ItemID: "1_inv_5"}, recordID: recordID}, // same doc
	}
	siblings := []resolvedInventoryItem{
		// Branch B: cross-doc item sharing "pump" with I2.
		{view: inventoryItemView{ItemID: "4_inv_2", Categories: []string{"pump"}}, recordID: 4},
	}

	// No cap first: verify branch coverage + dedup + exclusion.
	got := assembleInventoryMatches(recordID, docItems, hybridMatches, entEdges, resolved, siblings, 0)

	// I1 (index 0): 2_inv_9 (A) and 3_inv_7 (A+C deduped) = 2 matches; 1_inv_5 excluded.
	if len(got[0]) != 2 {
		t.Fatalf("I1 matches = %d, want 2 (%v)", len(got[0]), got[0])
	}
	ids := map[string]bool{}
	for _, m := range got[0] {
		ids[m.view.ItemID] = true
		if m.recordID == recordID {
			t.Errorf("I1 match %s is same-document, should be excluded", m.view.ItemID)
		}
	}
	if !ids["2_inv_9"] || !ids["3_inv_7"] {
		t.Errorf("I1 matches missing expected ids: %v", ids)
	}

	// I2 (index 1): 4_inv_2 via category branch.
	if len(got[1]) != 1 || got[1][0].view.ItemID != "4_inv_2" || got[1][0].via != "item_category" {
		t.Fatalf("I2 matches = %v, want [4_inv_2 via item_category]", got[1])
	}

	// Cap = 1: I1 keeps only the highest-confidence match (2_inv_9 @0.9).
	capped := assembleInventoryMatches(recordID, docItems, hybridMatches, entEdges, resolved, siblings, 1)
	if len(capped[0]) != 1 || capped[0][0].view.ItemID != "2_inv_9" {
		t.Fatalf("capped I1 = %v, want [2_inv_9]", capped[0])
	}
}

func TestAssembleInventoryMatches_BranchA_DedupAndExclusion(t *testing.T) {
	const recordID = int64(1)
	docItems := []docInventoryItem{di("1_inv_1")} // index 0

	// A single live hybrid search per doc item returns a flat, direction-free hit list.
	// Duplicate ids collapse (first-seen wins) and same-document hits are excluded.
	hybridMatches := map[int][]docprocessing.OnTheFlySemanticMatch{
		0: {
			{ArtifactID: "2_inv_9", RecordID: 2, RRFScore: 0.7},
			{ArtifactID: "5_inv_3", RecordID: 5, RRFScore: 0.9},
			{ArtifactID: "2_inv_9", RecordID: 2, RRFScore: 0.4},        // duplicate -> collapse
			{ArtifactID: "1_inv_8", RecordID: recordID, RRFScore: 0.6}, // same doc -> excluded
		},
	}
	resolved := map[string]resolvedInventoryItem{
		"2_inv_9": {view: inventoryItemView{ItemID: "2_inv_9"}, recordID: 2},
		"5_inv_3": {view: inventoryItemView{ItemID: "5_inv_3"}, recordID: 5},
		"1_inv_8": {view: inventoryItemView{ItemID: "1_inv_8"}, recordID: recordID},
	}

	got := assembleInventoryMatches(recordID, docItems, hybridMatches, nil, resolved, nil, 0)

	if len(got[0]) != 2 {
		t.Fatalf("I1 matches = %d, want 2 (%v)", len(got[0]), got[0])
	}
	ids := map[string]bool{}
	for _, m := range got[0] {
		ids[m.view.ItemID] = true
		if m.recordID == recordID {
			t.Errorf("I1 match %s is same-document, should be excluded", m.view.ItemID)
		}
	}
	if !ids["2_inv_9"] || !ids["5_inv_3"] {
		t.Errorf("I1 matches missing expected ids: %v", ids)
	}
}

func TestReviewItem_PayloadAndFindingTagging(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{"title": "Conflict", "description": "manufacturers differ"},
			},
		},
	}
	r := &inventoryItemsReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_INVENTORY"),
	}
	doc := docInventoryItem{
		view:  inventoryItemView{ItemID: "1_inv_1", ItemName: "球阀", ModelNumber: "BV-2200", Manufacturer: "Acme Valve Co."},
		spans: []string{"12:14"},
	}
	ms := []matchedInventoryItem{
		{view: inventoryItemView{ItemID: "2_inv_9", ModelNumber: "BV-2200", Manufacturer: "Beta Industrial"}, recordID: 2, filename: "other.pdf", via: "hybrid_search", confidence: 0.9},
	}

	findings := r.reviewItem(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "inventory-model",
		PromptText: "compare items",
		PromptRef:  "prompt-review-inventory-items-v1.md",
	}, doc, ms, "", false)

	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "inventory_items" {
		t.Errorf("pass/aspect = %q/%q, want P5/inventory_items", f.Pass, f.Aspect)
	}
	if f.FindingType != "issue" || f.Severity != "low" {
		t.Errorf("defaults: finding_type=%q severity=%q, want issue/low", f.FindingType, f.Severity)
	}
	if f.Location != "12:14" {
		t.Errorf("location = %q, want 12:14", f.Location)
	}
	if len(fake.documentFirst) != 1 || !fake.documentFirst[0] {
		t.Errorf("documentFirst = %v, want [true]", fake.documentFirst)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-inventory-items-v1.md" {
		t.Errorf("promptNames = %v", fake.promptNames)
	}
	in := fake.inputTexts[0]
	for _, want := range []string{"inventory_item_under_review", "matching_items", "2_inv_9", "hybrid_search"} {
		if !strings.Contains(in, want) {
			t.Errorf("input JSON missing %q: %s", want, in)
		}
	}
}

func TestRawJSONArrayOrNil(t *testing.T) {
	cases := []struct {
		in   string
		want string // "" means nil
	}{
		{`[{"name":"d","value":"50"}]`, `[{"name":"d","value":"50"}]`},
		{``, ``},
		{`[]`, ``},
		{`null`, ``},
		{`{}`, ``},
		{`not json`, ``},
		{` [1,2] `, `[1,2]`},
	}
	for _, c := range cases {
		got := rawJSONArrayOrNil([]byte(c.in))
		if c.want == "" {
			if got != nil {
				t.Errorf("rawJSONArrayOrNil(%q) = %q, want nil", c.in, string(got))
			}
			continue
		}
		if string(got) != c.want {
			t.Errorf("rawJSONArrayOrNil(%q) = %q, want %q", c.in, string(got), c.want)
		}
	}
}
