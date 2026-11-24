package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/idmanager"
)

// Compliance review storage (openspec compliance-review-storage; KnowledgeStore design
// 2026112301). A normalized compliance finding keeps a small summary in
// kb.doc_review_findings (storage_version 2) and stores its content, activities, candidate
// assessments, searches and search results in their own tables, written in one transaction.

const (
	findingStorageLegacy     = 1
	findingStorageNormalized = 2

	// complianceActivitySequence is the shared ID manager sequence of activity IDs.
	complianceActivitySequence = "compliance_activity_id"
	// complianceInsertBatch caps the rows of one multi-row INSERT.
	complianceInsertBatch = 400
)

// Candidate assessment statuses. Only a judged candidate carries a verdict and binding.
const (
	candidateRejected        = "rejected"
	candidateRelevanceFailed = "relevance_failed"
	candidatePending         = "pending"
	candidateJudgementFailed = "judgement_failed"
	candidateJudged          = "judged"
)

// complianceRecord is what a normalized compliance finding stores beyond its summary.
type complianceRecord struct {
	Run        *complianceRunContext
	Activities []complianceActivityRecord
	// StageModels names the model of each stage's last call in this finding (relevance,
	// judgement, verify). Configured models belong to the run context.
	StageModels map[string]string
	// Extra holds bounded details of findings without activities (the unchecked notice).
	Extra map[string]any
	// Languages is the recorded language bookkeeping of a migrated finding; nil derives it
	// from ReviewFinding.Language.
	Languages *complianceLanguages
}

// complianceLanguages is a finding's canonical and source language bookkeeping.
type complianceLanguages struct {
	Canonical, Origin, Source string
	SourceConfidence          float64
}

// complianceRunContext is the configuration shared by every compliance finding of a run.
type complianceRunContext struct {
	AssessmentDate string
	SchemaVersion  int
	Prompts        map[string]string
	Models         map[string]string // primary, correction, fallback
	Settings       map[string]any    // nil when unknown (migrated runs)
	ReviewFlags    []string
}

// complianceActivityRecord is one activity occurrence with its searches and candidates.
type complianceActivityRecord struct {
	LocalLabel  string
	Kind        string
	Label       string
	LineStart   int
	LineEnd     int
	SourceType  string // metric or provision; empty for a text activity
	SourceID    string
	SourceRowID int64
	SourceText  string
	Details     map[string]any
	Searches    []complianceSearchRecord
	Candidates  []complianceCandidateRecord
}

// complianceSearchRecord is one search made for an activity and what it returned.
type complianceSearchRecord struct {
	Purpose              string
	Query                string
	Reference            string
	CitedDocument        string
	Keywords             []string
	Filters              map[string]any
	Limit                int
	PotentiallyTruncated bool
	Results              []complianceTraceResult
}

// complianceCandidateRecord is one activity–candidate pair sent to the relevance check.
type complianceCandidateRecord struct {
	Type            string
	ID              string
	RecordID        int64
	Title           string
	Cosine          *float64
	RelevanceAnswer string
	RelevanceScore  *int
	Kept            bool
	Status          string
	Verdict         string
	Binding         string
	Summary         string
	Note            string
	Error           string
	Evidence        map[string]any
	DisplayOrder    int
	JudgementOrder  *int
}

// complianceCounts are the counters stored with a normalized finding's summary. They count
// activity–candidate pairs: a candidate judged for two activities counts twice.
type complianceCounts struct {
	Activities    int
	SearchResults int
	Selected      int
	Relevant      int
	Judged        int
	Verdicts      map[string]int
}

func (r *complianceRecord) counts() complianceCounts {
	c := complianceCounts{Activities: len(r.Activities), Verdicts: map[string]int{}}
	for _, a := range r.Activities {
		for _, s := range a.Searches {
			c.SearchResults += len(s.Results)
		}
		for _, cand := range a.Candidates {
			c.Selected++
			if cand.Kept {
				c.Relevant++
			}
			if cand.Status == candidateJudged {
				c.Judged++
				c.Verdicts[cand.Verdict]++
			}
		}
	}
	return c
}

// complianceMetadataMarkers is the whole metadata of a normalized compliance finding.
func complianceMetadataMarkers(resultKind string) []byte {
	m := map[string]any{"schema_version": 1, "storage_version": findingStorageNormalized}
	if resultKind != "" {
		m["result_kind"] = resultKind
	}
	b, _ := json.Marshal(m)
	return b
}

