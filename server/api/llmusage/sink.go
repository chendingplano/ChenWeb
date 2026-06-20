package llmusage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	sharedllm "github.com/chendingplano/shared/go/api/llm"
)

type Sink struct {
	DB            *sql.DB
	ArchiveRoot   string
	WorkspaceTZ   *time.Location
	NewID         func() string
	Now           func() time.Time
	DefaultStatus int
}

func (s *Sink) Capture(ctx context.Context, record sharedllm.UsageCaptureRecord) error {
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
			return err
		}
		if record.AccountID == "" {
			record.AccountID = accountID
		}
		if record.ProfileID == "" {
			record.ProfileID = profileID
		}
	}

	paths := sharedllm.BuildUsageArchivePaths(s.ArchiveRoot, workspaceDay, record.AccountID, eventID)
	inputRef := ""
	outputRef := ""

	if len(record.InputBody) > 0 {
		if err := sharedllm.WriteGzipFile(paths.InputBodyPath, record.InputBody); err != nil {
			return err
		}
		inputRef = archiveRef(s.ArchiveRoot, paths.InputBodyPath)
	}
	if len(record.OutputBody) > 0 {
		if err := sharedllm.WriteGzipFile(paths.OutputBodyPath, record.OutputBody); err != nil {
			return err
		}
		outputRef = archiveRef(s.ArchiveRoot, paths.OutputBodyPath)
	}

	if s.DB == nil || record.AccountID == "" || record.ProfileID == "" {
		return nil
	}

	latencyMS := finishedAt.Sub(startedAt).Milliseconds()
	metadataJSON, err := json.Marshal(map[string]any{
		"capture_source": "shared_llm",
	})
	if err != nil {
		return err
	}

	const stmt = `INSERT INTO llm_usage_event (
    id, account_id, profile_id, provider, model_name, prompt_name,
    request_started_at, request_finished_at, workspace_day,
    input_tokens, output_tokens, total_tokens, latency_ms, http_status,
    error_message, input_body_ref, output_body_ref, provider_request_id, metadata_json
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9,
    $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19::jsonb
)`

	_, err = s.DB.ExecContext(
		ctx,
		stmt,
		eventID,
		record.AccountID,
		record.ProfileID,
		string(record.Provider),
		record.ModelName,
		record.PromptName,
		startedAt,
		finishedAt,
		workspaceDay,
		int64(record.InputTokens),
		int64(record.OutputTokens),
		int64(record.TotalTokens),
		latencyMS,
		s.DefaultStatus,
		record.ErrorMessage,
		inputRef,
		outputRef,
		record.ProviderRequestID,
		string(metadataJSON),
	)
	return err
}

func (s *Sink) resolveAccountProfileIDs(ctx context.Context, record sharedllm.UsageCaptureRecord) (accountID string, profileID string, err error) {
	if s.DB == nil {
		return "", "", nil
	}
	provider := strings.TrimSpace(string(record.Provider))
	baseURL := strings.TrimSpace(record.BaseURL)
	apiKey := strings.TrimSpace(record.APIKey)
	profileName := strings.TrimSpace(record.ProfileName)
	if provider == "" || baseURL == "" || apiKey == "" || profileName == "" {
		return "", "", nil
	}

	const query = `SELECT a.id, p.id
FROM llm_account a
JOIN llm_account_model_profile p ON p.account_id = a.id
WHERE LOWER(a.provider) = LOWER($1)
  AND a.base_url = $2
  AND a.api_key_ref = $3
  AND LOWER(p.profile_name) = LOWER($4)
LIMIT 1`

	if err := s.DB.QueryRowContext(ctx, query, provider, baseURL, apiKey, profileName).Scan(&accountID, &profileID); err != nil {
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
