package llmadminhandler

// Decision Model Playground (requirement 2026100502-rqmt): lets developers run
// a real decision request against a model from .models.toml, with an optional
// policy from the decision policy store (2026100503-devdoc) and typed Jev
// questions (2026100502-devdoc).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/decisionmodel"
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
	Criteria     json.RawMessage `json:"criteria,omitempty"`
}

type playgroundRunRequest struct {
	ModelKey      string               `json:"model_key"`
	PolicyID      int64                `json:"policy_id"`
	PolicyVersion int                  `json:"policy_version"`
	Policy        string               `json:"policy"`
	Text          string               `json:"text"`
	Questions     []playgroundQuestion `json:"questions"`
}

// playgroundJevQuestions validates the questions and converts them to the
// shared llm type. Choice criteria are {"option": "description"}; score
// criteria are an ordered list of levels; noul takes none.
func playgroundJevQuestions(in []playgroundQuestion) (llmclients.JevQuestions, error) {
	if len(in) == 0 {
		return nil, errors.New("add at least one question")
	}
	out := make(llmclients.JevQuestions, len(in))
	for _, q := range playgroundQuestionsWithIDs(in) {
		id := q.ID
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

// playgroundQuestionsWithIDs returns the questions with blank IDs set to
// q<position>, the IDs their answers are keyed by.
func playgroundQuestionsWithIDs(in []playgroundQuestion) []playgroundQuestion {
	out := make([]playgroundQuestion, len(in))
	for i, q := range in {
		q.ID = strings.TrimSpace(q.ID)
		if q.ID == "" {
			q.ID = "q" + strconv.Itoa(i+1)
		}
		out[i] = q
	}
	return out
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
		if provider, ok := decisionmodel.Provider(cfg); ok {
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

type playgroundPolicySaveRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Note        string `json:"note"`
	MakeCurrent *bool  `json:"make_current"`
}

// policySaveStatus maps store errors to HTTP statuses for the save endpoints.
func policySaveStatus(err error) int {
	switch {
	case errors.Is(err, decisionpolicy.ErrInvalidInput):
		return http.StatusBadRequest
	case errors.Is(err, decisionpolicy.ErrNameTaken):
		return http.StatusConflict
	case errors.Is(err, decisionpolicy.ErrNotFound), errors.Is(err, decisionpolicy.ErrDeleted):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// CreateDecisionPlaygroundPolicy handles POST /api/v1/llm/decision-playground/policies
// and returns the new policy's version 1.
func CreateDecisionPlaygroundPolicy(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DMP_105")
	defer rc.Close()
	logger := rc.GetLogger()

	userID := strings.TrimSpace(rc.GetUserID())
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{"message": "authenticated user is required"})
	}
	var req playgroundPolicySaveRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "invalid policy request"})
	}
	store, err := playgroundPolicyStore(logger)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": err.Error()})
	}
	ctx := c.Request().Context()
	pol, err := store.Create(ctx, decisionpolicy.CreateInput{
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		Content:     req.Content,
		Note:        strings.TrimSpace(req.Note),
		Actor:       userID,
	})
	if err == nil {
		var ver *decisionpolicy.PolicyVersion
		if ver, err = store.GetVersion(ctx, pol.ID, pol.CurrentVersion); err == nil {
			return c.JSON(http.StatusOK, ver)
		}
	}
	// The store logs the failure itself.
	return c.JSON(policySaveStatus(err), map[string]any{"message": "failed to create policy", "error": err.Error()})
}