// complianceActivityIDs allocates n consecutive activity IDs and returns the first.
type complianceActivityIDs func(ctx context.Context, n int64) (int64, error)

var (
	activityIDManagerMu sync.Mutex
	activityIDManager   *idmanager.Manager
)

// defaultComplianceActivityIDs allocates from the shared ID manager, provisioning the
// sequence on first use in the process.
func defaultComplianceActivityIDs(db *sql.DB) complianceActivityIDs {
	return func(ctx context.Context, n int64) (int64, error) {
		m, err := complianceIDManager(ctx, db)
		if err != nil {
			return 0, err
		}
		r, err := m.Generate(ctx, complianceActivitySequence, n)
		if err != nil {
			return 0, fmt.Errorf("allocate %d compliance activity IDs: %w", n, err)
		}
		return r.FirstID, nil
	}
}

func complianceIDManager(ctx context.Context, db *sql.DB) (*idmanager.Manager, error) {
	activityIDManagerMu.Lock()
	defer activityIDManagerMu.Unlock()
	if activityIDManager != nil {
		return activityIDManager, nil
	}
	if db == nil {
		db = ApiTypes.SharedDBHandle
	}
	if db == nil {
		return nil, fmt.Errorf("compliance activity IDs: shared database is unavailable")
	}
	m, err := idmanager.New(db, "chenweb-compliance-review")
	if err != nil {
		return nil, fmt.Errorf("compliance activity IDs: %w", err)
	}
	if err := m.Provision(ctx, idmanager.Sequence{Name: complianceActivitySequence,
		Desc: "Compliance Review activity IDs (kb.doc_review_compliance_activities.activity_id)", AllocateSize: 100}); err != nil {
		return nil, fmt.Errorf("provision %s: %w", complianceActivitySequence, err)
	}
	activityIDManager = m
	return m, nil
}

// complianceTx is what the writer needs of a transaction (a field so tests can use sqlmock).
type complianceTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// upsertComplianceRunContext stores a run's context before its findings; the findings of the
// run share it. A run reviewed again (a restarted crashed run) replaces it.
func upsertComplianceRunContext(ctx context.Context, db complianceTx, runID int64, rc *complianceRunContext) error {
	if rc == nil {
		return nil
	}
	var settings any
	if rc.Settings != nil {
		settings = jsonText(rc.Settings)
	}
	flags := rc.ReviewFlags
	if flags == nil {
		flags = []string{}
	}
	_, err := db.ExecContext(ctx, `INSERT INTO kb.doc_review_compliance_run_context
		(run_id, assessment_date, schema_version, prompts, models, settings, review_flags)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (run_id) DO UPDATE SET assessment_date = EXCLUDED.assessment_date,
			schema_version = EXCLUDED.schema_version, prompts = EXCLUDED.prompts, models = EXCLUDED.models,
			settings = EXCLUDED.settings, review_flags = EXCLUDED.review_flags`,
		runID, rc.AssessmentDate, max(rc.SchemaVersion, 1), jsonText(nonNilMap(rc.Prompts)), jsonText(nonNilMap(rc.Models)), settings, jsonText(flags))
	if err != nil {
		return fmt.Errorf("save compliance run context of run %d: %w", runID, err)
	}
	return nil
}

