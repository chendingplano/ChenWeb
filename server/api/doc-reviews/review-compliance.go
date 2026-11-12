package docreviews

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// Compliance Review (research 2026111103-rsch-compliance-reviewer, delivery phase 1 thin slice).
//
// Independent, per-chunk only (no block or document fallback). Per chunk it:
//  1. detects activities from the chunk alone (never reads the knowledge base; the first
//     chunk also yields the document-level activity),
//  2. searches the knowledge base for each activity through searchengine.Search
//     (provisions and original chunks, reviewed record excluded, no MinCosine),
//  3. reports one informational row per activity: grounding candidates, or "none found".
//
// Not yet built (later phases): applicability, requirement comparison, omission gaps and the
// verifier. Candidates are therefore retrieval results, NOT analysis-accepted grounding, and
// "no grounding" means a completed search returned nothing, never "unregulated".
const (
	complianceSearchLimit = 10
	complianceMaxQueries  = 3
)

type complianceSearchFunc func(ctx context.Context, req searchengine.Request) (*searchengine.Response, error)

type complianceReviewer struct {
	client LLMJSONExtractor
	logger ApiTypes.JimoLogger
	search complianceSearchFunc
	// loadPrompt resolves the document-level activity prompt (a field so tests need no prompts dir).
	loadPrompt func(ref string) (text, ref2 string, err error)
	// docActivityPromptRef is [reviewers.compliance].doc_activity_prompt in doc-review.local.toml.
	docActivityPromptRef string
	maxTasks             int
}

func newComplianceReviewer(client LLMJSONExtractor, logger ApiTypes.JimoLogger, db *sql.DB, maxTasks int) *complianceReviewer {
	cfg := searchengine.LoadConfig()
	docPrompt := ""
	if dc, _ := GetDocReviewConfig(); dc != nil {
		docPrompt = strings.TrimSpace(dc.Reviewers["compliance"].DocActivityPrompt)
	}
	return &complianceReviewer{
		docActivityPromptRef: docPrompt,
		client:               client,
		logger:               logger,
		maxTasks:             maxTasks,
		loadPrompt: func(ref string) (string, string, error) {
			text, out, _, err := loadPromptByRef(ref)
			return text, out, err
		},
		search: func(ctx context.Context, req searchengine.Request) (*searchengine.Response, error) {
			return searchengine.Search(ctx, db, cfg, req)
		},
	}
}

func (r *complianceReviewer) Name() string             { return "compliance" }
func (r *complianceReviewer) Group() string            { return "P5" }
func (r *complianceReviewer) Strategy() ReviewStrategy { return StrategyDocument }

// ReviewDocument is deliberately an error: the reviewer runs per chunk only.
func (r *complianceReviewer) ReviewDocument(context.Context, int64, ReviewerConfig) ([]ReviewFinding, error) {
	return nil, fmt.Errorf("compliance: runs per chunk only; no document-level fallback")
}

type complianceActivity struct {
	Activity      string
	SourceText    string
	LineStart     int
	LineEnd       int
	Actor         string
	Action        string
	Object        string
	Purpose       string
	DocumentLevel bool
	Queries       []string
}

func (r *complianceReviewer) processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) ([]ReviewFinding, error) {
	start := time.Now()
	activities, err := r.detect(ctx, cfg.PromptRef, cfg.PromptText, cfg.ModelName, input, false)
	if err != nil {
		r.logger.Warn("compliance activity detection failed; aborting reviewer", "record_id", recordID, "chunk", index, "error", err)
		return nil, err
	}
	if index == 0 {
		if r.docActivityPromptRef == "" {
			return nil, fmt.Errorf("compliance: doc_activity_prompt is not set in [reviewers.compliance] of doc-review.local.toml")
		}
		text, ref, perr := r.loadPrompt(r.docActivityPromptRef)
		if perr != nil {
			return nil, fmt.Errorf("compliance: load %s: %w", r.docActivityPromptRef, perr)
		}
		docActs, derr := r.detect(ctx, ref, text, cfg.ModelName, input, true)
		if derr != nil {
			r.logger.Warn("compliance document-level activity failed; aborting reviewer", "record_id", recordID, "error", derr)
			return nil, derr
		}
		activities = append(docActs, activities...)
	}

	var findings []ReviewFinding
	for _, a := range activities {
		f, err := r.ground(ctx, recordID, a, input)
		if err != nil {
			r.logger.Warn("compliance knowledge-base search failed; aborting reviewer",
				"record_id", recordID, "chunk", index, "activity", a.Activity, "error", err)
			return nil, err
		}
		findings = append(findings, f)
	}
	r.logger.Info("compliance review chunk end", "record_id", recordID, "chunk", index,
		"activities", len(activities), "ms_used", time.Since(start).Milliseconds())
	return findings, nil
}

