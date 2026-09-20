package llmreporthandler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/llmreconcile"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	sharedllm "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
	toml "github.com/pelletier/go-toml/v2"
)

type reportStore interface {
	ListDailyReports(ctx context.Context, limit int) ([]DailyReport, error)
	ListModelActivityReports(ctx context.Context, limit int, filters ModelActivityReportFilters) ([]ModelActivityReport, error)
	ListUsageEvents(ctx context.Context, limit int) ([]UsageEvent, error)
	ListCurrentBalances(ctx context.Context, limit int) ([]CurrentBalance, error)
	ListBalanceHistory(ctx context.Context, limit int) ([]BalanceHistory, error)
	GetTodaySummary(ctx context.Context, workspaceDay time.Time, timezoneName string) (TodaySummary, error)
	ListUsageEventsAdmin(ctx context.Context, page, pageSize int, filters UsageEventAdminFilters) ([]UsageEventAdmin, int64, error)
	GetUsageEventBodyRefs(ctx context.Context, id string) (inputRef, outputRef string, err error)
	ListUsageEventsByIDs(ctx context.Context, ids []string) ([]UsageEventAdmin, error)
}

type reconciliationRunner interface {
	Run(ctx context.Context) error
	RunWithResult(ctx context.Context) (llmreconcile.RunResult, error)
}

type usageReportRunner interface {
	Run(ctx context.Context) error
	RunWithResult(ctx context.Context) (DailyUsageRunResult, error)
}

var reportStoreFactory = func() reportStore {
	if ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	return NewStore(ApiTypes.ProjectDBHandle)
}

var reconciliationRunnerFactory = func() reconciliationRunner {
	if ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	llmCfg := config.GetLLMConfig()
	loc, err := time.LoadLocation(llmCfg.WorkspaceTimezone)
	if err != nil {
		return nil
	}
	return &llmreconcile.Runner{
		Store:        llmreconcile.NewStore(ApiTypes.ProjectDBHandle),
		BalanceAPI:   &llmreconcile.DeepSeekBalanceClient{},
		ArchiveRoot:  llmCfg.ArchiveRoot,
		WorkspaceTZ:  loc,
		TimezoneName: llmCfg.WorkspaceTimezone,
	}
}

var usageReportRunnerFactory = func() usageReportRunner {
	if ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	llmCfg := config.GetLLMConfig()
	loc, err := time.LoadLocation(llmCfg.WorkspaceTimezone)
	if err != nil {
		return nil
	}
	return &DailyUsageReportRunner{
		DB:           ApiTypes.ProjectDBHandle,
		WorkspaceTZ:  loc,
		TimezoneName: llmCfg.WorkspaceTimezone,
		RunHour:      llmCfg.ReconciliationRunHour,
	}
}

func ListDailyReports(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	limit := intParamDefault(c.QueryParam("limit"), 30)
	rows, err := store.ListDailyReports(c.Request().Context(), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list daily llm reports", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"reports": rows})
}

func ListUsageEvents(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	limit := intParamDefault(c.QueryParam("limit"), 50)
	rows, err := store.ListUsageEvents(c.Request().Context(), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list llm usage events", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"usage_events": rows})
}

func ListModelActivityReports(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	limit := intParamDefault(c.QueryParam("limit"), 30)
	filters, err := parseModelActivityReportFilters(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
	}
	apiKeys, keyRefs, keyModels, err := loadModelAPIKeyOptions()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to load model API keys", "error": err.Error()})
	}
	if filters.APIKeyRef, err = apiKeyRefForName(c.QueryParam("api_key"), keyRefs); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
	}
	selectedAPIKeyName := strings.TrimSpace(c.QueryParam("api_key"))
	filters.ModelNames = keyModels[selectedAPIKeyName]
	rows, err := store.ListModelActivityReports(c.Request().Context(), limit, filters)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list llm model activity reports", "error": err.Error()})
	}
	refToName := make(map[string]string, len(keyRefs))
	for name, ref := range keyRefs {
		if _, exists := refToName[ref]; !exists {
			refToName[ref] = name
		}
	}
	for i := range rows {
		applyDeepSeekLocalCNYPricing(&rows[i])
		if selectedAPIKeyName != "" {
			rows[i].APIKeyName = selectedAPIKeyName
			continue
		}
		if name, ok := refToName[rows[i].APIKeyName]; ok {
			rows[i].APIKeyName = name
		} else {
			rows[i].APIKeyName = "Unknown"
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"reports": rows, "api_keys": apiKeys})
}

