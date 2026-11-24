package docreviews

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
	"github.com/lib/pq"
)

// Migration of legacy compliance findings to normalized storage (openspec
// compliance-review-storage, design D10). Each finding is rebuilt from its recorded payload,
// validated, written and switched to storage_version 2 in one transaction; its legacy payload
// stays until a separate strip step has written a recovery copy.

// ComplianceMigrationOptions selects what MigrateComplianceFindings does.
type ComplianceMigrationOptions struct {
	RunIDs []int64 // empty means every run with legacy compliance findings
	Batch  int     // findings loaded per query
	DryRun bool    // validate and write inside transactions that are rolled back
	// ActivityIDs allocates activity IDs; nil uses the shared ID manager. A dry run never
	// allocates real IDs.
	ActivityIDs complianceActivityIDs
	Logf        func(format string, args ...any)
}

// ComplianceMigrationReport says what happened to each finding.
type ComplianceMigrationReport struct {
	Migrated    []int64            `json:"migrated"`
	Unsupported map[int64]string   `json:"unsupported"`
	Failed      map[int64]string   `json:"failed"`
	RunFlags    map[int64][]string `json:"run_flags"`
}

// legacyComplianceFields is metadata.artifact_fields of a version-3 finding or an unchecked
// notice.
type legacyComplianceFields struct {
	Version           int                           `json:"version"`
	AssessmentDate    string                        `json:"assessment_date"`
	Activities        []complianceMember            `json:"activities"`
	RetrievalTrace    []complianceRetrievalTrace    `json:"retrieval_trace"`
	Relevance         []complianceRelevanceDecision `json:"relevance"`
	CandidateVerdicts []complianceVerdict           `json:"candidate_verdicts"`
	Verification      []complianceCheck             `json:"verification"`
	Prompts           map[string]string             `json:"prompts"`
	Models            map[string]string             `json:"models"`
	Errors            []string                      `json:"errors"`

	UncheckedMetrics    *int `json:"unchecked_metrics"`
	UncheckedProvisions *int `json:"unchecked_provisions"`
	WithoutLines        *int `json:"without_lines"`
	MaxMetrics          *int `json:"max_metrics"`
	MaxProvisions       *int `json:"max_provisions"`
}

// legacyComplianceRow is a legacy finding row as the migration reads it.
type legacyComplianceRow struct {
	ID          int64
	RunID       int64
	FindingType string
	Title       string
	Description string
	Evidence    string
	Suggestion  string
	Location    string
	ErrorMsg    string
	Metadata    []byte
}

var errUnsupportedShape = errors.New("unsupported legacy shape")

// legacyJudgedListing is one entry of a legacy description's judged-candidate list.
type legacyJudgedListing struct {
	key     string
	record  int64
	cosine  *float64
	title   string
	excerpt string
}

var legacyListingRef = regexp.MustCompile(`(?m)^- \[(\S+) ([^\]]+)\](?: \(cosine ([0-9.]+)\))? record (\d+) `)

