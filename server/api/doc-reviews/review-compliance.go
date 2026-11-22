package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// Compliance Review detects activities per chunk, independently of the knowledge base, then
// searches provisions, metrics and chunks independently. Production reviewers assess original
// source requirements against the whole canonical reviewed document, verify quoted evidence and
// quantitative comparisons, and retain coverage and uncertainty. No document/block fallback.
// A completed empty search means no candidates found, never that an activity is unregulated.
const (
	complianceSearchLimit = 10
	complianceMaxQueries  = 3
	// complianceEvidenceMax caps the evidence quote derived from the document lines.
	complianceEvidenceMax = 400
)

type complianceSearchFunc func(ctx context.Context, req searchengine.Request) (*searchengine.Response, error)

type complianceReviewer struct {
	client        LLMJSONExtractor
	logger        ApiTypes.JimoLogger
	search        complianceSearchFunc
	loadArtifacts func(context.Context, []searchengine.ArtifactRef) ([]searchengine.Result, error)
	// loadPrompt resolves the document-level activity prompt (a field so tests need no prompts dir).
	loadPrompt func(ref string) (text, ref2 string, err error)
	// docActivityPromptRef is [reviewers.compliance].doc_activity_prompt in doc-review.local.toml.
	docActivityPromptRef string
	maxTasks             int
	// minCosine is [reviewers.compliance].min_cosine; 0 means no relevance floor.
	minCosine float64
	// duplicateIDs returns the reviewed record and every record with the same md5 (a field so
	// tests need no database). Nil means the record alone.
	duplicateIDs      func(ctx context.Context, recordID int64) ([]int64, error)
	excluded          sync.Map // recordID -> []int64, one lookup per review
	assessmentPrompts map[string]string
	loadDocument      func(context.Context, int64) (complianceDocument, error)
	documents         sync.Map
	maxSourceRecords  int
	maxEvidenceChunks int
	assessmentDate    string
}

func newComplianceReviewer(client LLMJSONExtractor, logger ApiTypes.JimoLogger, db *sql.DB, maxTasks int) *complianceReviewer {
	cfg := searchengine.LoadConfig()
	var rc ReviewAspectConfig
	if dc, _ := GetDocReviewConfig(); dc != nil {
		rc = dc.Reviewers["compliance"]
	}
	minCosine := 0.0
	if rc.MinCosine != nil {
		minCosine = *rc.MinCosine
	}
	return &complianceReviewer{
		assessmentPrompts: complianceAssessmentPromptRefs(rc),
		loadDocument: func(ctx context.Context, id int64) (complianceDocument, error) {
			return loadComplianceDocument(ctx, db, id)
		},
		maxSourceRecords:     8,
		maxEvidenceChunks:    32,
		assessmentDate:       time.Now().UTC().Format("2006-01-02"),
		docActivityPromptRef: strings.TrimSpace(rc.DocActivityPrompt),
		minCosine:            minCosine,
		duplicateIDs:         func(ctx context.Context, id int64) ([]int64, error) { return sameMD5RecordIDs(ctx, db, id) },
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
		loadArtifacts: func(ctx context.Context, refs []searchengine.ArtifactRef) ([]searchengine.Result, error) {
			return searchengine.Load(ctx, db, cfg, refs)
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
		f.ModelName = cfg.ModelName
		findings = append(findings, f)
		if r.assessmentPrompts != nil && f.FindingType != "compliance_no_grounding" {
			assessed, aerr := r.assessActivity(ctx, recordID, a, cfg, f)
			if aerr != nil {
				r.logger.Warn("compliance assessment failed; aborting reviewer", "record_id", recordID, "chunk", index, "activity", a.Activity, "error", aerr)
				return nil, aerr
			}
			findings[len(findings)-1] = assessed
		}
	}
	r.logger.Info("compliance review chunk end", "record_id", recordID, "chunk", index,
		"activities", len(activities), "ms_used", time.Since(start).Milliseconds())
	return findings, nil
}

func (r *complianceReviewer) detect(ctx context.Context, promptRef, promptText, model string, input chunkInput, docLevel bool) ([]complianceActivity, error) {
	payload, err := r.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, promptRef, promptText, model, input.inputJSON,
		"review_compliance", "MID-20261112-01"))
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
			Relationship:  str(m, "document_relationship"),
			DocumentType:  str(m, "document_type"),
		}
		derivedEvidence := a.SourceText == ""
		if a.SourceText == "" && a.Activity != "" {
			a.SourceText = complianceLinesText(input.inputJSON, a.LineStart, a.LineEnd)
		}
		if lines, valid := parseComplianceInput(input.inputJSON); valid {
			quote := a.SourceText
			if derivedEvidence {
				quote = strings.TrimSuffix(quote, "…")
			}
			if err := verifyComplianceEvidence(complianceEvidence{RecordID: 1, LineStart: a.LineStart, LineEnd: a.LineEnd, Quote: quote}, map[int64]complianceDocument{1: {RecordID: 1, Lines: lines}}); err != nil {
				return nil, err
			}
		}
		if rawFacts, exists := m["context_facts"]; exists {
			facts, err := complianceArray[complianceContextFact](map[string]any{"context_facts": rawFacts}, "context_facts")
			if err != nil {
				return nil, err
			}
			lines, valid := parseComplianceInput(input.inputJSON)
			for _, fact := range facts {
				if fact.Name == "" || !complianceOneOf(fact.Status, "explicit", "inferred", "unknown", "conflicting") || (fact.Status == "inferred" && fact.Rationale == "") || (fact.Status == "unknown" && fact.Value != "") {
					return nil, fmt.Errorf("compliance: malformed activity context fact")
				}
				if fact.Status != "unknown" {
					if !valid {
						return nil, fmt.Errorf("compliance: context facts need canonical lines")
					}
					if err = verifyComplianceEvidence(complianceEvidence{RecordID: 1, LineStart: fact.LineStart, LineEnd: fact.LineEnd, Quote: fact.Quote}, map[int64]complianceDocument{1: {Lines: lines}}); err != nil {
						return nil, err
					}
				}
			}
			a.ContextFacts = facts
		}
		if a.Activity == "" || a.SourceText == "" {
			entry, _ := json.Marshal(m)
			if len(entry) > 300 {
				entry = entry[:300]
			}
			return nil, fmt.Errorf("compliance model response: activity %d needs activity and source_text (chunk lines %d-%d, entry %s)", i, input.startLine, input.endLine, entry)
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
				return nil, err
			}
			if len(list) > 10 {
				return nil, fmt.Errorf("compliance: too many cited documents")
			}
			a.CitedDocs = list
		}
		if len(a.Queries) == 0 {
			a.Queries = []complianceQuery{{Text: a.SourceText}}
		}
		out = append(out, a)
	}
	return out, nil
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