type deepSeekCNYPricing struct {
	InputCacheHit  map[string]string `toml:"input-cache-hit"`
	InputCacheMiss map[string]string `toml:"input-cache-miss"`
	Output         map[string]string `toml:"output"`
}

// applyDeepSeekLocalCNYPricing intentionally applies only to Flash. The
// current config has one generic DeepSeek CNY rate card; Pro remains unknown
// until it has an explicit model-specific rate rather than borrowing Flash's.
func applyDeepSeekLocalCNYPricing(row *ModelActivityReport) {
	if !strings.EqualFold(strings.TrimSpace(row.Provider), "deepseek") || !strings.Contains(strings.ToLower(row.ModelName), "flash") {
		return
	}
	path := strings.TrimSpace(os.Getenv("CHENWEB_MODELS_TOML"))
	if path == "" {
		path = filepath.Join(".", ".models.toml")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var config struct {
		DeepSeekBilling struct {
			ChineseYuan deepSeekCNYPricing `toml:"chinese-yuan"`
		} `toml:"deepseek-billing"`
	}
	if toml.Unmarshal(raw, &config) != nil {
		return
	}
	parse := func(values map[string]string) float64 {
		value, _ := strconv.ParseFloat(strings.TrimSpace(values["off-peak"]), 64)
		return value
	}
	hit, miss, output := parse(config.DeepSeekBilling.ChineseYuan.InputCacheHit), parse(config.DeepSeekBilling.ChineseYuan.InputCacheMiss), parse(config.DeepSeekBilling.ChineseYuan.Output)
	if hit == 0 && miss == 0 && output == 0 {
		return
	}
	row.CurrencyCode = "CNY"
	row.SpendAmount = (float64(row.PromptCacheHitTokens)*hit + float64(row.PromptCacheMissTokens)*miss + float64(row.OutputTokens)*output) / 1_000_000
}

type modelAPIKeyEntry struct {
	APIKey string `toml:"api_key"`
}

type modelAPIKeyConfig struct {
	ModelAPIKeys map[string][]modelAPIKeyEntry `toml:"model-api-keys"`
	Profiles     map[string]modelProfile
}

type modelProfile struct {
	ModelName string
	APIKey    string
}

type ModelAPIKeyOption struct {
	Name string `json:"name"`
}

func parseModelActivityReportFilters(c echo.Context) (ModelActivityReportFilters, error) {
	filters := ModelActivityReportFilters{}
	for value, target := range map[string]**time.Time{
		"from": &filters.From,
		"to":   &filters.To,
	} {
		raw := strings.TrimSpace(c.QueryParam(value))
		if raw == "" {
			continue
		}
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return filters, fmt.Errorf("%s must be a YYYY-MM-DD date", value)
		}
		*target = &parsed
	}
	if filters.From != nil && filters.To != nil && filters.From.After(*filters.To) {
		return filters, fmt.Errorf("from date must not be after to date")
	}
	return filters, nil
}

func loadModelAPIKeyOptions() ([]ModelAPIKeyOption, map[string]string, map[string][]string, error) {
	path := strings.TrimSpace(os.Getenv("CHENWEB_MODELS_TOML"))
	if path == "" {
		path = filepath.Join(".", ".models.toml")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ModelAPIKeyOption{}, map[string]string{}, map[string][]string{}, nil
		}
		return nil, nil, nil, err
	}
	var config modelAPIKeyConfig
	looseConfig, looseErr := parseLooseModelAPIKeys(raw)
	if err := toml.Unmarshal(raw, &config); err != nil {
		if looseErr != nil {
			return nil, nil, nil, err
		}
		config = looseConfig
	} else if looseErr == nil {
		config.Profiles = looseConfig.Profiles
	}
	keys := make([]string, 0, len(config.ModelAPIKeys))
	refs := make(map[string]string, len(config.ModelAPIKeys))
	for name, entries := range config.ModelAPIKeys {
		if len(entries) == 0 || strings.TrimSpace(entries[0].APIKey) == "" {
			continue
		}
		keys = append(keys, name)
		refs[name] = strings.TrimSpace(entries[0].APIKey)
	}
	sort.Strings(keys)
	options := make([]ModelAPIKeyOption, 0, len(keys))
	for _, name := range keys {
		options = append(options, ModelAPIKeyOption{Name: name})
	}
	modelNames := make(map[string][]string, len(keys))
	for _, name := range keys {
		modelNames[name] = modelNamesForAPIKey(name, refs[name], config.Profiles)
	}
	return options, refs, modelNames, nil
}

