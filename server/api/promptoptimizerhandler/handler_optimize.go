package promptoptimizerhandler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/security"
	"github.com/labstack/echo/v4"
)

// placeholderRE matches {{ name }} with optional surrounding whitespace. Names
// are restricted to letters, digits, and underscores so a stray "{{" in user
// prose can't accidentally consume the rest of the template.
var placeholderRE = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

// substitutePlaceholders replaces every {{name}} in tpl with vars[name]. Keys
// with no matching variable are left as the original literal text so the user
// can see which placeholder was unresolved.
func substitutePlaceholders(tpl string, vars map[string]string) string {
	return placeholderRE.ReplaceAllStringFunc(tpl, func(match string) string {
		sub := placeholderRE.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		if v, ok := vars[sub[1]]; ok {
			return v
		}
		return match
	})
}

// buildSubstitutionMap layers variables in order of precedence:
//  1. User-defined variables (lowest).
//  2. Per-request Variables JSON (middle) — overrides user vars.
//  3. Built-in request fields (highest) — originalPrompt, optimizedPrompt,
//     feedback, testInput.
func buildSubstitutionMap(userVars map[string]string, reqVars json.RawMessage, req *OptimizeRequest) map[string]string {
	out := make(map[string]string, len(userVars)+8)
	for k, v := range userVars {
		out[k] = v
	}
	if len(reqVars) > 0 {
		var overlay map[string]any
		if err := json.Unmarshal(reqVars, &overlay); err == nil {
			for k, v := range overlay {
				out[k] = fmt.Sprint(v)
			}
		}
	}
	if req.OriginalPrompt != "" {
		out["originalPrompt"] = req.OriginalPrompt
	}
	if req.OptimizedPrompt != "" {
		out["optimizedPrompt"] = req.OptimizedPrompt
	}
	if req.Feedback != "" {
		out["feedback"] = req.Feedback
	}
	if req.TestInput != "" {
		out["testInput"] = req.TestInput
	}
	return out
}

// resolveTemplate returns the template the request asked for, or the newest
// built-in seed matching kind if req.TemplateID is empty.
func resolveTemplate(userID string, req *OptimizeRequest) (*Template, error) {
	db := ApiTypes.ProjectDBHandle
	if id := strings.TrimSpace(req.TemplateID); id != "" {
		return getTemplateForUser(db, userID, id)
	}
	list, err := listTemplatesForUser(db, userID, req.Kind)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].BuiltIn {
			t := list[i]
			return &t, nil
		}
	}
	if len(list) > 0 {
		t := list[0]
		return &t, nil
	}
	return nil, errTemplateNotFound
}

