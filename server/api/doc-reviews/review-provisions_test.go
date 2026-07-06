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

	// Branch A: live hybrid-search hits for P1 (index 0), keyed by doc provision index.
	hybridMatches := map[int][]docprocessing.OnTheFlySemanticMatch{
		0: {
			{ArtifactID: "2_prv_9", RecordID: 2, RRFScore: 0.9},
			// Also reached via the object-anchor branch below -> must dedup.
			{ArtifactID: "3_prv_7", RecordID: 3, RRFScore: 0.2},
			// Same-document hit -> excluded by add().
			{ArtifactID: "1_prv_5", RecordID: recordID, RRFScore: 0.5},
		},
	}
	resolved := map[string]resolvedProvision{
		"2_prv_9": {view: provisionView{ProvID: "2_prv_9", Categories: []string{"safety/pressure"}}, recordID: 2},
		"3_prv_7": {view: provisionView{ProvID: "3_prv_7", Categories: []string{"safety/pressure"}}, recordID: 3},
		"4_prv_2": {view: provisionView{ProvID: "4_prv_2", Categories: []string{"process/temp"}}, recordID: 4},
		"1_prv_5": {view: provisionView{ProvID: "1_prv_5"}, recordID: recordID}, // same doc
	}
	objectMatches := map[int][]resolvedProvision{
		0: {
			// Branch B: object-anchor -> provision 3_prv_7 for P1.
			{view: provisionView{ProvID: "3_prv_7", Categories: []string{"safety/pressure"}}, recordID: 3},
		},
		1: {
			// Branch B: object-anchor -> provision 4_prv_2 for P2.
			{view: provisionView{ProvID: "4_prv_2", Categories: []string{"process/temp"}}, recordID: 4},
		},
	}

	got := assembleProvisionMatches(recordID, docProvs, hybridMatches, objectMatches, resolved, 0)

	// P1: 2_prv_9 (A) and 3_prv_7 (A+B deduped) = 2; 1_prv_5 excluded.
	if len(got[0]) != 2 {
		t.Fatalf("P1 matches = %d, want 2 (%v)", len(got[0]), got[0])
	}
	for _, m := range got[0] {
		if m.recordID == recordID {
			t.Errorf("P1 match %s is same-document, should be excluded", m.view.ProvID)
		}
	}
	// P2: 4_prv_2 via object-anchor branch.
	if len(got[1]) != 1 || got[1][0].view.ProvID != "4_prv_2" || got[1][0].via != "object_anchor" {
		t.Fatalf("P2 matches = %v, want [4_prv_2 via object_anchor]", got[1])
	}

	// Cap = 1: P1 keeps only highest-confidence match (2_prv_9 @0.9).
	capped := assembleProvisionMatches(recordID, docProvs, hybridMatches, objectMatches, resolved, 1)
	if len(capped[0]) != 1 || capped[0][0].view.ProvID != "2_prv_9" {
		t.Fatalf("capped P1 = %v, want [2_prv_9]", capped[0])
	}
}

