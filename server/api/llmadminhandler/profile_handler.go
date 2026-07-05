package llmadminhandler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type createProfileRequest struct {
	AccountID            string `json:"account_id"`
	ProfileName          string `json:"profile_name"`
	ModelName            string `json:"model_name"`
	ThinkingType         string `json:"thinking_type"`
	TimeoutSec           int    `json:"timeout_sec"`
	MaxInflight          int    `json:"max_inflight"`
	MaxRequestsPerMinute int    `json:"max_requests_per_minute"`
	MaxTokensPerMinute   int    `json:"max_tokens_per_minute"`
	TokenReservePerCall  int    `json:"token_reserve_per_call"`
	IsActive             bool   `json:"is_active"`
}

func ListProfiles(c echo.Context) error {
	store := adminStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"ok":      false,
			"message": "project database is not initialized",
		})
	}
	profiles, err := store.ListProfiles(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to list model profiles",
			"error":   err.Error(),
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"profiles": profiles,
	})
}

func CreateProfile(c echo.Context) error {
	var req createProfileRequest
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
	profile, err := store.CreateProfile(c.Request().Context(), CreateProfileInput{
		AccountID:            req.AccountID,
		ProfileName:          req.ProfileName,
		ModelName:            req.ModelName,
		ThinkingType:         req.ThinkingType,
		TimeoutSec:           req.TimeoutSec,
		MaxInflight:          req.MaxInflight,
		MaxRequestsPerMinute: req.MaxRequestsPerMinute,
		MaxTokensPerMinute:   req.MaxTokensPerMinute,
		TokenReservePerCall:  req.TokenReservePerCall,
		IsActive:             req.IsActive,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to create model profile",
			"error":   err.Error(),
		})
	}
	return c.JSON(http.StatusCreated, profile)
}

func UpdateProfile(c echo.Context) error {
	var req createProfileRequest
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
	profile, err := store.UpdateProfile(c.Request().Context(), c.Param("id"), CreateProfileInput{
		AccountID:            req.AccountID,
		ProfileName:          req.ProfileName,
		ModelName:            req.ModelName,
		ThinkingType:         req.ThinkingType,
		TimeoutSec:           req.TimeoutSec,
		MaxInflight:          req.MaxInflight,
		MaxRequestsPerMinute: req.MaxRequestsPerMinute,
		MaxTokensPerMinute:   req.MaxTokensPerMinute,
		TokenReservePerCall:  req.TokenReservePerCall,
		IsActive:             req.IsActive,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to update model profile",
			"error":   err.Error(),
		})
	}
	return c.JSON(http.StatusOK, profile)
}
