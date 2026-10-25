package kbhandler

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// The hybrid search benchmark runs stored generated questions through the live metric search in
// three modes and records where each question's target metric ranks (design: openspec change
// hybrid-search-benchmark, spec doc-2026060201).

const (
	hybridBenchArtifactType         = "metric"
	hybridBenchMaxDescription       = 500
	hybridBenchHybridTopCount       = 10
	hybridBenchHeartbeat            = 10 * time.Minute
	hybridBenchQuestionTimeout      = 2 * time.Minute
	hybridBenchModeLexical          = "lexical"
	hybridBenchModeSemantic         = "semantic"
	hybridBenchModeHybrid           = "hybrid"
	hybridBenchGroupAll             = "all"
	hybridBenchGroupHopType         = "hop_type"
	hybridBenchGroupLang            = "lang"
	hybridBenchErrEmbedding         = "query embedding failed"
	hybridBenchErrKeywords          = "query keywords failed or empty (see LLM Usage Logs); lexical used the raw query"
	hybridBenchRunListLimit         = 50
	hybridBenchPurposeSearchMetrics = "search_metrics"
	hybridBenchMaxWeight            = 100
	hybridBenchMaxCandidates        = 1000
	hybridBenchDefaultParallelism   = 8
	hybridBenchMaxParallelism       = 32
)

// hybridBenchPurposes lists the question purposes a run accepts; each needs its own artifact type.
var hybridBenchPurposes = []string{hybridBenchPurposeSearchMetrics}

var hybridBenchModes = []string{hybridBenchModeLexical, hybridBenchModeSemantic, hybridBenchModeHybrid}

var hybridBenchHopOrder = []string{"one_hop", "two_hop", "multi_hop"}

// hybridBenchOptions are the search settings a run can choose; the defaults are the configured
// live-search settings.
type hybridBenchOptions struct {
	QueryKeywords  bool    `json:"query_keywords"`
	LexicalWeight  float64 `json:"lexical_weight"`
	SemanticWeight float64 `json:"semantic_weight"`
	CandidateLimit int     `json:"candidate_limit"`
	Parallelism    int     `json:"parallelism"` // questions searched at the same time
}

func hybridBenchDefaults(cfg registrySearchConfig, keywords bool) hybridBenchOptions {
	return hybridBenchOptions{QueryKeywords: keywords, LexicalWeight: cfg.lexicalWeight, SemanticWeight: cfg.semanticWeight,
		CandidateLimit: cfg.candidates(), Parallelism: hybridBenchDefaultParallelism}
}

// Omitted options take the defaults.
type hybridBenchRequest struct {
	Purpose        string   `json:"purpose"`
	Description    string   `json:"description"`
	QueryKeywords  *bool    `json:"query_keywords"`
	LexicalWeight  *float64 `json:"lexical_weight"`
	SemanticWeight *float64 `json:"semantic_weight"`
	CandidateLimit *int     `json:"candidate_limit"`
	Parallelism    *int     `json:"parallelism"`
	QuestionIDs    []int64  `json:"question_ids"` // empty: every question of the purpose
}

var errHybridBenchSelection = errors.New("some selected questions do not exist or have another purpose")

func (r *hybridBenchRequest) normalize(defaults hybridBenchOptions) (hybridBenchOptions, error) {
	r.Purpose = strings.TrimSpace(r.Purpose)
	r.Description = strings.TrimSpace(r.Description)
	opts := defaults
	if r.QueryKeywords != nil {
		opts.QueryKeywords = *r.QueryKeywords
	}
	if r.LexicalWeight != nil {
		opts.LexicalWeight = *r.LexicalWeight
	}
	if r.SemanticWeight != nil {
		opts.SemanticWeight = *r.SemanticWeight
	}
	if r.CandidateLimit != nil {
		opts.CandidateLimit = *r.CandidateLimit
	}
	if r.Parallelism != nil {
		opts.Parallelism = *r.Parallelism
	}
	switch {
	case !slices.Contains(hybridBenchPurposes, r.Purpose):
		return opts, errors.New("unsupported purpose")
	case utf8.RuneCountInString(r.Description) > hybridBenchMaxDescription:
		return opts, errors.New("description is too long")
	case opts.LexicalWeight < 0 || opts.LexicalWeight > hybridBenchMaxWeight || opts.SemanticWeight < 0 || opts.SemanticWeight > hybridBenchMaxWeight:
		return opts, errors.New("RRF weights must be between 0 and 100")
	case opts.LexicalWeight+opts.SemanticWeight <= 0:
		return opts, errors.New("at least one RRF weight must be above 0")
	case opts.CandidateLimit < 1 || opts.CandidateLimit > hybridBenchMaxCandidates:
		return opts, errors.New("candidates per list must be between 1 and 1000")
	case opts.Parallelism < 1 || opts.Parallelism > hybridBenchMaxParallelism:
		return opts, errors.New("parallel queries must be between 1 and 32")
	case len(r.QuestionIDs) > hybridBenchMaxSelected:
		return opts, errors.New("too many selected questions")
	}
	slices.Sort(r.QuestionIDs)
	r.QuestionIDs = slices.Compact(r.QuestionIDs)
	return opts, nil
}

