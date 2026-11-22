package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/llmusage"
	"github.com/chendingplano/deepdoc/server/api/searchengine"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// Compliance Review (openspec compliance-reviewer-redesign; devdoc 2026111201). For each chunk the
// reviewer collects metric, provision and text activities, groups metrics and provisions that share
// lines, searches the knowledge base per activity kind, removes unrelated candidates with a decision
// model, judges each activity or group in one call and verifies the strong verdicts. An error for one
// activity is recorded with its finding and never stops the reviewer; only a missing prerequisite or
// setting does. A completed empty search means no candidates found, never that an activity is
// unregulated.
const (
	complianceDefaultTop           = 30
	complianceDefaultMaxActivities = 100
	complianceMaxQueries           = 3
	// complianceEvidenceMax caps the evidence quote derived from the document lines.
	complianceEvidenceMax = 400
	// complianceUnitConcurrency bounds the activities and groups of one chunk reviewed at once;
	// chunks already run in parallel under MAX_DOC_REVIEWER_TASKS.
	complianceUnitConcurrency = 4

	envComplianceCorrectionModel = "DOC_REVIEWER_ERROR_CORRECTION_MODEL_NAME"
	envComplianceFallbackModel   = "DOC_REVIEWER_FALLBACK_MODEL_NAME"
	envComplianceRelevanceModel  = "DOC_REVIEWER_RELEVANCE_MODEL_NAME"
)

// complianceStageSettings maps each prompt stage to its [reviewers.compliance] key. The detect
// prompt (`prompt`) is resolved by the review processor and arrives in ReviewerConfig.
var complianceStageSettings = map[string]string{
	"doc_activity": "doc_activity_prompt",
	"relevance":    "relevance_prompt",
	"judgement":    "judgement_prompt",
	"verify":       "verifier_prompt",
	"correction":   "correction_prompt",
}

type complianceSearchFunc func(ctx context.Context, req searchengine.Request) (*searchengine.Response, error)

type complianceSettings struct {
	TopMetrics, TopProvisions, TopArtifacts      int
	MaxCitedDocuments, MaxMetrics, MaxProvisions int
}

func complianceSettingsFrom(rc ReviewAspectConfig) complianceSettings {
	pick := func(v *int, def int) int {
		if v != nil {
			return *v
		}
		return def
	}
	return complianceSettings{
		TopMetrics:        pick(rc.TopMetrics, complianceDefaultTop),
		TopProvisions:     pick(rc.TopProvisions, complianceDefaultTop),
		TopArtifacts:      pick(rc.TopArtifacts, complianceDefaultTop),
		MaxCitedDocuments: pick(rc.MaxCitedDocuments, 0),
		MaxMetrics:        pick(rc.MaxMetrics, complianceDefaultMaxActivities),
		MaxProvisions:     pick(rc.MaxProvisions, complianceDefaultMaxActivities),
	}
}

type compliancePrompt struct {
	Text, Ref string
}

type complianceReviewer struct {
	// client answers the first try of every structured call; correction and fallback answer the
	// second and third (see callWithTries).
	client               LLMJSONExtractor
	correction, fallback complianceTry
	relevance            complianceRelevanceDecider
	logger               ApiTypes.JimoLogger
	store                complianceStore
	search               complianceSearchFunc
	loadArtifacts        func(context.Context, []searchengine.ArtifactRef) ([]searchengine.Result, error)
	// prompts holds the loaded prompt of each stage in complianceStageSettings.
	prompts  map[string]compliancePrompt
	settings complianceSettings
	// configErr is a missing setting, prompt or model variable; every chunk task returns it.
	configErr error
	// duplicateIDs returns the reviewed record and every record with the same md5 (a field so
	// tests need no database). Nil means the record alone.
	duplicateIDs   func(ctx context.Context, recordID int64) ([]int64, error)
	excluded       sync.Map // recordID -> []int64, one lookup per review
	plans          sync.Map // recordID -> *compliancePlanEntry
	mentions       sync.Map // recordID -> *complianceMentionsEntry
	recordFailed   func(ctx context.Context, call llmusage.FailedCall)
	assessmentDate string
}