// complianceQuery is one knowledge-base search for an activity. The model gives it as bilingual
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

type complianceCandidate struct {
	artifactType string
	artifactID   string
	recordID     int64
	title        string
	snippet      string
	rank         int
	cosine       *float64
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

// ground searches the knowledge base for one activity and builds its report row.
func (r *complianceReviewer) ground(ctx context.Context, recordID int64, a complianceActivity, input chunkInput) (ReviewFinding, error) {
	exclude, err := r.excludeIDs(ctx, recordID)
	if err != nil {
		return ReviewFinding{}, err
	}
	seen := map[string]bool{}
	var cands []complianceCandidate
	var traces []complianceRetrievalTrace
	queries := append([]complianceQuery(nil), a.Queries...)
	for _, citation := range a.CitedDocs {
		queries = append(queries, complianceQuery{Text: citation})
	}
	for _, q := range queries {
		for _, artifactType := range []string{"provision", "metric", "chunk"} {
			floor := r.minCosine
			if r.assessmentPrompts != nil {
				floor = 0
			}
			results, err := r.searchCandidates(ctx, searchengine.Request{
				Query:    searchengine.Query{Text: q.Text},
				Criteria: searchengine.Criteria{Keywords: q.Keywords, MinCosine: floor},
				Filters: searchengine.Filters{
					ArtifactTypes:    []string{artifactType},
					ExcludeRecordIDs: exclude,
				},
				Options: searchengine.Options{Limit: complianceSearchLimit},
			})
			if err != nil {
				return ReviewFinding{}, fmt.Errorf("compliance search %q: %w", q.Text, err)
			}
			traces = append(traces, complianceRetrievalTrace{Query: q.Text, ArtifactType: artifactType, Limit: complianceSearchLimit, PotentiallyTruncated: len(results) >= complianceSearchLimit, Results: results})
			for _, res := range results {
				key := res.ArtifactType + "/" + res.ArtifactID
				if seen[key] {
					continue
				}
				seen[key] = true
				title := res.SourceTitle
				if title == "" {
					title = res.SourceFilename
				}
				cands = append(cands, complianceCandidate{res.ArtifactType, res.ArtifactID, res.InputRecordID, title, res.Snippet, len(cands) + 1, res.Diag.Cosine})
			}
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
	f.ArtifactFields, _ = json.Marshal(map[string]any{"activity": a, "assessment_date": r.assessmentDate, "retrieval_trace": traces, "searched_artifact_types": []string{"provision", "metric", "chunk"}, "authority_discovery_uses_cosine_floor": r.assessmentPrompts == nil && r.minCosine > 0})
	if len(cands) == 0 {
		f.FindingType = "compliance_no_grounding"
		f.Title = "Activity with no grounding candidates found: " + label
		f.Description = "A completed search of the knowledge base returned no candidate artifact for this activity. " +
			"This does not mean the activity is unregulated."
		return f, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Activity: %s\n\n", label)
	fmt.Fprintf(&b, "Retrieved %d candidate artifact(s) from the knowledge base that may govern this activity. "+
		"Candidates are search results; applicability and requirement comparison are not assessed.\n", len(cands))
	for _, c := range cands {
		cos := ""
		if c.cosine != nil {
			cos = fmt.Sprintf(" (cosine %.3f)", *c.cosine)
		}
		fmt.Fprintf(&b, "- [%s %s]%s record %d %s: %s\n", c.artifactType, c.artifactID, cos, c.recordID, c.title, strings.TrimSpace(c.snippet))
	}
	f.FindingType = "compliance_grounding_candidates"
	f.Title = "Grounding candidates found: " + label
	f.Description = b.String()
	f.RelatedArtifactID = cands[0].artifactID
	f.RelatedRecordID = cands[0].recordID
	refs := make([]complianceArtifact, 0, len(cands))
	for _, c := range cands {
		refs = append(refs, complianceArtifact{Type: c.artifactType, ID: c.artifactID, RecordID: c.recordID})
	}
	f.RelatedArtifactFields, _ = json.Marshal(refs)
	return f, nil
}
