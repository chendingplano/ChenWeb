package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// Read side of compliance review storage (openspec compliance-review-storage): summary
// pages, details on demand, candidate evidence, search history and the audit export. No
// function here calls a model.

const (
	complianceSummaryPageDefault   = 25
	complianceActivityPageDefault  = 50
	complianceCandidatePageDefault = 25
	compliancePageMax              = 100
)

// complianceSeverityRank is the API's stable severity order, as SQL.
const complianceSeverityRank = `CASE severity WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END`

// ComplianceCounts are a normalized compliance finding's stored counters.
type ComplianceCounts struct {
	Activities    int            `json:"activities"`
	SearchResults int            `json:"search_results"`
	Selected      int            `json:"selected"`
	Relevant      int            `json:"relevant"`
	Judged        int            `json:"judged"`
	Verdicts      map[string]int `json:"verdicts"`
}

// ErrComplianceNotFound is returned when a finding, candidate or search does not exist.
var ErrComplianceNotFound = errors.New("not found")

// ErrComplianceBadRequest marks an invalid cursor, limit or filter.
var ErrComplianceBadRequest = errors.New("bad request")

func complianceLimit(limit, def int) int {
	if limit <= 0 {
		return def
	}
	return min(limit, compliancePageMax)
}

// parsePairCursor reads an "a:b" cursor of two integers; "" is the first page.
func parsePairCursor(cursor string) (int64, int64, bool, error) {
	if strings.TrimSpace(cursor) == "" {
		return 0, 0, false, nil
	}
	a, b, ok := strings.Cut(cursor, ":")
	x, err1 := strconv.ParseInt(a, 10, 64)
	y, err2 := strconv.ParseInt(b, 10, 64)
	if !ok || err1 != nil || err2 != nil {
		return 0, 0, false, fmt.Errorf("%w: cursor %q is not <number>:<number>", ErrComplianceBadRequest, cursor)
	}
	return x, y, true, nil
}

// parseLineRange reads "469-471" or "469" into a line range (0, 0 when unreadable).
func parseLineRange(location string) (int, int) {
	a, b, found := strings.Cut(strings.TrimSpace(location), "-")
	start, err := strconv.Atoi(strings.TrimSpace(a))
	if err != nil {
		return 0, 0
	}
	if !found {
		return start, start
	}
	end, err := strconv.Atoi(strings.TrimSpace(b))
	if err != nil || end < start {
		return start, start
	}
	return start, end
}

// FindingSummaryPage is one page of compliance finding summaries.
type FindingSummaryPage struct {
	Items      []FindingItem `json:"items"`
	Total      int           `json:"total"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

// ListRunFindingSummaries returns a page of summaries of one run's findings of an aspect
// (compliance), in the API's severity order with ID as tie-breaker. excludeStatuses hides
// review statuses (the report view hides deleted findings).
func (c *DocReviewController) ListRunFindingSummaries(ctx context.Context, runID int64, aspect string, excludeStatuses []string, cursor string, limit int) (*FindingSummaryPage, error) {
	if strings.TrimSpace(aspect) == "" {
		return nil, fmt.Errorf("%w: aspect is required", ErrComplianceBadRequest)
	}
	rank, afterID, hasCursor, err := parsePairCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = complianceLimit(limit, complianceSummaryPageDefault)
	if excludeStatuses == nil {
		excludeStatuses = []string{}
	}
	page := &FindingSummaryPage{Items: []FindingItem{}}
	if err := c.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM kb.doc_review_findings
		WHERE run_id = $1 AND aspect = $2 AND NOT (COALESCE(review_status, 'pending') = ANY($3))`,
		runID, aspect, stringArray(excludeStatuses)).Scan(&page.Total); err != nil {
		return nil, fmt.Errorf("count findings of run %d: %w", runID, err)
	}
	rows, err := c.DB.QueryContext(ctx, `SELECT id, pass, aspect, severity, finding_type, title, COALESCE(location, ''),
			COALESCE(line_start, 0), COALESCE(line_end, 0), COALESCE(review_status, 'pending'), COALESCE(proc_status, ''),
			COALESCE(confidence, 0), COALESCE(artifact_id, ''), storage_version,
			activity_count, search_result_count, selected_candidate_count, relevant_candidate_count, judged_candidate_count,
			COALESCE(verdict_counts, '{}'::jsonb)::text, `+complianceSeverityRank+`
		FROM kb.doc_review_findings
		WHERE run_id = $1 AND aspect = $2 AND NOT (COALESCE(review_status, 'pending') = ANY($3))
		  AND (NOT $4 OR (`+complianceSeverityRank+`, id) > ($5, $6))
		ORDER BY `+complianceSeverityRank+`, id
		LIMIT $7`, runID, aspect, stringArray(excludeStatuses), hasCursor, rank, afterID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("list findings of run %d: %w", runID, err)
	}
	defer rows.Close()
	var lastRank int64
	for rows.Next() {
		var f FindingItem
		var acts, results, selected, relevant, judged sql.NullInt64
		var verdicts string
		var sevRank int64
		if err := rows.Scan(&f.ID, &f.Pass, &f.Aspect, &f.Severity, &f.FindingType, &f.Title, &f.Location,
			&f.LineStart, &f.LineEnd, &f.ReviewStatus, &f.ProcStatus, &f.Confidence, &f.ArtifactID, &f.StorageVersion,
			&acts, &results, &selected, &relevant, &judged, &verdicts, &sevRank); err != nil {
			return nil, fmt.Errorf("scan finding summary: %w", err)
		}
		if f.LineStart == 0 {
			f.LineStart, f.LineEnd = parseLineRange(f.Location)
		}
		f.Counts = complianceCountsOf(f.StorageVersion, acts, results, selected, relevant, judged, verdicts)
		if len(page.Items) == limit {
			page.NextCursor = fmt.Sprintf("%d:%d", lastRank, page.Items[len(page.Items)-1].ID)
			break
		}
		lastRank = sevRank
		page.Items = append(page.Items, f)
	}
	return page, rows.Err()
}