func newComplianceReviewer(client LLMJSONExtractor, logger ApiTypes.JimoLogger, db *sql.DB, _ int) *complianceReviewer {
	cfg := searchengine.LoadConfig()
	var rc ReviewAspectConfig
	if dc, _ := GetDocReviewConfig(); dc != nil {
		rc = dc.Reviewers["compliance"]
	}
	r := &complianceReviewer{
		client:         client,
		logger:         logger,
		store:          complianceSQLStore{db: db},
		settings:       complianceSettingsFrom(rc),
		assessmentDate: time.Now().UTC().Format("2006-01-02"),
		duplicateIDs:   func(ctx context.Context, id int64) ([]int64, error) { return sameMD5RecordIDs(ctx, db, id) },
		search: func(ctx context.Context, req searchengine.Request) (*searchengine.Response, error) {
			return searchengine.Search(ctx, db, cfg, req)
		},
		loadArtifacts: func(ctx context.Context, refs []searchengine.ArtifactRef) ([]searchengine.Result, error) {
			return searchengine.Load(ctx, db, cfg, refs)
		},
		recordFailed: func(ctx context.Context, call llmusage.FailedCall) {
			llmusage.RecordFailedCall(ctx, db, logger, call)
		},
	}
	r.configErr = r.configure(rc)
	return r
}

// configure loads the stage prompts and builds the correction, fallback and relevance clients.
// It reports every missing setting and variable at once, naming each.
func (r *complianceReviewer) configure(rc ReviewAspectConfig) error {
	var problems []string
	refs := map[string]string{
		"doc_activity": rc.DocActivityPrompt,
		"relevance":    rc.RelevancePrompt,
		"judgement":    rc.JudgementPrompt,
		"verify":       rc.VerifierPrompt,
		"correction":   rc.CorrectionPrompt,
	}
	r.prompts = map[string]compliancePrompt{}
	for stage, ref := range refs {
		key := complianceStageSettings[stage]
		ref = strings.TrimSpace(ref)
		if ref == "" {
			problems = append(problems, key+" is not set in [reviewers.compliance] of doc-review.local.toml")
			continue
		}
		text, resolved, _, err := loadPromptByRef(ref)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s names %s, which cannot be loaded: %v", key, ref, err))
			continue
		}
		r.prompts[stage] = compliancePrompt{Text: text, Ref: resolved}
	}
	if _, ok := r.client.(docprocessing.LLMStructuredJSONExtractor); !ok {
		problems = append(problems, "the reviewer's model client does not support structured output")
	}
	var err error
	if r.correction, err = complianceTryFromEnv(envComplianceCorrectionModel, "correction"); err != nil {
		problems = append(problems, err.Error())
	}
	if r.fallback, err = complianceTryFromEnv(envComplianceFallbackModel, "fallback"); err != nil {
		problems = append(problems, err.Error())
	}
	if p, ok := r.prompts["relevance"]; ok {
		if r.relevance, err = newComplianceRelevanceDeciderFromEnv(r.logger, p); err != nil {
			problems = append(problems, err.Error())
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("compliance: configuration incomplete: %s", strings.Join(problems, "; "))
	}
	return nil
}

// complianceTryFromEnv builds the client of the second or third try from a model profile named by
// an environment variable. The profile's own thinking setting applies.
func complianceTryFromEnv(env, label string) (complianceTry, error) {
	ref := strings.TrimSpace(os.Getenv(env))
	if ref == "" {
		return complianceTry{}, fmt.Errorf("%s is not set (the %s model of Compliance Review)", env, label)
	}
	client, model, err := docprocessing.BuildReviewerLLMClientWithThinking(ref, "")
	if err != nil {
		return complianceTry{}, fmt.Errorf("%s=%q cannot be used: %v", env, ref, err)
	}
	structured, ok := client.(docprocessing.LLMStructuredJSONExtractor)
	if !ok {
		return complianceTry{}, fmt.Errorf("%s=%q: client does not support structured output", env, ref)
	}
	return complianceTry{client: structured, model: model, label: label}, nil
}

