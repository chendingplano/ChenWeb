package kbhandler

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func benchResults(pairs ...any) []artifactSearchResult {
	out := []artifactSearchResult{}
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, artifactSearchResult{ArtifactID: pairs[i].(string), Score: pairs[i+1].(float64)})
	}
	return out
}

func intPtr(v int) *int { return &v }

func TestHybridBenchRequestNormalize(t *testing.T) {
	defaults := hybridBenchOptions{QueryKeywords: true, LexicalWeight: 1, SemanticWeight: 1, CandidateLimit: 200}
	ok := hybridBenchRequest{Purpose: " search_metrics ", Description: " baseline "}
	opts, err := ok.normalize(defaults)
	if err != nil || ok.Purpose != "search_metrics" || ok.Description != "baseline" || opts != defaults {
		t.Fatalf("normalize = %+v %+v, %v", ok, opts, err)
	}
	off, half, fifty := false, 0.5, 50
	custom := hybridBenchRequest{Purpose: "search_metrics", QueryKeywords: &off, LexicalWeight: &half, CandidateLimit: &fifty}
	if opts, err := custom.normalize(defaults); err != nil || opts.QueryKeywords || opts.LexicalWeight != 0.5 || opts.SemanticWeight != 1 || opts.CandidateLimit != 50 {
		t.Fatalf("custom options = %+v, %v", opts, err)
	}
	zero, negative, tooMany := 0.0, -1.0, 1001
	for name, req := range map[string]hybridBenchRequest{
		"purpose":     {Purpose: "search_provisions"},
		"description": {Purpose: "search_metrics", Description: strings.Repeat("长", hybridBenchMaxDescription+1)},
		"negative":    {Purpose: "search_metrics", SemanticWeight: &negative},
		"both zero":   {Purpose: "search_metrics", LexicalWeight: &zero, SemanticWeight: &zero},
		"candidates":  {Purpose: "search_metrics", CandidateLimit: &tooMany},
	} {
		if _, err := req.normalize(defaults); err == nil {
			t.Errorf("%s: invalid request accepted", name)
		}
	}
}

func TestRegistrySearchConfigCandidates(t *testing.T) {
	if got := (registrySearchConfig{}).candidates(); got != hybridCandidateLimit {
		t.Fatalf("default candidates = %d", got)
	}
	if got := (registrySearchConfig{candidateLimit: 50}).candidates(); got != 50 {
		t.Fatalf("candidates = %d, want 50", got)
	}
}

func TestHybridBenchTargetRank(t *testing.T) {
	results := benchResults("a", 0.03, "b", 0.02, "c", 0.0, "d", 0.0)
	if got := hybridBenchTargetRank(results, "b"); got == nil || *got != 2 {
		t.Fatalf("rank of b = %v, want 2", got)
	}
	// "d" came only from the zero-weighted list: a miss, not rank 4.
	if got := hybridBenchTargetRank(results, "d"); got != nil {
		t.Fatalf("rank of zero-score d = %d, want nil", *got)
	}
	if got := hybridBenchTargetRank(results, "z"); got != nil {
		t.Fatalf("rank of absent z = %d, want nil", *got)
	}
	if top := hybridBenchTop(results); len(top) != 2 || top[0] != "a" || top[1] != "b" {
		t.Fatalf("top = %v, want [a b]", top)
	}
}

func TestHybridBenchModeWeights(t *testing.T) {
	cfg := registrySearchConfig{lexicalWeight: 0.5, semanticWeight: 1.5}
	for mode, want := range map[string][2]float64{
		hybridBenchModeLexical:  {1, 0},
		hybridBenchModeSemantic: {0, 1},
		hybridBenchModeHybrid:   {0.5, 1.5},
	} {
		l, s := hybridBenchModeWeights(mode, cfg)
		if l != want[0] || s != want[1] {
			t.Errorf("%s weights = %v/%v, want %v", mode, l, s, want)
		}
	}
}

