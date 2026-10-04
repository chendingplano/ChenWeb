package agentservicehandler

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

type RunStore interface {
	LoadResumeState(context.Context, string, string) (ResumeState, error)
	LoadSourceDependencies(context.Context, string, string) (map[string][]SourceRecord, error)
	ListGrantedStores(context.Context, string, []string) ([]GrantedKnowledgeStore, error)
	CreateRunAttempt(context.Context, string, string, string) (ResponseAttempt, bool, error)
	CreateMessage(context.Context, string, CreateMessageInput) (Message, error)
	AppendMessageDelta(context.Context, string, string, string, string) (Message, error)
	RecordToolCall(context.Context, string, ToolCallInput) (string, error)
	FinalizeAssistantMessage(context.Context, string, string, bool, []SourceInput) (Message, error)
	SetAttemptOutcome(context.Context, string, string, AttemptOutcome) (ResponseAttempt, error)
	GetRunAttempt(context.Context, string, string, string) (ResponseAttempt, error)
	LoadHistorySummary(context.Context, string, string) (HistorySummary, error)
	SaveHistorySummary(context.Context, string, string, int, HistorySummary) (bool, error)
	ClearHistorySummary(context.Context, string, string, int) error
	SetConversationTitleIfEmpty(context.Context, string, string, string) error
}

type GatewayBridge interface {
	Start(context.Context, GatewayRunRequest) (io.ReadCloser, error)
	Cancel(context.Context, string) error
	Decide(context.Context, string, string, bool) error
	ModelContextWindow(context.Context, string, string) (int, error)
	Summarize(context.Context, GatewaySummaryRequest) (string, error)
}

type RunSourceChecker interface {
	CheckSourceWithGroups(context.Context, string, []string, []string, SourceRecord) error
}

type RunHandler struct {
	store    RunStore
	profiles *ProfileRegistry
	sources  RunSourceChecker
	gateway  GatewayBridge
	signer   *CapabilitySigner
	// defaultStoreName returns [frontend].default_knowledge_store; nil means
	// no default.
	defaultStoreName func() string
}

// runInBackground runs post-turn work (history folding) after the response
// has finished; tests replace it to run synchronously.
var runInBackground = func(f func()) { go f() }

func NewRunHandler(store RunStore, profiles *ProfileRegistry, sources RunSourceChecker, gateway GatewayBridge, signer *CapabilitySigner) *RunHandler {
	return &RunHandler{store: store, profiles: profiles, sources: sources, gateway: gateway, signer: signer}
}

// SetDefaultKnowledgeStoreName supplies the configured default knowledge store
// name. The default is offered to Pi only when the user is granted that store.
func (h *RunHandler) SetDefaultKnowledgeStoreName(name func() string) *RunHandler {
	h.defaultStoreName = name
	return h
}

func RegisterRunRoutes(group *echo.Group, handler *RunHandler) {
	group.POST("/conversations/:id/runs", handler.Start)
	group.POST("/conversations/:id/runs/:runId/cancel", handler.Cancel)
	group.POST("/conversations/:id/runs/:runId/permissions/:requestId", handler.Decide)
}

func (h *RunHandler) available() bool {
	return h != nil && h.store != nil && h.profiles != nil && h.gateway != nil && h.signer != nil
}