type hybridBenchRun struct {
	ID             int64      `json:"id"`
	Purpose        string     `json:"purpose"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	ErrorMsg       string     `json:"error_msg"`
	QuestionCount  int        `json:"question_count"`
	DoneCount      int        `json:"done_count"`
	LexicalBackend string     `json:"lexical_backend"`
	QueryKeywords  bool       `json:"query_keywords"`
	EmbeddingModel string     `json:"embedding_model"`
	KeywordsModel  string     `json:"keywords_model"`
	KeywordsPrompt string     `json:"keywords_prompt"`
	LexicalWeight  float64    `json:"lexical_weight"`
	SemanticWeight float64    `json:"semantic_weight"`
	CandidateLimit int        `json:"candidate_limit"`
	Parallelism    int        `json:"parallelism"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	HybridR10      *float64   `json:"hybrid_r10,omitempty"`
}

// hybridBenchQuestion is the part of a generated question the benchmark uses.
type hybridBenchQuestion struct {
	ID               int64
	InputRecordID    int64
	HopType          string
	Lang             string
	Question         string
	TargetArtifactID string
}

type hybridBenchResult struct {
	ID               int64    `json:"id"`
	SearchRunID      int64    `json:"search_run_id"`
	QuestionID       *int64   `json:"question_id"`
	InputRecordID    int64    `json:"input_record_id"`
	HopType          string   `json:"hop_type"`
	Lang             string   `json:"lang"`
	Question         string   `json:"question"`
	TargetArtifactID string   `json:"target_artifact_id"`
	Keywords         []string `json:"keywords"`
	LexicalRank      *int     `json:"lexical_rank"`
	SemanticRank     *int     `json:"semantic_rank"`
	HybridRank       *int     `json:"hybrid_rank"`
	HybridTop        []string `json:"hybrid_top"`
	ErrorMsg         string   `json:"error_msg"`
	EmbeddingMs      *int     `json:"embedding_ms"`
	KeywordsMs       *int     `json:"keywords_ms"`
	LexicalMs        *int     `json:"lexical_ms"`
	SemanticMs       *int     `json:"semantic_ms"`
	HybridMs         *int     `json:"hybrid_ms"`
}

func (r hybridBenchResult) rank(mode string) *int {
	switch mode {
	case hybridBenchModeLexical:
		return r.LexicalRank
	case hybridBenchModeSemantic:
		return r.SemanticRank
	}
	return r.HybridRank
}

// hybridBenchScore is one mode's scores over one group of questions (all, one hop type or one
// language). Misses count as 0 in MRR.
type hybridBenchScore struct {
	Mode  string  `json:"mode"`
	Group string  `json:"group"`
	Value string  `json:"value"`
	Count int     `json:"count"`
	R1    float64 `json:"r1"`
	R5    float64 `json:"r5"`
	R10   float64 `json:"r10"`
	MRR   float64 `json:"mrr"`
}

// hybridBenchModeWeights returns the RRF weights that make the hybrid query rank one mode:
// a zero weight leaves the other list's order unchanged.
func hybridBenchModeWeights(mode string, cfg registrySearchConfig) (lexical, semantic float64) {
	switch mode {
	case hybridBenchModeLexical:
		return 1, 0
	case hybridBenchModeSemantic:
		return 0, 1
	}
	return cfg.lexicalWeight, cfg.semanticWeight
}

// hybridBenchTargetRank returns the 1-based rank of target, or nil. Results are ordered by fused
// score, so the zero-score tail (rows only the zero-weighted list found) ends the search.
func hybridBenchTargetRank(results []artifactSearchResult, target string) *int {
	for i, r := range results {
		if r.Score <= 0 {
			return nil
		}
		if r.ArtifactID == target {
			rank := i + 1
			return &rank
		}
	}
	return nil
}

func hybridBenchTop(results []artifactSearchResult) []string {
	top := []string{}
	for _, r := range results {
		if len(top) == hybridBenchHybridTopCount || r.Score <= 0 {
			break
		}
		top = append(top, r.ArtifactID)
	}
	return top
}

func scoreHybridBench(results []hybridBenchResult) []hybridBenchScore {
	type group struct{ name, value string }
	groups := []group{{hybridBenchGroupAll, ""}}
	langs := []string{}
	for _, hop := range hybridBenchHopOrder {
		if slices.ContainsFunc(results, func(r hybridBenchResult) bool { return r.HopType == hop }) {
			groups = append(groups, group{hybridBenchGroupHopType, hop})
		}
	}
	for _, r := range results {
		if !slices.Contains(langs, r.Lang) {
			langs = append(langs, r.Lang)
		}
	}
	slices.Sort(langs)
	for _, lang := range langs {
		groups = append(groups, group{hybridBenchGroupLang, lang})
	}

	scores := []hybridBenchScore{}
	if len(results) == 0 {
		return scores
	}
	for _, mode := range hybridBenchModes {
		for _, g := range groups {
			s := hybridBenchScore{Mode: mode, Group: g.name, Value: g.value}
			for _, r := range results {
				if (g.name == hybridBenchGroupHopType && r.HopType != g.value) || (g.name == hybridBenchGroupLang && r.Lang != g.value) {
					continue
				}
				s.Count++
				rank := r.rank(mode)
				if rank == nil {
					continue
				}
				if *rank <= 1 {
					s.R1++
				}
				if *rank <= 5 {
					s.R5++
				}
				if *rank <= 10 {
					s.R10++
				}
				s.MRR += 1 / float64(*rank)
			}
			n := float64(s.Count)
			s.R1, s.R5, s.R10, s.MRR = s.R1/n, s.R5/n, s.R10/n, s.MRR/n
			scores = append(scores, s)
		}
	}
	return scores
}