func TestScoreHybridBench(t *testing.T) {
	results := []hybridBenchResult{
		{HopType: "one_hop", Lang: "en", HybridRank: intPtr(1), LexicalRank: intPtr(4), SemanticRank: intPtr(12)},
		{HopType: "two_hop", Lang: "zh-cn"},
	}
	scores := scoreHybridBench(results)
	find := func(mode, group, value string) hybridBenchScore {
		for _, s := range scores {
			if s.Mode == mode && s.Group == group && s.Value == value {
				return s
			}
		}
		t.Fatalf("no score for %s/%s/%s", mode, group, value)
		return hybridBenchScore{}
	}
	if s := find(hybridBenchModeHybrid, hybridBenchGroupAll, ""); s.Count != 2 || s.R1 != 0.5 || s.R10 != 0.5 || s.MRR != 0.5 {
		t.Fatalf("hybrid all = %+v", s)
	}
	if s := find(hybridBenchModeLexical, hybridBenchGroupAll, ""); s.R1 != 0 || s.R5 != 0.5 || s.MRR != 0.125 {
		t.Fatalf("lexical all = %+v", s)
	}
	if s := find(hybridBenchModeSemantic, hybridBenchGroupHopType, "one_hop"); s.Count != 1 || s.R10 != 0 || s.MRR != 1.0/12 {
		t.Fatalf("semantic one_hop = %+v", s)
	}
	if s := find(hybridBenchModeHybrid, hybridBenchGroupLang, "zh-cn"); s.Count != 1 || s.R10 != 0 {
		t.Fatalf("hybrid zh-cn = %+v", s)
	}
	// 3 modes x (all + 2 hop types + 2 languages)
	if len(scores) != 15 {
		t.Fatalf("len(scores) = %d, want 15", len(scores))
	}
	if got := scoreHybridBench(nil); len(got) != 0 {
		t.Fatalf("empty scores = %v", got)
	}
}

func TestBenchmarkHybridSearchQuestion(t *testing.T) {
	oldEmbed, oldKeywords, oldSearch := hybridBenchEmbed, hybridBenchKeywords, hybridBenchSearch
	t.Cleanup(func() { hybridBenchEmbed, hybridBenchKeywords, hybridBenchSearch = oldEmbed, oldKeywords, oldSearch })
	logger := loggerutil.CreateDefaultLogger("20261024-745")
	q := hybridBenchQuestion{ID: 7, InputRecordID: 416, HopType: "one_hop", Lang: "en", Question: "q", TargetArtifactID: "416_mtc_2"}

	hybridBenchEmbed = func(ctx context.Context, _ string) ([]float64, bool) {
		if got := searchCallFrom(ctx); got.runID != 42 || got.userID != "u1" {
			t.Errorf("embedding search call = %+v, want run 42 user u1", got)
		}
		return []float64{1}, true
	}
	keywordCalls := 0
	hybridBenchKeywords = func(ctx context.Context, _ string) []string {
		keywordCalls++
		if got := searchCallFrom(ctx); got.runID != 42 || !got.freshKeywords {
			t.Errorf("keyword search call = %+v, want run 42, fresh", got)
		}
		return []string{"k"}
	}
	hybridBenchSearch = func(_ *sql.DB, _ string, _ []float64, cfg registrySearchConfig) ([]artifactSearchResult, error) {
		if cfg.call.freshKeywords {
			t.Error("searches must read the keyword cache")
		}
		switch {
		case cfg.semanticWeight == 0: // lexical
			return benchResults("416_mtc_1", 0.02, "416_mtc_2", 0.01), nil
		case cfg.lexicalWeight == 0: // semantic
			return nil, errors.New("boom")
		}
		return benchResults("416_mtc_2", 0.03), nil
	}
	cfg := registrySearchConfig{lexicalWeight: 1, semanticWeight: 1, call: searchCall{runID: 42, userID: "u1"}}
	r := benchmarkHybridSearchQuestion(context.Background(), nil, q, cfg, logger)
	if r.SearchRunID != 42 {
		t.Fatalf("search run id = %d, want 42", r.SearchRunID)
	}
	if r.LexicalRank == nil || *r.LexicalRank != 2 || r.SemanticRank != nil || r.HybridRank == nil || *r.HybridRank != 1 {
		t.Fatalf("ranks = %v %v %v", r.LexicalRank, r.SemanticRank, r.HybridRank)
	}
	if !strings.Contains(r.ErrorMsg, "semantic: boom") || len(r.Keywords) != 1 || len(r.HybridTop) != 1 || *r.QuestionID != 7 {
		t.Fatalf("result = %+v", r)
	}
	if r.EmbeddingMs == nil || r.KeywordsMs == nil || r.LexicalMs == nil || r.SemanticMs == nil || r.HybridMs == nil {
		t.Fatalf("timings missing: %+v", r)
	}

	// Keywords off for the run: no keyword call and no keyword time.
	off := false
	cfg.call.keywords = &off
	keywordCalls = 0
	r = benchmarkHybridSearchQuestion(context.Background(), nil, q, cfg, logger)
	if keywordCalls != 0 || r.KeywordsMs != nil || len(r.Keywords) != 0 {
		t.Fatalf("keywords off: calls=%d result=%+v", keywordCalls, r)
	}

	hybridBenchEmbed = func(context.Context, string) ([]float64, bool) { return nil, false }
	r = benchmarkHybridSearchQuestion(context.Background(), nil, q, cfg, logger)
	if r.ErrorMsg != hybridBenchErrEmbedding || r.LexicalRank != nil || r.HybridRank != nil {
		t.Fatalf("embedding failure result = %+v", r)
	}
}
