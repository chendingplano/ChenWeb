package llmusage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	sharedllm "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

var sinkLogger = loggerutil.CreateDefaultLogger("LMU_07050001")

type Sink struct {
	DB            *sql.DB
	ArchiveRoot   string
	WorkspaceTZ   *time.Location
	NewID         func() string
	Now           func() time.Time
	DefaultStatus int
}

func (s *Sink) Capture(ctx context.Context, record sharedllm.UsageCaptureRecord) (string, error) {
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	finishedAt := record.RequestFinishedAt
	if finishedAt.IsZero() {
		finishedAt = now().UTC()
	}

	workspaceTZ := time.UTC
	if s.WorkspaceTZ != nil {
		workspaceTZ = s.WorkspaceTZ
	}
	startedAt := record.RequestStartedAt
	if startedAt.IsZero() {
		startedAt = finishedAt
	}
	workspaceDay := startedAt.In(workspaceTZ)
	workspaceDay = time.Date(workspaceDay.Year(), workspaceDay.Month(), workspaceDay.Day(), 0, 0, 0, 0, workspaceTZ)

	newID := func() string { return ApiUtils.GenerateRequestID("llm") }
	if s.NewID != nil {
		newID = s.NewID
	}
	eventID := newID()

	if s.DB != nil && (record.AccountID == "" || record.ProfileID == "") {
		accountID, profileID, err := s.resolveAccountProfileIDs(ctx, record)
		if err != nil {
			return "", err
		}
		if record.AccountID == "" {
			record.AccountID = accountID
		}
		if record.ProfileID == "" {
			record.ProfileID = profileID
		}
	}

	if record.AccountID == "" || record.ProfileID == "" {
		sinkLogger.Warn("(MID-20260708-01) llm usage event account/profile not resolved; event will be logged without account linkage",
			"provider", string(record.Provider),
			"base_url", record.BaseURL,
			"model", record.ModelName,
			"profile_name", record.ProfileName,
			"prompt_name", record.PromptName,
			"call_loc", record.CallLoc,
		)
	}

	if s.DB == nil {
		return "", nil
	}

	paths := sharedllm.BuildUsageArchivePaths(s.ArchiveRoot, workspaceDay, record.AccountID, eventID)
	inputRef := ""
	outputRef := ""

	if len(record.InputBody) > 0 {
		if err := sharedllm.WriteGzipFile(paths.InputBodyPath, record.InputBody); err != nil {
			return "", err
		}
		inputRef = archiveRef(s.ArchiveRoot, paths.InputBodyPath)
	}
	if len(record.OutputBody) > 0 {
		if err := sharedllm.WriteGzipFile(paths.OutputBodyPath, record.OutputBody); err != nil {
			return "", err
		}
		outputRef = archiveRef(s.ArchiveRoot, paths.OutputBodyPath)
	}

	latencyMS := finishedAt.Sub(startedAt).Milliseconds()
	metadata := map[string]any{
		"capture_source": "shared_llm",
	}
	if strings.TrimSpace(record.PromptName) == "" {
		metadata["prompt_name_missing"] = true
	}
	maps.Copy(metadata, record.Metadata)
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}

	const stmt = `INSERT INTO llm_usage_event (
    id, account_id, profile_id, user_id, provider, model_name, prompt_name,
    request_started_at, request_finished_at, workspace_day,
    input_tokens, output_tokens, total_tokens, prompt_cache_hit_tokens, prompt_cache_miss_tokens, latency_ms, http_status,
    error_message, input_body_ref, output_body_ref, provider_request_id, metadata_json,
    record_id, call_reason, call_loc, run_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17,
    $18, $19, $20, $21, $22::jsonb,
    $23, $24, $25, $26
)`

	var recordID any
	if record.RecordID > 0 {
		recordID = record.RecordID
	}
	var runID any
	if record.RunID > 0 {
		runID = record.RunID
	}
	var accountID any
	if record.AccountID != "" {
		accountID = record.AccountID
	}
	var profileID any
	if record.ProfileID != "" {
		profileID = record.ProfileID
	}
	var userID any
	if strings.TrimSpace(record.UserID) != "" {
		userID = strings.TrimSpace(record.UserID)
	}

	_, err = s.DB.ExecContext(
		ctx,
		stmt,
		eventID,
		accountID,
		profileID,
		userID,
		string(record.Provider),
		record.ModelName,
		record.PromptName,
		startedAt,
		finishedAt,
		workspaceDay,
		int64(record.InputTokens),
		int64(record.OutputTokens),
		int64(record.TotalTokens),
		int64(record.PromptCacheHitTokens),
		int64(record.PromptCacheMissTokens),
		latencyMS,
		s.DefaultStatus,
		record.ErrorMessage,
		inputRef,
		outputRef,
		record.ProviderRequestID,
		string(metadataJSON),
		recordID,
		record.CallReason,
		record.CallLoc,
		runID,
	)
	if err != nil {
		return "", err
	}
	if userID == nil {
		s.alarmForMissingUserID(ctx, record)
	}
	return eventID, nil
}

