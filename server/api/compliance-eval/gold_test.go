package complianceeval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	docreviews "github.com/chendingplano/deepdoc/server/api/doc-reviews"
	"github.com/chendingplano/deepdoc/server/api/searchengine"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// fakeModel answers by prompt name with a handler; it records the calls it got.
type fakeModel struct {
	handle func(prompt string, in string) map[string]any
	calls  map[string]int
}

func (f *fakeModel) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[in.PromptName]++
	return f.handle(in.PromptName, in.InputText), nil
}

const chunkJSON = `{"doc_context":"D","lines":[["flag","line_number","page_number","line_type","content"],["n",1,1,"heading","Title"],["n",2,1,"paragraph","The operator logs user actions."],["n",3,1,"paragraph","Logs are kept six months."]]}`

func testPipeline(a1, a2, adj *fakeModel, search func(searchengine.Request) *searchengine.Response) *Pipeline {
	return &Pipeline{
		A1: Labeller{"p1", "m1", a1}, A2: Labeller{"p2", "m2", a2}, Adj: Labeller{"pa", "ma", adj},
		Prompts:   Prompts{Activities: "rules\n\nact", Judge: "rules\n\njudge", AdjActivity: "rules\n\nadja", AdjJudgement: "rules\n\nadjj"},
		PoolLimit: 50,
		Chunks: func(context.Context, int64) ([]docreviews.GoldChunk, error) {
			return []docreviews.GoldChunk{{Index: 0, StartLine: 1, EndLine: 3, InputJSON: chunkJSON}}, nil
		},
		Exclude: func(_ context.Context, id int64) ([]int64, error) { return []int64{id, 198}, nil },
		Search: func(_ context.Context, req searchengine.Request) (*searchengine.Response, error) {
			return search(req), nil
		},
	}
}

func acts(entries ...map[string]any) map[string]any {
	l := make([]any, len(entries))
	for i, e := range entries {
		l[i] = e
	}
	return map[string]any{"activities": l, "document_activity": nil}
}

func act(name string, s, e int) map[string]any {
	return map[string]any{"activity": name, "object": "o", "line_start": float64(s), "line_end": float64(e)}
}

func twoResults(req searchengine.Request) *searchengine.Response {
	c1, c2 := 0.8, 0.4
	return &searchengine.Response{Results: []searchengine.Result{
		{ArtifactType: "chunk", ArtifactID: "c1", InputRecordID: 9, Snippet: "logs shall be kept", Diag: searchengine.Diagnostics{Cosine: &c1}},
		{ArtifactType: "chunk", ArtifactID: "c2", InputRecordID: 10, Snippet: "background", Diag: searchengine.Diagnostics{Cosine: &c2}},
	}}
}

func judgeAll(label map[string]string) func(string, string) map[string]any {
	return func(prompt, in string) map[string]any {
		var req struct {
			Candidates []struct{ ID string } `json:"candidates"`
		}
		_ = json.Unmarshal([]byte(in), &req)
		var ls []any
		for _, c := range req.Candidates {
			ls = append(ls, map[string]any{"id": c.ID, "label": label[c.ID], "reason": "r"})
		}
		return map[string]any{"labels": ls}
	}
}

func TestLabelDocumentAgreementAdjudicationAndPool(t *testing.T) {
	// A1 finds two activities, A2 finds the first (same span) and a document-level one; the adjudicator
	// keeps A1's second and rejects A2's document-level.
	a1 := &fakeModel{handle: func(p, in string) map[string]any {
		if p == JudgePromptFile {
			return judgeAll(map[string]string{"chunk/c1": "governs", "chunk/c2": "topical"})(p, in)
		}
		return acts(act("log_user_actions", 2, 2), act("retain_logs", 3, 3))
	}}
	a2 := &fakeModel{handle: func(p, in string) map[string]any {
		if p == JudgePromptFile {
			return judgeAll(map[string]string{"chunk/c1": "governs", "chunk/c2": "unrelated"})(p, in)
		}
		r := acts(act("log_actions", 2, 3))
		r["document_activity"] = act("document_x", 1, 1)
		return r
	}}
	var adjActivities []string
	adj := &fakeModel{handle: func(p, in string) map[string]any {
		if p == AdjActivityPromptFile {
			adjActivities = append(adjActivities, in)
			return map[string]any{"keep": !strings.Contains(in, "document_x"), "rationale": "why"}
		}
		return map[string]any{"label": "topical", "rationale": "resolved"}
	}}
	var excluded, scoped []int64
	p := testPipeline(a1, a2, adj, func(req searchengine.Request) *searchengine.Response {
		excluded, scoped = req.Filters.ExcludeRecordIDs, req.Filters.RecordIDs
		return twoResults(req)
	})
	p.Scope = []int64{9, 10, 416}
	doc, err := p.LabelDocument(context.Background(), GoldDoc{RecordID: 416, Covered: true})
	if err != nil {
		t.Fatal(err)
	}
	// A1:2-2 overlaps A2:2-3 by 1 of the shorter's 1 line -> both; A1:3-3 is single (A2's 2-3 was taken) -> adjudicated keep.
	if len(doc.Activities) != 2 || doc.Dropped != 1 {
		t.Fatalf("activities=%d dropped=%d", len(doc.Activities), doc.Dropped)
	}
	first := doc.Activities[0]
	if first.Agreement != agreeBoth || first.LineStart != 2 || first.LineEnd != 3 {
		t.Errorf("first = %+v", first.Activity)
	}
	if doc.Activities[1].Agreement != agreeAdjudicated || doc.Activities[1].Rationale != "why" {
		t.Errorf("second = %+v", doc.Activities[1].Activity)
	}
	if len(excluded) != 2 || excluded[1] != 198 {
		t.Errorf("pool search excluded %v, want the document and its copy", excluded)
	}
	if len(scoped) != 3 || scoped[0] != 9 {
		t.Errorf("pool search scope = %v, want the snapshot members", scoped)
	}
	js := first.Judgements
	if len(js) != 2 {
		t.Fatalf("judgements = %d", len(js))
	}
	byID := map[string]Judgement{}
	for _, j := range js {
		byID[j.Candidate.key()] = j
	}
	if j := byID["chunk/c1"]; j.Final != "governs" || j.Adjudicated {
		t.Errorf("agreed candidate = %+v", j)
	}
	if j := byID["chunk/c2"]; j.Final != "topical" || !j.Adjudicated || j.Rationale != "resolved" || j.Candidate.Cosine == nil || *j.Candidate.Cosine != 0.4 {
		t.Errorf("disputed candidate = %+v", j)
	}
	if adj.calls[AdjJudgementPromptFile] != 2 { // one disagreement per final activity
		t.Errorf("judgement adjudications = %d", adj.calls[AdjJudgementPromptFile])
	}
}

