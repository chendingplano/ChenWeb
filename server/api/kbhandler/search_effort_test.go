package kbhandler

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
)

// effortStub answers the analysis call and then one judge call per attempt, in order.
type effortStub struct {
	analysis  map[string]any
	analysisE error
	judges    []map[string]any
	judgeE    error
	calls     []llmclients.JSONExtractionInput
}

func (s *effortStub) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	s.calls = append(s.calls, in)
	if in.CallReason == "search_effort_analyze" {
		return s.analysis, s.analysisE
	}
	if s.judgeE != nil {
		return nil, s.judgeE
	}
	n := 0
	for _, c := range s.calls {
		if c.CallReason == "search_effort_judge" {
			n++
		}
	}
	return s.judges[min(n, len(s.judges))-1], nil
}

func useEffortStub(t *testing.T, stub *effortStub, search func(query string) []artifactSearchResult) *[]string {
	t.Helper()
	oldClient, oldModel, oldPrompt, oldQuery, oldDetails := newSearchEffortClient, loadSearchEffortModel, loadSearchEffortPrompt, searchEffortQuery, searchEffortDetails
	t.Cleanup(func() {
		newSearchEffortClient, loadSearchEffortModel, loadSearchEffortPrompt, searchEffortQuery, searchEffortDetails = oldClient, oldModel, oldPrompt, oldQuery, oldDetails
	})
	searchEffortDetails = func(_ context.Context, _ *sql.DB, results []artifactSearchResult) (map[string]string, error) {
		details := map[string]string{}
		for _, r := range results {
			details[searchEffortKey(r)] = "details of " + r.ArtifactID
		}
		return details, nil
	}
	newSearchEffortClient = func(string, ApiTypes.LLMModelDef, ApiTypes.JimoLogger) (metricJSONExtractor, error) { return stub, nil }
	loadSearchEffortModel = func(string) (ApiTypes.LLMModelDef, error) { return ApiTypes.LLMModelDef{ModelName: "stub"}, nil }
	loadSearchEffortPrompt = func(string) (string, string, string, error) { return "prompt", "p", "p", nil }
	queries := &[]string{}
	searchEffortQuery = func(_ *sql.DB, _ string, q string, _ artifactSearchFilters, _ int, _ registrySearchConfig) ([]artifactSearchResult, error) {
		*queries = append(*queries, q)
		return search(q), nil
	}
	return queries
}

func metricHits(ids ...string) []artifactSearchResult {
	out := []artifactSearchResult{}
	for _, id := range ids {
		out = append(out, artifactSearchResult{ArtifactType: "metric", ArtifactID: id, PrimaryLabel: id})
	}
	return out
}

func TestParseSearchEffortAndLimits(t *testing.T) {
	for raw, want := range map[string]string{"": "low", "LOW": "low", " medium ": "medium", "high": "high"} {
		if got, err := parseSearchEffort(raw); err != nil || got != want {
			t.Errorf("parseSearchEffort(%q) = %q, %v", raw, got, err)
		}
	}
	if _, err := parseSearchEffort("max"); err == nil {
		t.Error("effort max accepted")
	}
	t.Setenv(envSearchEffortMedium, "")
	t.Setenv(envSearchEffortHigh, "7")
	if searchEffortMaxAttempts("low") != 0 || searchEffortMaxAttempts("medium") != 2 || searchEffortMaxAttempts("high") != 7 {
		t.Fatal("attempt limits wrong")
	}
	t.Setenv(envSearchEffortHigh, "0")
	if searchEffortMaxAttempts("high") != defaultSearchEffortHigh {
		t.Fatal("invalid high limit not defaulted")
	}
}

func TestSearchWithEffortLowMakesNoLLMCall(t *testing.T) {
	stub := &effortStub{}
	queries := useEffortStub(t, stub, func(string) []artifactSearchResult { return metricHits("a", "b") })
	results, trace, err := searchWithEffort(context.Background(), nil, "metric", "direct question", artifactSearchFilters{}, 10, registrySearchConfig{}, searchEffortLow)
	if err != nil || trace != nil || len(stub.calls) != 0 || len(results) != 2 || len(*queries) != 1 || (*queries)[0] != "direct question" {
		t.Fatalf("low: results=%d trace=%v calls=%d queries=%v err=%v", len(results), trace, len(stub.calls), *queries, err)
	}
}