// insertComplianceFinding writes one normalized finding: its summary row, details,
// activities, searches, results and candidates. The caller owns the transaction.
// firstActivityID is the first of len(rec.Activities) IDs reserved for it.
func insertComplianceFinding(ctx context.Context, tx complianceTx, recordID, runID int64, f ReviewFinding, firstActivityID int64) (int64, error) {
	rec := f.Compliance
	c := rec.counts()
	confidence := f.Confidence
	if confidence <= 0 {
		confidence = 0.5
	}
	var artifactID, location any
	if f.ArtifactID != "" {
		artifactID = f.ArtifactID
	}
	if f.Location != "" {
		location = f.Location
	}
	var procStatus any
	if f.ProcStatus != "" {
		procStatus = f.ProcStatus
	}
	var findingID int64
	err := tx.QueryRowContext(ctx, `INSERT INTO kb.doc_review_findings
		(input_record_id, run_id, pass, aspect, severity, finding_type, title, description, location,
		 confidence, metadata, artifact_id, proc_status, storage_version, line_start, line_end,
		 activity_count, search_result_count, selected_candidate_count, relevant_candidate_count,
		 judged_candidate_count, verdict_counts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, '', $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		RETURNING id`,
		recordID, runID, f.Pass, f.Aspect, f.Severity, f.FindingType, f.Title, location,
		confidence, string(complianceMetadataMarkers(f.ResultKind)), artifactID, procStatus, findingStorageNormalized,
		nullInt(f.LineStart), nullInt(f.LineEnd), c.Activities, c.SearchResults, c.Selected, c.Relevant, c.Judged, jsonText(c.Verdicts),
	).Scan(&findingID)
	if err != nil {
		return 0, fmt.Errorf("insert compliance finding: %w", err)
	}
	if err := insertComplianceDetails(ctx, tx, findingID, f); err != nil {
		return 0, err
	}
	for i := range rec.Activities {
		if err := insertComplianceActivity(ctx, tx, findingID, i, firstActivityID+int64(i), &rec.Activities[i]); err != nil {
			return 0, fmt.Errorf("finding %d activity %s: %w", findingID, rec.Activities[i].LocalLabel, err)
		}
	}
	return findingID, nil
}

func insertComplianceDetails(ctx context.Context, tx complianceTx, findingID int64, f ReviewFinding) error {
	lang := complianceLanguages{Canonical: normalizeReviewFindingLanguage(f.Language), Origin: "original"}
	lang.Source = lang.Canonical
	if strings.TrimSpace(f.Language) != "" {
		lang.SourceConfidence = 1.0
	}
	if f.Compliance.Languages != nil {
		lang = *f.Compliance.Languages
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO kb.doc_review_finding_details
		(finding_id, description, evidence, suggestion, canonical_language, canonical_origin,
		 source_language, source_language_confidence, error_message, stage_models, extra)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		findingID, strings.TrimSpace(f.Description), f.Evidence, strings.TrimSpace(f.Suggestion), lang.Canonical, lang.Origin,
		lang.Source, lang.SourceConfidence, f.ErrorMsg, jsonText(nonNilMap(f.Compliance.StageModels)), jsonText(nonNilAnyMap(f.Compliance.Extra)))
	if err != nil {
		return fmt.Errorf("insert details of finding %d: %w", findingID, err)
	}
	return nil
}

// insertComplianceActivity writes one activity and its children. A retried insert of the
// same finding and local label keeps the activity ID it already has and replaces the
// children.
func insertComplianceActivity(ctx context.Context, tx complianceTx, findingID int64, order int, newID int64, a *complianceActivityRecord) error {
	var activityID int64
	err := tx.QueryRowContext(ctx, `INSERT INTO kb.doc_review_compliance_activities
		(activity_id, finding_id, local_label, display_order, kind, label, line_start, line_end,
		 source_artifact_type, source_artifact_id, source_row_id, source_text, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (finding_id, local_label) DO UPDATE SET display_order = EXCLUDED.display_order, kind = EXCLUDED.kind,
			label = EXCLUDED.label, line_start = EXCLUDED.line_start, line_end = EXCLUDED.line_end,
			source_artifact_type = EXCLUDED.source_artifact_type, source_artifact_id = EXCLUDED.source_artifact_id,
			source_row_id = EXCLUDED.source_row_id, source_text = EXCLUDED.source_text, details = EXCLUDED.details
		RETURNING activity_id`,
		newID, findingID, a.LocalLabel, order, a.Kind, a.Label, nullInt(a.LineStart), nullInt(a.LineEnd),
		a.SourceType, a.SourceID, nullInt64(a.SourceRowID), a.SourceText, jsonText(nonNilAnyMap(a.Details)),
	).Scan(&activityID)
	if err != nil {
		return fmt.Errorf("insert activity: %w", err)
	}
	if activityID != newID {
		for _, q := range []string{`DELETE FROM kb.doc_review_compliance_searches WHERE activity_id = $1`,
			`DELETE FROM kb.doc_review_compliance_candidates WHERE activity_id = $1`} {
			if _, err := tx.ExecContext(ctx, q, activityID); err != nil {
				return fmt.Errorf("clear children of activity %d: %w", activityID, err)
			}
		}
	}
	for i := range a.Searches {
		if err := insertComplianceSearch(ctx, tx, activityID, i+1, &a.Searches[i]); err != nil {
			return err
		}
	}
	rows := make([][]any, 0, len(a.Candidates))
	for _, cand := range a.Candidates {
		var verdict, binding any
		if cand.Status == candidateJudged {
			verdict, binding = cand.Verdict, cand.Binding
		}
		var score any
		if cand.RelevanceScore != nil {
			score = *cand.RelevanceScore
		}
		var judgementOrder any
		if cand.JudgementOrder != nil {
			judgementOrder = *cand.JudgementOrder
		}
		rows = append(rows, []any{activityID, cand.Type, cand.ID, nullInt64(cand.RecordID), cand.Title, nullFloat(cand.Cosine),
			cand.RelevanceAnswer, score, cand.Kept, cand.Status, verdict, binding, cand.Summary, cand.Note, cand.Error,
			jsonText(nonNilAnyMap(cand.Evidence)), cand.DisplayOrder, judgementOrder})
	}
	return insertComplianceRows(ctx, tx, "kb.doc_review_compliance_candidates",
		[]string{"activity_id", "candidate_type", "candidate_id", "candidate_record_id", "document_title", "cosine",
			"relevance_answer", "relevance_score", "kept", "status", "verdict", "binding", "summary", "note", "error_message",
			"evidence", "display_order", "judgement_order"}, rows)
}

