package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/lib/pq"
)

// Search effort levels (openspec hybrid-search-effort-levels). Low searches the input as written.
// Medium and high let an LLM analyse the input, compose queries when it describes a need instead of
// naming it, judge whether the results answer it, and reformulate until they do or the attempts
// run out. "Found" is the judge's decision: live search has no ground truth.
const (
	searchEffortLow    = "low"
	searchEffortMedium = "medium"
	searchEffortHigh   = "high"

	envSearchEffortModel     = "HYBRID_SEARCH_EFFORT_MODEL_NAME"
	envSearchEffortMedium    = "HYBRID_SEARCH_MEDIUM_EFFORTS"
	envSearchEffortHigh      = "HYBRID_SEARCH_HIGH_EFFORTS"
	envSearchEffortTimeoutMS = "HYBRID_SEARCH_EFFORT_TIMEOUT_MS"

	defaultSearchEffortMedium  = 2
	defaultSearchEffortHigh    = 5
	defaultSearchEffortTimeout = 20 * time.Second

	searchEffortAnalyzePrompt = "prompt-search-effort-analyze-v1.md"
	searchEffortJudgePrompt   = "prompt-search-effort-judge-v1.md"

	searchEffortPerQuery      = 20 // results kept from each query before fusion
	searchEffortJudgeTop      = 10 // fused candidates shown to the judge
	searchEffortMaxQueries    = 3  // queries per attempt
	searchEffortMaxQueryRunes = 200
	searchEffortDetailRunes   = 800 // search document text shown to the judge per candidate
)

var searchEffortLevels = []string{searchEffortLow, searchEffortMedium, searchEffortHigh}

var (
	// Replaced in tests.
	newSearchEffortClient = func(modelRef string, cfg ApiTypes.LLMModelDef, logger ApiTypes.JimoLogger) (metricJSONExtractor, error) {
		return defaultNewExtractMetricsClient(modelRef, cfg, logger)
	}
	loadSearchEffortModel  = loadModelDefByRef
	loadSearchEffortPrompt = docprocessing.LoadPromptByRef
	searchEffortQuery      = func(db *sql.DB, artifactType, query string, filters artifactSearchFilters, limit int, cfg registrySearchConfig) ([]artifactSearchResult, error) {
		return queryRegistrySearchResults(db, artifactType, query, filters, 1, limit, cfg)
	}
	searchEffortDetails = readSearchEffortDetails
)

// readSearchEffortDetails returns the start of each candidate's registry search document (which
// holds its value, unit and source wording), keyed by searchEffortKey.
func readSearchEffortDetails(ctx context.Context, db *sql.DB, results []artifactSearchResult) (map[string]string, error) {
	types, ids := make([]string, len(results)), make([]string, len(results))
	for i, r := range results {
		types[i], ids[i] = r.ArtifactType, r.ArtifactID
	}
	rows, err := db.QueryContext(ctx, `SELECT sa.artifact_type, sa.artifact_id, left(sa.search_document, $3)
 FROM kb.search_artifacts sa JOIN unnest($1::text[], $2::text[]) AS k(t, id)
   ON sa.artifact_type = k.t AND sa.artifact_id = k.id`, pq.Array(types), pq.Array(ids), searchEffortDetailRunes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	details := map[string]string{}
	for rows.Next() {
		var t, id, doc string
		if err := rows.Scan(&t, &id, &doc); err != nil {
			return nil, err
		}
		details[t+"/"+id] = doc
	}
	return details, rows.Err()
}

// parseSearchEffort accepts "", low, medium or high ("" means low).
func parseSearchEffort(raw string) (string, error) {
	effort := strings.ToLower(strings.TrimSpace(raw))
	if effort == "" {
		return searchEffortLow, nil
	}
	if !slices.Contains(searchEffortLevels, effort) {
		return "", fmt.Errorf("effort must be low, medium or high")
	}
	return effort, nil
}

// searchEffortMaxAttempts returns the configured attempt limit (0 for low).
func searchEffortMaxAttempts(effort string) int {
	env, def := "", 0
	switch effort {
	case searchEffortMedium:
		env, def = envSearchEffortMedium, defaultSearchEffortMedium
	case searchEffortHigh:
		env, def = envSearchEffortHigh, defaultSearchEffortHigh
	default:
		return 0
	}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(env))); err == nil && n >= 1 {
		return n
	}
	return def
}

func searchEffortTimeout() time.Duration {
	if ms, err := strconv.Atoi(strings.TrimSpace(os.Getenv(envSearchEffortTimeoutMS))); err == nil && ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return defaultSearchEffortTimeout
}

type searchEffortAttempt struct {
	Queries     []string `json:"queries"`
	Found       bool     `json:"found"`
	MatchingIDs []string `json:"matching_ids"`
	Reason      string   `json:"reason"`
	Error       string   `json:"error,omitempty"`
	Ms          int64    `json:"ms"`
}

