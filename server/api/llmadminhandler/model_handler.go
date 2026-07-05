package llmadminhandler

import (
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

type addModelRequest struct {
	ProfileName          string `json:"profile_name"`
	ModelName            string `json:"model_name"`
	ThinkingType         string `json:"thinking_type"`
	TimeoutSec           int    `json:"timeout_sec"`
	MaxInflight          int    `json:"max_inflight"`
	MaxRequestsPerMinute int    `json:"max_requests_per_minute"`
	MaxTokensPerMinute   int    `json:"max_tokens_per_minute"`
	TokenReservePerCall  int    `json:"token_reserve_per_call"`
	Host                 string `json:"host"`
	AccountName          string `json:"account_name"`
	Provider             string `json:"provider"`
	BaseURL              string `json:"base_url"`
	APIKey               string `json:"api_key"`
}

// AddModel implements ADR 2026070501 steps 1–3:
// writes to .models.toml, upserts llm_account, upserts llm_account_model_profile.
func AddModel(c echo.Context) error {
	var req addModelRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "invalid json body",
		})
	}
	req.ProfileName = strings.TrimSpace(req.ProfileName)
	if req.ProfileName == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "profile_name is required",
		})
	}

	// Step 1: write to .models.toml
	tomlPath := modelsTOMLPath()
	if err := UpsertModelsTOMLEntry(tomlPath, req.ProfileName, ApiTypes.LLMModelDef{
		Host:                 req.Host,
		ModelName:            strings.TrimSpace(req.ModelName),
		APIKey:               strings.TrimSpace(req.APIKey),
		BaseURL:              strings.TrimSpace(req.BaseURL),
		TimeoutSec:           req.TimeoutSec,
		ThinkingType:         req.ThinkingType,
		MaxInflight:          req.MaxInflight,
		MaxRequestsPerMinute: req.MaxRequestsPerMinute,
		MaxTokensPerMinute:   req.MaxTokensPerMinute,
		TokenReservePerCall:  req.TokenReservePerCall,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to write .models.toml",
			"error":   err.Error(),
		})
	}

	// Steps 2–3: upsert account and profile in DB
	store := adminStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"ok":      false,
			"message": "project database is not initialized",
		})
	}

	accountName := strings.TrimSpace(req.AccountName)
	if accountName == "" {
		accountName = strings.TrimSpace(req.Provider) + ":" + strings.TrimSpace(req.BaseURL)
	}

	profile, err := store.UpsertAccountAndProfile(c.Request().Context(),
		CreateAccountInput{
			AccountName:             accountName,
			Provider:                strings.TrimSpace(req.Provider),
			BaseURL:                 strings.TrimSpace(req.BaseURL),
			APIKeyRef:               strings.TrimSpace(req.APIKey),
			Status:                  "active",
			ReconciliationKind:      "provider_balance",
			IsReconciliationEnabled: false,
			DefaultModelName:        strings.TrimSpace(req.ModelName),
		},
		CreateProfileInput{
			ProfileName:          req.ProfileName,
			ModelName:            strings.TrimSpace(req.ModelName),
			ThinkingType:         req.ThinkingType,
			TimeoutSec:           req.TimeoutSec,
			MaxInflight:          req.MaxInflight,
			MaxRequestsPerMinute: req.MaxRequestsPerMinute,
			MaxTokensPerMinute:   req.MaxTokensPerMinute,
			TokenReservePerCall:  req.TokenReservePerCall,
			IsActive:             true,
		},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to save model to database",
			"error":   err.Error(),
		})
	}
	return c.JSON(http.StatusCreated, profile)
}
