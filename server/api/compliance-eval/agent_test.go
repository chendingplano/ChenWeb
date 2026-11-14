package complianceeval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	docreviews "github.com/chendingplano/deepdoc/server/api/doc-reviews"
	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

const chunk2JSON = `{"doc_context":"D","lines":[["flag","line_number","page_number","line_type","content"],["n",4,2,"paragraph","Waste is sorted."],["n",5,2,"paragraph","Bins are labelled."]]}`

func twoChunks() []docreviews.GoldChunk {
	return []docreviews.GoldChunk{
		{Index: 0, StartLine: 1, EndLine: 3, InputJSON: chunkJSON},
		{Index: 1, StartLine: 4, EndLine: 5, InputJSON: chunk2JSON},
	}
}

func TestCheckActivities(t *testing.T) {
	got, err := CheckActivities(twoChunks(), []AgentActivity{
		{Activity: "document_specifies_logging", Object: "system", LineStart: 1, LineEnd: 1, DocumentLevel: true},
		{Activity: "sort_waste", Object: "waste", LineStart: 4, LineEnd: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].SourceText != "Waste is sorted.\nBins are labelled." || got[1].Agreement != agreeSingle || !got[0].DocumentLevel {
		t.Fatalf("activities = %+v", got)
	}
	withLists, err := CheckActivities(twoChunks(), []AgentActivity{{Activity: "sort_waste", LineStart: 4, LineEnd: 4,
		Queries: []string{" waste sorting ", "垃圾分类"}, CitedDocs: []string{"GB/T 19095-2019"}}})
	if err != nil || strings.Join(withLists[0].Queries, "|") != "waste sorting|垃圾分类" || withLists[0].CitedDocs[0] != "GB/T 19095-2019" {
		t.Fatalf("queries and citations not kept: %+v err=%v", withLists, err)
	}
	for name, tc := range map[string]struct {
		in   []AgentActivity
		want string
	}{
		"across chunks":       {[]AgentActivity{{Activity: "a", LineStart: 3, LineEnd: 4}}, "not inside one chunk"},
		"outside document":    {[]AgentActivity{{Activity: "a", LineStart: 9, LineEnd: 9}}, "not inside one chunk"},
		"reversed":            {[]AgentActivity{{Activity: "a", LineStart: 3, LineEnd: 2}}, "line_end is before line_start"},
		"empty name":          {[]AgentActivity{{Activity: " ", LineStart: 2, LineEnd: 2}}, "name is empty"},
		"doc level late":      {[]AgentActivity{{Activity: "d", LineStart: 4, LineEnd: 4, DocumentLevel: true}}, "first chunk"},
		"two document levels": {[]AgentActivity{{Activity: "d", LineStart: 1, LineEnd: 1, DocumentLevel: true}, {Activity: "e", LineStart: 2, LineEnd: 2, DocumentLevel: true}}, "only one document-level"},
		"listed twice":        {[]AgentActivity{{Activity: "a", LineStart: 2, LineEnd: 2}, {Activity: "a", LineStart: 2, LineEnd: 2}}, "listed twice"},
		"too many queries":    {[]AgentActivity{{Activity: "a", LineStart: 2, LineEnd: 2, Queries: []string{"1", "2", "3", "4", "5", "6", "7"}}}, "at most 6"},
		"empty query":         {[]AgentActivity{{Activity: "a", LineStart: 2, LineEnd: 2, Queries: []string{"x", " "}}}, "empty entry in queries"},
		"query twice":         {[]AgentActivity{{Activity: "a", LineStart: 2, LineEnd: 2, Queries: []string{"x", " x"}}}, "listed twice in queries"},
		"too many citations":  {[]AgentActivity{{Activity: "a", LineStart: 2, LineEnd: 2, CitedDocs: strings.Split("a,b,c,d,e,f,g,h,i,j,k", ",")}}, "at most 10"},
	} {
		if _, err := CheckActivities(twoChunks(), tc.in); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", name, err, tc.want)
		}
	}
}

func agentPool() (PoolFile, []Activity) {
	cos := 0.7
	pf := PoolFile{SnapshotID: 1, RecordID: 416, MaxCandidates: 50, Activities: []PoolActivity{
		{ID: "a1", AgentActivity: AgentActivity{Activity: "log_user_actions", LineStart: 2, LineEnd: 2}, SearchedRecords: []int64{9}, Candidates: []PoolCandidate{
			{ID: "chunk/c1", ArtifactType: "chunk", ArtifactID: "c1", RecordID: 9, Cosine: &cos, FoundBy: "q1/fused", Text: "logs shall be kept"},
			{ID: "provision/p1", ArtifactType: "provision", ArtifactID: "p1", RecordID: 10, FoundBy: "q2/lexical", Text: "background"},
		}},
		{ID: "a2", AgentActivity: AgentActivity{Activity: "sort_waste", LineStart: 4, LineEnd: 4}, Candidates: []PoolCandidate{}},
	}}
	acts := []Activity{{Name: "log_user_actions", LineStart: 2, LineEnd: 2, Agreement: agreeSingle}, {Name: "sort_waste", LineStart: 4, LineEnd: 4, Agreement: agreeSingle}}
	return pf, acts
}

func TestBuildAgentDocument(t *testing.T) {
	pf, acts := agentPool()
	doc, err := BuildAgentDocument(pf, acts, []AgentLabel{
		{Activity: "a1", Candidate: "chunk/c1", Label: "governs", Reason: "states the retention duty"},
		{Activity: "a1", Candidate: "provision/p1", Label: "topical"},
	}, GoldDoc{RecordID: 416})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Activities) != 2 || len(doc.Activities[0].Judgements) != 2 || len(doc.Activities[1].Judgements) != 0 {
		t.Fatalf("doc = %+v", doc)
	}
	j := doc.Activities[0].Judgements[0]
	if j.Final != "governs" || j.Label1 != "governs" || j.Label2 != "" || j.Adjudicated || j.Candidate.RecordID != 9 || *j.Candidate.Cosine != 0.7 ||
		j.Candidate.FoundBy != "q1/fused" {
		t.Errorf("judgement = %+v", j)
	}
	// A governs label on a record the pool never searched within needs expansion first.
	unexp, _ := agentPool()
	unexp.Activities[0].SearchedRecords = nil
	_, err = BuildAgentDocument(unexp, acts, []AgentLabel{
		{Activity: "a1", Candidate: "chunk/c1", Label: "governs", Reason: "states the retention duty"},
		{Activity: "a1", Candidate: "provision/p1", Label: "topical"},
	}, GoldDoc{RecordID: 416})
	if err == nil || !strings.Contains(err.Error(), "run compliance-gold expand") || !strings.Contains(err.Error(), "a1 record 9") {
		t.Errorf("unexpanded governing record: err = %v", err)
	}
	unexp.BaseRun = "20261114_150000" // an augmentation pool is not expanded
	if _, err := BuildAgentDocument(unexp, acts, []AgentLabel{
		{Activity: "a1", Candidate: "chunk/c1", Label: "governs", Reason: "r"}, {Activity: "a1", Candidate: "provision/p1", Label: "topical"},
	}, GoldDoc{RecordID: 416}); err != nil {
		t.Errorf("augmentation pool: err = %v", err)
	}
	for name, tc := range map[string]struct {
		labels []AgentLabel
		want   string
	}{
		"missing":        {[]AgentLabel{{Activity: "a1", Candidate: "chunk/c1", Label: "unrelated"}}, "1 pooled candidate(s) have no label, for example a1 provision/p1"},
		"twice":          {[]AgentLabel{{Activity: "a1", Candidate: "chunk/c1", Label: "unrelated"}, {Activity: "a1", Candidate: "chunk/c1", Label: "topical"}}, "labelled twice"},
		"not pooled":     {[]AgentLabel{{Activity: "a2", Candidate: "chunk/c1", Label: "unrelated"}}, `has no pooled candidate "chunk/c1"`},
		"bad label":      {[]AgentLabel{{Activity: "a1", Candidate: "chunk/c1", Label: "relevant"}}, `label "relevant" is not`},
		"governs bare":   {[]AgentLabel{{Activity: "a1", Candidate: "chunk/c1", Label: "governs"}}, "needs a reason"},
		"ambiguous bare": {[]AgentLabel{{Activity: "a1", Candidate: "chunk/c1", Label: "ambiguous", Reason: " "}}, "needs a reason"},
	} {
		if _, err := BuildAgentDocument(pf, acts, tc.labels, GoldDoc{RecordID: 416}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", name, err, tc.want)
		}
	}
}