// searchEffortTrace explains a medium/high search: the analysis and every attempt.
type searchEffortTrace struct {
	Effort      string                `json:"effort"`
	MaxAttempts int                   `json:"max_attempts"`
	Strategy    string                `json:"strategy"`
	Need        string                `json:"need"`
	Found       bool                  `json:"found"`
	Attempts    []searchEffortAttempt `json:"attempts"`
	Error       string                `json:"error,omitempty"`
}

type searchEffortLLM struct {
	client                     metricJSONExtractor
	modelName                  string
	analyzePrompt, judgePrompt string
}

func loadSearchEffortLLM(logger ApiTypes.JimoLogger) (searchEffortLLM, error) {
	modelRef := strings.TrimSpace(os.Getenv(envSearchEffortModel))
	if modelRef == "" {
		return searchEffortLLM{}, fmt.Errorf("%s is required", envSearchEffortModel)
	}
	cfg, err := loadSearchEffortModel(modelRef)
	if err != nil {
		return searchEffortLLM{}, fmt.Errorf("model %q: %w", modelRef, err)
	}
	cfg.ThinkingType = "disabled" // on the search's critical path
	llm := searchEffortLLM{modelName: cfg.ModelName}
	for _, p := range []struct {
		ref    string
		target *string
	}{{searchEffortAnalyzePrompt, &llm.analyzePrompt}, {searchEffortJudgePrompt, &llm.judgePrompt}} {
		text, _, _, err := loadSearchEffortPrompt(p.ref)
		if err != nil {
			return searchEffortLLM{}, fmt.Errorf("prompt %q: %w", p.ref, err)
		}
		*p.target = text
	}
	if llm.client, err = newSearchEffortClient(modelRef, cfg, logger); err != nil {
		return searchEffortLLM{}, fmt.Errorf("client for %q: %w", modelRef, err)
	}
	return llm, nil
}

func (l searchEffortLLM) call(ctx context.Context, promptRef, promptText, reason, loc string, input any) (map[string]any, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	call := searchCallFrom(ctx)
	cctx, cancel := context.WithTimeout(ctx, searchEffortTimeout())
	defer cancel()
	return l.client.ExtractJSON(cctx, llmclients.JSONExtractionInput{
		UserID:     call.userID,
		RunID:      call.runID,
		PromptName: llmclients.EnsurePromptName(promptRef, reason, loc, l.modelName),
		PromptText: promptText,
		ModelName:  l.modelName,
		InputText:  string(body),
		CallReason: reason,
		CallLoc:    loc,
	})
}

// cleanSearchEffortQueries keeps up to searchEffortMaxQueries non-empty queries that were not
// tried before (case-insensitive), each at most searchEffortMaxQueryRunes characters.
func cleanSearchEffortQueries(raw any, tried []string) []string {
	seen := map[string]bool{}
	for _, q := range tried {
		seen[strings.ToLower(q)] = true
	}
	items, _ := raw.([]any)
	out := []string{}
	for _, it := range items {
		q, _ := it.(string)
		q = strings.TrimSpace(q)
		if r := []rune(q); len(r) > searchEffortMaxQueryRunes {
			q = string(r[:searchEffortMaxQueryRunes])
		}
		if q == "" || seen[strings.ToLower(q)] {
			continue
		}
		seen[strings.ToLower(q)] = true
		out = append(out, q)
		if len(out) == searchEffortMaxQueries {
			break
		}
	}
	return out
}

type searchEffortCandidate struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	SecondaryLabel string `json:"secondary_label,omitempty"`
	Snippet        string `json:"snippet"`
	Details        string `json:"details,omitempty"`
}

func searchEffortKey(r artifactSearchResult) string { return r.ArtifactType + "/" + r.ArtifactID }

