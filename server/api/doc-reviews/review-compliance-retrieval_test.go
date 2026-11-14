package docreviews

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

func TestComplianceSearchCandidatesKeepsKeywordOnlyMetric(t *testing.T) {
	r := &complianceReviewer{search: func(_ context.Context, req searchengine.Request) (*searchengine.Response, error) {
		if !req.Options.Trace {
			t.Fatal("independent rankings require trace")
		}
		return &searchengine.Response{Results: []searchengine.Result{{ArtifactType: "metric", ArtifactID: "fused", Snippet: "combined hit"}}, Trace: &searchengine.Trace{
			Lexical: []searchengine.TraceHit{{ArtifactType: "metric", ArtifactID: "lexical-only", Score: 8}},
			Vector:  []searchengine.TraceHit{{ArtifactType: "metric", ArtifactID: "vector-only", Score: 0.4}},
		}}, nil
	}, loadArtifacts: func(_ context.Context, refs []searchengine.ArtifactRef) ([]searchengine.Result, error) {
		if len(refs) != 2 {
			t.Fatalf("load only missing refs: %v", refs)
		}
		return []searchengine.Result{{ArtifactType: "metric", ArtifactID: "lexical-only", Snippet: "maximum temperature 30 C"}, {ArtifactType: "metric", ArtifactID: "vector-only", Snippet: "semantic hit"}}, nil
	}}
	got, err := r.searchCandidates(context.Background(), searchengine.Request{Options: searchengine.Options{Limit: 3}})
	if err != nil || len(got) != 3 {
		t.Fatalf("results=%v err=%v", got, err)
	}
	if got[0].ArtifactID != "fused" || got[1].ArtifactID != "lexical-only" || got[2].ArtifactID != "vector-only" {
		t.Fatalf("rank interleave=%v", got)
	}
	if got[1].Snippet == "" || got[1].Diag.LexicalRank == nil || *got[1].Diag.LexicalRank != 1 || got[1].Diag.LexicalScore == nil || *got[1].Diag.LexicalScore != 8 || got[1].Diag.Cosine != nil {
		t.Fatalf("keyword-only diagnostics=%+v", got[1])
	}
	if got[2].Diag.Cosine == nil || *got[2].Diag.Cosine != 0.4 {
		t.Fatalf("vector diagnostics=%+v", got[2])
	}
}

func TestComplianceSearchCandidatesMissingLoaderAndArtifactsFail(t *testing.T) {
	r := &complianceReviewer{search: func(context.Context, searchengine.Request) (*searchengine.Response, error) {
		return &searchengine.Response{Trace: &searchengine.Trace{Lexical: []searchengine.TraceHit{{ArtifactType: "metric", ArtifactID: "missing"}}}}, nil
	}}
	if _, err := r.searchCandidates(context.Background(), searchengine.Request{Options: searchengine.Options{Limit: 1}}); err == nil || !strings.Contains(err.Error(), "loader") {
		t.Fatalf("missing loader err=%v", err)
	}
	r.loadArtifacts = func(context.Context, []searchengine.ArtifactRef) ([]searchengine.Result, error) { return nil, nil }
	if _, err := r.searchCandidates(context.Background(), searchengine.Request{Options: searchengine.Options{Limit: 1}}); err == nil || !strings.Contains(err.Error(), "metric/missing") {
		t.Fatalf("missing artifact err=%v", err)
	}
}

func TestComplianceSearchCandidatesPreservesCriteriaFiltering(t *testing.T) {
	r := &complianceReviewer{search: func(context.Context, searchengine.Request) (*searchengine.Response, error) {
		return &searchengine.Response{Results: []searchengine.Result{{ArtifactType: "metric", ArtifactID: "allowed"}}, Trace: &searchengine.Trace{Lexical: []searchengine.TraceHit{{ArtifactType: "metric", ArtifactID: "filtered"}}}}, nil
	}}
	for _, criteria := range []searchengine.Criteria{{MinCosine: 0.7}, {MinMatched: 1}, {Keywords: []searchengine.Keyword{{Term: "excluded", Excluded: true}}}} {
		got, err := r.searchCandidates(context.Background(), searchengine.Request{Criteria: criteria, Options: searchengine.Options{Limit: 3}})
		if err != nil || len(got) != 1 || got[0].ArtifactID != "allowed" {
			t.Fatalf("filtered candidates resurrected: %v err=%v", got, err)
		}
	}
}

func TestComplianceSearchCandidatesDeduplicatesAndPreservesDiagnostics(t *testing.T) {
	cos := 0.91
	r := &complianceReviewer{search: func(context.Context, searchengine.Request) (*searchengine.Response, error) {
		return &searchengine.Response{Results: []searchengine.Result{{ArtifactType: "metric", ArtifactID: "same", Diag: searchengine.Diagnostics{Cosine: &cos}}}, Trace: &searchengine.Trace{Lexical: []searchengine.TraceHit{{ArtifactType: "metric", ArtifactID: "same", Score: 3}}, Vector: []searchengine.TraceHit{{ArtifactType: "metric", ArtifactID: "same", Score: 0.5}}}}, nil
	}}
	got, err := r.searchCandidates(context.Background(), searchengine.Request{Options: searchengine.Options{Limit: 3}})
	if err != nil || len(got) != 1 || got[0].Diag.Cosine == nil || *got[0].Diag.Cosine != 0.91 || got[0].Diag.LexicalRank == nil {
		t.Fatalf("dedup/diagnostics=%v err=%v", got, err)
	}
}

func TestComplianceSearchCandidatesErrorsAndTraceFallback(t *testing.T) {
	failure := errors.New("unavailable")
	r := &complianceReviewer{search: func(context.Context, searchengine.Request) (*searchengine.Response, error) { return nil, failure }}
	if _, err := r.searchCandidates(context.Background(), searchengine.Request{}); !errors.Is(err, failure) {
		t.Fatalf("search err=%v", err)
	}
	r.search = func(context.Context, searchengine.Request) (*searchengine.Response, error) { return nil, nil }
	if _, err := r.searchCandidates(context.Background(), searchengine.Request{}); err == nil {
		t.Fatal("nil response accepted")
	}
	r.search = func(context.Context, searchengine.Request) (*searchengine.Response, error) {
		return &searchengine.Response{Results: []searchengine.Result{{ArtifactType: "metric", ArtifactID: "existing"}}}, nil
	}
	got, err := r.searchCandidates(context.Background(), searchengine.Request{})
	if err != nil || len(got) != 1 || got[0].ArtifactID != "existing" {
		t.Fatalf("trace fallback=%v err=%v", got, err)
	}
	r.search = func(context.Context, searchengine.Request) (*searchengine.Response, error) {
		return &searchengine.Response{Trace: &searchengine.Trace{Vector: []searchengine.TraceHit{{ArtifactType: "metric", ArtifactID: "m"}}}}, nil
	}
	r.loadArtifacts = func(context.Context, []searchengine.ArtifactRef) ([]searchengine.Result, error) { return nil, failure }
	if _, err := r.searchCandidates(context.Background(), searchengine.Request{}); !errors.Is(err, failure) {
		t.Fatalf("load err=%v", err)
	}
}