// OptimizePrompt handles POST /api/v1/prompt-optimizer/optimize. It loads the
// requested model + template, decrypts the API key, substitutes the user's
// variables into the template, then either completes or streams the prompt
// against the provider. A history row is inserted on success so the user can
// revisit or favorite the run later.
func OptimizePrompt(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_600")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_601")
	defer rc.Close()
	logger := rc.GetLogger()

	var req OptimizeRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid request body (CWB_POH_602)")
	}
	req.Kind = strings.TrimSpace(req.Kind)
	req.ModelID = strings.TrimSpace(req.ModelID)
	if !validHistoryKinds[req.Kind] {
		return jsonError(c, http.StatusBadRequest, "unsupported kind (CWB_POH_603)")
	}
	if req.ModelID == "" {
		return jsonError(c, http.StatusBadRequest, "model_id is required (CWB_POH_604)")
	}
	if err := validateRequiredFieldsForKind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, err.Error()+" (CWB_POH_605)")
	}

	db := ApiTypes.ProjectDBHandle

	model, err := getModelForUser(db, userID, req.ModelID)
	if errors.Is(err, errModelNotFound) {
		return jsonError(c, http.StatusNotFound, "model not found (CWB_POH_606)")
	}
	if err != nil {
		logger.Error("get model failed", "id", req.ModelID, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to load model (CWB_POH_607)")
	}
	if !model.Enabled {
		return jsonError(c, http.StatusBadRequest, "model is disabled (CWB_POH_608)")
	}

	ciphertext, err := getAPIKeyCiphertextForUser(db, userID, req.ModelID)
	if err != nil {
		logger.Error("get api key ciphertext failed", "id", req.ModelID, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to load credentials (CWB_POH_609)")
	}
	key, err := getKey()
	if err != nil {
		logger.Error("encryption key not initialized", "err", err)
		return jsonError(c, http.StatusServiceUnavailable, "server not initialized (CWB_POH_610)")
	}
	plaintextKey, err := security.DecryptString(ciphertext, key)
	if err != nil {
		logger.Error("decrypt api key failed", "id", req.ModelID, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to decode credentials (CWB_POH_611)")
	}

	tpl, err := resolveTemplate(userID, &req)
	if errors.Is(err, errTemplateNotFound) {
		return jsonError(c, http.StatusNotFound, "template not found for kind (CWB_POH_612)")
	}
	if err != nil {
		logger.Error("resolve template failed", "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to load template (CWB_POH_613)")
	}
	if tpl.Kind != req.Kind {
		return jsonError(c, http.StatusBadRequest, "template kind does not match request kind (CWB_POH_614)")
	}

	userVars, err := varsByName(db, userID)
	if err != nil {
		logger.Error("load user variables failed", "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to load variables (CWB_POH_615)")
	}
	subs := buildSubstitutionMap(userVars, req.Variables, &req)
	finalPrompt := substitutePlaceholders(tpl.Content, subs)

	client, err := llm.NewClient(llm.ProviderConfig{
		ID:      llm.ProviderID(model.Provider),
		BaseURL: model.BaseURL,
		APIKey:  plaintextKey,
	})
	if err != nil {
		logger.Error("build llm client failed", "provider", model.Provider, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to initialize provider (CWB_POH_616)")
	}

	llmReq := llm.Request{
		Model:      model.Model,
		PromptName: llm.EnsurePromptName(strings.TrimSpace(tpl.Name), "prompt_optimizer_"+req.Kind, "MID-CWB-PROMPT-OPTIMIZER", model.Model),
		CallReason: "prompt_optimizer_" + req.Kind,
		CallLoc:    "MID-CWB-PROMPT-OPTIMIZER",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: finalPrompt},
		},
		Stream: req.Stream,
	}
	mergeDefaultParams(&llmReq, model.DefaultParams)

	tplID := tpl.ID
	modelID := model.ID

	if req.Stream {
		return streamOptimize(c, "CWB_POH_601", client, llmReq, userID, &req, &tplID, &modelID)
	}
	return completeOptimize(c, logger, client, llmReq, userID, &req, &tplID, &modelID)
}

// validateRequiredFieldsForKind enforces per-kind input expectations to catch
// obvious client bugs before a provider call is made.
func validateRequiredFieldsForKind(req *OptimizeRequest) error {
	switch req.Kind {
	case "optimize", "analyze":
		if strings.TrimSpace(req.OriginalPrompt) == "" {
			return errors.New("original_prompt is required")
		}
	case "iterate":
		if strings.TrimSpace(req.OptimizedPrompt) == "" {
			return errors.New("optimized_prompt is required")
		}
		if strings.TrimSpace(req.Feedback) == "" {
			return errors.New("feedback is required")
		}
	case "test":
		if strings.TrimSpace(req.OptimizedPrompt) == "" {
			return errors.New("optimized_prompt is required")
		}
		if strings.TrimSpace(req.TestInput) == "" {
			return errors.New("test_input is required")
		}
	}
	return nil
}

// mergeDefaultParams copies a handful of known knobs out of the model's
// stored default_params JSON onto the llm.Request. Unknown keys are ignored
// so operators can safely stash provider-specific values there.
func mergeDefaultParams(req *llm.Request, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return
	}
	if v, ok := m["temperature"]; ok {
		if f, ok := toFloat64(v); ok {
			req.Temperature = &f
		}
	}
	if v, ok := m["top_p"]; ok {
		if f, ok := toFloat64(v); ok {
			req.TopP = &f
		}
	}
	if v, ok := m["max_tokens"]; ok {
		if f, ok := toFloat64(v); ok {
			n := int(f)
			req.MaxTokens = &n
		}
	}
}

