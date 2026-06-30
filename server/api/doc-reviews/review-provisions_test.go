package docreviews

import (
	"context"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func dp(provID string, cats ...string) docProvision {
	return docProvision{view: provisionView{ProvID: provID, Categories: cats}}
}

func TestAssembleProvisionMatches_BranchesDedupExclusionCap(t *testing.T) {
	const recordID = int64(1)
	docProvs := []docProvision{
		dp("1_prv_1", "safety/pressure"), // index 0
		dp("1_prv_2", "process/temp"),    // index 1
	}

	hsEdges := []docprocessing.Connection{
		// Branch A: P1 -> cross-doc provision 2_prv_9 (semantically_related).
		{SourceID: "1_prv_1", TargetID: "2_prv_9", RelationName: docprocessing.RelationSemanticallyRelated, Confidence: 0.9},
		// Duplicate target also reached via entity branch -> dedup.
		{SourceID: "1_prv_1", TargetID: "3_prv_7", RelationName: docprocessing.RelationSemanticallyRelated, Confidence: 0.2},
		// Wrong relation_name -> ignored.
		{SourceID: "1_prv_1", TargetID: "9_prv_9", RelationName: "other", Confidence: 1.0},
		// Same-document target -> excluded.
		{SourceID: "1_prv_1", TargetID: "1_prv_5", RelationName: docprocessing.RelationSemanticallyRelated, Confidence: 0.5},
	}
	entEdges := []docprocessing.Connection{
		// Branch B: entity -> provision 3_prv_7 (shares "safety/pressure" with P1).
		{TargetID: "3_prv_7", Confidence: 0.3},
		// entity -> provision 4_prv_2 (shares "process/temp" with P2).
		{TargetID: "4_prv_2", Confidence: 0.4},
	}
	resolved := map[string]resolvedProvision{
		"2_prv_9": {view: provisionView{ProvID: "2_prv_9", Categories: []string{"safety/pressure"}}, recordID: 2},
		"3_prv_7": {view: provisionView{ProvID: "3_prv_7", Categories: []string{"safety/pressure"}}, recordID: 3},
		"4_prv_2": {view: provisionView{ProvID: "4_prv_2", Categories: []string{"process/temp"}}, recordID: 4},
		"1_prv_5": {view: provisionView{ProvID: "1_prv_5"}, recordID: recordID}, // same doc
	}

	got := assembleProvisionMatches(recordID, docProvs, hsEdges, entEdges, resolved, 0)

	// P1: 2_prv_9 (A) and 3_prv_7 (A+B deduped) = 2; 1_prv_5 excluded.
	if len(got[0]) != 2 {
		t.Fatalf("P1 matches = %d, want 2 (%v)", len(got[0]), got[0])
	}
	for _, m := range got[0] {
		if m.recordID == recordID {
			t.Errorf("P1 match %s is same-document, should be excluded", m.view.ProvID)
		}
	}
	// P2: 4_prv_2 via entity branch (shared category process/temp).
	if len(got[1]) != 1 || got[1][0].view.ProvID != "4_prv_2" || got[1][0].via != "entity" {
		t.Fatalf("P2 matches = %v, want [4_prv_2 via entity]", got[1])
	}

	// Cap = 1: P1 keeps only highest-confidence match (2_prv_9 @0.9).
	capped := assembleProvisionMatches(recordID, docProvs, hsEdges, entEdges, resolved, 1)
	if len(capped[0]) != 1 || capped[0][0].view.ProvID != "2_prv_9" {
		t.Fatalf("capped P1 = %v, want [2_prv_9]", capped[0])
	}
}

func TestReviewProvision_PayloadAndFindingTagging(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{"title": "Conflict", "description": "requirements differ"},
			},
		},
	}
	r := &provisionsReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_PROVISIONS"),
	}
	doc := docProvision{
		view:  provisionView{ProvID: "1_prv_1", ProvName: "Relief valve", Provision: "shall be 1.6 MPa"},
		spans: []string{"20:24"},
	}
	ms := []matchedProvision{
		{view: provisionView{ProvID: "2_prv_9", Provision: "shall be 2.5 MPa"}, recordID: 2, filename: "other.pdf", via: "hybrid_search", confidence: 0.9},
	}

	findings := r.reviewProvision(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "prov-model",
		PromptText: "compare provisions",
		PromptRef:  "prompt-review-provisions-v1.md",
	}, doc, ms)

	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "provisions" {
		t.Errorf("pass/aspect = %q/%q, want P5/provisions", f.Pass, f.Aspect)
	}
	if f.FindingType != "issue" || f.Severity != "low" {
		t.Errorf("defaults: finding_type=%q severity=%q, want issue/low", f.FindingType, f.Severity)
	}
	if f.Location != "20:24" {
		t.Errorf("location = %q, want 20:24", f.Location)
	}
	if len(fake.documentFirst) != 1 || !fake.documentFirst[0] {
		t.Errorf("documentFirst = %v, want [true]", fake.documentFirst)
	}
	in := fake.inputTexts[0]
	for _, want := range []string{"provision_under_review", "matching_provisions", "2_prv_9", "hybrid_search"} {
		if !strings.Contains(in, want) {
			t.Errorf("input JSON missing %q: %s", want, in)
		}
	}
}
