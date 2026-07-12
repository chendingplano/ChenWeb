package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

const EntryTypePipelineFinish = "pipeline finish"
const EntryTypeLLMCall = "llm_call"
const EntryTypeGenerateSummary = "generate_summary"
const EntryTypeGenerateSummaryFinish = "generate_summary_finish"
const EntryTypeExtractTopics = "extract_topics"
const EntryTypeExtractTopicsFinish = "extract_topics_finish"
const EntryTypeStaticAnalyzer = "static_analyzer"
const EntryTypeBlocking = "blocking"
const EntryTypeExtractMetrics = "extract_metrics"
const EntryTypeExtractMetricsFinish = "extract_metrics_finish"
const EntryTypeExtractProjections = "extract_projections"
const EntryTypeExtractProjectionsFinish = "extract_projections_finish"
const EntryTypeExtractProvisions = "extract_provisions"
const EntryTypeExtractProvisionsFinish = "extract_provisions_finish"
const EntryTypeExtractSceneBlocks = "extract_scene_blocks"
const EntryTypeEnrichSceneBlocks = "enrich_scene_blocks"
const EntryTypeExtractSceneBlocksFinish = "extract_scene_blocks_finish"
const EntryTypeExtractStructuredKnowledge = "extract_structured_knowledge"
const EntryTypeEnrichStructuredKnowledge = "enrich_structured_knowledge"
const EntryTypeExtractEntities = "extract_entities"
const EntryTypeExtractRelations = "extract_relations"
const EntryTypeExtractEntityRelationFinish = "extract_entity_relation_finish"
const EntryTypeExtractInventoryItems = "extract_inventory_items"
const EntryTypeExtractInventoryItemsFinish = "extract_inventory_items_finish"
const EntryTypeExtractDocMetadata = "extract_doc_metadata"
const EntryTypeChunking = "chunking"
const EntryTypeGenerateTopics = "generate_topics"
const EntryTypeReconcileObject = "reconcile_object"

// DocProcLogRecord is a single log entry inserted into kb.doc_proc_logs.
type DocProcLogRecord struct {
	CallReason    string
	DocProcName   string
	ModelNames    []string
	PromptName    string
	RecordID      *int64
	ProcProgress  *string
	EntryType     string // EntryTypeLLMCall | EntryTypeSummary | ...
	Pass          *int
	LLMCallID     *string
	ActivityName  *string
	ArtifactJSON  *string // serialized JSON, nil → NULL
	Errors        *string
	ExtraInfoJSON *string // serialized JSON, nil → NULL
	MSUsed        *int64
	LogLoc        *string // "<filename>_<line>" of the Log* call site
	// Provider prompt-cache counters for the LLM call this entry records.
	// nil → NULL (non-LLM entries). Populated from llmclients.Usage via
	// extractorCacheTokens at the call site. See ADR 2026062501 / doc-processor cache ADR.
	PromptCacheHitTokens  *int64
	PromptCacheMissTokens *int64
}

// DocProcLogFilter specifies optional server-side filters for ListDocProcLogs.
type DocProcLogFilter struct {
	EntryType       string     // empty = all
	DocProcName     string     // empty = all
	ActivityName    string     // empty = all
	LLMCallID       string     // empty = all
	RecordID        *int64     // nil = all
	RunID           *int64     // nil = all; kb.doc_process_runs.id (ADR 2026071201)
	CreateTimeStart *time.Time // nil = all; inclusive
	CreateTimeEnd   *time.Time // nil = all; inclusive
	Page            int        // 1-based; 0 treated as 1
	PageSize        int        // 0 → 50
	OrderBy         string     // empty → create_time
	OrderDir        string     // empty → desc
}

// DocProcLogRow is a row returned from ListDocProcLogs.
type DocProcLogRow struct {
	ID            int64
	CallReason    string
	DocProcName   string
	ModelNames    []string
	PromptName    string
	RecordID      *int64
	ProcProgress  *string
	EntryType     string
	Pass          *int
	LLMCallID     *string
	ActivityName  *string
	ArtifactJSON  *string
	Errors        *string
	ExtraInfoJSON *string
	MSUsed        *int64
	LogLoc        *string
	CreateTime    string

	PromptCacheHitTokens  *int64
	PromptCacheMissTokens *int64
	RunID                 *int64
}

