// Package dspyhandler provides REST API handlers for DSPy-optimized prompt management.
//
// DSPy (Declarative Self-improving Language Programs) lets users declare what they
// want their prompts to achieve instead of manually crafting them. These handlers
// support the full DSPy workflow: define signature → choose module → add examples →
// select optimizer → run optimization → save the optimized prompt.
//
// Endpoints:
//
//	POST   /api/v1/dspy/prompts           – Create a new DSPy prompt configuration
//	GET    /api/v1/dspy/prompts           – List all saved prompts
//	GET    /api/v1/dspy/prompts/:id       – Get a single prompt by ID
//	PUT    /api/v1/dspy/prompts/:id       – Update a prompt
//	DELETE /api/v1/dspy/prompts/:id       – Delete a prompt
//	POST   /api/v1/dspy/optimize          – Run DSPy optimization on a configuration
package dspyhandler

import (
	"fmt"
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/appdatastores"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// ─── Request / Response types ─────────────────────────────────────────────────

// CreatePromptRequest holds the payload for creating a new DSPy prompt.
type CreatePromptRequest struct {
	PromptName         string `json:"prompt_name"`
	PromptDesc         string `json:"prompt_desc"`
	TaskType           string `json:"task_type"`
	SignatureInputs    string `json:"signature_inputs"`     // JSON string
	SignatureOutputs   string `json:"signature_outputs"`    // JSON string
	SignatureDocstring string `json:"signature_docstring"`
	ModuleType         string `json:"module_type"`
	Examples           string `json:"examples"`             // JSON string
	Optimizer          string `json:"optimizer"`
	OptimizerConfig    string `json:"optimizer_config"`     // JSON string
	Status             string `json:"status"`
}

// UpdatePromptRequest holds the payload for updating an existing DSPy prompt.
type UpdatePromptRequest struct {
	PromptName            string `json:"prompt_name"`
	PromptDesc            string `json:"prompt_desc"`
	TaskType              string `json:"task_type"`
	SignatureInputs       string `json:"signature_inputs"`
	SignatureOutputs      string `json:"signature_outputs"`
	SignatureDocstring    string `json:"signature_docstring"`
	ModuleType            string `json:"module_type"`
	Examples              string `json:"examples"`
	Optimizer             string `json:"optimizer"`
	OptimizerConfig       string `json:"optimizer_config"`
	OptimizedInstructions string `json:"optimized_instructions"`
	OptimizedExamples     string `json:"optimized_examples"`
	Status                string `json:"status"`
}

// OptimizeRequest holds the payload for triggering DSPy optimization.
type OptimizeRequest struct {
	PromptName string `json:"prompt_name"`
	ModuleType string `json:"module_type"`
	Optimizer  string `json:"optimizer"`
	Examples   string `json:"examples"` // JSON string
}

// listResponse wraps a slice of prompts for the list endpoint.
type listResponse struct {
	Status  bool                             `json:"status"`
	Results []appdatastores.TableDspyPromptDef `json:"results"`
}

// singleResponse wraps a single prompt for get/create endpoints.
type singleResponse struct {
	Status bool                            `json:"status"`
	Result *appdatastores.TableDspyPromptDef `json:"result"`
}

// optimizeResponse wraps the result of an optimization run.
type optimizeResponse struct {
	Status                bool   `json:"status"`
	OptimizedInstructions string `json:"optimized_instructions"`
	Message               string `json:"message"`
}

// errorResponse is the standard error payload.
type errorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func tableName() string {
	return config.AppConfig.AppTableNames.TableName_DspyPrompts
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// CreatePrompt handles POST /api/v1/dspy/prompts.
// Stub: validates the request, inserts a new record, and returns the new ID.
func CreatePrompt(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DSP_071")
	defer rc.Close()
	logger := rc.GetLogger()
	logger.Info("CreatePrompt called")

	var req CreatePromptRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid request body (CWB_DSP_076): %s", err.Error()),
		})
	}

	if req.PromptName == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "prompt_name is required (CWB_DSP_081)",
		})
	}

	status := req.Status
	if status == "" {
		status = "draft"
	}

	record := appdatastores.TableDspyPromptDef{
		PromptName:         req.PromptName,
		PromptDesc:         req.PromptDesc,
		TaskType:           req.TaskType,
		SignatureInputs:    req.SignatureInputs,
		SignatureOutputs:   req.SignatureOutputs,
		SignatureDocstring: req.SignatureDocstring,
		ModuleType:         req.ModuleType,
		Examples:           req.Examples,
		Optimizer:          req.Optimizer,
		OptimizerConfig:    req.OptimizerConfig,
		Status:             status,
	}

	// TODO: extract user_id from the authenticated session context
	// record.UserID = rc.GetUserID()

	db := ApiTypes.ProjectDBHandle
	newID, err := appdatastores.InsertDspyPrompt(db, tableName(), record)
	if err != nil {
		logger.Error("InsertDspyPrompt failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("failed to create prompt (CWB_DSP_096): %s", err.Error()),
		})
	}

	record.PromptID = fmt.Sprintf("%d", newID)
	logger.Info("CreatePrompt success", "prompt_id", newID)
	return c.JSON(http.StatusCreated, singleResponse{Status: true, Result: &record})
}