func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

// completeOptimize runs a non-streaming call, persists history, and returns a
// JSON envelope. On provider failure the history row is skipped and the error
// is surfaced to the caller.
func completeOptimize(
	c echo.Context,
	logger ApiTypes.JimoLogger,
	client llm.Client,
	req llm.Request,
	userID string,
	opt *OptimizeRequest,
	templateID, modelID *string,
) error {
	resp, err := client.Complete(c.Request().Context(), req)
	if err != nil {
		logger.Error("llm complete failed", "err", err)
		return jsonError(c, http.StatusBadGateway, "provider call failed (CWB_POH_620)")
	}

	output := resp.Content
	historyID, hErr := insertHistory(
		ApiTypes.ProjectDBHandle, userID, opt.Kind,
		opt.OriginalPrompt, outputPromptForKind(opt, output),
		opt.TestInput, testOutputForKind(opt, output),
		templateID, modelID, opt.Variables,
	)
	if hErr != nil {
		logger.Error("insert history failed", "err", hErr)
		// History insert failure does not fail the response — the user still
		// gets their output. The error is logged for follow-up.
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":     true,
		"id":         historyID,
		"output":     output,
		"kind":       opt.Kind,
		"model_id":   *modelID,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// streamOptimize proxies SSE chunks from the llm.Client to the HTTP response
// and persists one history row at the end of the stream. The client receives:
//
//	data: {"delta":"...chunk..."}
//	...
//	data: {"done":true,"id":"...","output":"...full text..."}
//	data: [DONE]
//
// so a browser EventSource (or fetch+ReadableStream) can assemble the full
// text by concatenating deltas or by reading the final "output" field.
func streamOptimize(
	c echo.Context,
	loc string,
	client llm.Client,
	req llm.Request,
	userID string,
	opt *OptimizeRequest,
	templateID, modelID *string,
) error {
	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	w.Flush()

	var builder strings.Builder
	writeSSE := func(payload any) error {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return err
		}
		w.Flush()
		return nil
	}

	err := client.Stream(c.Request().Context(), req, func(chunk llm.StreamChunk) error {
		if chunk.Delta != "" {
			builder.WriteString(chunk.Delta)
			return writeSSE(map[string]any{"delta": chunk.Delta})
		}
		return nil
	})
	if err != nil {
		_ = writeSSE(map[string]any{
			"error":     err.Error(),
			"error_loc": loc,
		})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		w.Flush()
		return nil
	}

	output := builder.String()
	historyID, _ := insertHistory(
		ApiTypes.ProjectDBHandle, userID, opt.Kind,
		opt.OriginalPrompt, outputPromptForKind(opt, output),
		opt.TestInput, testOutputForKind(opt, output),
		templateID, modelID, opt.Variables,
	)

	_ = writeSSE(map[string]any{
		"done":   true,
		"id":     historyID,
		"output": output,
	})
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	w.Flush()
	return nil
}

// outputPromptForKind maps the provider's single output string onto the
// history row's optimized_prompt column. For "test" runs, the output belongs
// in test_output instead, so optimized_prompt falls back to whatever the
// caller already had.
func outputPromptForKind(opt *OptimizeRequest, output string) string {
	switch opt.Kind {
	case "optimize", "iterate", "analyze":
		return output
	}
	return opt.OptimizedPrompt
}

// testOutputForKind returns the output to persist under history.test_output.
// Only the "test" kind populates this column; everything else returns "".
func testOutputForKind(opt *OptimizeRequest, output string) string {
	if opt.Kind == "test" {
		return output
	}
	return ""
}
