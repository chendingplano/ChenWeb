package docreviews

import (
	"context"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func de(entityID, name string, aliases ...string) docEntity {
	return docEntity{view: entityView{EntityID: entityID, Name: name, Aliases: aliases}}
}

func TestAssembleEntityMatches_BranchesDedupExclusionCap(t *testing.T) {
	const recordID = int64(1)
	docEntities := []docEntity{
		de("1_e_1", "Sinopec"),  // index 0
		de("1_e_2", "PetroNew"), // index 1
	}

	// Branch A: live hybrid-search hits for E1 (index 0), keyed by doc entity index.
	hybridMatches := map[int][]docprocessing.OnTheFlySemanticMatch{
		0: {
			{ArtifactID: "2_e_9", RecordID: 2, RRFScore: 0.9},
			// Also reached via the name branch below -> must dedup.
			{ArtifactID: "3_e_7", RecordID: 3, RRFScore: 0.2},
			// Same-document hit -> excluded by add().
			{ArtifactID: "1_e_5", RecordID: recordID, RRFScore: 0.5},
		},
	}
	resolved := map[string]resolvedEntity{
		"2_e_9": {view: entityView{EntityID: "2_e_9", Name: "Sinopec"}, recordID: 2},
		"3_e_7": {view: entityView{EntityID: "3_e_7", Name: "Sinopec"}, recordID: 3},
		"1_e_5": {view: entityView{EntityID: "1_e_5", Name: "Sinopec"}, recordID: recordID}, // same doc
	}
	siblings := []resolvedEntity{
		// Shares name "Sinopec" with E1 -> attached to index 0. Same id as the
		// Branch A target 3_e_7 -> must dedup to a single match.
		{view: entityView{EntityID: "3_e_7", Name: "Sinopec"}, recordID: 3},
		// Shares name "PetroNew" with E2 -> attached to index 1.
		{view: entityView{EntityID: "4_e_2", Name: "PetroNew"}, recordID: 4},
		// Same-document sibling -> excluded by the add() guard.
		{view: entityView{EntityID: "1_e_8", Name: "Sinopec"}, recordID: recordID},
	}

	got := assembleEntityMatches(recordID, docEntities, hybridMatches, resolved, siblings, 0)

	// E1: 2_e_9 (A) and 3_e_7 (A+B deduped) = 2; 1_e_5 / 1_e_8 excluded (same doc).
	if len(got[0]) != 2 {
		t.Fatalf("E1 matches = %d, want 2 (%v)", len(got[0]), got[0])
	}
	for _, m := range got[0] {
		if m.recordID == recordID {
			t.Errorf("E1 match %s is same-document, should be excluded", m.view.EntityID)
		}
	}
	// E2: 4_e_2 via name branch.
	if len(got[1]) != 1 || got[1][0].view.EntityID != "4_e_2" || got[1][0].via != "name" {
		t.Fatalf("E2 matches = %v, want [4_e_2 via name]", got[1])
	}

	// Cap = 1: E1 keeps only highest-confidence match (2_e_9 @0.9).
	capped := assembleEntityMatches(recordID, docEntities, hybridMatches, resolved, siblings, 1)
	if len(capped[0]) != 1 || capped[0][0].view.EntityID != "2_e_9" {
		t.Fatalf("capped E1 = %v, want [2_e_9]", capped[0])
	}
}

func TestAssembleEntityMatches_BranchA_DedupAndExclusion(t *testing.T) {
	const recordID = int64(1)
	docEntities := []docEntity{de("1_e_1", "Sinopec")} // index 0

	// A single live hybrid search per doc entity returns a flat, direction-free hit list.
	// Duplicate ids collapse (first-seen wins) and same-document hits are excluded.
	hybridMatches := map[int][]docprocessing.OnTheFlySemanticMatch{
		0: {
			{ArtifactID: "2_e_9", RecordID: 2, RRFScore: 0.7},
			{ArtifactID: "5_e_3", RecordID: 5, RRFScore: 0.9},
			{ArtifactID: "2_e_9", RecordID: 2, RRFScore: 0.4},        // duplicate -> collapse
			{ArtifactID: "1_e_8", RecordID: recordID, RRFScore: 0.6}, // same doc -> excluded
		},
	}
	resolved := map[string]resolvedEntity{
		"2_e_9": {view: entityView{EntityID: "2_e_9", Name: "Sinopec"}, recordID: 2},
		"5_e_3": {view: entityView{EntityID: "5_e_3", Name: "Sinopec"}, recordID: 5},
		"1_e_8": {view: entityView{EntityID: "1_e_8", Name: "Sinopec"}, recordID: recordID},
	}

	got := assembleEntityMatches(recordID, docEntities, hybridMatches, resolved, nil, 0)

	if len(got[0]) != 2 {
		t.Fatalf("E1 matches = %d, want 2 (%v)", len(got[0]), got[0])
	}
	ids := map[string]bool{}
	for _, m := range got[0] {
		ids[m.view.EntityID] = true
		if m.recordID == recordID {
			t.Errorf("E1 match %s is same-document, should be excluded", m.view.EntityID)
		}
	}
	if !ids["2_e_9"] || !ids["5_e_3"] {
		t.Errorf("E1 matches missing expected ids: %v", ids)
	}
}

func TestEntityNameKeys_AliasAndCaseNormalization(t *testing.T) {
	keys := entityNameKeys(entityView{Name: "中国石化", NameEN: "  Sinopec  ", Aliases: []string{"Sinopec Group", ""}})
	for _, want := range []string{"中国石化", "sinopec", "sinopec group"} {
		if _, ok := keys[want]; !ok {
			t.Errorf("name keys missing %q: %v", want, keys)
		}
	}
	if _, ok := keys[""]; ok {
		t.Errorf("empty alias should not produce a key: %v", keys)
	}
}

func TestReviewEntity_PayloadAndFindingTagging(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{"title": "Type conflict", "description": "classified differently"},
			},
		},
	}
	r := &entitiesReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_ENTITIES"),
	}
	doc := docEntity{
		view:  entityView{EntityID: "1_e_1", Name: "Sinopec", Type: "organization"},
		spans: []string{"20:24"},
	}
	ms := []matchedEntity{
		{view: entityView{EntityID: "2_e_9", Name: "Sinopec", Type: "standards body"}, recordID: 2, filename: "other.pdf", via: "name", confidence: 0.9},
	}

	findings := r.reviewEntity(context.Background(), 1, 0, ReviewerConfig{
		ModelName:  "ent-model",
		PromptText: "compare entities",
		PromptRef:  "prompt-review-entities-v1.md",
	}, doc, ms)

	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}
	f := findings[0]
	if f.Pass != "P5" || f.Aspect != "entities" {
		t.Errorf("pass/aspect = %q/%q, want P5/entities", f.Pass, f.Aspect)
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
	for _, want := range []string{"entity_under_review", "matching_entities", "2_e_9", "name"} {
		if !strings.Contains(in, want) {
			t.Errorf("input JSON missing %q: %s", want, in)
		}
	}
}
