package llmadminhandler

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/llmimport"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
	toml "github.com/pelletier/go-toml/v2"
)

type depositRequest struct {
	APIKeyName    string  `json:"api_key_name"`
	CurrencyCode  string  `json:"currency_code"`
	DepositAmount float64 `json:"deposit_amount"`
	BalanceAmount float64 `json:"balance_amount"`
	CapturedAt    string  `json:"captured_at"`
	Note          string  `json:"note"`
}

func AddDeposit(c echo.Context) error {
	return addManualEntry(c, "deposit")
}
func SetTotalSpending(c echo.Context) error {
	return addManualEntry(c, "set-total-spending")
}
func addManualEntry(c echo.Context, entryKind string) error {
	var in depositRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "invalid deposit"})
	}
	if strings.TrimSpace(in.APIKeyName) == "" || strings.TrimSpace(in.CurrencyCode) == "" || in.DepositAmount <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "API key, currency, and a positive deposit amount are required"})
	}
	apiKeyRefs, err := loadDepositAPIKeyRefs()
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "failed to read configured API keys", "error": err.Error()})
	}
	apiKeyRef, ok := apiKeyRefs[strings.TrimSpace(in.APIKeyName)]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "selected API key is not configured"})
	}
	store := adminStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": "project database is not initialized"})
	}
	accountID, err := store.FindAccountIDByAPIKeyRef(c.Request().Context(), apiKeyRef)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": "failed to find account for API key", "error": err.Error()})
	}
	if accountID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "no matching account exists for the selected API key"})
	}
	captured := time.Now().UTC()
	if strings.TrimSpace(in.CapturedAt) != "" {
		parsed, err := time.Parse(time.RFC3339, in.CapturedAt)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"message": "captured_at must be RFC3339"})
		}
		captured = parsed
	}
	if err := store.AddDeposit(c.Request().Context(), depositRecord{AccountID: accountID, CapturedAt: captured, CurrencyCode: strings.ToUpper(in.CurrencyCode), DepositAmount: in.DepositAmount, BalanceAmount: in.BalanceAmount, Note: strings.TrimSpace(in.Note), EntryKind: entryKind}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": "failed to save deposit", "error": err.Error()})
	}
	return c.JSON(http.StatusCreated, map[string]any{"ok": true})
}

func ListManualRecords(c echo.Context) error {
	store := adminStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": "project database is not initialized"})
	}
	rows, err := store.ListManualRecords(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": "failed to list manual records"})
	}
	refs, err := loadDepositAPIKeyRefs()
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "failed to read configured API keys"})
	}
	names := map[string]string{}
	for name, ref := range refs {
		names[ref] = name
	}
	for i := range rows {
		rows[i].APIKeyName = names[rows[i].APIKeyRef]
		rows[i].APIKeyRef = ""
	}
	return c.JSON(http.StatusOK, map[string]any{"records": rows})
}

type modelAPIKeyConfig struct {
	ModelAPIKeys map[string][]struct {
		APIKey string `toml:"api_key"`
	} `toml:"model-api-keys"`
}

func loadDepositAPIKeyRefs() (map[string]string, error) {
	path := strings.TrimSpace(os.Getenv("CHENWEB_MODELS_TOML"))
	if path == "" {
		path = filepath.Join(".", ".models.toml")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config modelAPIKeyConfig
	if err := toml.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	refs := make(map[string]string, len(config.ModelAPIKeys))
	for name, entries := range config.ModelAPIKeys {
		if len(entries) == 0 || strings.TrimSpace(entries[0].APIKey) == "" {
			continue
		}
		refs[name] = strings.TrimSpace(entries[0].APIKey)
	}
	return refs, nil
}

func ListDepositAPIKeys(c echo.Context) error {
	refs, err := loadDepositAPIKeyRefs()
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "failed to read configured API keys", "error": err.Error()})
	}
	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	sort.Strings(names)
	return c.JSON(http.StatusOK, map[string]any{"api_keys": names})
}