// MigrateComplianceFindings migrates the legacy compliance findings of the selected runs.
func MigrateComplianceFindings(ctx context.Context, db *sql.DB, opts ComplianceMigrationOptions) (*ComplianceMigrationReport, error) {
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if opts.Batch <= 0 {
		opts.Batch = 50
	}
	ids := opts.ActivityIDs
	if ids == nil {
		ids = defaultComplianceActivityIDs(db)
	}
	rep := &ComplianceMigrationReport{Unsupported: map[int64]string{}, Failed: map[int64]string{}, RunFlags: map[int64][]string{}}
	runs := opts.RunIDs
	if len(runs) == 0 {
		rows, err := db.QueryContext(ctx, `SELECT DISTINCT run_id FROM kb.doc_review_findings
			WHERE aspect = 'compliance' AND storage_version = 1 ORDER BY run_id`)
		if err != nil {
			return nil, fmt.Errorf("list runs with legacy compliance findings: %w", err)
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			runs = append(runs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	for _, runID := range runs {
		if err := migrateComplianceRun(ctx, db, ids, runID, opts, rep, logf); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

func migrateComplianceRun(ctx context.Context, db *sql.DB, ids complianceActivityIDs, runID int64, opts ComplianceMigrationOptions,
	rep *ComplianceMigrationReport, logf func(string, ...any)) error {
	rc, flags, err := legacyRunContext(ctx, db, runID)
	if err != nil {
		return err
	}
	if rc != nil {
		rep.RunFlags[runID] = flags
		if !opts.DryRun {
			if err := insertMigratedRunContext(ctx, db, runID, rc, flags, logf); err != nil {
				return err
			}
		}
	}
	var after int64
	for {
		batch, err := loadLegacyComplianceRows(ctx, db, runID, after, opts.Batch)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for _, row := range batch {
			after = row.ID
			err := migrateComplianceFinding(ctx, db, ids, row, opts.DryRun)
			switch {
			case errors.Is(err, errUnsupportedShape):
				rep.Unsupported[row.ID] = err.Error()
				logf("finding %d (run %d): left as legacy: %v", row.ID, runID, err)
			case err != nil:
				rep.Failed[row.ID] = err.Error()
				logf("finding %d (run %d): FAILED: %v", row.ID, runID, err)
			default:
				rep.Migrated = append(rep.Migrated, row.ID)
			}
		}
	}
}

func loadLegacyComplianceRows(ctx context.Context, db *sql.DB, runID, after int64, limit int) ([]legacyComplianceRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, run_id, finding_type, title, description, COALESCE(evidence, ''),
			COALESCE(suggestion, ''), COALESCE(location, ''), COALESCE(error_msg, ''), COALESCE(metadata, '{}'::jsonb)::text
		FROM kb.doc_review_findings
		WHERE run_id = $1 AND aspect = 'compliance' AND storage_version = 1 AND id > $2
		ORDER BY id LIMIT $3`, runID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("load legacy compliance findings of run %d: %w", runID, err)
	}
	defer rows.Close()
	var out []legacyComplianceRow
	for rows.Next() {
		var r legacyComplianceRow
		var meta string
		if err := rows.Scan(&r.ID, &r.RunID, &r.FindingType, &r.Title, &r.Description, &r.Evidence, &r.Suggestion, &r.Location, &r.ErrorMsg, &meta); err != nil {
			return nil, err
		}
		r.Metadata = []byte(meta)
		out = append(out, r)
	}
	return out, rows.Err()
}

// legacyRunContext builds a run's context from what its version-3 findings recorded:
// prompts, assessment date and configured models. Findings that disagree become review
// flags; nothing comes from current settings.
func legacyRunContext(ctx context.Context, db *sql.DB, runID int64) (*complianceRunContext, []string, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, COALESCE(metadata->>'model_name', ''),
			COALESCE(metadata->'artifact_fields'->>'assessment_date', ''),
			COALESCE(metadata->'artifact_fields'->'prompts', '{}'::jsonb)::text,
			COALESCE(metadata->'artifact_fields'->'models', '{}'::jsonb)::text
		FROM kb.doc_review_findings
		WHERE run_id = $1 AND aspect = 'compliance' AND (metadata->'artifact_fields'->>'version')::int = 3
		ORDER BY id`, runID)
	if err != nil {
		return nil, nil, fmt.Errorf("load recorded configuration of run %d: %w", runID, err)
	}
	defer rows.Close()
	var rc *complianceRunContext
	var flags []string
	var firstID int64
	for rows.Next() {
		var id int64
		var modelName, date, promptsText, modelsText string
		if err := rows.Scan(&id, &modelName, &date, &promptsText, &modelsText); err != nil {
			return nil, nil, err
		}
		var prompts, all map[string]string
		_ = json.Unmarshal([]byte(promptsText), &prompts)
		_ = json.Unmarshal([]byte(modelsText), &all)
		models := map[string]string{}
		for _, k := range []string{"primary", "correction", "fallback"} {
			if v := all[k]; v != "" {
				models[k] = v
			}
		}
		if modelName != "" && models["primary"] != "" && modelName != models["primary"] {
			flags = append(flags, fmt.Sprintf("finding %d: model_name %q differs from its configured primary model %q", id, modelName, models["primary"]))
		}
		if rc == nil {
			rc = &complianceRunContext{AssessmentDate: date, SchemaVersion: 3, Prompts: prompts, Models: models}
			firstID = id
			continue
		}
		if date != rc.AssessmentDate {
			flags = append(flags, fmt.Sprintf("finding %d: assessment date %q differs from finding %d's %q", id, date, firstID, rc.AssessmentDate))
		}
		flags = append(flags, mapDifferences(fmt.Sprintf("finding %d", id), "prompts", prompts, rc.Prompts, firstID)...)
		flags = append(flags, mapDifferences(fmt.Sprintf("finding %d", id), "models", models, rc.Models, firstID)...)
	}
	return rc, flags, rows.Err()
}

func mapDifferences(who, what string, got, want map[string]string, firstID int64) []string {
	keys := map[string]bool{}
	for k := range got {
		keys[k] = true
	}
	for k := range want {
		keys[k] = true
	}
	var sorted []string
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var out []string
	for _, k := range sorted {
		if got[k] != want[k] {
			out = append(out, fmt.Sprintf("%s: %s.%s %q differs from finding %d's %q", who, what, k, got[k], firstID, want[k]))
		}
	}
	return out
}

// insertMigratedRunContext stores a migrated run's context unless the run already has one;
// an existing one that differs is reported, never overwritten.
func insertMigratedRunContext(ctx context.Context, db *sql.DB, runID int64, rc *complianceRunContext, flags []string, logf func(string, ...any)) error {
	var prompts, models string
	err := db.QueryRowContext(ctx, `SELECT prompts::text, models::text FROM kb.doc_review_compliance_run_context WHERE run_id = $1`, runID).Scan(&prompts, &models)
	if errors.Is(err, sql.ErrNoRows) {
		rc.ReviewFlags = flags
		_, err = db.ExecContext(ctx, `INSERT INTO kb.doc_review_compliance_run_context
			(run_id, assessment_date, schema_version, prompts, models, settings, review_flags)
			VALUES ($1, $2, $3, $4, $5, NULL, $6) ON CONFLICT (run_id) DO NOTHING`,
			runID, rc.AssessmentDate, rc.SchemaVersion, jsonText(nonNilMap(rc.Prompts)), jsonText(nonNilMap(rc.Models)), jsonText(append([]string{}, flags...)))
		if err != nil {
			return fmt.Errorf("save run context of run %d: %w", runID, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("load run context of run %d: %w", runID, err)
	}
	if prompts != jsonText(nonNilMap(rc.Prompts)) || models != jsonText(nonNilMap(rc.Models)) {
		logf("run %d: existing run context differs from the findings' recorded configuration; kept the existing one", runID)
	}
	return nil
}

// migrateComplianceFinding rebuilds, validates and writes one finding in one transaction.
func migrateComplianceFinding(ctx context.Context, db *sql.DB, ids complianceActivityIDs, row legacyComplianceRow, dryRun bool) error {
	f, translations, err := buildLegacyComplianceFinding(row)
	if err != nil {
		return err
	}
	var first int64
	if n := len(f.Compliance.Activities); n > 0 {
		if dryRun {
			first = -row.ID * 10000
		} else if first, err = ids(ctx, int64(n)); err != nil {
			return err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := writeMigratedFinding(ctx, tx, row.ID, f, translations, first); err != nil {
		_ = tx.Rollback()
		return err
	}
	if dryRun {
		return tx.Rollback()
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// writeMigratedFinding writes the rebuilt records of an existing finding row, compares the
// stored counts with the rebuilt ones and only then switches the finding's storage version.
func writeMigratedFinding(ctx context.Context, tx *sql.Tx, findingID int64, f ReviewFinding, translations map[string]FindingLocalizedContent, firstActivityID int64) error {
	if err := insertComplianceDetails(ctx, tx, findingID, f); err != nil {
		return err
	}
	for i := range f.Compliance.Activities {
		if err := insertComplianceActivity(ctx, tx, findingID, i, firstActivityID+int64(i), &f.Compliance.Activities[i]); err != nil {
			return fmt.Errorf("activity %s: %w", f.Compliance.Activities[i].LocalLabel, err)
		}
	}
	var langs []string
	for lang := range translations {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	for _, lang := range langs {
		tr := translations[lang]
		if _, err := tx.ExecContext(ctx, `INSERT INTO kb.doc_review_finding_translations (finding_id, language, title, description, suggestion, provenance)
			VALUES ($1, $2, $3, $4, $5, $6)`, findingID, lang, tr.Title, tr.Description, tr.Suggestion, tr.Provenance); err != nil {
			return fmt.Errorf("insert %s translation: %w", lang, err)
		}
	}
	want := f.Compliance.counts()
	var got complianceCounts
	if err := tx.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM kb.doc_review_compliance_activities WHERE finding_id = $1),
			(SELECT COUNT(*) FROM kb.doc_review_compliance_search_results r JOIN kb.doc_review_compliance_searches s ON s.id = r.search_id
				JOIN kb.doc_review_compliance_activities a ON a.activity_id = s.activity_id WHERE a.finding_id = $1),
			(SELECT COUNT(*) FROM kb.doc_review_compliance_candidates c JOIN kb.doc_review_compliance_activities a ON a.activity_id = c.activity_id WHERE a.finding_id = $1),
			(SELECT COUNT(*) FROM kb.doc_review_compliance_candidates c JOIN kb.doc_review_compliance_activities a ON a.activity_id = c.activity_id WHERE a.finding_id = $1 AND c.kept),
			(SELECT COUNT(*) FROM kb.doc_review_compliance_candidates c JOIN kb.doc_review_compliance_activities a ON a.activity_id = c.activity_id WHERE a.finding_id = $1 AND c.status = 'judged')`,
		findingID).Scan(&got.Activities, &got.SearchResults, &got.Selected, &got.Relevant, &got.Judged); err != nil {
		return fmt.Errorf("re-read counts: %w", err)
	}
	if got.Activities != want.Activities || got.SearchResults != want.SearchResults || got.Selected != want.Selected ||
		got.Relevant != want.Relevant || got.Judged != want.Judged {
		return fmt.Errorf("stored counts %+v differ from rebuilt counts %+v", got, want)
	}
	res, err := tx.ExecContext(ctx, `UPDATE kb.doc_review_findings SET storage_version = 2, line_start = $2, line_end = $3,
			activity_count = $4, search_result_count = $5, selected_candidate_count = $6, relevant_candidate_count = $7,
			judged_candidate_count = $8, verdict_counts = $9
		WHERE id = $1 AND storage_version = 1`,
		findingID, nullInt(f.LineStart), nullInt(f.LineEnd), want.Activities, want.SearchResults, want.Selected, want.Relevant, want.Judged, jsonText(want.Verdicts))
	if err != nil {
		return fmt.Errorf("switch storage version: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("finding is no longer legacy")
	}
	return nil
}

// buildLegacyComplianceFinding rebuilds a finding's records from its legacy payload and
// validates them against it. It returns the additional-language copies to store as
// translations.
func buildLegacyComplianceFinding(row legacyComplianceRow) (ReviewFinding, map[string]FindingLocalizedContent, error) {
	var env FindingMetadataEnvelope
	if err := json.Unmarshal(row.Metadata, &env); err != nil {
		return ReviewFinding{}, nil, fmt.Errorf("metadata: %w", err)
	}
	var lf legacyComplianceFields
	if len(env.ArtifactFields) > 0 {
		if err := json.Unmarshal(env.ArtifactFields, &lf); err != nil {
			return ReviewFinding{}, nil, fmt.Errorf("artifact fields: %w", err)
		}
	}
	start, end := parseLineRange(row.Location)
	f := ReviewFinding{Aspect: "compliance", FindingType: row.FindingType, Title: row.Title, Location: row.Location,
		LineStart: start, LineEnd: end, Evidence: row.Evidence, Suggestion: row.Suggestion, ErrorMsg: row.ErrorMsg,
		Language: env.I18N.CanonicalLanguage}
	if f.ErrorMsg == "" && len(lf.Errors) > 0 {
		f.ErrorMsg = strings.Join(lf.Errors, "\n")
	}
	switch {
	case lf.Version == 3:
		rec, desc, err := legacyVersion3Record(row, lf)
		if err != nil {
			return f, nil, err
		}
		f.Compliance, f.Description = rec, desc
	case row.FindingType == "compliance_unchecked" && lf.UncheckedMetrics != nil:
		extra := map[string]any{}
		for k, v := range map[string]*int{"unchecked_metrics": lf.UncheckedMetrics, "unchecked_provisions": lf.UncheckedProvisions,
			"without_lines": lf.WithoutLines, "max_metrics": lf.MaxMetrics, "max_provisions": lf.MaxProvisions} {
			if v != nil {
				extra[k] = *v
			}
		}
		f.Compliance, f.Description = &complianceRecord{Extra: extra}, row.Description
	case row.FindingType == "compliance_incomplete" && len(env.ArtifactFields) == 0 && row.ErrorMsg != "":
		// Text activities of a chunk could not be found; the finding has no activity.
		f.Compliance, f.Description = &complianceRecord{}, row.Description
	default:
		return f, nil, fmt.Errorf("%w: finding type %s without version-3 fields", errUnsupportedShape, row.FindingType)
	}
	f.Compliance.Languages = &complianceLanguages{Canonical: env.I18N.CanonicalLanguage, Origin: env.I18N.CanonicalOrigin,
		Source: env.I18N.SourceLanguage, SourceConfidence: env.I18N.SourceLanguageConfidence}
	translations, err := legacyTranslations(env, row)
	if err != nil {
		return f, nil, err
	}
	return f, translations, nil
}

// legacyTranslations compares each language copy with the finding's canonical content. An
// identical copy is dropped; a copy in another language becomes a translation; a differing
// copy in the canonical language fails validation.
func legacyTranslations(env FindingMetadataEnvelope, row legacyComplianceRow) (map[string]FindingLocalizedContent, error) {
	canonical := strings.ToLower(env.I18N.CanonicalLanguage)
	out := map[string]FindingLocalizedContent{}
	for lang, c := range env.I18N.Translations {
		same := c.Title == strings.TrimSpace(row.Title) && c.Description == strings.TrimSpace(row.Description) && c.Suggestion == strings.TrimSpace(row.Suggestion)
		switch {
		case same:
			continue
		case strings.EqualFold(lang, canonical) || canonical == "":
			return nil, fmt.Errorf("the %s copy differs from the finding's canonical content", lang)
		default:
			out[lang] = c
		}
	}
	return out, nil
}

// legacyVersion3Record rebuilds the records of a version-3 finding and returns the
// finding-level part of its description.
func legacyVersion3Record(row legacyComplianceRow, lf legacyComplianceFields) (*complianceRecord, string, error) {
	labels := map[string]*complianceMember{}
	for i := range lf.Activities {
		m := &lf.Activities[i]
		if m.ID == "" || labels[m.ID] != nil {
			return nil, "", fmt.Errorf("activity label %q is empty or repeated", m.ID)
		}
		labels[m.ID] = m
	}
	var problems []string
	for _, t := range lf.RetrievalTrace {
		if labels[t.ActivityID] == nil {
			problems = append(problems, fmt.Sprintf("a search names activity %s, which the finding does not have", t.ActivityID))
		}
	}
	kept := map[string]bool{}
	for _, d := range lf.Relevance {
		if labels[d.ActivityID] == nil {
			problems = append(problems, fmt.Sprintf("a relevance decision names activity %s, which the finding does not have", d.ActivityID))
		}
		if d.Kept {
			kept[d.ActivityID+"\x00"+d.ArtifactKey] = true
		}
	}
	for _, v := range lf.CandidateVerdicts {
		if !kept[v.ActivityID+"\x00"+v.ArtifactKey] {
			problems = append(problems, fmt.Sprintf("the verdict for activity %s, candidate %s has no kept relevance decision", v.ActivityID, v.ArtifactKey))
		}
	}
	if len(problems) > 0 {
		return nil, "", errors.New(strings.Join(problems, "; "))
	}

	pool := legacyCandidatePool(lf)
	unit := &complianceUnit{}
	for i := range lf.Activities {
		unit.Members = append(unit.Members, &lf.Activities[i])
	}
	s := &complianceUnitState{unit: unit, pool: pool, traces: lf.RetrievalTrace, relevance: lf.Relevance,
		verdicts: lf.CandidateVerdicts, checks: lf.Verification, found: map[string][]string{},
		relevanceErrs: map[string]string{}, judgeErrs: map[string]string{}}
	// A kept candidate without a verdict can only come from a failed judgement batch.
	judgeFailed := strings.Contains(row.ErrorMsg+strings.Join(lf.Errors, "\n"), "judgement of ")
	if judgeFailed {
		for pair := range kept {
			s.judgeErrs[pair] = ""
		}
	}
	activities := complianceActivityRecords(s)

	intro, listings, err := splitLegacyDescription(row.Description, activities)
	if err != nil {
		return nil, "", err
	}
	if err := applyLegacyListings(activities, listings); err != nil {
		return nil, "", err
	}
	if row.FindingType == "compliance_assessment" && intro != "" {
		return nil, "", fmt.Errorf("the description holds text besides its verdicts, activities and judged list: %q", complianceCap(intro, 120))
	}
	stage := map[string]string{}
	for k, v := range lf.Models {
		if k != "primary" && k != "correction" && k != "fallback" && v != "" {
			stage[k] = v
		}
	}
	return &complianceRecord{Activities: activities, StageModels: stage}, intro, nil
}

// legacyCandidatePool replays how the reviewer filled its candidate pool from the stored
// searches, so each candidate gets the record, title and cosine the reviewer held: per
// activity, a metric or provision adds its activity search's results; a text activity adds
// its searches' results taken in turn up to the limit; cited-document searches add theirs;
// the first search that returned a candidate wins.
func legacyCandidatePool(lf legacyComplianceFields) map[string]*complianceCandidate {
	pool := map[string]*complianceCandidate{}
	add := func(results []searchengine.Result) {
		for _, r := range results {
			key := r.ArtifactType + "/" + r.ArtifactID
			if _, ok := pool[key]; !ok {
				pool[key] = complianceCandidateFrom(r)
			}
		}
	}
	asResults := func(rs []complianceTraceResult) []searchengine.Result {
		out := make([]searchengine.Result, len(rs))
		for i, r := range rs {
			out[i] = searchengine.Result{ArtifactType: r.ArtifactType, ArtifactID: r.ArtifactID, InputRecordID: r.RecordID,
				SourceTitle: r.Title, Rank: r.Rank, Score: r.Score, Diag: r.Diag}
		}
		return out
	}
	for _, m := range lf.Activities {
		var own [][]searchengine.Result
		limit := 0
		var others []complianceRetrievalTrace
		for _, t := range lf.RetrievalTrace {
			if t.ActivityID != m.ID {
				continue
			}
			if t.Purpose == "activity" {
				own = append(own, asResults(t.Results))
				limit = t.Limit
			} else {
				others = append(others, t)
			}
		}
		if m.Kind == "text" && len(own) > 0 {
			add(complianceRoundRobin(own, limit))
		} else {
			for _, list := range own {
				add(list)
			}
		}
		for _, t := range others {
			add(asResults(t.Results))
		}
	}
	return pool
}

// splitLegacyDescription checks that the legacy description is exactly the verdict rows and
// activity lines the rebuilt records render, plus the judged-candidate list, and returns the
// finding-level text left over and the parsed list.
func splitLegacyDescription(desc string, activities []complianceActivityRecord) (string, []legacyJudgedListing, error) {
	var verdicts []complianceReportVerdict
	var acts []complianceReportActivity
	judgedKeys := map[string]bool{}
	type ordered struct {
		order int
		v     complianceReportVerdict
	}
	var all []ordered
	for _, a := range activities {
		acts = append(acts, complianceReportActivity{label: a.LocalLabel, kind: a.Kind, name: a.Label, start: a.LineStart, end: a.LineEnd})
		for _, c := range a.Candidates {
			if c.Status == candidateJudged {
				judgedKeys[c.Type+"/"+c.ID] = true
				all = append(all, ordered{c.DisplayOrder, complianceReportVerdict{label: a.LocalLabel, activityLabel: a.Label, verdict: c.Verdict,
					ctype: c.Type, cid: c.ID, record: c.RecordID, summary: c.Summary, note: c.Note}})
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].order < all[j].order })
	for _, o := range all {
		verdicts = append(verdicts, o.v)
	}
	head, listingText := desc, ""
	if len(judgedKeys) > 0 {
		marker := fmt.Sprintf("\n\nJudged %d candidate artifact(s) from the knowledge base:\n", len(judgedKeys))
		i := strings.Index(desc, marker)
		if i < 0 {
			return "", nil, fmt.Errorf("the description has no list of %d judged candidates", len(judgedKeys))
		}
		head, listingText = desc[:i], desc[i+len(marker):]
	}
	actsText := renderComplianceActivityLines(acts)
	verdictText := renderComplianceVerdictRows(verdicts, len(acts) > 1)
	if !strings.HasSuffix(head, actsText) {
		return "", nil, fmt.Errorf("the description's activity lines do not match the activity records")
	}
	intro := strings.TrimSuffix(head, actsText)
	if verdictText != "" {
		if !strings.HasPrefix(intro, verdictText) {
			return "", nil, fmt.Errorf("the description's verdict rows do not match the candidate rows")
		}
		intro = strings.TrimPrefix(intro, verdictText)
	}
	intro = strings.TrimSpace(intro)
	if renderComplianceReportText(intro, verdicts, acts) != strings.TrimSpace(head) {
		return "", nil, fmt.Errorf("the description does not match the text rendered from its records")
	}
	listings, err := parseLegacyListings(listingText)
	if err != nil {
		return "", nil, err
	}
	if len(listings) != len(judgedKeys) {
		return "", nil, fmt.Errorf("the judged list names %d candidates; the records have %d judged candidates", len(listings), len(judgedKeys))
	}
	return intro, listings, nil
}

// parseLegacyListings reads the judged-candidate list in the scorer's way: each entry starts a
// line with "- [type id] (cosine c) record N ", followed by "title: excerpt".
func parseLegacyListings(text string) ([]legacyJudgedListing, error) {
	matches := legacyListingRef.FindAllStringSubmatchIndex(text, -1)
	out := make([]legacyJudgedListing, 0, len(matches))
	for i, m := range matches {
		l := legacyJudgedListing{key: text[m[2]:m[3]] + "/" + text[m[4]:m[5]]}
		if m[6] >= 0 {
			v, err := strconv.ParseFloat(text[m[6]:m[7]], 64)
			if err != nil {
				return nil, fmt.Errorf("judged list cosine: %w", err)
			}
			l.cosine = &v
		}
		l.record, _ = strconv.ParseInt(text[m[8]:m[9]], 10, 64)
		restEnd := len(text)
		if i+1 < len(matches) {
			restEnd = matches[i+1][0]
		}
		rest := strings.TrimSuffix(text[m[1]:restEnd], "\n")
		l.title, l.excerpt = rest, ""
		if j := strings.Index(rest, ": "); j >= 0 {
			l.title, l.excerpt = rest[:j], rest[j+2:]
		}
		out = append(out, l)
	}
	return out, nil
}

// applyLegacyListings checks every judged candidate against the judged list (record and
// cosine) and takes from it the judgement order, the document title and the text excerpt the
// reviewer showed.
func applyLegacyListings(activities []complianceActivityRecord, listings []legacyJudgedListing) error {
	byKey := map[string]int{}
	for i, l := range listings {
		if _, dup := byKey[l.key]; dup {
			return fmt.Errorf("the judged list repeats %s", l.key)
		}
		byKey[l.key] = i
	}
	for ai := range activities {
		for ci := range activities[ai].Candidates {
			c := &activities[ai].Candidates[ci]
			if c.Status != candidateJudged {
				continue
			}
			key := c.Type + "/" + c.ID
			i, ok := byKey[key]
			if !ok {
				return fmt.Errorf("judged candidate %s is not in the description's judged list", key)
			}
			l := listings[i]
			if l.record != c.RecordID {
				return fmt.Errorf("judged candidate %s: the list says record %d, the searches record %d", key, l.record, c.RecordID)
			}
			if (l.cosine == nil) != (c.Cosine == nil) || (l.cosine != nil && fmt.Sprintf("%.3f", *c.Cosine) != fmt.Sprintf("%.3f", *l.cosine)) {
				return fmt.Errorf("judged candidate %s: the list's cosine differs from the searches' cosine", key)
			}
			order := i
			c.JudgementOrder = &order
			c.Title = l.title
			if c.Evidence == nil {
				c.Evidence = map[string]any{}
			}
			if l.excerpt != "" {
				c.Evidence["candidate_excerpt"] = l.excerpt
			}
		}
	}
	return nil
}

// StripMigratedComplianceFindings removes the legacy payload of migrated (storage version 2)
// compliance findings after writing a gzip JSON-lines recovery copy of each original row to
// recoveryDir: metadata becomes its markers, the description column becomes empty, and
// evidence, suggestion, cited documents and error message are cleared. It returns the
// recovery file and the number of findings stripped.
func StripMigratedComplianceFindings(ctx context.Context, db *sql.DB, runIDs []int64, recoveryDir string) (string, int, error) {
	if strings.TrimSpace(recoveryDir) == "" {
		return "", 0, fmt.Errorf("a recovery directory is required")
	}
	if err := os.MkdirAll(recoveryDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create recovery directory: %w", err)
	}
	cond := `aspect = 'compliance' AND storage_version = 2 AND (description <> '' OR evidence IS NOT NULL OR suggestion IS NOT NULL
		OR cited_documents IS NOT NULL OR error_msg IS NOT NULL OR metadata - 'schema_version' - 'result_kind' - 'storage_version' <> '{}'::jsonb)`
	args := []any{}
	if len(runIDs) > 0 {
		cond += ` AND run_id = ANY($1)`
		args = append(args, pq.Array(runIDs))
	}
	rows, err := db.QueryContext(ctx, `SELECT id, row_to_json(f)::text FROM kb.doc_review_findings f WHERE `+cond+` ORDER BY id`, args...)
	if err != nil {
		return "", 0, fmt.Errorf("load findings to strip: %w", err)
	}
	var ids []int64
	path := filepath.Join(recoveryDir, fmt.Sprintf("compliance-storage-recovery-%s.jsonl.gz", time.Now().UTC().Format("20060102T150405Z")))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		rows.Close()
		return "", 0, fmt.Errorf("create recovery file: %w", err)
	}
	zw := gzip.NewWriter(file)
	for rows.Next() {
		var id int64
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			rows.Close()
			file.Close()
			return path, 0, err
		}
		if _, err := zw.Write([]byte(text + "\n")); err != nil {
			rows.Close()
			file.Close()
			return path, 0, fmt.Errorf("write recovery copy: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		file.Close()
		return path, 0, err
	}
	if err := zw.Close(); err != nil {
		file.Close()
		return path, 0, fmt.Errorf("finish recovery file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return path, 0, fmt.Errorf("sync recovery file: %w", err)
	}
	if err := file.Close(); err != nil {
		return path, 0, fmt.Errorf("close recovery file: %w", err)
	}
	stripped := 0
	for start := 0; start < len(ids); start += 200 {
		batch := ids[start:min(start+200, len(ids))]
		res, err := db.ExecContext(ctx, `UPDATE kb.doc_review_findings
			SET metadata = jsonb_strip_nulls(jsonb_build_object('schema_version', metadata->'schema_version',
				'result_kind', metadata->'result_kind', 'storage_version', 2)),
			    description = '', evidence = NULL, suggestion = NULL, cited_documents = NULL, error_msg = NULL
			WHERE id = ANY($1) AND storage_version = 2`, pq.Array(batch))
		if err != nil {
			return path, stripped, fmt.Errorf("strip findings: %w", err)
		}
		n, _ := res.RowsAffected()
		stripped += int(n)
	}
	return path, stripped, nil
}

// RestoreStrippedComplianceFindings writes the legacy payload of each row in a recovery file
// back to its finding (metadata, description, evidence, suggestion, cited documents, error
// message). The normalized records stay; it undoes a strip for readers of the old columns.
func RestoreStrippedComplianceFindings(ctx context.Context, db *sql.DB, path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open recovery file: %w", err)
	}
	defer file.Close()
	zr, err := gzip.NewReader(file)
	if err != nil {
		return 0, fmt.Errorf("read recovery file: %w", err)
	}
	defer zr.Close()
	dec := json.NewDecoder(zr)
	restored := 0
	for dec.More() {
		var row struct {
			ID             int64           `json:"id"`
			Metadata       json.RawMessage `json:"metadata"`
			Description    string          `json:"description"`
			Evidence       *string         `json:"evidence"`
			Suggestion     *string         `json:"suggestion"`
			CitedDocuments json.RawMessage `json:"cited_documents"`
			ErrorMsg       *string         `json:"error_msg"`
		}
		if err := dec.Decode(&row); err != nil {
			return restored, fmt.Errorf("recovery row %d: %w", restored+1, err)
		}
		var cited any
		if len(row.CitedDocuments) > 0 && string(row.CitedDocuments) != "null" {
			cited = string(row.CitedDocuments)
		}
		res, err := db.ExecContext(ctx, `UPDATE kb.doc_review_findings SET metadata = $2, description = $3, evidence = $4,
				suggestion = $5, cited_documents = $6, error_msg = $7
			WHERE id = $1 AND aspect = 'compliance'`, row.ID, string(row.Metadata), row.Description, row.Evidence, row.Suggestion, cited, row.ErrorMsg)
		if err != nil {
			return restored, fmt.Errorf("restore finding %d: %w", row.ID, err)
		}
		if n, _ := res.RowsAffected(); n == 1 {
			restored++
		}
	}
	return restored, nil
}