func parseLooseModelAPIKeys(raw []byte) (modelAPIKeyConfig, error) {
	section := false
	var current string
	config := modelAPIKeyConfig{
		ModelAPIKeys: map[string][]modelAPIKeyEntry{},
		Profiles:     map[string]modelProfile{},
	}
	namePattern := regexp.MustCompile(`^\s*([A-Za-z0-9_-]+)\s*=\s*\[\s*$`)
	keyPattern := regexp.MustCompile(`^\s*api_key\s*=\s*['"]([^'"]+)['"]`)
	valuePattern := regexp.MustCompile(`^\s*(model_name|api_key)\s*=\s*['"]([^'"]+)['"]`)
	sectionName := regexp.MustCompile(`^\[([^]]+)\]$`)
	currentSection := ""
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if matches := sectionName.FindStringSubmatch(trimmed); len(matches) == 2 {
			currentSection = matches[1]
			section = currentSection == "model-api-keys"
			current = ""
			continue
		}
		if section {
			if matches := namePattern.FindStringSubmatch(line); len(matches) == 2 {
				current = matches[1]
				continue
			}
			if current != "" {
				if matches := keyPattern.FindStringSubmatch(line); len(matches) == 2 {
					config.ModelAPIKeys[current] = []modelAPIKeyEntry{{APIKey: matches[1]}}
					current = ""
				}
			}
			continue
		}
		if currentSection != "" {
			if matches := valuePattern.FindStringSubmatch(line); len(matches) == 3 {
				profile := config.Profiles[currentSection]
				if matches[1] == "model_name" {
					profile.ModelName = matches[2]
				} else {
					profile.APIKey = matches[2]
				}
				config.Profiles[currentSection] = profile
			}
		}
	}
	return config, nil
}

func modelNamesForAPIKey(keyName, apiKey string, profiles map[string]modelProfile) []string {
	keyTokens := normalizedTokens(keyName)
	bestScore := 0
	best := make([]string, 0)
	for profileName, profile := range profiles {
		if profile.APIKey != apiKey || profile.ModelName == "" {
			continue
		}
		score := tokenOverlap(keyTokens, normalizedTokens(profileName))
		score += tokenOverlap(keyTokens, normalizedTokens(profile.ModelName))
		if score > bestScore {
			bestScore = score
			best = []string{profile.ModelName}
		} else if score > 0 && score == bestScore {
			best = append(best, profile.ModelName)
		}
	}
	sort.Strings(best)
	return best
}

func normalizedTokens(value string) []string {
	raw := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for i, token := range raw {
		if strings.HasPrefix(token, "v") && len(token) > 1 && strings.Trim(token[1:], "0123456789") == "" {
			raw[i] = token[1:]
		}
	}
	return raw
}

func tokenOverlap(left, right []string) int {
	set := make(map[string]struct{}, len(right))
	for _, token := range right {
		set[token] = struct{}{}
	}
	score := 0
	for _, token := range left {
		if _, ok := set[token]; ok {
			score++
		}
	}
	return score
}

func apiKeyRefForName(name string, refs map[string]string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	ref, ok := refs[name]
	if !ok {
		return "", fmt.Errorf("unknown API key %q", name)
	}
	return ref, nil
}