type accountAdminStore interface {
	ListAccounts(ctx context.Context) ([]Account, error)
	CreateAccount(ctx context.Context, in CreateAccountInput) (Account, error)
	UpdateAccount(ctx context.Context, id string, in CreateAccountInput) (Account, error)
	ImportParsedModels(ctx context.Context, parsed llmimport.ParsedModels) (ImportResult, error)
	ListProfiles(ctx context.Context) ([]ModelProfile, error)
	CreateProfile(ctx context.Context, in CreateProfileInput) (ModelProfile, error)
	UpdateProfile(ctx context.Context, id string, in CreateProfileInput) (ModelProfile, error)
	UpsertAccountAndProfile(ctx context.Context, accountIn CreateAccountInput, profileIn CreateProfileInput) (ModelProfile, error)
	FindAccountIDByAPIKeyRef(ctx context.Context, apiKeyRef string) (string, error)
	AddDeposit(ctx context.Context, in depositRecord) error
	ListManualRecords(ctx context.Context) ([]ManualRecord, error)
}

var adminStoreFactory = func() accountAdminStore {
	if ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	return NewStore(ApiTypes.ProjectDBHandle)
}

type createAccountRequest struct {
	AccountName             string `json:"account_name"`
	Provider                string `json:"provider"`
	BaseURL                 string `json:"base_url"`
	APIKey                  string `json:"api_key"`
	Status                  string `json:"status"`
	ReconciliationKind      string `json:"reconciliation_kind"`
	IsReconciliationEnabled bool   `json:"is_reconciliation_enabled"`
	DefaultModelName        string `json:"default_model_name"`
}

func ImportModelsTOMLPreview(c echo.Context) error {
	path := strings.TrimSpace(os.Getenv("CHENWEB_MODELS_TOML"))
	if path == "" {
		path = filepath.Join(".", ".models.toml")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "failed to read .models.toml",
			"path":    path,
			"error":   err.Error(),
		})
	}

	parsed, err := llmimport.ParseModelsTOML(raw)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "failed to parse .models.toml",
			"path":    path,
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":       true,
		"path":     path,
		"accounts": parsed.Accounts,
		"profiles": parsed.Profiles,
	})
}

func ImportModelsTOMLApply(c echo.Context) error {
	store := adminStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"ok":      false,
			"message": "project database is not initialized",
		})
	}

	path := strings.TrimSpace(os.Getenv("CHENWEB_MODELS_TOML"))
	if path == "" {
		path = filepath.Join(".", ".models.toml")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "failed to read .models.toml",
			"path":    path,
			"error":   err.Error(),
		})
	}

	parsed, err := llmimport.ParseModelsTOML(raw)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "failed to parse .models.toml",
			"path":    path,
			"error":   err.Error(),
		})
	}

	result, err := store.ImportParsedModels(c.Request().Context(), parsed)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to import .models.toml",
			"path":    path,
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":                true,
		"path":              path,
		"accounts_imported": result.AccountsImported,
		"profiles_imported": result.ProfilesImported,
	})
}

func ListAccounts(c echo.Context) error {
	store := adminStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"ok":      false,
			"message": "project database is not initialized",
		})
	}
	accounts, err := store.ListAccounts(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to list llm accounts",
			"error":   err.Error(),
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"accounts": accounts,
	})
}

func CreateAccount(c echo.Context) error {
	var req createAccountRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "invalid json body",
		})
	}

	store := adminStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"ok":      false,
			"message": "project database is not initialized",
		})
	}
	account, err := store.CreateAccount(c.Request().Context(), CreateAccountInput{
		AccountName:             req.AccountName,
		Provider:                req.Provider,
		BaseURL:                 req.BaseURL,
		APIKeyRef:               req.APIKey,
		Status:                  req.Status,
		ReconciliationKind:      req.ReconciliationKind,
		IsReconciliationEnabled: req.IsReconciliationEnabled,
		DefaultModelName:        req.DefaultModelName,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to create llm account",
			"error":   err.Error(),
		})
	}
	return c.JSON(http.StatusCreated, account)
}

func UpdateAccount(c echo.Context) error {
	var req createAccountRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "invalid json body",
		})
	}

	store := adminStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"ok":      false,
			"message": "project database is not initialized",
		})
	}

	account, err := store.UpdateAccount(c.Request().Context(), c.Param("id"), CreateAccountInput{
		AccountName:             req.AccountName,
		Provider:                req.Provider,
		BaseURL:                 req.BaseURL,
		APIKeyRef:               req.APIKey,
		Status:                  req.Status,
		ReconciliationKind:      req.ReconciliationKind,
		IsReconciliationEnabled: req.IsReconciliationEnabled,
		DefaultModelName:        req.DefaultModelName,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to update llm account",
			"error":   err.Error(),
		})
	}
	return c.JSON(http.StatusOK, account)
}