// alarmForMissingUserID raises an operator alarm (alarms_errors, surfaced
// alongside every other alarm on /semos/admin/alarms) whenever a row is
// inserted into llm_usage_event with no user_id -- the backstop for every
// caller, known or not yet audited, that still fails to supply one upstream
// (doc-processing's own generators are expected to have already alarmed at
// publish time, see docprocessing.RoutingAlarmKindMissingUserID). Dedup'd by
// run_id/record_id where available, same as routing alarms, so a hot loop of
// unattributed calls in one run doesn't flood the alarms table. Write
// failures are logged only -- an alarms-table outage must not affect usage
// capture, which has already succeeded by this point.
func (s *Sink) alarmForMissingUserID(ctx context.Context, record sharedllm.UsageCaptureRecord) {
	if s.DB == nil {
		return
	}
	const kind = "missing_user_id"
	message := "llm_usage_event inserted with no user_id; call_loc=" + record.CallLoc + " call_reason=" + record.CallReason
	var err error
	switch {
	case record.RunID != 0:
		_, err = s.DB.ExecContext(ctx, `INSERT INTO alarms_errors (severity, message, run_id, kind) VALUES ('warning',$1,$2,$3)
ON CONFLICT (run_id, kind) WHERE run_id IS NOT NULL AND kind IS NOT NULL DO NOTHING`, message, record.RunID, kind)
	case record.RecordID != 0:
		_, err = s.DB.ExecContext(ctx, `INSERT INTO alarms_errors (severity, message, record_id, kind) VALUES ('warning',$1,$2,$3)
ON CONFLICT (record_id, kind) WHERE run_id IS NULL AND record_id IS NOT NULL AND kind IS NOT NULL DO NOTHING`, message, record.RecordID, kind)
	default:
		_, err = s.DB.ExecContext(ctx, `INSERT INTO alarms_errors (severity, message, kind) VALUES ('warning',$1,$2)`, message, kind)
	}
	if err != nil {
		sinkLogger.Warn("failed writing missing-user-id alarm", "error", err, "call_loc", record.CallLoc)
	}
}

func (s *Sink) resolveAccountProfileIDs(ctx context.Context, record sharedllm.UsageCaptureRecord) (accountID string, profileID string, err error) {
	if s.DB == nil {
		return "", "", nil
	}
	provider := strings.TrimSpace(string(record.Provider))
	baseURL := strings.TrimSpace(record.BaseURL)
	apiKey := strings.TrimSpace(record.APIKey)
	profileName := strings.TrimSpace(record.ProfileName)
	modelName := strings.TrimSpace(record.ModelName)
	if provider == "" || baseURL == "" || apiKey == "" || (profileName == "" && modelName == "") {
		return "", "", nil
	}

	const profileQuery = `SELECT a.id, p.id
FROM llm_account a
JOIN llm_account_model_profile p ON p.account_id = a.id
WHERE LOWER(a.provider) = LOWER($1)
  AND LOWER(TRIM(TRAILING '/' FROM a.base_url)) = LOWER(TRIM(TRAILING '/' FROM $2))
  AND a.api_key_ref = $3
  AND LOWER(p.profile_name) = LOWER($4)
LIMIT 1`

	if profileName != "" {
		if err := s.DB.QueryRowContext(ctx, profileQuery, provider, baseURL, apiKey, profileName).Scan(&accountID, &profileID); err == nil {
			return accountID, profileID, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return "", "", err
		}
	}

	if modelName == "" {
		return "", "", nil
	}

	const modelQuery = `SELECT a.id, p.id
FROM llm_account a
JOIN llm_account_model_profile p ON p.account_id = a.id
WHERE LOWER(a.provider) = LOWER($1)
  AND LOWER(TRIM(TRAILING '/' FROM a.base_url)) = LOWER(TRIM(TRAILING '/' FROM $2))
  AND a.api_key_ref = $3
  AND LOWER(p.model_name) = LOWER($4)
LIMIT 1`

	if err := s.DB.QueryRowContext(ctx, modelQuery, provider, baseURL, apiKey, modelName).Scan(&accountID, &profileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", nil
		}
		return "", "", err
	}
	return accountID, profileID, nil
}

func InstallDefaultSink() error {
	llmCfg := config.GetLLMConfig()
	loc, err := time.LoadLocation(llmCfg.WorkspaceTimezone)
	if err != nil {
		return err
	}
	sharedllm.DefaultUsageCaptureSink = &Sink{
		DB:            ApiTypes.ProjectDBHandle,
		ArchiveRoot:   llmCfg.ArchiveRoot,
		WorkspaceTZ:   loc,
		DefaultStatus: 0,
	}
	return nil
}

func archiveRef(root, fullPath string) string {
	if root == "" {
		return filepath.Clean(fullPath)
	}
	rel, err := filepath.Rel(root, fullPath)
	if err != nil {
		return filepath.Clean(fullPath)
	}
	return filepath.Clean(rel)
}