// ListPrompts handles GET /api/v1/dspy/prompts.
// Stub: returns all saved prompts (optionally filtered by user_id query param).
func ListPrompts(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DSP_106")
	defer rc.Close()
	logger := rc.GetLogger()
	logger.Info("ListPrompts called")

	userID := c.QueryParam("user_id")
	db := ApiTypes.ProjectDBHandle
	prompts, err := appdatastores.ListDspyPrompts(db, tableName(), userID, 200)
	if err != nil {
		logger.Error("ListDspyPrompts failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("failed to list prompts (CWB_DSP_111): %s", err.Error()),
		})
	}

	if prompts == nil {
		prompts = []appdatastores.TableDspyPromptDef{}
	}
	return c.JSON(http.StatusOK, listResponse{Status: true, Results: prompts})
}

// GetPrompt handles GET /api/v1/dspy/prompts/:id.
// Stub: retrieves a single prompt by its ID.
func GetPrompt(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DSP_121")
	defer rc.Close()
	logger := rc.GetLogger()

	id := c.Param("id")
	logger.Info("GetPrompt called", "id", id)

	db := ApiTypes.ProjectDBHandle
	prompt, err := appdatastores.GetDspyPromptByID(db, tableName(), id)
	if err != nil {
		logger.Error("GetDspyPromptByID failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("failed to get prompt (CWB_DSP_126): %s", err.Error()),
		})
	}
	if prompt == nil {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("prompt not found: %s (CWB_DSP_131)", id),
		})
	}
	return c.JSON(http.StatusOK, singleResponse{Status: true, Result: prompt})
}

// UpdatePrompt handles PUT /api/v1/dspy/prompts/:id.
// Stub: updates name, description, and optimized content fields.
func UpdatePrompt(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DSP_141")
	defer rc.Close()
	logger := rc.GetLogger()

	id := c.Param("id")
	logger.Info("UpdatePrompt called", "id", id)

	var req UpdatePromptRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid request body (CWB_DSP_146): %s", err.Error()),
		})
	}

	record := appdatastores.TableDspyPromptDef{
		PromptID:              id,
		PromptName:            req.PromptName,
		PromptDesc:            req.PromptDesc,
		TaskType:              req.TaskType,
		SignatureInputs:       req.SignatureInputs,
		SignatureOutputs:      req.SignatureOutputs,
		SignatureDocstring:    req.SignatureDocstring,
		ModuleType:            req.ModuleType,
		Examples:              req.Examples,
		Optimizer:             req.Optimizer,
		OptimizerConfig:       req.OptimizerConfig,
		OptimizedInstructions: req.OptimizedInstructions,
		OptimizedExamples:     req.OptimizedExamples,
		Status:                req.Status,
	}

	db := ApiTypes.ProjectDBHandle
	if err := appdatastores.UpdateDspyPrompt(db, tableName(), record); err != nil {
		logger.Error("UpdateDspyPrompt failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("failed to update prompt (CWB_DSP_151): %s", err.Error()),
		})
	}

	logger.Info("UpdatePrompt success", "id", id)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "message": "updated"})
}

// DeletePrompt handles DELETE /api/v1/dspy/prompts/:id.
// Stub: permanently removes a prompt by ID.
func DeletePrompt(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DSP_161")
	defer rc.Close()
	logger := rc.GetLogger()

	id := c.Param("id")
	logger.Info("DeletePrompt called", "id", id)

	db := ApiTypes.ProjectDBHandle
	if err := appdatastores.DeleteDspyPrompt(db, tableName(), id); err != nil {
		logger.Error("DeleteDspyPrompt failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("failed to delete prompt (CWB_DSP_166): %s", err.Error()),
		})
	}

	logger.Info("DeletePrompt success", "id", id)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "message": "deleted"})
}

// OptimizePrompt handles POST /api/v1/dspy/optimize.
// Stub: would invoke the DSPy optimizer (Python/external service) to generate
// optimized instructions and few-shot examples from the provided configuration.
func OptimizePrompt(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DSP_176")
	defer rc.Close()
	logger := rc.GetLogger()
	logger.Info("OptimizePrompt called")

	var req OptimizeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid request body (CWB_DSP_181): %s", err.Error()),
		})
	}

	// TODO: implement real DSPy optimization.
	// Options:
	//   1. Spawn a Python subprocess running dspy.BootstrapFewShot / MIPROv2
	//   2. Call an external DSPy microservice over HTTP
	//   3. Use a pre-built DSPy REST API bridge
	//
	// For now, return a stub response so the frontend can proceed end-to-end.
	logger.Info("OptimizePrompt stub invoked",
		"prompt_name", req.PromptName,
		"module", req.ModuleType,
		"optimizer", req.Optimizer,
	)

	stubInstructions := fmt.Sprintf(
		"[STUB] Optimized instructions for '%s' using %s + %s optimizer.\n"+
			"In production this would contain the LLM-generated instructions "+
			"and selected few-shot demonstrations produced by DSPy.",
		req.PromptName, req.ModuleType, req.Optimizer,
	)

	return c.JSON(http.StatusOK, optimizeResponse{
		Status:                true,
		OptimizedInstructions: stubInstructions,
		Message:               "Stub optimization complete. Wire a real DSPy backend to replace this.",
	})
}
