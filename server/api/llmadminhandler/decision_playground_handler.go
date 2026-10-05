package llmadminhandler

// Decision Model Playground (requirement 2026100502-rqmt): lets developers run
// a real decision request against a model from .models.toml, with an optional
// policy from the decision policy store (2026100503-devdoc) and typed Jev
// questions (2026100502-devdoc).

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/decisionpolicy"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
)

type playgroundModel struct {
	Key       string `json:"key"`
	ModelName string `json:"model_name"`
	ModelType string `json:"model_type"`
	Provider  string `json:"provider"`
}

type playgroundQuestion struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Instructions string          `json:"instructions"`
	Criteria     json.RawMessage `json:"criteria"`
}

type playgroundRunRequest struct {
	ModelKey      string               `json:"model_key"`
	PolicyID      int64                `json:"policy_id"`
	PolicyVersion int                  `json:"policy_version"`
	Policy        string               `json:"policy"`
	Text          string               `json:"text"`
	Questions     []playgroundQuestion `json:"questions"`
}

// playgroundProvider maps a .models.toml entry to the decision provider used
// to run it: real Jev for decision-model entries, the logprob emulation for
// ordinary chat models.
func playgroundProvider(cfg ApiTypes.LLMModelDef) (llmclients.ProviderID, bool) {
	switch strings.TrimSpace(cfg.ModelType) {
	case "decision-model":
		return llmclients.ProviderJevCompatible, true
	case "llm":
		return llmclients.ProviderJevEmulated, true
	default:
		return "", false
	}
}

// playgroundProviderConfig builds the client config for a model entry.
func playgroundProviderConfig(key string, cfg ApiTypes.LLMModelDef) (llmclients.ProviderConfig, error) {
	provider, ok := playgroundProvider(cfg)
	if !ok {
		return llmclients.ProviderConfig{}, fmt.Errorf("model %q (type %q) cannot run decisions", key, cfg.ModelType)
	}
	pc := llmclients.ProviderConfig{
		ID:          provider,
		BaseURL:     strings.TrimSpace(cfg.BaseURL),
		APIKey:      strings.TrimSpace(cfg.APIKey),
		ProfileName: key,
	}
	if provider == llmclients.ProviderJevEmulated {
		pc.Extra = map[string]string{}
		// DashScope returns at most 5 alternatives per token.
		if strings.Contains(strings.ToLower(pc.BaseURL), "dashscope") {
			pc.Extra["top_logprobs"] = "5"
		}
		if cfg.OmitTemperature {
			pc.Extra["temperature"] = "omit"
		}
	}
	return pc, nil
}

// playgroundJevQuestions validates the questions and converts them to the
// shared llm type. Choice criteria are {"option": "description"}; score
// criteria are an ordered list of levels; noul takes none.
func playgroundJevQuestions(in []playgroundQuestion) (llmclients.JevQuestions, error) {
	if len(in) == 0 {
		return nil, errors.New("add at least one question")
	}
	out := make(llmclients.JevQuestions, len(in))
	for i, q := range in {
		id := strings.TrimSpace(q.ID)
		if id == "" {
			id = "q" + strconv.Itoa(i+1)
		}
		if _, dup := out[id]; dup {
			return nil, fmt.Errorf("duplicate question id %q", id)
		}
		instructions := strings.TrimSpace(q.Instructions)
		if instructions == "" {
			return nil, fmt.Errorf("question %q: instructions are required", id)
		}
		jq := llmclients.JevQuestion{Type: q.Type, Instructions: instructions}
		switch q.Type {
		case "noul":
		case "choice":
			var criteria map[string]string
			if err := json.Unmarshal(q.Criteria, &criteria); err != nil || len(criteria) < 2 {
				return nil, fmt.Errorf("question %q: choice needs at least 2 options", id)
			}
			jq.Criteria = criteria
		case "score":
			var criteria []string
			if err := json.Unmarshal(q.Criteria, &criteria); err != nil || len(criteria) < 2 {
				return nil, fmt.Errorf("question %q: score needs at least 2 levels", id)
			}
			jq.Criteria = criteria
		default:
			return nil, fmt.Errorf("question %q: unknown type %q", id, q.Type)
		}
		out[id] = jq
	}
	return out, nil
}

// playgroundState builds the state message: the judged text and the policy,
// so the policy leads every question (see 2026100503-devdoc).
func playgroundState(text, policy string) (string, error) {
	state := map[string]string{}
	if strings.TrimSpace(text) != "" {
		state["text"] = text
	}
	if strings.TrimSpace(policy) != "" {
		state["policy"] = policy
	}
	if len(state) == 0 {
		return "", errors.New("enter the text to judge or a policy")
	}
	raw, err := json.Marshal(state)
	return string(raw), err
}

func playgroundPolicyStore(logger ApiTypes.JimoLogger) (*decisionpolicy.Store, error) {
	db := ApiTypes.SharedDBHandle
	if db == nil {
		return nil, errors.New("shared database is not available")
	}
	return decisionpolicy.NewStore(db, logger), nil
}