// DocProcLogger wraps a db connection and exposes logging helpers for processors.
type DocProcLogger struct {
	DB *sql.DB
}

// callerLoc returns "<basename>_<line>" for the caller at the given depth.
// depth 2 = the caller of the Log* method (the log instrument site).
func callerLoc(depth int) *string {
	_, file, line, ok := runtime.Caller(depth)
	if !ok {
		return nil
	}
	s := filepath.Base(file) + "_" + strconv.Itoa(line)
	return &s
}

// LogLLMCall inserts an llm_call entry. Callers marshal the artifact to JSON before passing it.
func (l DocProcLogger) LogLLMCall(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeLLMCall
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractDocMetadata(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractDocMetadata
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

// LogSummary inserts a per-operation summary row. Every summary shares
// entry_type = doc_proc_summary; the operation is identified by rec.DocProcName.
func (l DocProcLogger) LogSummary(ctx context.Context, entry_type string, rec DocProcLogRecord, loc string) error {
	rec.EntryType = entry_type
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

// LogExtractMetricsSummary inserts an extract_metrics summary entry.
func (l DocProcLogger) LogExtractMetricsSummary(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractMetrics
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogGenerateSummary(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeGenerateSummary
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogGenerateSummaryFinish(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeGenerateSummaryFinish
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractTopics(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractTopics
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractTopicsFinish(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractTopicsFinish
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogStaticAnalyzer(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeStaticAnalyzer
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogBlocking(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeBlocking
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractMetrics(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractMetrics
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractMetricsFinish(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractMetricsFinish
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogEnrichMetrics(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractMetrics
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

// LogExtractProjections logs a per-chunk Pass 1 entry and the finish record for
// extract_semantic_projections (both share entry_type = 'extract_projections' per spec).
func (l DocProcLogger) LogExtractProjections(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractProjections
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractProjectionsFinish(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractProjectionsFinish
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

// LogEnrichProjections logs a per-block Pass 2 entry for extract_semantic_projections.
func (l DocProcLogger) LogEnrichProjections(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractProjections
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractProvisions(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractProvisions
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractProvisionsFinish(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractProvisionsFinish
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractSceneBlocks(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractSceneBlocks
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogEnrichSceneBlocks(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeEnrichSceneBlocks
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractStructuredKnowledge(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractStructuredKnowledge
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogEnrichStructuredKnowledge(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeEnrichStructuredKnowledge
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractEntities(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractEntities
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractRelations(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractRelations
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogExtractInventoryItems(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeExtractInventoryItems
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func (l DocProcLogger) LogPipelineFinish(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypePipelineFinish
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

// LogReconcileObject inserts one per-object LLM object-reconciliation entry
// (resolved, unresolved, or failed). See reconcileArtifactObjectsWithLLM.
func (l DocProcLogger) LogReconcileObject(ctx context.Context, rec DocProcLogRecord, loc string) error {
	rec.EntryType = EntryTypeReconcileObject
	rec.LogLoc = callerLoc(2)
	return insertDocProcLog(ctx, resolveDocProcLogDB(l.DB), rec, loc)
}

func allowedDocProcLogEntryType(entryType string) bool {
	switch entryType {
	case EntryTypePipelineFinish,
		EntryTypeLLMCall,
		EntryTypeGenerateSummary,
		EntryTypeGenerateSummaryFinish,
		EntryTypeExtractTopics, EntryTypeExtractTopicsFinish,
		EntryTypeStaticAnalyzer, EntryTypeBlocking,
		EntryTypeExtractMetrics, EntryTypeExtractMetricsFinish,
		EntryTypeExtractProjections,
		EntryTypeExtractProjectionsFinish,
		EntryTypeExtractProvisions, EntryTypeExtractProvisionsFinish,
		EntryTypeExtractSceneBlocks, EntryTypeEnrichSceneBlocks,
		EntryTypeExtractStructuredKnowledge, EntryTypeEnrichStructuredKnowledge,
		EntryTypeExtractEntities, 
		EntryTypeExtractRelations, 
		EntryTypeExtractEntityRelationFinish,
		EntryTypeExtractInventoryItems, EntryTypeExtractInventoryItemsFinish,
		EntryTypeExtractSceneBlocksFinish,
		EntryTypeChunking,
		EntryTypeGenerateTopics,
		EntryTypeReconcileObject,
		EntryTypeExtractDocMetadata:
		return true
	default:
		return false
	}
}

func insertDocProcLog(ctx context.Context, db *sql.DB, rec DocProcLogRecord, loc string) error {
	recordDocProcLogSpan(ctx, rec)
	if db == nil {
		return errors.New("db is nil")
	}
	if strings.TrimSpace(rec.DocProcName) == "" {
		return errors.New("doc_proc_name is required")
	}
	if !allowedDocProcLogEntryType(rec.EntryType) {
		return fmt.Errorf("entry_type is not allowed:%s", rec.EntryType)
	}

	modelNamesArr := "{}"
	if len(rec.ModelNames) > 0 {
		modelNamesArr = "{" + strings.Join(rec.ModelNames, ",") + "}"
	}

	const stmt = `
INSERT INTO kb.doc_proc_logs (
    call_reason,
    doc_proc_name,
    model_names,
    prompt_name,
    record_id,
    proc_progress,
    entry_type,
    pass,
    llm_call_id,
    activity_name,
    artifact,
    errors,
    extra_info,
    ms_used,
    log_loc,
    prompt_cache_hit_tokens,
    prompt_cache_miss_tokens,
    run_id,
    create_time
) VALUES (
    $1, $2, $3::text[], $4, $5, $6, $7, $8, $9, $10,
    $11::jsonb, $12, $13::jsonb,
    $14, $15, $16, $17, $18, NOW()
)`
	var runIDArg *int64
	if runID, ok := runIDFromContext(ctx); ok {
		runIDArg = &runID
	}
	_, err := db.ExecContext(ctx, stmt,
		nullableString(rec.CallReason),
		rec.DocProcName,
		modelNamesArr,
		nullableString(rec.PromptName),
		rec.RecordID,
		rec.ProcProgress,
		rec.EntryType,
		rec.Pass,
		rec.LLMCallID,
		rec.ActivityName,
		rec.ArtifactJSON,
		rec.Errors,
		rec.ExtraInfoJSON,
		rec.MSUsed,
		loc,
		rec.PromptCacheHitTokens,
		rec.PromptCacheMissTokens,
		runIDArg,
	)
	if err != nil {
		return err
	}
	if rec.RecordID != nil && rec.ProcProgress != nil && strings.TrimSpace(*rec.ProcProgress) != "" {
		_ = updateStatusProgress(ctx, db, *rec.RecordID, rec.DocProcName, strings.TrimSpace(*rec.ProcProgress))
	}
	return nil
}

// updateStatusProgress sets the `progress` field of the matching operation entry inside
// kb.inputs.status (a JSONB array) for the given record. Failures are tolerated by callers.
func updateStatusProgress(ctx context.Context, db *sql.DB, recordID int64, operation, progress string) error {
	const stmt = `
UPDATE kb.inputs
SET status = (
    SELECT jsonb_agg(
        CASE
            WHEN elem->>'operation' = $2
            THEN jsonb_set(elem, '{progress}', to_jsonb($3::text))
            ELSE elem
        END
    )
    FROM jsonb_array_elements(COALESCE(status, '[]'::jsonb)) AS elem
),
modify_time = NOW()
WHERE id = $1
  AND status IS NOT NULL
  AND jsonb_typeof(status) = 'array'`
	_, err := db.ExecContext(ctx, stmt, recordID, operation, progress)
	return err
}

// InsertDocProcLog inserts a log record from an SQLStore (satisfies existing pattern).
func (s SQLStore) InsertDocProcLog(ctx context.Context, rec DocProcLogRecord, loc string) error {
	return insertDocProcLog(ctx, resolveDocProcLogDB(s.DB), rec, loc)
}

// ListDocProcLogs returns paginated log rows with optional filtering.
func (s SQLStore) ListDocProcLogs(ctx context.Context, f DocProcLogFilter) ([]DocProcLogRow, int64, error) {
	db := resolveDocProcLogDB(s.DB)
	if db == nil {
		return nil, 0, errors.New("db is nil")
	}
	pageSize := f.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	var where []string
	var args []any
	argIdx := 1

	if f.EntryType != "" {
		where = append(where, "entry_type = $"+itoa(argIdx))
		args = append(args, f.EntryType)
		argIdx++
	}
	if f.DocProcName != "" {
		where = append(where, "doc_proc_name = $"+itoa(argIdx))
		args = append(args, f.DocProcName)
		argIdx++
	}
	if f.ActivityName != "" {
		where = append(where, "activity_name = $"+itoa(argIdx))
		args = append(args, f.ActivityName)
		argIdx++
	}
	if f.LLMCallID != "" {
		where = append(where, "llm_call_id = $"+itoa(argIdx))
		args = append(args, f.LLMCallID)
		argIdx++
	}
	if f.RecordID != nil {
		where = append(where, "record_id = $"+itoa(argIdx))
		args = append(args, *f.RecordID)
		argIdx++
	}
	if f.RunID != nil {
		where = append(where, "run_id = $"+itoa(argIdx))
		args = append(args, *f.RunID)
		argIdx++
	}
	if f.CreateTimeStart != nil {
		where = append(where, "create_time >= $"+itoa(argIdx))
		args = append(args, *f.CreateTimeStart)
		argIdx++
	}
	if f.CreateTimeEnd != nil {
		where = append(where, "create_time <= $"+itoa(argIdx))
		args = append(args, *f.CreateTimeEnd)
		argIdx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	countStmt := "SELECT COUNT(*) FROM kb.doc_proc_logs " + whereClause
	var total int64
	if err := db.QueryRowContext(ctx, countStmt, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := docProcLogOrderBy(f.OrderBy)
	orderDir := docProcLogOrderDir(f.OrderDir)
	listArgs := append(args, pageSize, offset)
	listStmt := `
SELECT id, COALESCE(call_reason,''), doc_proc_name,
       COALESCE(model_names, ARRAY[]::text[]),
       COALESCE(prompt_name,''), record_id, proc_progress, entry_type,
       pass, llm_call_id, activity_name,
       artifact::text, errors, extra_info::text,
       ms_used, log_loc, COALESCE(to_char(create_time, 'YYYY-MM-DD\"T\"HH24:MI:SSOF'), ''),
       prompt_cache_hit_tokens, prompt_cache_miss_tokens, run_id
FROM kb.doc_proc_logs
` + whereClause + `
ORDER BY ` + orderBy + ` ` + orderDir + `
LIMIT $` + itoa(argIdx) + ` OFFSET $` + itoa(argIdx+1)

	rows, err := db.QueryContext(ctx, listStmt, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []DocProcLogRow
	for rows.Next() {
		var r DocProcLogRow
		var modelArr []string
		var artifact, extraInfo sql.NullString
		var msUsed, cacheHit, cacheMiss, runID sql.NullInt64
		if err := rows.Scan(
			&r.ID,
			&r.CallReason,
			&r.DocProcName,
			pq_text_array(&modelArr),
			&r.PromptName,
			&r.RecordID,
			&r.ProcProgress,
			&r.EntryType,
			&r.Pass,
			&r.LLMCallID,
			&r.ActivityName,
			&artifact,
			&r.Errors,
			&extraInfo,
			&msUsed,
			&r.LogLoc,
			&r.CreateTime,
			&cacheHit,
			&cacheMiss,
			&runID,
		); err != nil {
			return nil, 0, err
		}
		r.ModelNames = modelArr
		if artifact.Valid {
			r.ArtifactJSON = &artifact.String
		}
		if extraInfo.Valid {
			r.ExtraInfoJSON = &extraInfo.String
		}
		if msUsed.Valid {
			r.MSUsed = &msUsed.Int64
		}
		if cacheHit.Valid {
			r.PromptCacheHitTokens = &cacheHit.Int64
		}
		if cacheMiss.Valid {
			r.PromptCacheMissTokens = &cacheMiss.Int64
		}
		if runID.Valid {
			r.RunID = &runID.Int64
		}
		results = append(results, r)
	}
	return results, total, rows.Err()
}

func docProcLogOrderBy(orderBy string) string {
	switch strings.TrimSpace(orderBy) {
	case "entry_type":
		return "entry_type"
	case "doc_proc_name":
		return "doc_proc_name"
	case "activity_name":
		return "activity_name"
	case "model_names":
		return "model_names"
	case "pass":
		return "pass"
	case "ms_used":
		return "ms_used"
	case "run_id":
		return "run_id"
	case "errors":
		return "errors"
	default:
		return "create_time"
	}
}

func docProcLogOrderDir(orderDir string) string {
	if strings.EqualFold(strings.TrimSpace(orderDir), "asc") {
		return "ASC"
	}
	return "DESC"
}

// DeleteOldDocProcLogs removes log entries older than retentionDays days.
// Returns the number of rows deleted.
func (s SQLStore) DeleteOldDocProcLogs(ctx context.Context, retentionDays int) (int64, error) {
	db := resolveDocProcLogDB(s.DB)
	if db == nil {
		return 0, errors.New("db is nil")
	}
	if retentionDays < 1 {
		return 0, errors.New("retentionDays must be >= 1")
	}
	const stmt = `
DELETE FROM kb.doc_proc_logs
WHERE create_time < NOW() - ($1 || ' days')::interval`
	res, err := db.ExecContext(ctx, stmt, itoa(retentionDays))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func resolveDocProcLogDB(db *sql.DB) *sql.DB {
	if db != nil {
		return db
	}
	return ApiTypes.ProjectDBHandle
}

func int64Ptr(v int64) *int64 {
	return &v
}

// nullableString converts an empty string to nil so the DB stores NULL.
func nullableString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func nullableStringPtr(s string) *string {
	return nullableString(s)
}

// itoa is a compact int-to-string helper used for query building only.
func itoa(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		buf = append([]byte{digits[n%10]}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// pq_text_array returns a scan destination that decodes a PostgreSQL text[] column into []string.
// We avoid importing lib/pq directly; instead we use the TextArray wrapper below.
func pq_text_array(dest *[]string) interface{ Scan(src any) error } {
	return &pgTextArray{dest: dest}
}

type pgTextArray struct{ dest *[]string }

func (a *pgTextArray) Scan(src any) error {
	if src == nil {
		*a.dest = nil
		return nil
	}
	var raw string
	switch v := src.(type) {
	case []byte:
		raw = string(v)
	case string:
		raw = v
	default:
		*a.dest = nil
		return nil
	}
	// PostgreSQL text[] format: {elem1,elem2,...} or {"elem with space","elem2"}
	raw = strings.TrimSpace(raw)
	if raw == "{}" || raw == "" {
		*a.dest = []string{}
		return nil
	}
	raw = strings.TrimPrefix(raw, "{")
	raw = strings.TrimSuffix(raw, "}")
	if raw == "" {
		*a.dest = []string{}
		return nil
	}
	// Simple CSV split — elements containing commas must be quoted.
	// For model names this is sufficient.
	parts := splitPGArray(raw)
	*a.dest = parts
	return nil
}

func splitPGArray(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuote = !inQuote
		case c == ',' && !inQuote:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