func TestAssembleProvisionMatches_BranchA_DedupAndExclusion(t *testing.T) {
	const recordID = int64(1)
	docProvs := []docProvision{dp("1_prv_1")} // index 0

	// A single live hybrid search per doc provision returns a flat, direction-free hit list.
	// Duplicate ids collapse (first-seen wins) and same-document hits are excluded.
	hybridMatches := map[int][]docprocessing.OnTheFlySemanticMatch{
		0: {
			{ArtifactID: "2_prv_9", RecordID: 2, RRFScore: 0.7},
			{ArtifactID: "5_prv_3", RecordID: 5, RRFScore: 0.9},
			{ArtifactID: "2_prv_9", RecordID: 2, RRFScore: 0.4},        // duplicate -> collapse
			{ArtifactID: "1_prv_8", RecordID: recordID, RRFScore: 0.6}, // same doc -> excluded
		},
	}
	resolved := map[string]resolvedProvision{
		"2_prv_9": {view: provisionView{ProvID: "2_prv_9"}, recordID: 2},
		"5_prv_3": {view: provisionView{ProvID: "5_prv_3"}, recordID: 5},
		"1_prv_8": {view: provisionView{ProvID: "1_prv_8"}, recordID: recordID},
	}

	got := assembleProvisionMatches(recordID, docProvs, hybridMatches, nil, resolved, 0)

	if len(got[0]) != 2 {
		t.Fatalf("P1 matches = %d, want 2 (%v)", len(got[0]), got[0])
	}
	ids := map[string]bool{}
	for _, m := range got[0] {
		ids[m.view.ProvID] = true
		if m.recordID == recordID {
			t.Errorf("P1 match %s is same-document, should be excluded", m.view.ProvID)
		}
	}
	if !ids["2_prv_9"] || !ids["5_prv_3"] {
		t.Errorf("P1 matches missing expected ids: %v", ids)
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
		{
			view:     provisionView{ProvID: "2_prv_9", Provision: "shall be 2.5 MPa", SourceLineSpans: []string{"30:32"}},
			recordID: 2,
			filename: "other.pdf",
			via:      "hybrid_search",
			context: []map[string]any{
				{"line_number": 20, "content": "context before"},
				{"line_number": 30, "content": "matched provision line"},
			},
			confidence: 0.9,
		},
	}

	findings := r.reviewProvision(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "prov-model",
		PromptText: "compare provisions",
		PromptRef:  "prompt-review-provisions-v1.md",
	}, doc, ms, "", false)

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
	for _, want := range []string{"provision_under_review", "matching_provisions", "2_prv_9", "hybrid_search", "source_line_spans", "source_context", "matched provision line"} {
		if !strings.Contains(in, want) {
			t.Errorf("input JSON missing %q: %s", want, in)
		}
	}
}

func TestReviewProvision_SetsCallLocCallReasonAndMetadata(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{"findings": []any{}},
	}
	r := &provisionsReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_PROVISIONS"),
	}
	doc := docProvision{
		view:  provisionView{ProvID: "1_prv_1"},
		spans: []string{"20:24"},
	}

	ctx := withLLMRunID(context.Background(), 123)
	r.reviewProvision(ctx, 1, 0, ReviewerConfig{
		ModelName:  "prov-model",
		PromptText: "compare provisions",
		PromptRef:  "prompt-review-provisions-v1.md",
	}, doc, nil, "", false)

	if len(fake.callReasons) != 1 || fake.callReasons[0] != "review-provision" {
		t.Fatalf("callReasons = %v, want [review-provision]", fake.callReasons)
	}
	if len(fake.callLocs) != 1 || fake.callLocs[0] != "MID-20260706-0001" {
		t.Fatalf("callLocs = %v, want [MID-20260706-0001]", fake.callLocs)
	}
	if len(fake.metadatas) != 1 {
		t.Fatalf("metadatas = %v, want 1 entry", fake.metadatas)
	}
	meta := fake.metadatas[0]
	if meta["provision_id"] != "1_prv_1" {
		t.Errorf("metadata provision_id = %v, want 1_prv_1", meta["provision_id"])
	}
	if meta["run_id"] != int64(123) {
		t.Errorf("metadata run_id = %v, want 123", meta["run_id"])
	}
}

func TestBuildProvisionReviewUnitsIncludesUnmatchedProvisions(t *testing.T) {
	docProvs := []docProvision{
		dp("1_prv_1", "safety/pressure"),
		dp("1_prv_2", "process/temp"),
	}
	matches := map[int][]matchedProvision{
		1: {
			{view: provisionView{ProvID: "2_prv_9"}, recordID: 2, via: "hybrid_search"},
		},
	}

	units := buildProvisionReviewUnits(docProvs, matches)
	if len(units) != 2 {
		t.Fatalf("review units = %d, want one per provision", len(units))
	}
	if len(units[0].matches) != 0 {
		t.Fatalf("first provision matches = %d, want 0", len(units[0].matches))
	}
	if len(units[1].matches) != 1 || units[1].matches[0].view.ProvID != "2_prv_9" {
		t.Fatalf("second provision matches = %+v, want 2_prv_9", units[1].matches)
	}
}