// recordSearch answers a search restricted to one record with results, and any other search with none.
func recordSearch(byRecord map[int64][]searchengine.Result) func(context.Context, searchengine.Request) (*searchengine.Response, error) {
	return func(_ context.Context, req searchengine.Request) (*searchengine.Response, error) {
		if len(req.Filters.RecordIDs) == 1 {
			return &searchengine.Response{Results: byRecord[req.Filters.RecordIDs[0]]}, nil
		}
		return &searchengine.Response{Results: byRecord[0]}, nil
	}
}

func TestExpandPoolSearchesGoverningRecordsOnce(t *testing.T) {
	pf, acts := agentPool()
	pf.Activities[0].SearchedRecords = nil
	acts[0].SourceText = "The operator logs user actions."
	p := &Pipeline{MaxCandidates: 50, Search: recordSearch(map[int64][]searchengine.Result{
		9: {res("c1", 9, 0.8, 1, 1), res("c9", 9, 0.6, 2, 2)}, // c1 is already pooled
	})}
	labels := []AgentLabel{{Activity: "a1", Candidate: "chunk/c1", Label: "governs", Reason: "r"}, {Activity: "a1", Candidate: "provision/p1", Label: "topical"}}
	n, err := ExpandPool(context.Background(), p, &pf, acts, labels)
	if err != nil || n != 1 {
		t.Fatalf("added %d, err %v", n, err)
	}
	last := pf.Activities[0].Candidates[2]
	if last.ID != "chunk/c9" || last.FoundBy != "expand:9" || len(pf.Activities[0].SearchedRecords) != 1 || pf.Activities[0].SearchedRecords[0] != 9 {
		t.Fatalf("activity after expansion = %+v", pf.Activities[0])
	}
	if n, err := ExpandPool(context.Background(), p, &pf, acts, labels); err != nil || n != 0 {
		t.Errorf("second expansion added %d, err %v; want 0", n, err)
	}
	if _, err := ExpandPool(context.Background(), p, &pf, acts, []AgentLabel{{Activity: "a2", Candidate: "chunk/c1", Label: "governs"}}); err == nil {
		t.Error("a label for a pair the pool does not hold must be an error")
	}
	pf.BaseRun = "k"
	if _, err := ExpandPool(context.Background(), p, &pf, acts, labels); err == nil {
		t.Error("an augmentation pool must not be expanded")
	}
}