func TestActivityOutsideChunkIsRefused(t *testing.T) {
	bad := &fakeModel{handle: func(string, string) map[string]any { return acts(act("x", 2, 9)) }}
	p := testPipeline(bad, bad, bad, twoResults)
	if _, err := p.LabelDocument(context.Background(), GoldDoc{RecordID: 1}); err == nil || !strings.Contains(err.Error(), "invalid activity") {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingLabelIsRefused(t *testing.T) {
	a1 := &fakeModel{handle: func(p, in string) map[string]any {
		if p == JudgePromptFile {
			return judgeAll(map[string]string{"chunk/c1": "governs"})(p, in) // c2 gets label ""
		}
		return acts(act("a", 2, 2))
	}}
	p := testPipeline(a1, a1, a1, twoResults)
	p.A2.Client = &fakeModel{handle: func(p, in string) map[string]any { return acts(act("a", 2, 2)) }}
	if _, err := p.LabelDocument(context.Background(), GoldDoc{RecordID: 1}); err == nil || !strings.Contains(err.Error(), "no valid label for 1 candidate(s)") {
		t.Fatalf("err = %v", err)
	}
}

func TestEmptyPoolMeansNoJudgements(t *testing.T) {
	m := &fakeModel{handle: func(p, in string) map[string]any { return acts(act("a", 2, 2)) }}
	p := testPipeline(m, m, m, func(searchengine.Request) *searchengine.Response { return &searchengine.Response{} })
	doc, err := p.LabelDocument(context.Background(), GoldDoc{RecordID: 1})
	if err != nil || len(doc.Activities) != 1 || len(doc.Activities[0].Judgements) != 0 {
		t.Fatalf("err=%v doc=%+v", err, doc)
	}
}

func TestOverlaps(t *testing.T) {
	a := Activity{LineStart: 10, LineEnd: 19}
	for _, tc := range []struct {
		b    Activity
		want bool
	}{
		{Activity{LineStart: 15, LineEnd: 30}, true},                       // 5 of shorter 10... overlap 5/10 = 0.5
		{Activity{LineStart: 18, LineEnd: 30}, false},                      // 2/10
		{Activity{LineStart: 12, LineEnd: 13}, true},                       // contained
		{Activity{LineStart: 10, LineEnd: 19, DocumentLevel: true}, false}, // level differs
	} {
		if got := overlaps(a, tc.b); got != tc.want {
			t.Errorf("overlaps(%v,%v) = %v want %v", a, tc.b, got, tc.want)
		}
	}
}

func TestMissingLabelIsRetriedForMissingCandidatesOnly(t *testing.T) {
	asked := map[int][]string{}
	n := 0
	a1 := &fakeModel{handle: func(p, in string) map[string]any {
		if p != JudgePromptFile {
			return acts(act("a", 2, 2))
		}
		n++
		var req struct {
			Candidates []struct{ ID string } `json:"candidates"`
		}
		_ = json.Unmarshal([]byte(in), &req)
		var ls []any
		for _, c := range req.Candidates {
			asked[n] = append(asked[n], c.ID)
			if n == 1 && c.ID == "chunk/c2" {
				continue // the model skips one candidate the first time
			}
			ls = append(ls, map[string]any{"id": c.ID, "label": "topical", "reason": "r"})
		}
		return map[string]any{"labels": ls}
	}}
	a2 := &fakeModel{handle: func(p, in string) map[string]any {
		if p != JudgePromptFile {
			return acts(act("a", 2, 2))
		}
		return judgeAll(map[string]string{"chunk/c1": "topical", "chunk/c2": "topical"})(p, in)
	}}
	p := testPipeline(a1, a2, a1, twoResults)
	doc, err := p.LabelDocument(context.Background(), GoldDoc{RecordID: 1})
	if err != nil || len(doc.Activities[0].Judgements) != 2 {
		t.Fatalf("err=%v doc=%+v", err, doc)
	}
	if len(asked[1]) != 2 || len(asked[2]) != 1 || asked[2][0] != "chunk/c2" {
		t.Errorf("asked = %v, want the second ask to cover only the missing candidate", asked)
	}
}