func (r *complianceReviewer) Name() string             { return "compliance" }
func (r *complianceReviewer) Group() string            { return "P5" }
func (r *complianceReviewer) Strategy() ReviewStrategy { return StrategyDocument }

// ReviewDocument is deliberately an error: the reviewer runs per chunk only.
func (r *complianceReviewer) ReviewDocument(context.Context, int64, ReviewerConfig) ([]ReviewFinding, error) {
	return nil, fmt.Errorf("compliance: runs per chunk only; no document-level fallback")
}

// complianceActivity is a text activity: one the model found in the chunk.
type complianceActivity struct {
	Activity      string                  `json:"activity"`
	SourceText    string                  `json:"source_text"`
	LineStart     int                     `json:"line_start"`
	LineEnd       int                     `json:"line_end"`
	Actor         string                  `json:"actor,omitempty"`
	Action        string                  `json:"action,omitempty"`
	Object        string                  `json:"object"`
	Purpose       string                  `json:"purpose,omitempty"`
	DocumentLevel bool                    `json:"document_level"`
	Queries       []complianceQuery       `json:"queries"`
	Relationship  string                  `json:"document_relationship"`
	DocumentType  string                  `json:"document_type,omitempty"`
	CitedDocs     []string                `json:"cited_docs,omitempty"`
	ContextFacts  []complianceContextFact `json:"context_facts,omitempty"`
}

