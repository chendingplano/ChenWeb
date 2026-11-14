package complianceeval

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

func storedRun() *StoredRun {
	return &StoredRun{ID: 7, Key: "20261114_150000", SnapshotID: 3, RecordID: 416, RulesVersion: "1.1.0", Annotator1: "m", MaxCandidates: 50,
		Activities: []GoldActivity{{
			Activity:   Activity{Name: "log_user_actions", LineStart: 2, LineEnd: 2, Agreement: agreeSingle, Queries: []string{"audit log"}},
			Judgements: []Judgement{{Candidate: Candidate{ArtifactType: "chunk", ArtifactID: "c1", RecordID: 9, FoundBy: "q1/fused"}, Label1: "governs", Final: "governs"}},
		}}}
}

func TestAugmentPoolTakesOnlyUnjudgedSnapshotPassages(t *testing.T) {
	run := storedRun()
	acts := []Activity{{Name: "log_user_actions", LineStart: 2, LineEnd: 2, SourceText: "The operator logs user actions.", Queries: []string{"audit log"}}}
	cos := 0.55
	rows := []ReviewRow{
		{Record: 416, LineStart: 2, LineEnd: 3, Artifacts: []ReviewCandidate{
			{Key: "chunk/c1", Record: 9},                // already judged
			{Key: "chunk/c7", Record: 7, Cosine: &cos},  // to judge
			{Key: "chunk/c8", Record: 800},              // added after the snapshot
			{Key: "chunk/c0", Record: 198},              // a copy of the document
			{Key: "chunk/c7", Record: 7, Cosine: &cos}}, // repeated
		},
		{Record: 416, LineStart: 30, LineEnd: 31, Artifacts: []ReviewCandidate{{Key: "chunk/c5", Record: 7}}}, // covers no gold activity
	}
	var asked []searchengine.ArtifactRef
	load := func(refs []searchengine.ArtifactRef) (map[string]Candidate, error) {
		asked = refs
		return map[string]Candidate{"chunk/c7": {ArtifactType: "chunk", ArtifactID: "c7", Source: "GB 7", Text: "keep logs"}}, nil
	}
	pf, n, err := AugmentPool(run, acts, 101, rows, set(7, 9), set(416, 198), load)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	c := pf.Activities[0].Candidates
	if len(c) != 1 || c[0].ID != "chunk/c7" || c[0].RecordID != 7 || c[0].FoundBy != "review:101" || *c[0].Cosine != 0.55 || c[0].Text != "keep logs" {
		t.Fatalf("candidates = %+v", c)
	}
	if pf.BaseRun != run.Key || pf.ReviewRun != 101 || pf.SnapshotID != 3 || pf.Activities[0].SourceText == "" || pf.Activities[0].Queries[0] != "audit log" {
		t.Errorf("pool file = %+v", pf)
	}
	if len(asked) != 1 || asked[0] != (searchengine.ArtifactRef{Type: "chunk", ID: "c7"}) {
		t.Errorf("loaded %v", asked)
	}
	gone := func([]searchengine.ArtifactRef) (map[string]Candidate, error) { return map[string]Candidate{}, nil }
	if _, _, err := AugmentPool(run, acts, 101, rows, set(7, 9), set(416, 198), gone); err == nil || !strings.Contains(err.Error(), "no longer in the search index") {
		t.Errorf("vanished passage: err = %v", err)
	}
}

func TestMergeAugmentation(t *testing.T) {
	base := storedRun()
	pf := PoolFile{SnapshotID: 3, RecordID: 416, BaseRun: base.Key, ReviewRun: 101}
	aug := &GoldDocument{GoldDoc: GoldDoc{RecordID: 416}, Activities: []GoldActivity{{
		Activity:   Activity{Name: "log_user_actions", LineStart: 2, LineEnd: 2},
		Judgements: []Judgement{{Candidate: Candidate{ArtifactType: "chunk", ArtifactID: "c7", RecordID: 7, FoundBy: "review:101"}, Label1: "topical", Final: "topical"}},
	}}}
	got, err := MergeAugmentation(base, pf, aug)
	if err != nil {
		t.Fatal(err)
	}
	js := got.Activities[0].Judgements
	if len(js) != 2 || js[0].Candidate.ArtifactID != "c1" || js[1].Candidate.FoundBy != "review:101" || got.Activities[0].Queries[0] != "audit log" {
		t.Fatalf("merged = %+v", got.Activities[0])
	}
	for name, tc := range map[string]struct {
		pf   PoolFile
		aug  *GoldDocument
		want string
	}{
		"other snapshot": {PoolFile{SnapshotID: 4, RecordID: 416, BaseRun: base.Key}, aug, "snapshot"},
		"other run":      {PoolFile{SnapshotID: 3, RecordID: 416, BaseRun: "x"}, aug, "augments run x"},
		"other activity": {pf, &GoldDocument{Activities: []GoldActivity{{Activity: Activity{Name: "other", LineStart: 2, LineEnd: 2}}}}, "is not activity 1"},
		"judged twice": {pf, &GoldDocument{Activities: []GoldActivity{{Activity: Activity{Name: "log_user_actions", LineStart: 2, LineEnd: 2},
			Judgements: []Judgement{{Candidate: Candidate{ArtifactType: "chunk", ArtifactID: "c1"}}}}}}, "already judged chunk/c1"},
	} {
		if _, err := MergeAugmentation(base, tc.pf, tc.aug); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", name, err, tc.want)
		}
	}
}

func TestCitationIndex(t *testing.T) {
	ix := NewCitationIndex([]RecordName{
		{RecordID: 1, DocNo: "GB/T 19095-2008", Title: "生活垃圾分类标志"},
		{RecordID: 2, DocNo: "GB/T 19095-2019", Title: "生活垃圾分类标志"},
		{RecordID: 3, Title: "中华人民共和国 固体废物污染环境防治法"},
		{RecordID: 4, FileName: "164176875_GB_T 46121-2025_强制风冷制冷剂冷凝器性能试验方法_mineru.txt"},
	})
	for cite, want := range map[string]string{
		"GB/T 19095-2019": "[1 2]", // every edition the knowledge base holds
		"GB/T 19095":      "[1 2]",
		"《中华人民共和国固体废物污染环境防治法》": "[3]", // a title citation: brackets and spaces ignored
		"GB/T 46121-2025": "[4]",
		"GB 99999-2020":   "[]",
		"生活垃圾":            "[]", // titles match exactly, not by containment
	} {
		if got := fmt.Sprint(ix.Resolve(cite)); got != want {
			t.Errorf("Resolve(%q) = %s, want %s", cite, got, want)
		}
	}
}