func (h *RunHandler) Start(c echo.Context) error {
	userID, ok := agentUserID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "login required"})
	}
	if !h.available() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "agent service unavailable"})
	}
	logger := loggerutil.CreateDefaultLogger("20260915-319")
	var body struct {
		Message        string `json:"message"`
		IdempotencyKey string `json:"idempotency_key"`
		PermissionMode string `json:"permission_mode"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 20*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || strings.TrimSpace(body.Message) == "" || len(body.Message) > 16000 ||
		strings.TrimSpace(body.IdempotencyKey) == "" || len(body.IdempotencyKey) > 128 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid agent request"})
	}
	ctx := c.Request().Context()
	state, err := h.store.LoadResumeState(ctx, userID, c.Param("id"))
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "conversation not found"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not load conversation"})
	}
	profile, err := h.profiles.ResolveVersion(state.Conversation.ProfileSlug, state.Conversation.ProfileVersion, userID)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "service no longer available"})
	}
	if profile.Model != state.Conversation.ModelName {
		return c.JSON(http.StatusConflict, map[string]string{"error": "pinned model is no longer available"})
	}
	permission := profile.PermissionDefault
	if body.PermissionMode != "" {
		permission = body.PermissionMode
	}
	if permission != PermissionAsk && permission != PermissionAuto {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "permission mode must be ask or auto"})
	}
	dependencies, err := h.store.LoadSourceDependencies(ctx, userID, state.Conversation.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not check saved sources"})
	}
	visible := FilterResumeState(ctx, state, dependencies, h.sourceCheck(userID, profile))
	stores, err := h.store.ListGrantedStores(ctx, userID, profile.AllowedKnowledgeStores)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not check knowledge access"})
	}
	if len(stores) > 32 {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "knowledge scope is too broad"})
	}
	// Not every conversation needs the knowledge base. Without a granted store
	// the run proceeds with no knowledge tools, and the knowledge context tells
	// Pi so.
	storeIDs := make([]string, 0, len(stores))
	for _, store := range stores {
		storeIDs = append(storeIDs, store.ID)
	}
	allowedTools := []string{}
	if len(stores) > 0 {
		allowedTools = append(allowedTools, profile.AllowedTools...)
	}
	defaultStoreID := ""
	if h.defaultStoreName != nil {
		defaultStoreID = selectDefaultKnowledgeStore(stores, h.defaultStoreName())
	}
	knowledge, err := h.profiles.BuildKnowledgeContext(stores, defaultStoreID)
	if err != nil {
		logger.Error("build agent knowledge context failed", "service", profile.Slug, "error", err)
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "agent service unavailable"})
	}
	attempt, created, err := h.store.CreateRunAttempt(ctx, userID, state.Conversation.ID, body.IdempotencyKey)
	if errors.Is(err, ErrRunAlreadyActive) || (!created && err == nil) {
		return c.JSON(http.StatusConflict, map[string]any{"error": "run already exists", "run_id": attempt.ID, "status": attempt.Status})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not create run"})
	}
	logger.Info("agent run started", "run_id", attempt.ID, "service", profile.Slug)
	settleCtx := context.WithoutCancel(ctx)
	settleCtx, stopSettlement := context.WithTimeout(settleCtx, 10*time.Second)
	defer stopSettlement()
	outcome := AttemptOutcome{Status: "failed", ErrorCode: "run_failed", ErrorMessage: "The agent run could not complete."}
	streamStarted := false
	var verifiedSources []GatewaySource
	defer func() {
		if _, settleErr := h.store.SetAttemptOutcome(settleCtx, userID, attempt.ID, outcome); settleErr != nil {
			logger.Error("agent run settlement failed", "run_id", attempt.ID, "error", settleErr)
			outcome.Status, outcome.ErrorCode = "failed", "run_settlement_failed"
		} else {
			logger.Info("agent run settled", "run_id", attempt.ID, "status", outcome.Status)
		}
		if streamStarted && ctx.Err() == nil {
			if message := publicRunMessage(outcome); message != "" {
				_ = WriteAgentSSE(c.Response(), AgentPublicEvent{Type: "error", Message: message})
			}
			if outcome.Status == "completed" && len(verifiedSources) > 0 {
				_ = WriteAgentSSE(c.Response(), AgentPublicEvent{Type: "sources", Sources: verifiedSources})
			}
			_ = WriteAgentSSE(c.Response(), AgentPublicEvent{Type: "completion", Status: outcome.Status})
			c.Response().Flush()
		}
	}()
	nextSequence := 1
	for _, message := range state.Messages {
		if message.SequenceNo >= nextSequence {
			nextSequence = message.SequenceNo + 1
		}
	}
	attemptID := attempt.ID
	if _, err := h.store.CreateMessage(ctx, userID, CreateMessageInput{ConversationID: state.Conversation.ID, AttemptID: &attemptID, SequenceNo: nextSequence, Role: "user", Content: strings.TrimSpace(body.Message), Status: "complete"}); err != nil {
		logger.Error("create agent user message failed", "run_id", attempt.ID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not save user message"})
	}
	if state.Conversation.Title == "" {
		if err := h.store.SetConversationTitleIfEmpty(ctx, userID, state.Conversation.ID, titleFromMessage(body.Message)); err != nil {
			logger.Warn("set agent conversation title failed", "run_id", attempt.ID, "error", err)
		}
	}
	assistant, err := h.store.CreateMessage(ctx, userID, CreateMessageInput{ConversationID: state.Conversation.ID, AttemptID: &attemptID, SequenceNo: nextSequence + 1, Role: "assistant", Status: "streaming"})
	if err != nil {
		logger.Error("create agent answer failed", "run_id", attempt.ID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not start answer"})
	}
	claims := RunCapabilityClaims{UserID: userID, ProfileSlug: profile.Slug, ProfileVersion: profile.Version, RunID: attempt.ID,
		AllowedTools: allowedTools, KnowledgeStoreIDs: storeIDs,
		DocumentGroups: append([]string(nil), profile.AllowedDocumentGroups...), MaxEvidenceBytes: profile.Limits.MaxEvidenceBytes}
	capability, err := h.signer.Mint(claims, min(profile.Limits.MaxElapsed+30*time.Second, maxCapabilityLifetime))
	if err != nil {
		logger.Error("mint agent capability failed", "run_id", attempt.ID, "error", err)
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "run authorization unavailable"})
	}
	summary, err := h.store.LoadHistorySummary(ctx, userID, state.Conversation.ID)
	if err != nil {
		logger.Warn("load agent history summary failed", "run_id", attempt.ID, "error", err)
		summary = HistorySummary{}
	}
	if summaryCoversHidden(state.Messages, visible.HiddenMessageIDs, summary.ThroughSeq) {
		// The summary may repeat an answer the user can no longer see.
		logger.Info("agent history summary covers a hidden answer; not used", "run_id", attempt.ID)
		if err := h.store.ClearHistorySummary(ctx, userID, state.Conversation.ID, summary.ThroughSeq); err != nil {
			logger.Warn("clear agent history summary failed", "run_id", attempt.ID, "error", err)
		}
		summary = HistorySummary{}
	}
	budget := h.historyBudget(ctx, profile, logger)
	turns := completeTurns(visible.Messages, visible.SourcesByMessage, summary.ThroughSeq)
	selected := selectHistory(turns, budget, estimateTokens(summary.Text))
	gatewayProfile := MapGatewayProfile(profile, permission)
	gatewayProfile.AllowedTools = allowedTools
	request := GatewayRunRequest{RunID: attempt.ID, ConversationID: state.Conversation.ID, UserID: userID,
		Message: strings.TrimSpace(body.Message), Capability: capability, Profile: gatewayProfile, History: historyMessages(selected), Knowledge: &knowledge, HistorySummary: summary.Text}
	for len(selected) > 0 {
		if raw, err := json.Marshal(request); err == nil && len(raw) <= maxRunRequestBytes {
			break
		}
		selected = selected[1:]
		request.History = historyMessages(selected)
	}
	if len(selected) < len(turns) {
		logger.Info("agent history trimmed", "run_id", attempt.ID, "turns", len(turns), "sent", len(selected), "budget", budget)
	}
	stream, err := h.gateway.Start(ctx, request)
	if err != nil {
		logger.Error("Pi gateway start failed", "run_id", attempt.ID, "error", err)
		outcome.ErrorCode = "gateway_unavailable"
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "Pi gateway unavailable; retry later"})
	}
	defer stream.Close()
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream; charset=utf-8")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-store")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().Header().Set("X-Agent-Run-Id", attempt.ID)
	c.Response().WriteHeader(http.StatusOK)
	streamStarted = true
	collector := NewRunEventCollector(attempt.ID, min(65536, max(4096, profile.Limits.MaxOutputTokens*16)))
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 4096), 128*1024)
	for scanner.Scan() {
		event, acceptErr := collector.Accept(scanner.Bytes())
		if acceptErr != nil {
			logger.Error("unsafe Pi event", "run_id", attempt.ID, "error", acceptErr)
			outcome.ErrorCode = "unsafe_event"
			break
		}
		if event.Type == "" || event.Type == "completion" || event.Type == "sources" {
			continue
		}
		if err := WriteAgentSSE(c.Response(), event); err != nil {
			outcome.Status = "interrupted"
			outcome.ErrorCode = "browser_disconnected"
			break
		}
		c.Response().Flush()
	}
	if err := scanner.Err(); err != nil && outcome.Status != "interrupted" {
		outcome.Status = "interrupted"
		outcome.ErrorCode = "gateway_stream_interrupted"
	}
	if ctx.Err() != nil {
		outcome.Status = "interrupted"
		outcome.ErrorCode = "browser_disconnected"
	}
	result := collector.Result()
	if result.Answer != "" {
		if _, err := h.store.AppendMessageDelta(settleCtx, userID, assistant.ID, result.Answer, "streaming"); err != nil {
			logger.Error("save streamed answer failed", "run_id", attempt.ID, "error", err)
			outcome.ErrorCode = "answer_persistence_failed"
			return nil
		}
	}
	outcome.InputTokens, outcome.OutputTokens = result.InputTokens, result.OutputTokens
	if outcome.Status == "interrupted" {
		return nil
	}
	if result.Status == "" {
		outcome.Status = "interrupted"
		outcome.ErrorCode = "gateway_stream_incomplete"
		return nil
	}
	if result.Status != "completed" {
		outcome.Status, outcome.ErrorCode = mapRunOutcome(result.Status)
		return nil
	}
	toolIDs := make(map[string]string)
	for _, tool := range result.ToolCalls {
		status := "complete"
		if tool.Failed {
			status = "failed"
		}
		inputSummary, _ := json.Marshal(map[string]string{"gateway_tool_call_id": tool.GatewayID})
		id, err := h.store.RecordToolCall(settleCtx, userID, ToolCallInput{AttemptID: attempt.ID, ToolName: tool.ToolName, Status: status, InputSummary: inputSummary, OutputSummary: json.RawMessage(`{}`)})
		if err != nil {
			outcome.ErrorCode = "tool_audit_failed"
			return nil
		}
		toolIDs[tool.GatewayID] = id
	}
	sources := make([]SourceInput, 0, len(result.Sources))
	for _, source := range result.Sources {
		if !stringIn(storeIDs, source.KnowledgeStoreID) {
			outcome.ErrorCode = "source_scope_changed"
			return nil
		}
		for _, tool := range result.ToolCalls {
			if tool.GatewayID == source.ToolCallID && (tool.Failed || tool.ToolName != source.ToolName) {
				outcome.ErrorCode = "failed_tool_source"
				return nil
			}
		}
		toolID := toolIDs[source.ToolCallID]
		if toolID == "" {
			outcome.ErrorCode = "source_tool_missing"
			return nil
		}
		if h.sources == nil || h.sources.CheckSourceWithGroups(settleCtx, userID, profile.AllowedKnowledgeStores, profile.AllowedDocumentGroups,
			SourceRecord{DocumentID: source.DocumentID, Fingerprint: source.Fingerprint, SourceVersion: source.SourceVersion}) != nil {
			outcome.ErrorCode = "source_access_changed"
			return nil
		}
		sources = append(sources, SourceInput{AttemptID: attempt.ID, MessageID: assistant.ID, ToolCallID: toolID,
			DocumentID: source.DocumentID, DocumentTitle: source.SourceTitle, SourceVersion: source.SourceVersion, Fingerprint: source.Fingerprint,
			ArtifactType: source.ArtifactType, ArtifactID: source.ArtifactID, LineStart: source.LineStart, LineEnd: source.LineEnd,
			PageStart: source.Page, PageEnd: source.Page})
	}
	if _, err := h.store.FinalizeAssistantMessage(settleCtx, userID, assistant.ID, len(result.ToolCalls) > 0, sources); err != nil {
		logger.Error("finalize agent answer failed", "run_id", attempt.ID, "error", err)
		outcome.ErrorCode = "answer_finalization_failed"
		return nil
	}
	verifiedSources = result.Sources
	outcome.Status, outcome.ErrorCode, outcome.ErrorMessage = "completed", "", ""
	conversationID := state.Conversation.ID
	runInBackground(func() { h.foldHistory(userID, conversationID, profile, budget) })
	return nil
}

// maxRunRequestBytes leaves headroom under the gateway's 512 KiB body limit.
const maxRunRequestBytes = 480 * 1024

func (h *RunHandler) sourceCheck(userID string, profile PiProfile) func(context.Context, SourceRecord) error {
	return func(ctx context.Context, source SourceRecord) error {
		if h.sources == nil {
			return ErrKnowledgeAccessDenied
		}
		return h.sources.CheckSourceWithGroups(ctx, userID, profile.AllowedKnowledgeStores, profile.AllowedDocumentGroups, source)
	}
}

func (h *RunHandler) historyBudget(ctx context.Context, profile PiProfile, logger interface{ Warn(string, ...any) }) int {
	contextWindow, err := h.gateway.ModelContextWindow(ctx, profile.Provider, profile.Model)
	if err != nil {
		logger.Warn("model context window unavailable; using fallback history budget", "provider", profile.Provider, "model", profile.Model, "error", err)
		contextWindow = 0
	}
	return historyBudget(contextWindow, profile.Limits.MaxHistoryTokens)
}

// foldHistory folds the oldest unsummarized turns into the conversation's
// rolling summary once they no longer fit the history budget. It runs after
// the turn has finished; a failure only means the next turn trims instead.
func (h *RunHandler) foldHistory(userID, conversationID string, profile PiProfile, budget int) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	logger := loggerutil.CreateDefaultLogger("20261004-582")
	state, err := h.store.LoadResumeState(ctx, userID, conversationID)
	if err != nil {
		logger.Warn("history fold: load conversation failed", "conversation_id", conversationID, "error", err)
		return
	}
	dependencies, err := h.store.LoadSourceDependencies(ctx, userID, conversationID)
	if err != nil {
		logger.Warn("history fold: load sources failed", "conversation_id", conversationID, "error", err)
		return
	}
	visible := FilterResumeState(ctx, state, dependencies, h.sourceCheck(userID, profile))
	summary, err := h.store.LoadHistorySummary(ctx, userID, conversationID)
	if err != nil {
		logger.Warn("history fold: load summary failed", "conversation_id", conversationID, "error", err)
		return
	}
	if summaryCoversHidden(state.Messages, visible.HiddenMessageIDs, summary.ThroughSeq) {
		if err := h.store.ClearHistorySummary(ctx, userID, conversationID, summary.ThroughSeq); err != nil {
			logger.Warn("history fold: clear summary failed", "conversation_id", conversationID, "error", err)
			return
		}
		summary = HistorySummary{}
	}
	fold := planFold(completeTurns(visible.Messages, visible.SourcesByMessage, summary.ThroughSeq), budget)
	if len(fold) == 0 {
		return
	}
	text, err := h.gateway.Summarize(ctx, GatewaySummaryRequest{UserID: userID, Provider: profile.Provider, Model: profile.Model,
		SystemPrompt: h.profiles.HistorySummaryPrompt(), PreviousSummary: summary.Text, Messages: historyMessages(fold), MaxTokens: min(1024, budget/4)})
	if err != nil {
		logger.Warn("history fold: summarize failed", "conversation_id", conversationID, "turns", len(fold), "error", err)
		return
	}
	next := HistorySummary{Text: shortenForHistory(text, maxHistoryContentUnits), ThroughSeq: fold[len(fold)-1].AssistantSeq}
	stored, err := h.store.SaveHistorySummary(ctx, userID, conversationID, summary.ThroughSeq, next)
	if err != nil {
		logger.Warn("history fold: save summary failed", "conversation_id", conversationID, "error", err)
		return
	}
	logger.Info("history fold finished", "conversation_id", conversationID, "turns", len(fold), "through_seq", next.ThroughSeq, "stored", stored)
}

func mapRunOutcome(status string) (string, string) {
	switch status {
	case "cancelled":
		return "stopped", "cancelled"
	case "timed_out":
		return "interrupted", "timed_out"
	case "limit_reached":
		return "limit", "output_limit"
	default:
		return "failed", "gateway_failed"
	}
}

func publicRunMessage(outcome AttemptOutcome) string {
	switch outcome.Status {
	case "completed":
		return ""
	case "stopped":
		return "Run stopped. Any partial answer was saved."
	case "limit":
		return "The answer reached its limit. Please ask a narrower question."
	case "interrupted":
		return "The connection was interrupted. Any partial answer was saved."
	default:
		if outcome.ErrorCode == "source_access_changed" {
			return "A source changed or access was revoked. Please try again."
		}
		return "The answer could not be completed. Please try again."
	}
}

func (h *RunHandler) authorizedRunningAttempt(c echo.Context) (string, ResponseAttempt, error) {
	userID, ok := agentUserID(c)
	if !ok {
		return "", ResponseAttempt{}, errors.New("login required")
	}
	if !h.available() {
		return "", ResponseAttempt{}, errors.New("agent service unavailable")
	}
	attempt, err := h.store.GetRunAttempt(c.Request().Context(), userID, c.Param("id"), c.Param("runId"))
	if err != nil || attempt.Status != "running" {
		return "", ResponseAttempt{}, sql.ErrNoRows
	}
	return userID, attempt, nil
}

func (h *RunHandler) Cancel(c echo.Context) error {
	_, attempt, err := h.authorizedRunningAttempt(c)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "active run not found"})
	}
	if err := h.gateway.Cancel(c.Request().Context(), attempt.ID); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "could not stop run"})
	}
	return c.JSON(http.StatusAccepted, map[string]string{"status": "stop_requested"})
}

func (h *RunHandler) Decide(c echo.Context) error {
	_, attempt, err := h.authorizedRunningAttempt(c)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "active run not found"})
	}
	if c.Param("requestId") == "" || len(c.Param("requestId")) > 128 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid permission request"})
	}
	var body struct {
		Allowed *bool `json:"allowed"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.Allowed == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid permission decision"})
	}
	if err := h.gateway.Decide(c.Request().Context(), attempt.ID, c.Param("requestId"), *body.Allowed); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "permission request is no longer active"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "decision_recorded"})
}
