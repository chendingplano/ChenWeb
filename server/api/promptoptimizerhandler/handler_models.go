package promptoptimizerhandler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/security"
	"github.com/labstack/echo/v4"
)

// ListModels handles GET /api/v1/prompt-optimizer/models
func ListModels(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_100")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_101")
	defer rc.Close()
	logger := rc.GetLogger()

	models, err := listModelsForUser(ApiTypes.ProjectDBHandle, userID)
	if err != nil {
		logger.Error("list models failed", "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to list models (CWB_POH_102)")
	}
	return c.JSON(http.StatusOK, ModelListResponse{Status: true, Models: models})
}

// CreateModel handles POST /api/v1/prompt-optimizer/models
func CreateModel(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_110")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_111")
	defer rc.Close()
	logger := rc.GetLogger()

	var req CreateModelRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid request body (CWB_POH_112)")
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Provider = strings.TrimSpace(req.Provider)
	req.Model = strings.TrimSpace(req.Model)
	req.BaseURL = strings.TrimSpace(req.BaseURL)

	if req.Name == "" || req.Model == "" {
		return jsonError(c, http.StatusBadRequest, "name and model are required (CWB_POH_113)")
	}
	if !validProviders[req.Provider] {
		return jsonError(c, http.StatusBadRequest, "unsupported provider (CWB_POH_114)")
	}
	if req.Provider == "openai_compatible" && req.BaseURL == "" {
		return jsonError(c, http.StatusBadRequest, "base_url is required for openai_compatible (CWB_POH_115)")
	}
	if req.APIKey == "" {
		return jsonError(c, http.StatusBadRequest, "api_key is required (CWB_POH_116)")
	}

	key, err := getKey()
	if err != nil {
		logger.Error("encryption key not initialized", "err", err)
		return jsonError(c, http.StatusServiceUnavailable, "server not initialized (CWB_POH_117)")
	}
	ciphertext, err := security.EncryptString(req.APIKey, key)
	if err != nil {
		logger.Error("encrypt api key failed", "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to secure api key (CWB_POH_118)")
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	id, err := insertModel(ApiTypes.ProjectDBHandle,
		userID, req.Name, req.Provider, req.BaseURL, req.Model, ciphertext,
		req.DefaultParams, enabled,
	)
	if err != nil {
		logger.Error("insert model failed", "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to create model (CWB_POH_119)")
	}

	m, err := getModelForUser(ApiTypes.ProjectDBHandle, userID, id)
	if err != nil {
		logger.Error("reload model failed", "id", id, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to read new model (CWB_POH_119a)")
	}
	return c.JSON(http.StatusCreated, ModelResponse{Status: true, Model: m})
}

// UpdateModel handles PUT /api/v1/prompt-optimizer/models/:id
func UpdateModel(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_120")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_121")
	defer rc.Close()
	logger := rc.GetLogger()

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return jsonError(c, http.StatusBadRequest, "missing id (CWB_POH_122)")
	}

	var req UpdateModelRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid request body (CWB_POH_123)")
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Provider = strings.TrimSpace(req.Provider)
	req.Model = strings.TrimSpace(req.Model)
	req.BaseURL = strings.TrimSpace(req.BaseURL)

	if req.Name == "" || req.Model == "" {
		return jsonError(c, http.StatusBadRequest, "name and model are required (CWB_POH_124)")
	}
	if !validProviders[req.Provider] {
		return jsonError(c, http.StatusBadRequest, "unsupported provider (CWB_POH_125)")
	}
	if req.Provider == "openai_compatible" && req.BaseURL == "" {
		return jsonError(c, http.StatusBadRequest, "base_url is required for openai_compatible (CWB_POH_126)")
	}

	var ciphertext string
	if req.APIKey != "" {
		key, err := getKey()
		if err != nil {
			logger.Error("encryption key not initialized", "err", err)
			return jsonError(c, http.StatusServiceUnavailable, "server not initialized (CWB_POH_127)")
		}
		ciphertext, err = security.EncryptString(req.APIKey, key)
		if err != nil {
			logger.Error("encrypt api key failed", "err", err)
			return jsonError(c, http.StatusInternalServerError, "failed to secure api key (CWB_POH_128)")
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	err := updateModelForUser(ApiTypes.ProjectDBHandle,
		userID, id, req.Name, req.Provider, req.BaseURL, req.Model, ciphertext,
		req.DefaultParams, enabled,
	)
	if errors.Is(err, errModelNotFound) {
		return jsonError(c, http.StatusNotFound, "model not found (CWB_POH_129)")
	}
	if err != nil {
		logger.Error("update model failed", "id", id, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to update model (CWB_POH_129a)")
	}

	m, err := getModelForUser(ApiTypes.ProjectDBHandle, userID, id)
	if err != nil {
		logger.Error("reload model failed", "id", id, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to read updated model (CWB_POH_129b)")
	}
	return c.JSON(http.StatusOK, ModelResponse{Status: true, Model: m})
}

// DeleteModel handles DELETE /api/v1/prompt-optimizer/models/:id
func DeleteModel(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_130")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_131")
	defer rc.Close()
	logger := rc.GetLogger()

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return jsonError(c, http.StatusBadRequest, "missing id (CWB_POH_132)")
	}

	err := deleteModelForUser(ApiTypes.ProjectDBHandle, userID, id)
	if errors.Is(err, errModelNotFound) {
		return jsonError(c, http.StatusNotFound, "model not found (CWB_POH_133)")
	}
	if err != nil {
		logger.Error("delete model failed", "id", id, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to delete model (CWB_POH_134)")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