// complianceCountsOf reads a normalized finding's stored counters (nil for a legacy finding).
func complianceCountsOf(storage int, acts, results, selected, relevant, judged sql.NullInt64, verdicts string) *ComplianceCounts {
	if storage != findingStorageNormalized || !acts.Valid {
		return nil
	}
	c := &ComplianceCounts{Activities: int(acts.Int64), SearchResults: int(results.Int64), Selected: int(selected.Int64),
		Relevant: int(relevant.Int64), Judged: int(judged.Int64), Verdicts: map[string]int{}}
	_ = json.Unmarshal([]byte(verdicts), &c.Verdicts)
	return c
}

// FindingContent is a finding's full prose and execution details.
type FindingContent struct {
	Description       string            `json:"description"`
	Evidence          string            `json:"evidence"`
	Suggestion        string            `json:"suggestion"`
	ErrorMessage      string            `json:"error_message,omitempty"`
	CanonicalLanguage string            `json:"canonical_language,omitempty"`
	CanonicalOrigin   string            `json:"canonical_origin,omitempty"`
	SourceLanguage    string            `json:"source_language,omitempty"`
	StageModels       map[string]string `json:"stage_models,omitempty"`
	Extra             map[string]any    `json:"extra,omitempty"`
	// Language is the translation applied to the content, empty for the canonical language.
	Language string `json:"language,omitempty"`
}

// ComplianceRunContext is the configuration a compliance review run used.
type ComplianceRunContext struct {
	AssessmentDate string            `json:"assessment_date,omitempty"`
	SchemaVersion  int               `json:"schema_version"`
	Prompts        map[string]string `json:"prompts,omitempty"`
	Models         map[string]string `json:"models,omitempty"`
	Settings       map[string]any    `json:"settings,omitempty"`
	ReviewFlags    []string          `json:"review_flags,omitempty"`
}

// ComplianceActivitySummary is one activity as a list shows it.
type ComplianceActivitySummary struct {
	ActivityID         int64  `json:"activity_id"`
	LocalLabel         string `json:"local_label"`
	DisplayOrder       int    `json:"display_order"`
	Kind               string `json:"kind"`
	Label              string `json:"label"`
	LineStart          int    `json:"line_start,omitempty"`
	LineEnd            int    `json:"line_end,omitempty"`
	SourceArtifactType string `json:"source_artifact_type,omitempty"`
	SourceArtifactID   string `json:"source_artifact_id,omitempty"`
	DocumentLevel      bool   `json:"document_level,omitempty"`
}

