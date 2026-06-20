package llmadminhandler

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/llmimport"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

type accountAdminStore interface {
	ListAccounts(ctx context.Context) ([]Account, error)
	CreateAccount(ctx context.Context, in CreateAccountInput) (Account, error)
	UpdateAccount(ctx context.Context, id string, in CreateAccountInput) (Account, error)
	ImportParsedModels(ctx context.Context, parsed llmimport.ParsedModels) (ImportResult, error)
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