func ListCurrentBalances(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	limit := intParamDefault(c.QueryParam("limit"), 20)
	rows, err := store.ListCurrentBalances(c.Request().Context(), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list current llm balances", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"balances": rows})
}

// ListBalanceHistory serves the official provider balance track. It is kept
// separate from locally calculated, per-model token costs.
func ListBalanceHistory(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	limit := intParamDefault(c.QueryParam("limit"), 24*14)
	rows, err := store.ListBalanceHistory(c.Request().Context(), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list official balance history", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"balances": rows})
}

func GetTodaySummary(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	llmCfg := config.GetLLMConfig()
	loc, err := time.LoadLocation(llmCfg.WorkspaceTimezone)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to load workspace timezone", "error": err.Error()})
	}
	now := time.Now().In(loc)
	workspaceDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	summary, err := store.GetTodaySummary(c.Request().Context(), workspaceDay, llmCfg.WorkspaceTimezone)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to load today's llm summary", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"summary": summary})
}

func RunReconciliationNow(c echo.Context) error {
	usageRunner := usageReportRunnerFactory()
	runner := reconciliationRunnerFactory()
	if runner == nil || usageRunner == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "llm reconciliation is not available"})
	}

	usageResult, err := usageRunner.RunWithResult(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to generate daily usage reports", "error": err.Error()})
	}

	reconcileResult, err := runner.RunWithResult(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to run llm reconciliation", "error": err.Error()})
	}

	message := "Manual LLM run finished."
	if usageResult.RowsAffected == 0 && reconcileResult.ReportsReconciled == 0 {
		message = "Manual LLM run finished, but no visible daily report rows were created yet. This usually means there are no captured usage events yet, or DeepSeek only has its first balance snapshot so there is not enough history to reconcile yesterday."
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":                   true,
		"message":              message,
		"usage_days_processed": usageResult.DaysProcessed,
		"usage_rows_affected":  usageResult.RowsAffected,
		"accounts_considered":  reconcileResult.AccountsConsidered,
		"snapshots_created":    reconcileResult.SnapshotsCreated,
		"reports_reconciled":   reconcileResult.ReportsReconciled,
	})
}

func ListUsageEventsAdmin(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	page := intParamDefault(c.QueryParam("page"), 1)
	pageSize := intParamDefault(c.QueryParam("page_size"), 50)
	filters := UsageEventAdminFilters{
		Model:      c.QueryParam("model"),
		Prompt:     c.QueryParam("prompt"),
		CallReason: c.QueryParam("call_reason"),
		CallLoc:    c.QueryParam("call_loc"),
		MetaKey:    c.QueryParam("meta_key"),
		MetaValue:  c.QueryParam("meta_value"),
	}
	if v, ok := int64Param(c.QueryParam("run_id")); ok {
		filters.RunID = &v
	}
	if v, ok := timeParam(c.QueryParam("started_from")); ok {
		filters.StartedFrom = &v
	}
	if v, ok := timeParam(c.QueryParam("started_to")); ok {
		filters.StartedTo = &v
	}
	if v, ok := int64Param(c.QueryParam("in_tok_min")); ok {
		filters.InTokMin = &v
	}
	if v, ok := int64Param(c.QueryParam("in_tok_max")); ok {
		filters.InTokMax = &v
	}
	if v, ok := int64Param(c.QueryParam("out_tok_min")); ok {
		filters.OutTokMin = &v
	}
	if v, ok := int64Param(c.QueryParam("out_tok_max")); ok {
		filters.OutTokMax = &v
	}
	rows, total, err := store.ListUsageEventsAdmin(c.Request().Context(), page, pageSize, filters)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list llm usage events", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"events": rows, "total": total, "page": page, "page_size": pageSize})
}

// GetUsageEventsByIDs returns llm_usage_event rows for a comma-separated list
// of ids, e.g. the ids recorded in kb.doc_review_logs.detail.llm_usage_event_ids.
func GetUsageEventsByIDs(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	var ids []string
	for id := range strings.SplitSeq(c.QueryParam("ids"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	rows, err := store.ListUsageEventsByIDs(c.Request().Context(), ids)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to fetch llm usage events", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"usage_events": rows})
}

func GetUsageEventBody(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	id := c.Param("id")
	bodyType := c.QueryParam("type")
	if bodyType != "input" && bodyType != "output" {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "type must be 'input' or 'output'"})
	}
	inputRef, outputRef, err := store.GetUsageEventBodyRefs(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "event not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to fetch event", "error": err.Error()})
	}
	ref := inputRef
	if bodyType == "output" {
		ref = outputRef
	}
	if ref == "" {
		return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "body not archived for this event"})
	}
	archiveRoot := config.GetLLMConfig().ArchiveRoot
	fullPath := filepath.Join(archiveRoot, ref)
	data, err := sharedllm.ReadGzipFile(fullPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to read archive file", "error": err.Error()})
	}
	return c.JSONBlob(http.StatusOK, data)
}

func intParamDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// timeParam parses an RFC3339 timestamp query param. ok is false if raw is empty or invalid.
func timeParam(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// int64Param parses an integer query param. ok is false if raw is empty or invalid.
func int64Param(raw string) (int64, bool) {
	if raw == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