// ComplianceCandidateSummary is one candidate assessment as a list shows it.
type ComplianceCandidateSummary struct {
	ID                int64  `json:"id"`
	ActivityID        int64  `json:"activity_id"`
	ActivityLabel     string `json:"activity_label"`
	LocalLabel        string `json:"local_label"`
	CandidateType     string `json:"candidate_type"`
	CandidateID       string `json:"candidate_id"`
	CandidateRecordID int64  `json:"candidate_record_id,omitempty"`
	DocumentTitle     string `json:"document_title,omitempty"`
	RelevanceAnswer   string `json:"relevance_answer,omitempty"`
	RelevanceScore    *int   `json:"relevance_score,omitempty"`
	Kept              bool   `json:"kept"`
	Status            string `json:"status"`
	Verdict           string `json:"verdict,omitempty"`
	Binding           string `json:"binding,omitempty"`
	Summary           string `json:"summary,omitempty"`
	Note              string `json:"note,omitempty"`
	DisplayOrder      int    `json:"display_order"`
}

// ComplianceCandidateDetail adds what an expanded candidate shows.
type ComplianceCandidateDetail struct {
	ComplianceCandidateSummary
	FindingID      int64           `json:"finding_id"`
	Cosine         *float64        `json:"cosine,omitempty"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	JudgementOrder *int            `json:"judgement_order,omitempty"`
	Evidence       json.RawMessage `json:"evidence"`
}

// ComplianceActivityPage and ComplianceCandidatePage are cursor pages.
type ComplianceActivityPage struct {
	Items      []ComplianceActivitySummary `json:"items"`
	NextCursor string                      `json:"next_cursor,omitempty"`
}

type ComplianceCandidatePage struct {
	Items      []ComplianceCandidateSummary `json:"items"`
	Total      int                          `json:"total"`
	NextCursor string                       `json:"next_cursor,omitempty"`
}

// LegacyFindingContent is what the details of a finding that was not migrated show.
type LegacyFindingContent struct {
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
	Suggestion  string `json:"suggestion"`
	ErrorMsg    string `json:"error_msg,omitempty"`
}

// FindingDetails is what selecting a finding loads.
type FindingDetails struct {
	Finding    FindingItem              `json:"finding"`
	Content    *FindingContent          `json:"content,omitempty"`
	RunContext *ComplianceRunContext    `json:"run_context,omitempty"`
	Activities *ComplianceActivityPage  `json:"activities,omitempty"`
	Candidates *ComplianceCandidatePage `json:"candidates,omitempty"`
	Legacy     *LegacyFindingContent    `json:"legacy,omitempty"`
}

// FindingDetailsOptions sizes the first activity and candidate pages.
type FindingDetailsOptions struct {
	ActivityLimit   int
	CandidateLimit  int
	CandidateFilter string
	Language        string
}

type findingHead struct {
	item     FindingItem
	runID    int64
	errorMsg string
	evidence string
	desc     string
	sugg     string
}

// loadFindingHead reads a finding's summary columns, plus its legacy content columns.
func (c *DocReviewController) loadFindingHead(ctx context.Context, id int64) (*findingHead, error) {
	h := &findingHead{}
	f := &h.item
	var acts, results, selected, relevant, judged sql.NullInt64
	var verdicts string
	err := c.DB.QueryRowContext(ctx, `SELECT id, run_id, pass, aspect, severity, finding_type, title, COALESCE(location, ''),
			COALESCE(line_start, 0), COALESCE(line_end, 0), COALESCE(review_status, 'pending'), COALESCE(proc_status, ''),
			COALESCE(confidence, 0), COALESCE(artifact_id, ''), storage_version,
			COALESCE(description, ''), COALESCE(evidence, ''), COALESCE(suggestion, ''), COALESCE(error_msg, ''),
			activity_count, search_result_count, selected_candidate_count, relevant_candidate_count, judged_candidate_count,
			COALESCE(verdict_counts, '{}'::jsonb)::text
		FROM kb.doc_review_findings WHERE id = $1`, id).Scan(&f.ID, &h.runID, &f.Pass, &f.Aspect, &f.Severity, &f.FindingType,
		&f.Title, &f.Location, &f.LineStart, &f.LineEnd, &f.ReviewStatus, &f.ProcStatus, &f.Confidence, &f.ArtifactID,
		&f.StorageVersion, &h.desc, &h.evidence, &h.sugg, &h.errorMsg, &acts, &results, &selected, &relevant, &judged, &verdicts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("finding %d: %w", id, ErrComplianceNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("load finding %d: %w", id, err)
	}
	f.RunID = h.runID
	if f.LineStart == 0 {
		f.LineStart, f.LineEnd = parseLineRange(f.Location)
	}
	f.Counts = complianceCountsOf(f.StorageVersion, acts, results, selected, relevant, judged, verdicts)
	return h, nil
}

// GetFindingDetails returns a finding's content, run context and first activity and
// candidate pages. A finding that was not migrated returns its stored content instead.
func (c *DocReviewController) GetFindingDetails(ctx context.Context, id int64, opts FindingDetailsOptions) (*FindingDetails, error) {
	h, err := c.loadFindingHead(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &FindingDetails{Finding: h.item}
	if h.item.StorageVersion != findingStorageNormalized {
		out.Legacy = &LegacyFindingContent{Description: h.desc, Evidence: h.evidence, Suggestion: h.sugg, ErrorMsg: h.errorMsg}
		return out, nil
	}
	if out.Content, err = c.loadFindingContent(ctx, id, opts.Language); err != nil {
		return nil, err
	}
	if out.Content.Language != "" {
		if tr, err := c.findingTranslation(ctx, id, opts.Language); err == nil && tr != nil && tr.Title != "" {
			out.Finding.Title = tr.Title
		}
	}
	if out.RunContext, err = c.loadRunContext(ctx, h.runID); err != nil {
		return nil, err
	}
	if out.Activities, err = c.ListFindingActivities(ctx, id, "", opts.ActivityLimit); err != nil {
		return nil, err
	}
	if out.Candidates, err = c.ListFindingCandidates(ctx, id, opts.CandidateFilter, "", opts.CandidateLimit); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *DocReviewController) loadFindingContent(ctx context.Context, id int64, language string) (*FindingContent, error) {
	fc := &FindingContent{}
	var stage, extra string
	err := c.DB.QueryRowContext(ctx, `SELECT description, evidence, suggestion, error_message, canonical_language,
			canonical_origin, source_language, stage_models::text, extra::text
		FROM kb.doc_review_finding_details WHERE finding_id = $1`, id).Scan(&fc.Description, &fc.Evidence, &fc.Suggestion,
		&fc.ErrorMessage, &fc.CanonicalLanguage, &fc.CanonicalOrigin, &fc.SourceLanguage, &stage, &extra)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("details of finding %d: %w", id, ErrComplianceNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("load details of finding %d: %w", id, err)
	}
	_ = json.Unmarshal([]byte(stage), &fc.StageModels)
	_ = json.Unmarshal([]byte(extra), &fc.Extra)
	if len(fc.Extra) == 0 {
		fc.Extra = nil
	}
	if lang := strings.ToLower(strings.TrimSpace(language)); lang != "" && !strings.EqualFold(lang, fc.CanonicalLanguage) {
		tr, err := c.findingTranslation(ctx, id, lang)
		if err != nil {
			return nil, err
		}
		if tr != nil {
			fc.Language = lang
			if tr.Description != "" {
				fc.Description = tr.Description
			}
			if tr.Suggestion != "" {
				fc.Suggestion = tr.Suggestion
			}
		}
	}
	return fc, nil
}

// findingTranslation returns the stored translation of a finding into language, or nil.
func (c *DocReviewController) findingTranslation(ctx context.Context, id int64, language string) (*FindingTranslation, error) {
	for _, key := range translationMetadataKeys(language) {
		var tr FindingTranslation
		err := c.DB.QueryRowContext(ctx, `SELECT title, description, suggestion, provenance
			FROM kb.doc_review_finding_translations WHERE finding_id = $1 AND language = $2`, id, key).
			Scan(&tr.Title, &tr.Description, &tr.Suggestion, &tr.Provenance)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("load translation of finding %d: %w", id, err)
		}
		return &tr, nil
	}
	return nil, nil
}

func (c *DocReviewController) loadRunContext(ctx context.Context, runID int64) (*ComplianceRunContext, error) {
	rc := &ComplianceRunContext{}
	var prompts, models, flags string
	var settings sql.NullString
	err := c.DB.QueryRowContext(ctx, `SELECT assessment_date, schema_version, prompts::text, models::text, settings::text, review_flags::text
		FROM kb.doc_review_compliance_run_context WHERE run_id = $1`, runID).Scan(&rc.AssessmentDate, &rc.SchemaVersion, &prompts, &models, &settings, &flags)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load run context of run %d: %w", runID, err)
	}
	_ = json.Unmarshal([]byte(prompts), &rc.Prompts)
	_ = json.Unmarshal([]byte(models), &rc.Models)
	if settings.Valid {
		_ = json.Unmarshal([]byte(settings.String), &rc.Settings)
	}
	_ = json.Unmarshal([]byte(flags), &rc.ReviewFlags)
	return rc, nil
}

// ListFindingActivities returns a page of a finding's activities in display order.
func (c *DocReviewController) ListFindingActivities(ctx context.Context, findingID int64, cursor string, limit int) (*ComplianceActivityPage, error) {
	order, afterID, hasCursor, err := parsePairCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = complianceLimit(limit, complianceActivityPageDefault)
	rows, err := c.DB.QueryContext(ctx, `SELECT id, activity_id, local_label, display_order, kind, label, COALESCE(line_start, 0),
			COALESCE(line_end, 0), source_artifact_type, source_artifact_id, COALESCE((details->>'document_level')::boolean, false)
		FROM kb.doc_review_compliance_activities
		WHERE finding_id = $1 AND (NOT $2 OR (display_order, id) > ($3, $4))
		ORDER BY display_order, id LIMIT $5`, findingID, hasCursor, order, afterID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("list activities of finding %d: %w", findingID, err)
	}
	defer rows.Close()
	page := &ComplianceActivityPage{Items: []ComplianceActivitySummary{}}
	var lastID int64
	for rows.Next() {
		var a ComplianceActivitySummary
		var rowID int64
		if err := rows.Scan(&rowID, &a.ActivityID, &a.LocalLabel, &a.DisplayOrder, &a.Kind, &a.Label, &a.LineStart, &a.LineEnd,
			&a.SourceArtifactType, &a.SourceArtifactID, &a.DocumentLevel); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		if len(page.Items) == limit {
			last := page.Items[len(page.Items)-1]
			page.NextCursor = fmt.Sprintf("%d:%d", last.DisplayOrder, lastID)
			break
		}
		lastID = rowID
		page.Items = append(page.Items, a)
	}
	return page, rows.Err()
}

// complianceCandidateFilter turns a filter name into a condition on candidate rows c.
func complianceCandidateFilter(filter string) (string, error) {
	switch strings.TrimSpace(filter) {
	case "", "judged":
		return "c.status = 'judged'", nil
	case "relevant":
		return "c.kept", nil
	case "all":
		return "TRUE", nil
	}
	return "", fmt.Errorf("%w: filter %q is not judged, relevant or all", ErrComplianceBadRequest, filter)
}

const complianceCandidateColumns = `c.id, c.activity_id, a.label, a.local_label, c.candidate_type, c.candidate_id,
	COALESCE(c.candidate_record_id, 0), c.document_title, c.relevance_answer, c.relevance_score, c.kept, c.status,
	COALESCE(c.verdict, ''), COALESCE(c.binding, ''), c.summary, c.note, c.display_order`

func scanComplianceCandidate(scan func(...any) error, extra ...any) (ComplianceCandidateSummary, error) {
	var s ComplianceCandidateSummary
	var score sql.NullInt64
	dest := []any{&s.ID, &s.ActivityID, &s.ActivityLabel, &s.LocalLabel, &s.CandidateType, &s.CandidateID, &s.CandidateRecordID,
		&s.DocumentTitle, &s.RelevanceAnswer, &score, &s.Kept, &s.Status, &s.Verdict, &s.Binding, &s.Summary, &s.Note, &s.DisplayOrder}
	if err := scan(append(dest, extra...)...); err != nil {
		return s, err
	}
	if score.Valid {
		v := int(score.Int64)
		s.RelevanceScore = &v
	}
	return s, nil
}

// ListFindingCandidates returns a page of a finding's candidate assessments in display
// order: judged ones first, in verdict order.
func (c *DocReviewController) ListFindingCandidates(ctx context.Context, findingID int64, filter, cursor string, limit int) (*ComplianceCandidatePage, error) {
	cond, err := complianceCandidateFilter(filter)
	if err != nil {
		return nil, err
	}
	order, afterID, hasCursor, err := parsePairCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = complianceLimit(limit, complianceCandidatePageDefault)
	page := &ComplianceCandidatePage{Items: []ComplianceCandidateSummary{}}
	if err := c.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM kb.doc_review_compliance_candidates c
		JOIN kb.doc_review_compliance_activities a ON a.activity_id = c.activity_id
		WHERE a.finding_id = $1 AND `+cond, findingID).Scan(&page.Total); err != nil {
		return nil, fmt.Errorf("count candidates of finding %d: %w", findingID, err)
	}
	rows, err := c.DB.QueryContext(ctx, `SELECT `+complianceCandidateColumns+`
		FROM kb.doc_review_compliance_candidates c
		JOIN kb.doc_review_compliance_activities a ON a.activity_id = c.activity_id
		WHERE a.finding_id = $1 AND `+cond+` AND (NOT $2 OR (c.display_order, c.id) > ($3, $4))
		ORDER BY c.display_order, c.id LIMIT $5`, findingID, hasCursor, order, afterID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("list candidates of finding %d: %w", findingID, err)
	}
	defer rows.Close()
	for rows.Next() {
		s, err := scanComplianceCandidate(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan candidate: %w", err)
		}
		if len(page.Items) == limit {
			last := page.Items[len(page.Items)-1]
			page.NextCursor = fmt.Sprintf("%d:%d", last.DisplayOrder, last.ID)
			break
		}
		page.Items = append(page.Items, s)
	}
	return page, rows.Err()
}