// searchWithEffort runs a search at the given effort and returns up to limit results, best first.
// The trace is nil for low effort.
func searchWithEffort(ctx context.Context, db *sql.DB, artifactType, request string, filters artifactSearchFilters, limit int, cfg registrySearchConfig, effort string) ([]artifactSearchResult, *searchEffortTrace, error) {
	maxAttempts := searchEffortMaxAttempts(effort)
	if maxAttempts == 0 {
		results, err := searchEffortQuery(db, artifactType, request, filters, limit, cfg)
		return results, nil, err
	}
	logger := loggerutil.CreateDefaultLogger("20261025-748")
	ctx = withSearchCall(ctx, cfg.call)
	trace := &searchEffortTrace{Effort: effort, MaxAttempts: maxAttempts, Attempts: []searchEffortAttempt{}}
	fallback := func(reason string, err error) ([]artifactSearchResult, *searchEffortTrace, error) {
		trace.Error = reason + ": " + err.Error()
		logger.Warn("search effort fallback to the input as written", "effort", effort, "reason", reason, "error", err)
		results, serr := searchEffortQuery(db, artifactType, request, filters, limit, cfg)
		return results, trace, serr
	}

	llm, err := loadSearchEffortLLM(logger)
	if err != nil {
		return fallback("model or prompt unavailable", err)
	}
	analysis, err := llm.call(ctx, searchEffortAnalyzePrompt, llm.analyzePrompt, "search_effort_analyze", "MID-20261025-01",
		map[string]any{"request": request, "artifact_type": artifactType})
	if err != nil {
		return fallback("analysis failed", err)
	}
	trace.Strategy, _ = analysis["strategy"].(string)
	trace.Need, _ = analysis["need"].(string)
	queries := cleanSearchEffortQueries(analysis["queries"], nil)
	if trace.Strategy != "compose" || len(queries) == 0 {
		// A direct search starts with the input as written, then any extra angles proposed.
		queries = append([]string{request}, cleanSearchEffortQueries(analysis["queries"], []string{request})...)
		queries = queries[:min(len(queries), searchEffortMaxQueries)]
	}

	type fusedItem struct {
		result artifactSearchResult
		score  float64
	}
	fused := map[string]*fusedItem{}
	ranked := func() []artifactSearchResult {
		items := make([]*fusedItem, 0, len(fused))
		for _, it := range fused {
			items = append(items, it)
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].score != items[j].score {
				return items[i].score > items[j].score
			}
			return searchEffortKey(items[i].result) < searchEffortKey(items[j].result)
		})
		out := make([]artifactSearchResult, len(items))
		for i, it := range items {
			out[i] = it.result
			out[i].Score = it.score
		}
		return out
	}

	tried := []string{}
	matched := []string{}
	for attempt := 1; attempt <= maxAttempts && len(queries) > 0; attempt++ {
		start := time.Now()
		att := searchEffortAttempt{Queries: queries, MatchingIDs: []string{}}
		errs := []string{}
		for _, q := range queries {
			results, err := searchEffortQuery(db, artifactType, q, filters, searchEffortPerQuery, cfg)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			for i, r := range results {
				key := searchEffortKey(r)
				if fused[key] == nil {
					fused[key] = &fusedItem{result: r}
				}
				fused[key].score += 1 / float64(rrfK+i+1)
			}
		}
		tried = append(tried, queries...)
		list := ranked()
		top := list[:min(len(list), searchEffortJudgeTop)]
		details, err := searchEffortDetails(ctx, db, top)
		if err != nil {
			// The judge can still decide from labels and snippets.
			logger.Warn("load search effort candidate details failed", "error", err)
		}
		candidates := []searchEffortCandidate{}
		for _, r := range top {
			candidates = append(candidates, searchEffortCandidate{ID: searchEffortKey(r), Label: r.PrimaryLabel, SecondaryLabel: r.SecondaryLabel,
				Snippet: truncateRunesForEmbedding(r.Snippet, 400), Details: details[searchEffortKey(r)]})
		}
		judgement, err := llm.call(ctx, searchEffortJudgePrompt, llm.judgePrompt, "search_effort_judge", "MID-20261025-02", map[string]any{
			"request": request, "need": trace.Need, "artifact_type": artifactType,
			"attempt": attempt, "max_attempts": maxAttempts, "tried_queries": tried, "candidates": candidates,
		})
		if err != nil {
			errs = append(errs, "judge: "+err.Error())
			att.Error, att.Ms = strings.Join(errs, "; "), time.Since(start).Milliseconds()
			trace.Attempts = append(trace.Attempts, att)
			trace.Error = "judge failed; returned the results so far"
			break
		}
		shown := map[string]bool{}
		for _, c := range candidates {
			shown[c.ID] = true
		}
		ids, _ := judgement["matching_ids"].([]any)
		for _, v := range ids {
			if id, _ := v.(string); shown[id] && !slices.Contains(att.MatchingIDs, id) {
				att.MatchingIDs = append(att.MatchingIDs, id)
			}
		}
		found, _ := judgement["found"].(bool)
		att.Found = found && len(att.MatchingIDs) > 0
		att.Reason, _ = judgement["reason"].(string)
		att.Error, att.Ms = strings.Join(errs, "; "), time.Since(start).Milliseconds()
		trace.Attempts = append(trace.Attempts, att)
		if att.Found {
			trace.Found, matched = true, att.MatchingIDs
			break
		}
		queries = cleanSearchEffortQueries(judgement["next_queries"], tried)
	}

	// Matching results first (judge order), then everything else by fused score.
	list := ranked()
	out := make([]artifactSearchResult, 0, len(list))
	for _, id := range matched {
		for _, r := range list {
			if searchEffortKey(r) == id {
				out = append(out, r)
			}
		}
	}
	for _, r := range list {
		if !slices.Contains(matched, searchEffortKey(r)) {
			out = append(out, r)
		}
	}
	logger.Info("search effort finished", "effort", effort, "strategy", trace.Strategy, "attempts", len(trace.Attempts),
		"found", trace.Found, "results", len(out), "search_run_id", cfg.call.runID)
	return out[:min(len(out), limit)], trace, nil
}