// CreateDecisionPlaygroundPolicyVersion handles
// POST /api/v1/llm/decision-playground/policies/:id/versions.
func CreateDecisionPlaygroundPolicyVersion(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DMP_106")
	defer rc.Close()
	logger := rc.GetLogger()

	userID := strings.TrimSpace(rc.GetUserID())
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{"message": "authenticated user is required"})
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "invalid policy id"})
	}
	var req playgroundPolicySaveRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "invalid policy request"})
	}
	store, err := playgroundPolicyStore(logger)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": err.Error()})
	}
	ver, err := store.CreateVersion(c.Request().Context(), id, decisionpolicy.VersionInput{
		Content:     req.Content,
		Note:        strings.TrimSpace(req.Note),
		Actor:       userID,
		MakeCurrent: req.MakeCurrent,
	})
	if err != nil {
		return c.JSON(policySaveStatus(err), map[string]any{"message": "failed to save policy version", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, ver)
}

// playgroundRun is one row of testbed.decision_runs.
type playgroundRun struct {
	modelKey, modelName, provider string
	policyID                      int64
	policyVersion                 int
	policyEdited                  bool
	policy, text                  string
	questions                     []playgroundQuestion // as entered, IDs filled in
	answers                       string               // Response.Content, empty on failure
	raw                           json.RawMessage
	errMsg                        string
	usage                         *llmclients.Usage
	elapsedMS                     int64
	userID                        string
	saveErr                       string
}

func saveDecisionRun(c echo.Context, run playgroundRun) (int64, error) {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return 0, errors.New("project database is not initialized")
	}
	questions, err := json.Marshal(run.questions)
	if err != nil {
		return 0, err
	}
	var policyID, policyVersion, answers, raw any
	if run.policyID > 0 {
		policyID, policyVersion = run.policyID, run.policyVersion
	}
	status := "error"
	if run.errMsg == "" {
		status = "ok"
		answers = run.answers
	}
	if len(run.raw) > 0 {
		raw = string(run.raw)
	}
	var inTok, outTok int
	eventIDs := []string{}
	if run.usage != nil {
		inTok, outTok = run.usage.InputTokens, run.usage.OutputTokens
		eventIDs = append(eventIDs, run.usage.EventIDs...)
	}
	events, _ := json.Marshal(eventIDs)
	var id int64
	// Save even if the browser has disconnected during a long run.
	err = db.QueryRowContext(context.WithoutCancel(c.Request().Context()),
		`INSERT INTO testbed.decision_runs
		   (model_key, model_name, provider, policy_id, policy_version, policy_edited, policy_content,
		    input_text, questions, run_status, answers, raw_response, error_message,
		    input_tokens, output_tokens, elapsed_ms, usage_event_ids, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11::jsonb, $12::jsonb, $13, $14, $15, $16, $17::jsonb, $18)
		 RETURNING id`,
		run.modelKey, run.modelName, run.provider, policyID, policyVersion, run.policyEdited, run.policy,
		run.text, string(questions), status, answers, raw, run.errMsg,
		inTok, outTok, run.elapsedMS, string(events), run.userID).Scan(&id)
	return id, err
}