// GetComplianceCandidate returns one candidate with its evidence.
func (c *DocReviewController) GetComplianceCandidate(ctx context.Context, candidateID int64) (*ComplianceCandidateDetail, error) {
	d := &ComplianceCandidateDetail{}
	var cosine sql.NullFloat64
	var judgementOrder sql.NullInt64
	var evidence string
	s, err := scanComplianceCandidate(c.DB.QueryRowContext(ctx, `SELECT `+complianceCandidateColumns+`, a.finding_id, c.cosine,
			c.error_message, c.judgement_order, c.evidence::text
		FROM kb.doc_review_compliance_candidates c
		JOIN kb.doc_review_compliance_activities a ON a.activity_id = c.activity_id
		WHERE c.id = $1`, candidateID).Scan, &d.FindingID, &cosine, &d.ErrorMessage, &judgementOrder, &evidence)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("candidate %d: %w", candidateID, ErrComplianceNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("load candidate %d: %w", candidateID, err)
	}
	d.ComplianceCandidateSummary = s
	if cosine.Valid {
		d.Cosine = &cosine.Float64
	}
	if judgementOrder.Valid {
		v := int(judgementOrder.Int64)
		d.JudgementOrder = &v
	}
	d.Evidence = json.RawMessage(evidence)
	return d, nil
}

