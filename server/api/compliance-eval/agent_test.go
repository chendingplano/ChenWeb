package complianceeval

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	docreviews "github.com/chendingplano/deepdoc/server/api/doc-reviews"
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
	} {
		if _, err := CheckActivities(twoChunks(), tc.in); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", name, err, tc.want)
		}
	}
}

func agentPool() (PoolFile, []Activity) {
	cos := 0.7
	pf := PoolFile{SnapshotID: 1, RecordID: 416, MaxCandidates: 50, Activities: []PoolActivity{
		{ID: "a1", AgentActivity: AgentActivity{Activity: "log_user_actions", LineStart: 2, LineEnd: 2}, Candidates: []PoolCandidate{
			{ID: "chunk/c1", ArtifactType: "chunk", ArtifactID: "c1", RecordID: 9, Cosine: &cos, Text: "logs shall be kept"},
			{ID: "provision/p1", ArtifactType: "provision", ArtifactID: "p1", RecordID: 10, Text: "background"},
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
	}, GoldDoc{RecordID: 416, Covered: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Activities) != 2 || len(doc.Activities[0].Judgements) != 2 || len(doc.Activities[1].Judgements) != 0 {
		t.Fatalf("doc = %+v", doc)
	}
	j := doc.Activities[0].Judgements[0]
	if j.Final != "governs" || j.Label1 != "governs" || j.Label2 != "" || j.Adjudicated || j.Candidate.RecordID != 9 || *j.Candidate.Cosine != 0.7 {
		t.Errorf("judgement = %+v", j)
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
