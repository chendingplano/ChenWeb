package complianceeval

import (
	"context"
	"encoding/json"
	"fmt"
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
		Prompts:       Prompts{Activities: "rules\n\nact", Judge: "rules\n\njudge", AdjActivity: "rules\n\nadja", AdjJudgement: "rules\n\nadjj"},
		MaxCandidates: 50,
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
	doc, err := p.LabelDocument(context.Background(), GoldDoc{RecordID: 416})
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
	if len(scoped) != 0 {
		t.Errorf("pool search scope = %v, want the whole knowledge base", scoped)
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

// res is a search result with optional keyword and vector ranks (0 = not found by that list).
func res(id string, rec int64, cos float64, lex, vec int) searchengine.Result {
	r := searchengine.Result{ArtifactType: "chunk", ArtifactID: id, InputRecordID: rec, SourceTitle: "GB 1", Snippet: id,
		Diag: searchengine.Diagnostics{Cosine: &cos}}
	if lex > 0 {
		r.Diag.LexicalRank = &lex
	}
	if vec > 0 {
		r.Diag.VectorRank = &vec
	}
	return r
}

func ids(cs []Candidate) string {
	var out []string
	for _, c := range cs {
		out = append(out, c.ArtifactID)
	}
	return strings.Join(out, ",")
}

func TestPoolUsesThreeRankingsPerQueryAndStopsAtMaxCandidates(t *testing.T) {
	var reqs []searchengine.Request
	p := &Pipeline{MaxCandidates: 4, Search: func(_ context.Context, req searchengine.Request) (*searchengine.Response, error) {
		reqs = append(reqs, req)
		switch req.Query.Text {
		case "keep logs": // q1, the source text: a3 is the keyword list's best hit but last in the fused order
			return &searchengine.Response{Results: []searchengine.Result{res("a1", 9, 0.7, 2, 1), res("a2", 9, 0.6, 0, 2), res("a3", 9, 0.2, 1, 0)}}, nil
		case "log o": // q2, the name with its object
			return &searchengine.Response{Results: []searchengine.Result{res("b1", 9, 0.5, 0, 1), res("a1", 9, 0.9, 1, 0)}}, nil
		case "retention period": // q3, the agent's query
			return &searchengine.Response{Results: []searchengine.Result{res("r1", 9, 0.3, 1, 0)}}, nil
		}
		t.Fatalf("unexpected query %q", req.Query.Text)
		return nil, nil
	}}
	got, err := p.Pool(context.Background(), Activity{Name: "log", Object: "o", SourceText: "keep logs", Queries: []string{"retention period"}}, []int64{416})
	if err != nil {
		t.Fatal(err)
	}
	// rank 0 of the lists q1/fused, q1/lexical, q1/vector, q2/fused, ..., q3/fused: a1, a3, (a1), b1, (a1), (b1), r1.
	if ids(got) != "a1,a3,b1,r1" {
		t.Fatalf("pool = %s", ids(got))
	}
	want := map[string]string{"a1": "q1/provision/fused", "a3": "q1/provision/lexical", "b1": "q2/provision/fused", "r1": "q3/provision/fused"}
	for _, c := range got {
		if c.FoundBy != want[c.ArtifactID] {
			t.Errorf("%s found_by = %q, want %q", c.ArtifactID, c.FoundBy, want[c.ArtifactID])
		}
	}
	if *got[0].Cosine != 0.9 { // a1 came from two queries: highest cosine kept
		t.Errorf("a1 cosine = %v", *got[0].Cosine)
	}
	if len(reqs) != 9 {
		t.Fatalf("%d searches, want 9", len(reqs))
	}
	for i, r := range reqs {
		agent := i >= 6
		if r.Options.Limit != poolSearchLimit || r.Criteria.MinCosine != 0 || len(r.Filters.ExcludeRecordIDs) != 1 ||
			(r.Options.KeywordExtraction == searchengine.KeywordExtractionOff) != agent {
			t.Errorf("search %d = %+v, want limit %d, no floor, the exclusion, keyword extraction off only for the agent's query", i+1, r, poolSearchLimit)
		}
	}
}

func TestWithinSearchesOneRecordAndSkipsPooledPassages(t *testing.T) {
	var got []searchengine.Request
	p := &Pipeline{MaxCandidates: 50, Search: func(_ context.Context, req searchengine.Request) (*searchengine.Response, error) {
		got = append(got, req)
		var rs []searchengine.Result
		for i := 0; i < 12; i++ {
			rs = append(rs, res(fmt.Sprintf("x%d", i), 55, 0.5, 0, i+1))
		}
		return &searchengine.Response{Results: rs}, nil
	}}
	cands, err := p.Within(context.Background(), Activity{Name: "log", Object: "o", SourceText: "keep logs"}, 55,
		map[string]bool{"chunk/x0": true}, "cited:GB/T 19095-2019")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != WithinPerRecord || cands[0].ArtifactID != "x1" || cands[0].FoundBy != "cited:GB/T 19095-2019" {
		t.Fatalf("within = %s (first found_by %q)", ids(cands), cands[0].FoundBy)
	}
	seenTypes := map[string]int{}
	for _, req := range got {
		if len(req.Filters.ArtifactTypes) != 1 || req.Criteria.MinCosine != 0 {
			t.Fatalf("within must independently search types without floor: %+v", req)
		}
		seenTypes[req.Filters.ArtifactTypes[0]]++
	}
	for _, typ := range []string{"provision", "metric", "chunk"} {
		if seenTypes[typ] != 2 {
			t.Errorf("within %s searches=%d, want 2", typ, seenTypes[typ])
		}
	}
	if len(got[0].Filters.RecordIDs) != 1 || got[0].Filters.RecordIDs[0] != 55 || len(got[0].Filters.ExcludeRecordIDs) != 0 {
		t.Errorf("filters = %+v, want only record 55", got[0].Filters)
	}
}

func TestMetricPoolSeparatesArtifactTypesAndPreservesEachRanking(t *testing.T) {
	seen := map[string]int{}
	p := &Pipeline{MaxCandidates: 3, Search: func(_ context.Context, req searchengine.Request) (*searchengine.Response, error) {
		if len(req.Filters.ArtifactTypes) != 1 {
			t.Fatalf("pool must search each type independently: %+v", req.Filters)
		}
		typ := req.Filters.ArtifactTypes[0]
		seen[typ]++
		if req.Criteria.MinCosine != 0 {
			t.Fatal("pool imposed relevance floor")
		}
		var rs []searchengine.Result
		for i := 0; i < 12; i++ {
			r := res(fmt.Sprintf("%s%d", typ, i), 55, 0.01, 12-i, i+1)
			r.ArtifactType = typ
			rs = append(rs, r)
		}
		return &searchengine.Response{Results: rs}, nil
	}}
	got, err := p.Pool(context.Background(), Activity{Name: "limit", SourceText: "temperature"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, c := range got {
		counts[c.ArtifactType]++
	}
	if len(got) != 3 || counts["metric"] == 0 || counts["provision"] == 0 || counts["chunk"] == 0 {
		t.Fatalf("crowded out artifact type: %+v", got)
	}
	for _, typ := range []string{"provision", "metric", "chunk"} {
		if seen[typ] != 2 {
			t.Errorf("%s searches = %d, want 2", typ, seen[typ])
		}
	}
}

func TestPanelV2AcceptsAmbiguousMetricJudgement(t *testing.T) {
	model := &fakeModel{handle: func(_, _ string) map[string]any {
		return map[string]any{"labels": []any{map[string]any{"id": "metric/m1", "label": "ambiguous", "reason": "source provenance is incomplete"}}}
	}}
	p := &Pipeline{Prompts: Prompts{Judge: "rules"}}
	labels, err := p.judgeBy(context.Background(), Labeller{Name: "synthetic", Model: "synthetic", Client: model}, Activity{Name: "inspect"}, []Candidate{{ArtifactType: "metric", ArtifactID: "m1"}})
	if err != nil || labels["metric/m1"].label != "ambiguous" {
		t.Fatalf("labels=%v err=%v", labels, err)
	}
	if model.calls[JudgePromptFile] != 1 {
		t.Fatalf("valid ambiguous label retried: %v", model.calls)
	}
}

func TestPoolPreservesMetricOutsideFusedPageFromTrace(t *testing.T) {
	p := &Pipeline{MaxCandidates: 2, Search: func(_ context.Context, req searchengine.Request) (*searchengine.Response, error) {
		if !req.Options.Trace {
			t.Fatal("pool must request independent trace lists")
		}
		if req.Filters.ArtifactTypes[0] != "metric" {
			return &searchengine.Response{}, nil
		}
		return &searchengine.Response{Results: []searchengine.Result{{ArtifactType: "metric", ArtifactID: "fused", InputRecordID: 9}}, Trace: &searchengine.Trace{
			Lexical: []searchengine.TraceHit{{ArtifactType: "metric", ArtifactID: "keyword-only", Score: 7}},
			Vector:  []searchengine.TraceHit{{ArtifactType: "metric", ArtifactID: "fused", Score: 0.8}},
		}}, nil
	}, Load: func(_ context.Context, refs []searchengine.ArtifactRef) ([]searchengine.Result, error) {
		if len(refs) != 1 || refs[0].ID != "keyword-only" {
			t.Fatalf("unexpected refs=%v", refs)
		}
		return []searchengine.Result{{ArtifactType: "metric", ArtifactID: "keyword-only", InputRecordID: 10, Snippet: "temperature maximum 30 C"}}, nil
	}}
	got, err := p.Pool(context.Background(), Activity{Name: "inspect"}, nil)
	if err != nil || len(got) != 2 || got[1].ArtifactID != "keyword-only" || got[1].FoundBy != "q2/metric/lexical" || got[1].Text == "" {
		t.Fatalf("pool=%+v err=%v", got, err)
	}
	if got[1].Cosine != nil {
		t.Fatalf("keyword-only cosine invented: %+v", got[1])
	}
}

func TestPoolTraceMissingLoadersArtifactsAndResponsesFail(t *testing.T) {
	p := &Pipeline{MaxCandidates: 2, Search: func(context.Context, searchengine.Request) (*searchengine.Response, error) {
		return &searchengine.Response{Trace: &searchengine.Trace{Lexical: []searchengine.TraceHit{{ArtifactType: "metric", ArtifactID: "missing"}}}}, nil
	}}
	if _, err := p.Pool(context.Background(), Activity{Name: "inspect"}, nil); err == nil || !strings.Contains(err.Error(), "loader") {
		t.Fatalf("missing loader err=%v", err)
	}
	p.Load = func(context.Context, []searchengine.ArtifactRef) ([]searchengine.Result, error) { return nil, nil }
	if _, err := p.Pool(context.Background(), Activity{Name: "inspect"}, nil); err == nil || !strings.Contains(err.Error(), "metric/missing") {
		t.Fatalf("missing row err=%v", err)
	}
	p.Load = func(context.Context, []searchengine.ArtifactRef) ([]searchengine.Result, error) {
		return nil, fmt.Errorf("trace storage unavailable")
	}
	if _, err := p.Pool(context.Background(), Activity{Name: "inspect"}, nil); err == nil || !strings.Contains(err.Error(), "trace storage unavailable") {
		t.Fatalf("load error lost: %v", err)
	}
	p.Search = func(context.Context, searchengine.Request) (*searchengine.Response, error) { return nil, nil }
	if _, err := p.Pool(context.Background(), Activity{Name: "inspect"}, nil); err == nil || !strings.Contains(err.Error(), "nil response") {
		t.Fatalf("nil response err=%v", err)
	}
}