// ComplianceSearchSummary is one search as search history shows it.
type ComplianceSearchSummary struct {
	ID                   int64           `json:"id"`
	ActivityID           int64           `json:"activity_id"`
	LocalLabel           string          `json:"local_label"`
	ActivityLabel        string          `json:"activity_label"`
	Ordinal              int             `json:"ordinal"`
	Purpose              string          `json:"purpose"`
	Query                string          `json:"query,omitempty"`
	Reference            string          `json:"reference,omitempty"`
	CitedDocument        string          `json:"cited_document,omitempty"`
	Keywords             json.RawMessage `json:"keywords"`
	Filters              json.RawMessage `json:"filters"`
	ResultLimit          int             `json:"result_limit"`
	PotentiallyTruncated bool            `json:"potentially_truncated"`
	ResultCount          int             `json:"result_count"`
}

// ListFindingSearches returns every search of a finding's activities, without results.
func (c *DocReviewController) ListFindingSearches(ctx context.Context, findingID int64) ([]ComplianceSearchSummary, error) {
	rows, err := c.DB.QueryContext(ctx, `SELECT s.id, s.activity_id, a.local_label, a.label, s.ordinal, s.purpose, s.query, s.reference,
			s.cited_document, s.keywords::text, s.filters::text, s.result_limit, s.potentially_truncated,
			(SELECT COUNT(*) FROM kb.doc_review_compliance_search_results r WHERE r.search_id = s.id)
		FROM kb.doc_review_compliance_searches s
		JOIN kb.doc_review_compliance_activities a ON a.activity_id = s.activity_id
		WHERE a.finding_id = $1
		ORDER BY a.display_order, s.ordinal`, findingID)
	if err != nil {
		return nil, fmt.Errorf("list searches of finding %d: %w", findingID, err)
	}
	defer rows.Close()
	out := []ComplianceSearchSummary{}
	for rows.Next() {
		var s ComplianceSearchSummary
		var kw, filters string
		if err := rows.Scan(&s.ID, &s.ActivityID, &s.LocalLabel, &s.ActivityLabel, &s.Ordinal, &s.Purpose, &s.Query, &s.Reference,
			&s.CitedDocument, &kw, &filters, &s.ResultLimit, &s.PotentiallyTruncated, &s.ResultCount); err != nil {
			return nil, fmt.Errorf("scan search: %w", err)
		}
		s.Keywords, s.Filters = json.RawMessage(kw), json.RawMessage(filters)
		out = append(out, s)
	}
	return out, rows.Err()
}