func TestSearchWithEffortReformulatesUntilFound(t *testing.T) {
	t.Setenv(envSearchEffortHigh, "5")
	stub := &effortStub{
		analysis: map[string]any{"strategy": "compose", "need": "test duration", "queries": []any{"steady state recording time"}},
		judges: []map[string]any{
			{"found": false, "matching_ids": []any{}, "next_queries": []any{"steady state recording time", "minimum test duration"}, "reason": "only related"},
			{"found": true, "matching_ids": []any{"metric/target", "metric/unknown"}, "reason": "matches"},
		},
	}
	queries := useEffortStub(t, stub, func(q string) []artifactSearchResult {
		if q == "minimum test duration" {
			return metricHits("other", "target")
		}
		return metricHits("other", "related")
	})
	cfg := registrySearchConfig{call: searchCall{runID: 31, userID: "u"}}
	results, trace, err := searchWithEffort(context.Background(), nil, "metric", "how long must I record?", artifactSearchFilters{}, 10, cfg, searchEffortHigh)
	if err != nil || trace == nil {
		t.Fatalf("err=%v trace=%v", err, trace)
	}
	if !trace.Found || len(trace.Attempts) != 2 || trace.Strategy != "compose" || trace.Need != "test duration" {
		t.Fatalf("trace = %+v", trace)
	}
	// The repeated query was dropped; the judge's unknown id was ignored.
	if len(*queries) != 2 || (*queries)[1] != "minimum test duration" {
		t.Fatalf("queries = %v", *queries)
	}
	if got := trace.Attempts[1].MatchingIDs; len(got) != 1 || got[0] != "metric/target" {
		t.Fatalf("matching ids = %v", got)
	}
	if results[0].ArtifactID != "target" {
		t.Fatalf("first result = %s, want target", results[0].ArtifactID)
	}
	for _, c := range stub.calls {
		if c.RunID != 31 || c.UserID != "u" {
			t.Fatalf("LLM call without search run: %+v", c)
		}
		if c.CallReason == "search_effort_judge" && !strings.Contains(c.InputText, "details of related") {
			t.Fatalf("judge input lacks candidate details: %s", c.InputText)
		}
	}
}

func TestSearchWithEffortStopsAtMaximum(t *testing.T) {
	t.Setenv(envSearchEffortMedium, "")
	n := 0
	stub := &effortStub{analysis: map[string]any{"strategy": "direct", "need": "n", "queries": []any{"extra angle"}}}
	for range 5 {
		n++
		stub.judges = append(stub.judges, map[string]any{"found": false, "next_queries": []any{"q" + string(rune('0'+n))}})
	}
	queries := useEffortStub(t, stub, func(string) []artifactSearchResult { return metricHits("x") })
	results, trace, _ := searchWithEffort(context.Background(), nil, "metric", "the request", artifactSearchFilters{}, 10, registrySearchConfig{}, searchEffortMedium)
	if trace.Found || len(trace.Attempts) != 2 || len(results) != 1 {
		t.Fatalf("attempts=%d found=%v results=%d", len(trace.Attempts), trace.Found, len(results))
	}
	// Direct strategy: the first attempt searches the request as written, plus the extra angle.
	if (*queries)[0] != "the request" || (*queries)[1] != "extra angle" {
		t.Fatalf("queries = %v", *queries)
	}
}

func TestSearchWithEffortDegrades(t *testing.T) {
	stub := &effortStub{analysisE: errors.New("402")}
	queries := useEffortStub(t, stub, func(string) []artifactSearchResult { return metricHits("a") })
	results, trace, err := searchWithEffort(context.Background(), nil, "metric", "req", artifactSearchFilters{}, 10, registrySearchConfig{}, searchEffortHigh)
	if err != nil || len(results) != 1 || trace.Error == "" || len(*queries) != 1 || (*queries)[0] != "req" {
		t.Fatalf("analysis failure: results=%d trace=%+v queries=%v err=%v", len(results), trace, *queries, err)
	}

	stub = &effortStub{analysis: map[string]any{"strategy": "compose", "queries": []any{"q1"}}, judgeE: errors.New("timeout")}
	useEffortStub(t, stub, func(string) []artifactSearchResult { return metricHits("a", "b") })
	results, trace, err = searchWithEffort(context.Background(), nil, "metric", "req", artifactSearchFilters{}, 10, registrySearchConfig{}, searchEffortHigh)
	if err != nil || len(results) != 2 || len(trace.Attempts) != 1 || trace.Attempts[0].Error == "" || trace.Found {
		t.Fatalf("judge failure: results=%d trace=%+v err=%v", len(results), trace, err)
	}
}

func TestSearchRejectsInvalidEffort(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest("GET", "/?q=test&artifact_types=metric&effort=max", nil), rec)
	if err := SearchAllArtifacts(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "CWB_KB_GSR_015") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
