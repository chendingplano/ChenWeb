package kbhandler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestHybridBenchQuestionFilters(t *testing.T) {
	c := echo.New().NewContext(httptest.NewRequest("GET", "/?purpose=search_metrics&run_id=5&lang=en&hop_type=two_hop&target_id=416_mtc_7&question=heavy%20100%25&answer=%E9%95%89", nil), httptest.NewRecorder())
	f, err := parseHybridBenchQuestionFilters(c)
	if err != nil {
		t.Fatal(err)
	}
	where, args := f.where()
	for _, want := range []string{"purpose = $1", "run_id = $2", "lang = $3", "hop_type = $4", "artifact_id = $5",
		"question ILIKE '%' || $6 || '%'", "question ILIKE '%' || $7 || '%'", "answer ILIKE '%' || $8 || '%'"} {
		if !strings.Contains(where, want) {
			t.Errorf("where %q lacks %q", where, want)
		}
	}
	if len(args) != 8 || args[6] != `100\%` || args[7] != "镉" {
		t.Fatalf("args = %#v", args)
	}

	for _, q := range []string{"/?purpose=other", "/?purpose=search_metrics&hop_type=three_hop", "/?purpose=search_metrics&question=" + strings.Repeat("a", hybridBenchMaxWordsField+1)} {
		c := echo.New().NewContext(httptest.NewRequest("GET", q, nil), httptest.NewRecorder())
		if _, err := parseHybridBenchQuestionFilters(c); err == nil {
			t.Errorf("%s accepted", q)
		}
	}
}

func TestHybridBenchRequestQuestionIDs(t *testing.T) {
	defaults := hybridBenchOptions{LexicalWeight: 1, SemanticWeight: 1, CandidateLimit: 200, Parallelism: 8}
	req := hybridBenchRequest{Purpose: "search_metrics", QuestionIDs: []int64{9, 3, 9, 1}}
	if _, err := req.normalize(defaults); err != nil || len(req.QuestionIDs) != 3 || req.QuestionIDs[0] != 1 || req.QuestionIDs[2] != 9 {
		t.Fatalf("ids = %v, %v", req.QuestionIDs, err)
	}
	tooMany := hybridBenchRequest{Purpose: "search_metrics", QuestionIDs: make([]int64, hybridBenchMaxSelected+1)}
	if _, err := tooMany.normalize(defaults); err == nil {
		t.Fatal("too many selected questions accepted")
	}
}