// ComplianceSearchResult is one ranked result of a search.
type ComplianceSearchResult struct {
	Rank              int             `json:"rank"`
	CandidateType     string          `json:"candidate_type"`
	CandidateID       string          `json:"candidate_id"`
	CandidateRecordID int64           `json:"candidate_record_id,omitempty"`
	DocumentTitle     string          `json:"document_title,omitempty"`
	Score             *float64        `json:"score,omitempty"`
	Diagnostics       json.RawMessage `json:"diagnostics"`
}

// ComplianceSearchResultPage is a page of one search's results by rank.
type ComplianceSearchResultPage struct {
	Items      []ComplianceSearchResult `json:"items"`
	NextCursor string                   `json:"next_cursor,omitempty"`
}

// ListSearchResults returns a page of one search's results after rank cursor.
func (c *DocReviewController) ListSearchResults(ctx context.Context, searchID int64, cursor string, limit int) (*ComplianceSearchResultPage, error) {
	after := 0
	if strings.TrimSpace(cursor) != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil {
			return nil, fmt.Errorf("%w: cursor %q is not a rank", ErrComplianceBadRequest, cursor)
		}
		after = n
	}
	limit = complianceLimit(limit, complianceCandidatePageDefault)
	var exists bool
	if err := c.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM kb.doc_review_compliance_searches WHERE id = $1)`, searchID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check search %d: %w", searchID, err)
	}
	if !exists {
		return nil, fmt.Errorf("search %d: %w", searchID, ErrComplianceNotFound)
	}
	rows, err := c.DB.QueryContext(ctx, `SELECT rank, candidate_type, candidate_id, COALESCE(candidate_record_id, 0), document_title,
			score, diagnostics::text
		FROM kb.doc_review_compliance_search_results WHERE search_id = $1 AND rank > $2 ORDER BY rank LIMIT $3`, searchID, after, limit+1)
	if err != nil {
		return nil, fmt.Errorf("list results of search %d: %w", searchID, err)
	}
	defer rows.Close()
	page := &ComplianceSearchResultPage{Items: []ComplianceSearchResult{}}
	for rows.Next() {
		var r ComplianceSearchResult
		var score sql.NullFloat64
		var diag string
		if err := rows.Scan(&r.Rank, &r.CandidateType, &r.CandidateID, &r.CandidateRecordID, &r.DocumentTitle, &score, &diag); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		if score.Valid {
			r.Score = &score.Float64
		}
		r.Diagnostics = json.RawMessage(diag)
		if len(page.Items) == limit {
			page.NextCursor = strconv.Itoa(page.Items[len(page.Items)-1].Rank)
			break
		}
		page.Items = append(page.Items, r)
	}
	return page, rows.Err()
}

func stringArray(v []string) any { return pq.Array(v) }

// ExportFinding assembles a finding's complete audit record on the server: summary,
// details, run context, activities with their searches, results and candidates, and
// translations. A finding that was not migrated exports its stored columns and metadata.
func (c *DocReviewController) ExportFinding(ctx context.Context, id int64) (map[string]any, error) {
	h, err := c.loadFindingHead(ctx, id)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"finding": h.item, "storage_version": h.item.StorageVersion}
	if h.item.StorageVersion != findingStorageNormalized {
		var metadata, cited string
		if err := c.DB.QueryRowContext(ctx, `SELECT COALESCE(metadata, '{}'::jsonb)::text, COALESCE(cited_documents, 'null'::jsonb)::text
			FROM kb.doc_review_findings WHERE id = $1`, id).Scan(&metadata, &cited); err != nil {
			return nil, fmt.Errorf("load legacy payload of finding %d: %w", id, err)
		}
		out["description"], out["evidence"], out["suggestion"], out["error_msg"] = h.desc, h.evidence, h.sugg, h.errorMsg
		out["metadata"], out["cited_documents"] = json.RawMessage(metadata), json.RawMessage(cited)
		return out, nil
	}
	if out["content"], err = c.loadFindingContent(ctx, id, ""); err != nil {
		return nil, err
	}
	if out["run_context"], err = c.loadRunContext(ctx, h.runID); err != nil {
		return nil, err
	}
	activities, err := queryJSONRows(ctx, c.DB, `SELECT activity_id, local_label, display_order, kind, label, line_start, line_end,
			source_artifact_type, source_artifact_id, source_row_id, source_text, details
		FROM kb.doc_review_compliance_activities WHERE finding_id = $1 ORDER BY display_order, id`, id)
	if err != nil {
		return nil, err
	}
	searches, err := queryJSONRows(ctx, c.DB, `SELECT s.id, s.activity_id, s.ordinal, s.purpose, s.query, s.reference, s.cited_document,
			s.keywords, s.filters, s.result_limit, s.potentially_truncated,
			COALESCE((SELECT jsonb_agg(jsonb_build_object('rank', r.rank, 'candidate_type', r.candidate_type, 'candidate_id', r.candidate_id,
				'candidate_record_id', r.candidate_record_id, 'document_title', r.document_title, 'score', r.score, 'diagnostics', r.diagnostics)
				ORDER BY r.rank) FROM kb.doc_review_compliance_search_results r WHERE r.search_id = s.id), '[]'::jsonb) AS results
		FROM kb.doc_review_compliance_searches s
		JOIN kb.doc_review_compliance_activities a ON a.activity_id = s.activity_id
		WHERE a.finding_id = $1 ORDER BY s.activity_id, s.ordinal`, id)
	if err != nil {
		return nil, err
	}
	candidates, err := queryJSONRows(ctx, c.DB, `SELECT c.id, c.activity_id, c.candidate_type, c.candidate_id, c.candidate_record_id,
			c.document_title, c.cosine, c.relevance_answer, c.relevance_score, c.kept, c.status, c.verdict, c.binding, c.summary,
			c.note, c.error_message, c.evidence, c.display_order, c.judgement_order
		FROM kb.doc_review_compliance_candidates c
		JOIN kb.doc_review_compliance_activities a ON a.activity_id = c.activity_id
		WHERE a.finding_id = $1 ORDER BY c.display_order, c.id`, id)
	if err != nil {
		return nil, err
	}
	byActivity := map[float64]map[string]any{}
	for _, a := range activities {
		a["searches"], a["candidates"] = []map[string]any{}, []map[string]any{}
		byActivity[a["activity_id"].(float64)] = a
	}
	for _, s := range searches {
		if a := byActivity[s["activity_id"].(float64)]; a != nil {
			a["searches"] = append(a["searches"].([]map[string]any), s)
		}
	}
	for _, cand := range candidates {
		if a := byActivity[cand["activity_id"].(float64)]; a != nil {
			a["candidates"] = append(a["candidates"].([]map[string]any), cand)
		}
	}
	out["activities"] = activities
	if out["translations"], err = queryJSONRows(ctx, c.DB, `SELECT language, title, description, suggestion, provenance
		FROM kb.doc_review_finding_translations WHERE finding_id = $1 ORDER BY language`, id); err != nil {
		return nil, err
	}
	return out, nil
}

// queryJSONRows runs a query and returns each row as a JSON object (numbers as float64).
func queryJSONRows(ctx context.Context, db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, `SELECT row_to_json(q)::text FROM (`+query+`) q`, args...)
	if err != nil {
		return nil, fmt.Errorf("export query: %w", err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(text), &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