func (r *complianceReviewer) detect(ctx context.Context, promptRef, promptText, model string, input chunkInput, docLevel bool) ([]complianceActivity, error) {
	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, promptRef, promptText, model, input.inputJSON,
		"review_compliance", "MID-CWB-REVIEW-COMPLIANCE"))
	if err != nil {
		return nil, err
	}
	return parseComplianceActivities(payload, docLevel, input)
}

func parseComplianceActivities(payload map[string]any, docLevel bool, input chunkInput) ([]complianceActivity, error) {
	raw, ok := payload["activities"]
	if !ok {
		return nil, fmt.Errorf("compliance model response: missing activities array")
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("compliance model response: activities must be an array")
	}
	str := func(m map[string]any, k string) string { s, _ := m[k].(string); return strings.TrimSpace(s) }
	num := func(m map[string]any, k string, def int) int {
		if f, ok := m[k].(float64); ok {
			return int(f)
		}
		return def
	}
	var out []complianceActivity
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("compliance model response: activity %d must be an object", i)
		}
		a := complianceActivity{
			Activity:      str(m, "activity"),
			SourceText:    str(m, "source_text"),
			LineStart:     num(m, "line_start", input.startLine),
			LineEnd:       num(m, "line_end", input.endLine),
			Actor:         str(m, "actor"),
			Action:        str(m, "action"),
			Object:        str(m, "object"),
			Purpose:       str(m, "purpose"),
			DocumentLevel: docLevel,
		}
		if a.Activity == "" || a.SourceText == "" {
			return nil, fmt.Errorf("compliance model response: activity %d needs activity and source_text", i)
		}
		if qs, ok := m["search_queries"].([]any); ok {
			for _, q := range qs {
				if s, ok := q.(string); ok && strings.TrimSpace(s) != "" && len(a.Queries) < complianceMaxQueries {
					a.Queries = append(a.Queries, strings.TrimSpace(s))
				}
			}
		}
		if len(a.Queries) == 0 {
			a.Queries = []string{a.SourceText}
		}
		out = append(out, a)
	}
	return out, nil
}

type complianceCandidate struct {
	artifactType string
	artifactID   string
	recordID     int64
	title        string
	snippet      string
	rank         int
}

// ground searches the knowledge base for one activity and builds its report row.
func (r *complianceReviewer) ground(ctx context.Context, recordID int64, a complianceActivity, input chunkInput) (ReviewFinding, error) {
	seen := map[string]bool{}
	var cands []complianceCandidate
	for _, q := range a.Queries {
		resp, err := r.search(ctx, searchengine.Request{
			Query: searchengine.Query{Text: q},
			Filters: searchengine.Filters{
				ArtifactTypes:    []string{"provision", "chunk"},
				ExcludeRecordIDs: []int64{recordID},
			},
			Options: searchengine.Options{Limit: complianceSearchLimit},
		})
		if err != nil {
			return ReviewFinding{}, fmt.Errorf("compliance search %q: %w", q, err)
		}
		for _, res := range resp.Results {
			key := res.ArtifactType + "/" + res.ArtifactID
			if seen[key] {
				continue
			}
			seen[key] = true
			title := res.SourceTitle
			if title == "" {
				title = res.SourceFilename
			}
			cands = append(cands, complianceCandidate{res.ArtifactType, res.ArtifactID, res.InputRecordID, title, res.Snippet, len(cands) + 1})
		}
	}

	label := a.Activity
	if a.DocumentLevel {
		label = "document: " + a.Object
	}
	f := ReviewFinding{
		Pass:       "P5",
		Aspect:     "compliance",
		Severity:   "info",
		ResultKind: "compliance_activity",
		Evidence:   a.SourceText,
		Location:   fmt.Sprintf("%d-%d", a.LineStart, a.LineEnd),
		Confidence: 0,
	}
	if len(cands) == 0 {
		f.FindingType = "compliance_no_grounding"
		f.Title = "Activity with no grounding candidates found: " + label
		f.Description = "A completed search of the knowledge base returned no candidate artifact for this activity. " +
			"This does not mean the activity is unregulated."
		return f, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Retrieved %d candidate artifact(s) from the knowledge base that may govern this activity. "+
		"Candidates are search results; applicability and requirement comparison are not assessed.\n", len(cands))
	for _, c := range cands {
		fmt.Fprintf(&b, "- [%s %s] record %d %s: %s\n", c.artifactType, c.artifactID, c.recordID, c.title, strings.TrimSpace(c.snippet))
	}
	f.FindingType = "compliance_grounding_candidates"
	f.Title = "Grounding candidates found: " + label
	f.Description = b.String()
	f.RelatedArtifactID = cands[0].artifactID
	f.RelatedRecordID = cands[0].recordID
	return f, nil
}