// RunDecisionPlayground handles POST /api/v1/llm/decision-playground/run.
// Every run is logged and saved to testbed.decision_runs, including runs
// rejected before reaching the provider and runs the provider failed.
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
		logger.Warn("decision playground run rejected: unreadable request", "user_id", userID, "err", err)
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "invalid decision request", "error": err.Error()})
	}
	key := strings.TrimSpace(req.ModelKey)
	run := playgroundRun{
		modelKey: key, policyID: req.PolicyID, policyVersion: req.PolicyVersion,
		policy: req.Policy, text: req.Text, questions: playgroundQuestionsWithIDs(req.Questions), userID: userID,
	}
	// reject saves and logs a run that never reached the provider.
	reject := func(status int, msg string) error {
		run.errMsg = msg
		runID := saveAndLogRun(c, logger, &run)
		logger.Warn("decision playground run rejected", "model_key", key, "user_id", userID, "run_id", runID, "reason", msg)
		return c.JSON(status, runBody(map[string]any{"message": "decision request rejected", "error": msg}, runID))
	}

	// Record whether the policy text sent differs from the stored version it
	// started from, so a saved run says exactly which text it used.
	if req.PolicyID > 0 {
		store, err := playgroundPolicyStore(logger)
		if err != nil {
			return reject(http.StatusServiceUnavailable, err.Error())
		}
		ver, err := store.GetVersion(c.Request().Context(), req.PolicyID, req.PolicyVersion)
		if err != nil {
			return reject(http.StatusBadRequest, fmt.Sprintf("selected policy version not found: %v", err))
		}
		run.policyEdited = ver.Content != req.Policy
	}
	models, err := readModelsTOML(modelsTOMLPath())
	if err != nil {
		return reject(http.StatusInternalServerError, fmt.Sprintf("failed to read .models.toml: %v", err))
	}
	cfg, ok := models[key]
	if !ok {
		return reject(http.StatusBadRequest, fmt.Sprintf("model %q not found in .models.toml", key))
	}
	run.modelName = strings.TrimSpace(cfg.ModelName)
	pc, err := decisionmodel.ProviderConfig(key, cfg)
	if err != nil {
		return reject(http.StatusBadRequest, err.Error())
	}
	run.provider = string(pc.ID)
	questions, err := playgroundJevQuestions(req.Questions)
	if err != nil {
		return reject(http.StatusBadRequest, err.Error())
	}
	state, err := playgroundState(req.Text, req.Policy)
	if err != nil {
		return reject(http.StatusBadRequest, err.Error())
	}
	client, err := llmclients.NewClient(pc, logger)
	if err != nil {
		return reject(http.StatusInternalServerError, fmt.Sprintf("failed to initialize provider: %v", err))
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
		Model:        run.modelName,
		PromptName:   "decision_playground",
		CallReason:   "decision_playground",
		CallLoc:      "CWB_DMP_104",
		Metadata:     metadata,
		Messages:     []llmclients.Message{{Role: llmclients.RoleUser, Content: state}},
		JevQuestions: questions,
	})
	run.elapsedMS = time.Since(started).Milliseconds()
	var answers map[string]llmclients.JevAnswer
	if err == nil {
		run.answers, run.raw, run.usage = resp.Content, resp.Raw, resp.Usage
		answers, err = llmclients.ParseJevAnswers(resp.Content)
	}
	if err != nil {
		run.errMsg = err.Error()
	}
	runID := saveAndLogRun(c, logger, &run)

	if err != nil {
		logger.Error("decision playground run failed", "model_key", key, "provider", pc.ID, "questions", len(questions), "run_id", runID, "err", err)
		return c.JSON(http.StatusBadGateway, runBody(map[string]any{"message": "decision request failed", "error": err.Error(), "elapsed_ms": run.elapsedMS}, runID))
	}
	logger.Info("decision playground run", "model_key", key, "provider", pc.ID, "questions", len(questions), "elapsed_ms", run.elapsedMS, "run_id", runID)

	result := map[string]any{
		"model_key":  key,
		"provider":   string(pc.ID),
		"answers":    answers,
		"raw":        resp.Raw,
		"elapsed_ms": run.elapsedMS,
	}
	if runID == 0 {
		result["save_error"] = run.saveErr
	}
	if resp.Usage != nil {
		result["usage"] = map[string]any{"input_tokens": resp.Usage.InputTokens, "output_tokens": resp.Usage.OutputTokens}
	}
	return c.JSON(http.StatusOK, runBody(result, runID))
}

// saveAndLogRun saves the run, logging (not returning) a save failure so the
// caller still answers the page; it returns 0 when nothing was saved.
func saveAndLogRun(c echo.Context, logger ApiTypes.JimoLogger, run *playgroundRun) int64 {
	runID, err := saveDecisionRun(c, *run)
	if err != nil {
		run.saveErr = err.Error()
		logger.Error("save decision run failed", "model_key", run.modelKey, "user_id", run.userID, "err", err)
	}
	return runID
}

func runBody(body map[string]any, runID int64) map[string]any {
	if runID > 0 {
		body["run_id"] = runID
	}
	return body
}