type complianceContextFact struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Status    string `json:"status"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Quote     string `json:"quote"`
	Rationale string `json:"rationale"`
}

// processChunk reviews one chunk. Only a configuration error, a missing prerequisite or a failed
// plan load returns an error (and so aborts the aspect); every other failure stays with its finding.
func (r *complianceReviewer) processChunk(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput) ([]ReviewFinding, error) {
	if r.configErr != nil {
		return nil, r.configErr
	}
	plan, err := r.plan(ctx, recordID)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	members := plan.membersIn(input.startLine, input.endLine)
	var findings []ReviewFinding
	if index == 0 {
		if f, ok := complianceUncheckedFinding(plan, r.settings, input, cfg.ModelName); ok {
			findings = append(findings, f)
		}
	}
	texts, detectErrs := r.detectTextActivities(ctx, recordID, index, cfg, input, members)
	if len(detectErrs) > 0 {
		findings = append(findings, complianceDetectFailureFinding(input, errors.Join(detectErrs...), cfg.ModelName))
	}
	units := buildComplianceUnits(members, texts)
	unitFindings, err := runConcurrent(ctx, complianceUnitConcurrency, len(units), func(unitCtx context.Context, i int) (ReviewFinding, error) {
		return r.reviewUnit(unitCtx, recordID, index, cfg, input, units[i]), nil
	})
	if err != nil {
		return nil, err
	}
	findings = append(findings, unitFindings...)
	r.logger.Info("compliance review chunk end", "record_id", recordID, "chunk", index,
		"metric_provision_activities", len(members), "text_activities", len(texts), "units", len(units),
		"ms_used", time.Since(start).Milliseconds())
	return findings, nil
}

// detectTextActivities finds the chunk's text activities (and, for the first chunk, the document's
// own activity) through three tries each, then drops text activities whose lines the chunk's
// metrics and provisions already cover.
func (r *complianceReviewer) detectTextActivities(ctx context.Context, recordID int64, index int, cfg ReviewerConfig, input chunkInput, members []*complianceMember) ([]complianceActivity, []error) {
	var out []complianceActivity
	var errs []error
	chunk := index
	if index == 0 {
		p := r.prompts["doc_activity"]
		docActs, err := r.detect(ctx, recordID, &chunk, cfg, input, p.Ref, p.Text, "doc_activity", input.inputJSON, true)
		if err != nil {
			errs = append(errs, fmt.Errorf("document activity: %w", err))
		}
		out = append(out, docActs...)
	}
	acts, err := r.detect(ctx, recordID, &chunk, cfg, input, cfg.PromptRef, cfg.PromptText, "detect", complianceDetectInput(input.inputJSON, members), false)
	if err != nil {
		errs = append(errs, fmt.Errorf("text activities: %w", err))
	}
	covered := complianceCoveredLines(members)
	for _, a := range acts {
		if complianceLinesCovered(covered, a.LineStart, a.LineEnd) {
			continue
		}
		out = append(out, a)
	}
	return out, errs
}

func (r *complianceReviewer) detect(ctx context.Context, recordID int64, chunk *int, cfg ReviewerConfig, input chunkInput, promptRef, promptText, stage, inputText string, docLevel bool) ([]complianceActivity, error) {
	var acts []complianceActivity
	_, _, err := r.callWithTries(ctx, cfg, complianceCall{
		Stage: stage, PromptRef: promptRef, PromptText: promptText, Input: inputText,
		Schema:   map[string]any{"type": "object", "required": []any{"activities"}, "properties": map[string]any{"activities": map[string]any{"type": "array"}}},
		RecordID: recordID, ChunkIndex: chunk,
		Check: func(parsed map[string]any) error {
			var perr error
			acts, perr = parseComplianceActivities(parsed, docLevel, input)
			return perr
		},
	})
	if err != nil {
		return nil, err
	}
	return acts, nil
}

// complianceDetectInput adds the chunk's metric and provision activities to the detect input as
// existing_activities, inside the JSON envelope so the chunk prefix stays byte-identical for prompt
// caching. A bare compact table (no envelope) is left as it is.
func complianceDetectInput(inputJSON string, members []*complianceMember) string {
	if len(members) == 0 {
		return inputJSON
	}
	trimmed := strings.TrimRight(inputJSON, " \t\r\n")
	if !strings.HasPrefix(strings.TrimSpace(trimmed), "{") || !strings.HasSuffix(trimmed, "}") {
		return inputJSON
	}
	items := make([]map[string]any, 0, len(members))
	for _, m := range members {
		items = append(items, map[string]any{"kind": m.Kind, "label": m.Label, "line_start": m.LineStart, "line_end": m.LineEnd, "text": complianceCap(m.Text, 300)})
	}
	b, err := json.Marshal(items)
	if err != nil {
		return inputJSON
	}
	return trimmed[:len(trimmed)-1] + `,"existing_activities":` + string(b) + "}"
}

// parseComplianceActivities reads and checks the model's activities. It reports the problems of
// every activity at once, so one correction request can fix them all.
func parseComplianceActivities(payload map[string]any, docLevel bool, input chunkInput) ([]complianceActivity, error) {
	raw, ok := payload["activities"]
	if !ok {
		return nil, fmt.Errorf("compliance model response: missing activities array; return {\"activities\":[...]}")
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("compliance model response: activities must be an array")
	}
	var out []complianceActivity
	var problems []string
	for i, item := range arr {
		a, err := parseComplianceActivity(i, item, docLevel, input)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		out = append(out, a)
	}
	if len(problems) > 0 {
		return nil, errors.New(strings.Join(problems, "\n"))
	}
	return out, nil
}

func parseComplianceActivity(i int, item any, docLevel bool, input chunkInput) (complianceActivity, error) {
	m, ok := item.(map[string]any)
	if !ok {
		return complianceActivity{}, fmt.Errorf("activity %d must be an object", i+1)
	}
	str := func(k string) string { s, _ := m[k].(string); return strings.TrimSpace(s) }
	num := func(k string, def int) int {
		if f, ok := m[k].(float64); ok {
			return int(f)
		}
		return def
	}
	a := complianceActivity{
		Activity:      str("activity"),
		SourceText:    str("source_text"),
		LineStart:     num("line_start", input.startLine),
		LineEnd:       num("line_end", input.endLine),
		Actor:         str("actor"),
		Action:        str("action"),
		Object:        str("object"),
		Purpose:       str("purpose"),
		DocumentLevel: docLevel,
		Relationship:  str("document_relationship"),
		DocumentType:  str("document_type"),
	}
	label := fmt.Sprintf("activity %d (%q, lines %d-%d)", i+1, a.Activity, a.LineStart, a.LineEnd)
	if a.Activity == "" {
		return a, fmt.Errorf("%s: field activity is empty; give a short normalized name", label)
	}
	derivedEvidence := a.SourceText == ""
	if derivedEvidence {
		a.SourceText = complianceLinesText(input.inputJSON, a.LineStart, a.LineEnd)
	}
	if a.SourceText == "" {
		return a, fmt.Errorf("%s: lines %d-%d are not lines of this chunk (%d-%d); use line numbers from the lines table", label, a.LineStart, a.LineEnd, input.startLine, input.endLine)
	}
	lines, valid := parseComplianceInput(input.inputJSON)
	if valid {
		quote := a.SourceText
		if derivedEvidence {
			quote = strings.TrimSuffix(quote, "…")
		}
		if err := complianceQuoteInLines(lines, a.LineStart, a.LineEnd, quote, false); err != nil {
			return a, fmt.Errorf("%s: source_text %v; quote the lines exactly or leave source_text out", label, err)
		}
	}
	if rawFacts, exists := m["context_facts"]; exists {
		facts, err := complianceArray[complianceContextFact](map[string]any{"context_facts": rawFacts}, "context_facts")
		if err != nil {
			return a, fmt.Errorf("%s: context_facts: %v", label, err)
		}
		for j, fact := range facts {
			where := fmt.Sprintf("%s: context fact %d (%q)", label, j+1, fact.Name)
			switch {
			case fact.Name == "":
				return a, fmt.Errorf("%s: field name is empty", where)
			case !complianceOneOf(fact.Status, "explicit", "inferred", "unknown", "conflicting"):
				return a, fmt.Errorf("%s: status %q is not one of explicit, inferred, unknown, conflicting", where, fact.Status)
			case fact.Status == "inferred" && fact.Rationale == "":
				return a, fmt.Errorf("%s: an inferred fact needs a rationale", where)
			case fact.Status == "unknown" && fact.Value != "":
				return a, fmt.Errorf("%s: an unknown fact must have an empty value, not %q", where, fact.Value)
			}
			if fact.Status != "unknown" {
				if !valid {
					return a, fmt.Errorf("%s: context facts need the chunk's lines", where)
				}
				if err := complianceQuoteInLines(lines, fact.LineStart, fact.LineEnd, fact.Quote, false); err != nil {
					return a, fmt.Errorf("%s: quote %v; quote lines %d-%d exactly", where, err, fact.LineStart, fact.LineEnd)
				}
			}
		}
		a.ContextFacts = facts
	}
	if qs, ok := m["search_queries"].([]any); ok {
		for _, q := range qs {
			if len(a.Queries) >= complianceMaxQueries {
				break
			}
			if cq, ok := parseComplianceQuery(q); ok {
				a.Queries = append(a.Queries, cq)
			}
		}
	}
	if refs, exists := m["cited_docs"]; exists {
		list, err := complianceArray[string](map[string]any{"cited_docs": refs}, "cited_docs")
		if err != nil {
			return a, fmt.Errorf("%s: cited_docs: %v", label, err)
		}
		if len(list) > 10 {
			return a, fmt.Errorf("%s: cited_docs lists %d documents; list at most 10", label, len(list))
		}
		a.CitedDocs = list
	}
	if len(a.Queries) == 0 {
		a.Queries = []complianceQuery{{Text: a.SourceText}}
	}
	return a, nil
}

type complianceInputLine struct {
	LineNumber int    `json:"line_number"`
	Content    string `json:"content"`
}

// parseComplianceInput reads the shared compact chunk input: {"doc_context":..,"lines":[header,
// row,...]} where the header row names the columns of the rows after it (see
// docprocessing.RawLinesToCompactJSON), or the bare [header, row, ...] table. Other input reports ok=false.
func parseComplianceInput(inputJSON string) (lines []complianceInputLine, ok bool) {
	var env struct {
		Lines [][]any `json:"lines"`
	}
	if err := json.Unmarshal([]byte(inputJSON), &env); err != nil {
		// The compact table may also arrive bare, as [header, row, ...] without the envelope.
		var bare [][]any
		if json.Unmarshal([]byte(inputJSON), &bare) != nil {
			return nil, false
		}
		env.Lines = bare
	}
	if len(env.Lines) < 2 {
		return nil, false
	}
	numCol, contentCol := -1, -1
	for i, h := range env.Lines[0] {
		switch h {
		case "line_number":
			numCol = i
		case "content":
			contentCol = i
		}
	}
	if numCol < 0 || contentCol < 0 {
		return nil, false
	}
	for _, row := range env.Lines[1:] {
		if len(row) <= numCol || len(row) <= contentCol {
			continue
		}
		n, _ := row[numCol].(float64)
		c, _ := row[contentCol].(string)
		lines = append(lines, complianceInputLine{LineNumber: int(n), Content: c})
	}
	return lines, len(lines) > 0
}

// complianceLinesText returns the content of lines start..end of the chunk input (the evidence
// quote, derived from the document so the model need not repeat it), capped in length.
func complianceLinesText(inputJSON string, start, end int) string {
	lines, ok := parseComplianceInput(inputJSON)
	if !ok {
		return ""
	}
	var parts []string
	for _, l := range lines {
		if l.LineNumber >= start && l.LineNumber <= end {
			parts = append(parts, strings.TrimSpace(l.Content))
		}
	}
	text := strings.Join(parts, "\n")
	if r := []rune(text); len(r) > complianceEvidenceMax {
		text = string(r[:complianceEvidenceMax]) + "…"
	}
	return text
}

// complianceQuery is one knowledge-base search for a text activity. The model gives it as bilingual
// keywords, which go to the engine as the keyword list, so the engine's keyword LLM call is skipped;
// Text (the keywords joined) feeds the vector side. A plain-string query has no Keywords and takes
// the engine's default keyword handling.
type complianceQuery struct {
	Text     string
	Keywords []searchengine.Keyword
}

// parseComplianceQuery reads a search_queries element: a string, or a list of {"en","zh"} keywords
// (plain strings inside the list are accepted too).
func parseComplianceQuery(raw any) (complianceQuery, bool) {
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		return complianceQuery{Text: s}, s != ""
	}
	items, ok := raw.([]any)
	if !ok {
		return complianceQuery{}, false
	}
	var q complianceQuery
	var terms []string
	seen := map[string]bool{}
	add := func(v any) {
		t, _ := v.(string)
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			return
		}
		seen[t] = true
		terms = append(terms, t)
		q.Keywords = append(q.Keywords, searchengine.Keyword{Term: t})
	}
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			add(m["en"])
			add(m["zh"])
		} else {
			add(it)
		}
	}
	q.Text = strings.Join(terms, " ")
	return q, len(terms) > 0
}

// sameMD5RecordIDs returns recordID plus every other record with the same non-empty md5 (a
// re-uploaded copy of the reviewed file must not ground it).
func sameMD5RecordIDs(ctx context.Context, db *sql.DB, recordID int64) ([]int64, error) {
	ids := []int64{recordID}
	rows, err := db.QueryContext(ctx, `SELECT id FROM kb.inputs
		WHERE id <> $1 AND COALESCE(md5, '') <> '' AND md5 = (SELECT md5 FROM kb.inputs WHERE id = $1)
		ORDER BY id`, recordID)
	if err != nil {
		return nil, fmt.Errorf("compliance duplicate lookup for record %d: %w", recordID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *complianceReviewer) excludeIDs(ctx context.Context, recordID int64) ([]int64, error) {
	if v, ok := r.excluded.Load(recordID); ok {
		return v.([]int64), nil
	}
	ids := []int64{recordID}
	if r.duplicateIDs != nil {
		var err error
		if ids, err = r.duplicateIDs(ctx, recordID); err != nil {
			return nil, err
		}
	}
	r.excluded.Store(recordID, ids)
	return ids, nil
}

func complianceArray[T any](payload map[string]any, key string) ([]T, error) {
	value, ok := payload[key]
	if !ok || value == nil {
		return nil, fmt.Errorf("missing %s array", key)
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || b[0] != '[' {
		return nil, fmt.Errorf("%s must be an array", key)
	}
	var out []T
	if err = json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", key, err)
	}
	return out, nil
}

func complianceOneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}

func complianceCap(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