func insertComplianceSearch(ctx context.Context, tx complianceTx, activityID int64, ordinal int, s *complianceSearchRecord) error {
	keywords := s.Keywords
	if keywords == nil {
		keywords = []string{}
	}
	var searchID int64
	err := tx.QueryRowContext(ctx, `INSERT INTO kb.doc_review_compliance_searches
		(activity_id, ordinal, purpose, query, reference, cited_document, keywords, filters, result_limit, potentially_truncated)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		activityID, ordinal, s.Purpose, s.Query, s.Reference, s.CitedDocument, jsonText(keywords), jsonText(nonNilAnyMap(s.Filters)),
		s.Limit, s.PotentiallyTruncated).Scan(&searchID)
	if err != nil {
		return fmt.Errorf("insert search %d: %w", ordinal, err)
	}
	rows := make([][]any, 0, len(s.Results))
	for _, r := range s.Results {
		rows = append(rows, []any{searchID, r.Rank, r.ArtifactType, r.ArtifactID, nullInt64(r.RecordID), r.Title, r.Score, jsonText(r.Diag)})
	}
	return insertComplianceRows(ctx, tx, "kb.doc_review_compliance_search_results",
		[]string{"search_id", "rank", "candidate_type", "candidate_id", "candidate_record_id", "document_title", "score", "diagnostics"}, rows)
}

// insertComplianceRows inserts rows with multi-row INSERT statements of at most
// complianceInsertBatch rows each.
func insertComplianceRows(ctx context.Context, tx complianceTx, table string, cols []string, rows [][]any) error {
	for start := 0; start < len(rows); start += complianceInsertBatch {
		batch := rows[start:min(start+complianceInsertBatch, len(rows))]
		var b strings.Builder
		b.WriteString("INSERT INTO " + table + " (" + strings.Join(cols, ", ") + ") VALUES ")
		args := make([]any, 0, len(batch)*len(cols))
		for i, row := range batch {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("(")
			for j := range row {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString("$" + strconv.Itoa(len(args)+1))
				args = append(args, row[j])
			}
			b.WriteString(")")
		}
		if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
			return fmt.Errorf("insert %d rows into %s: %w", len(batch), table, err)
		}
	}
	return nil
}

// saveComplianceFinding reserves the finding's activity IDs, then writes it in one
// transaction. A failure leaves none of its records.
func saveComplianceFinding(ctx context.Context, db *sql.DB, ids complianceActivityIDs, recordID, runID int64, f ReviewFinding) (int64, error) {
	var first int64
	if n := len(f.Compliance.Activities); n > 0 {
		var err error
		if first, err = ids(ctx, int64(n)); err != nil {
			return 0, err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin compliance finding transaction: %w", err)
	}
	id, err := insertComplianceFinding(ctx, tx, recordID, runID, f, first)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit compliance finding: %w", err)
	}
	return id, nil
}

func jsonText(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

func nonNilMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func nonNilAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func nullInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