func TestPoolActivitiesSearchesCitedDocuments(t *testing.T) {
	p := &Pipeline{MaxCandidates: 50, Search: recordSearch(map[int64][]searchengine.Result{
		0:  {res("a1", 9, 0.7, 1, 1)},
		55: {res("x1", 55, 0.4, 1, 0)},
	})}
	cites := NewCitationIndex([]RecordName{{RecordID: 55, DocNo: "GB/T 19095-2008"}})
	a := Activity{Name: "sort_waste", Object: "waste", SourceText: "Waste is sorted.", CitedDocs: []string{"GB/T 19095-2019", "GB 99999-2020"}}
	pas, err := PoolActivities(context.Background(), p, []Activity{a}, []int64{416}, cites)
	if err != nil {
		t.Fatal(err)
	}
	pa := pas[0]
	if len(pa.Candidates) != 2 || pa.Candidates[0].ID != "chunk/a1" || pa.Candidates[1].ID != "chunk/x1" || pa.Candidates[1].FoundBy != "cited:GB/T 19095-2019" {
		t.Fatalf("candidates = %+v", pa.Candidates)
	}
	if len(pa.SearchedRecords) != 1 || pa.SearchedRecords[0] != 55 || len(pa.UnresolvedCitedDocs) != 1 || pa.UnresolvedCitedDocs[0] != "GB 99999-2020" {
		t.Errorf("searched %v, unresolved %v", pa.SearchedRecords, pa.UnresolvedCitedDocs)
	}
	if strings.Join(pa.CitedDocs, "|") != "GB/T 19095-2019|GB 99999-2020" {
		t.Errorf("cited docs not kept in the pool file: %v", pa.CitedDocs)
	}
}

func TestParseLabels(t *testing.T) {
	got, err := ParseLabels(strings.NewReader("{\"activity\":\"a1\",\"candidate\":\"chunk/c1\",\"label\":\"topical\"}\n\n{\"activity\":\"a1\",\"candidate\":\"chunk/c2\",\"label\":\"governs\",\"reason\":\"r\"}\n"))
	if err != nil || len(got) != 2 || got[1].Reason != "r" {
		t.Fatalf("labels = %+v err = %v", got, err)
	}
	if _, err := ParseLabels(strings.NewReader("{\"activity\":\"a1\",\"candidate\":\"c\",\"label\":\"topical\"}\n{\"activity\":\"a1\",\"lable\":\"x\"}\n")); err == nil || !strings.Contains(err.Error(), "labels line 2") {
		t.Errorf("typo field: err = %v", err)
	}
}

func TestLoadRulesVersion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "compliance-review-benchmark", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	text := []byte("# rules 1.0.0\n")
	sum := sha256.Sum256(text)
	hexSum := hex.EncodeToString(sum[:])
	_ = os.WriteFile(filepath.Join(dir, "1.0.0.md"), text, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "versions.json"), []byte(`{"versions":{"1.0.0":{"rules":"1.0.0.md","sha256":"`+hexSum+`"}}}`), 0o644)
	ref, err := LoadRulesVersion(dir, "1.0.0")
	if err != nil || ref.File != "compliance-review-benchmark/rules/1.0.0.md" || ref.SHA256 != hexSum || ref.Version != "1.0.0" {
		t.Fatalf("ref = %+v err = %v", ref, err)
	}
	if _, err := LoadRulesVersion(dir, "2.0.0"); err == nil || !strings.Contains(err.Error(), "known: 1.0.0") {
		t.Errorf("unknown version: err = %v", err)
	}
	_ = os.WriteFile(filepath.Join(dir, "1.0.0.md"), []byte("# rules 1.0.0, edited\n"), 0o644)
	if _, err := LoadRulesVersion(dir, "1.0.0"); err == nil || !strings.Contains(err.Error(), "edited after release") {
		t.Errorf("edited rules: err = %v", err)
	}
}
