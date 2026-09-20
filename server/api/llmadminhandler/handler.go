package llmadminhandler

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/llmimport"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

type depositRequest struct {
	AccountID     string  `json:"account_id"`
	CurrencyCode  string  `json:"currency_code"`
	DepositAmount float64 `json:"deposit_amount"`
	BalanceAmount float64 `json:"balance_amount"`
	CapturedAt    string  `json:"captured_at"`
	Note          string  `json:"note"`
}

func AddDeposit(c echo.Context) error {
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": "project database is not initialized"})
	}
	var in depositRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "invalid deposit"})
	}
	if strings.TrimSpace(in.AccountID) == "" || strings.TrimSpace(in.CurrencyCode) == "" || in.DepositAmount <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "account, currency, and a positive deposit amount are required"})
	}
	captured := time.Now().UTC()
	if strings.TrimSpace(in.CapturedAt) != "" {
		parsed, err := time.Parse(time.RFC3339, in.CapturedAt)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"message": "captured_at must be RFC3339"})
		}
		captured = parsed
	}
	_, err := ApiTypes.ProjectDBHandle.ExecContext(c.Request().Context(), `INSERT INTO llm_balance_snapshot (account_id,captured_at,workspace_day,balance_amount,currency_code,capture_source,raw_payload_ref,entry_kind,deposit_amount,note) VALUES ($1,$2,$3,$4,$5,'admin_deposit','', 'deposit',$6,$7)`, in.AccountID, captured, captured, in.BalanceAmount, strings.ToUpper(in.CurrencyCode), in.DepositAmount, strings.TrimSpace(in.Note))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": "failed to save deposit", "error": err.Error()})
	}
	return c.JSON(http.StatusCreated, map[string]any{"ok": true})
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