// GetDecisionPlaygroundOptions handles GET /api/v1/llm/decision-playground/options.
func GetDecisionPlaygroundOptions(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DMP_101")
	defer rc.Close()
	logger := rc.GetLogger()

	models, err := readModelsTOML(modelsTOMLPath())
	if err != nil {
		logger.Error("read .models.toml failed", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": "failed to read .models.toml", "error": err.Error()})
	}
	out := make([]playgroundModel, 0, len(models))
	for key, cfg := range models {
		if provider, ok := playgroundProvider(cfg); ok {
			out = append(out, playgroundModel{Key: key, ModelName: cfg.ModelName, ModelType: cfg.ModelType, Provider: string(provider)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })

	result := map[string]any{"models": out, "policies": []decisionpolicy.Policy{}}
	store, err := playgroundPolicyStore(logger)
	if err == nil {
		var policies []decisionpolicy.Policy
		policies, err = store.List(c.Request().Context(), decisionpolicy.ListFilter{Limit: 1000})
		if err == nil {
			result["policies"] = policies
		}
	}
	if err != nil {
		// Models are still usable without policies; report the reason.
		logger.Warn("list decision policies failed", "err", err)
		result["policy_error"] = err.Error()
	}
	return c.JSON(http.StatusOK, result)
}

// GetDecisionPlaygroundPolicy handles GET /api/v1/llm/decision-playground/policies/:id
// and returns the policy's current version.
func GetDecisionPlaygroundPolicy(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DMP_102")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "invalid policy id"})
	}
	store, err := playgroundPolicyStore(logger)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": err.Error()})
	}
	ctx := c.Request().Context()
	pol, err := store.Get(ctx, id)
	if err == nil {
		var ver *decisionpolicy.PolicyVersion
		if ver, err = store.GetVersion(ctx, id, pol.CurrentVersion); err == nil {
			return c.JSON(http.StatusOK, ver)
		}
	}
	if errors.Is(err, decisionpolicy.ErrNotFound) {
		return c.JSON(http.StatusNotFound, map[string]any{"message": "policy not found"})
	}
	logger.Error("load decision policy failed", "policy_id", id, "err", err)
	return c.JSON(http.StatusInternalServerError, map[string]any{"message": "failed to load policy", "error": err.Error()})
}

// RunDecisionPlayground handles POST /api/v1/llm/decision-playground/run.
func RunDecisionPlayground(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DMP_103")
	defer rc.Close()
	logger := rc.GetLogger()

	userID := strings.TrimSpace(rc.GetUserID())
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{"message": "authenticated user is required"})
	}
	var req playgroundRunRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "invalid decision request"})
	}
	models, err := readModelsTOML(modelsTOMLPath())
	if err != nil {
		logger.Error("read .models.toml failed", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": "failed to read .models.toml", "error": err.Error()})
	}
	key := strings.TrimSpace(req.ModelKey)
	cfg, ok := models[key]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": fmt.Sprintf("model %q not found in .models.toml", key)})
	}
	pc, err := playgroundProviderConfig(key, cfg)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": err.Error()})
	}
	questions, err := playgroundJevQuestions(req.Questions)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": err.Error()})
	}
	state, err := playgroundState(req.Text, req.Policy)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": err.Error()})
	}

	client, err := llmclients.NewClient(pc, logger)
	if err != nil {
		logger.Error("build decision client failed", "model_key", key, "provider", pc.ID, "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": "failed to initialize provider", "error": err.Error()})
	}
	metadata := map[string]any{"model_key": key}
	if req.PolicyID > 0 {
		// Traceability: record the policy version the run started from; the
		// text may have been edited on the page before running.
		metadata["policy_id"] = req.PolicyID
		metadata["policy_version"] = req.PolicyVersion
	}

	started := time.Now()
	resp, err := client.Complete(c.Request().Context(), llmclients.Request{
		UserID:       userID,
		Model:        strings.TrimSpace(cfg.ModelName),
		PromptName:   "decision_playground",
		CallReason:   "decision_playground",
		CallLoc:      "CWB_DMP_104",
		Metadata:     metadata,
		Messages:     []llmclients.Message{{Role: llmclients.RoleUser, Content: state}},
		JevQuestions: questions,
	})
	elapsedMS := time.Since(started).Milliseconds()
	if err != nil {
		logger.Error("decision playground run failed", "model_key", key, "provider", pc.ID, "questions", len(questions), "err", err)
		return c.JSON(http.StatusBadGateway, map[string]any{"message": "decision request failed", "error": err.Error(), "elapsed_ms": elapsedMS})
	}
	answers, err := llmclients.ParseJevAnswers(resp.Content)
	if err != nil {
		logger.Error("decision playground answers unreadable", "model_key", key, "err", err)
		return c.JSON(http.StatusBadGateway, map[string]any{"message": "decision answers could not be read", "error": err.Error(), "content": resp.Content})
	}
	logger.Info("decision playground run", "model_key", key, "provider", pc.ID, "questions", len(questions), "elapsed_ms", elapsedMS)

	result := map[string]any{
		"model_key":  key,
		"provider":   string(pc.ID),
		"answers":    answers,
		"raw":        resp.Raw,
		"elapsed_ms": elapsedMS,
	}
	if resp.Usage != nil {
		result["usage"] = map[string]any{"input_tokens": resp.Usage.InputTokens, "output_tokens": resp.Usage.OutputTokens}
	}
	return c.JSON(http.StatusOK, result)
}
