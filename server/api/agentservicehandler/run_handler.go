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
	ListGrantedStoreIDs(context.Context, string, []string) ([]string, error)
	CreateRunAttempt(context.Context, string, string, string) (ResponseAttempt, bool, error)
	CreateMessage(context.Context, string, CreateMessageInput) (Message, error)
	AppendMessageDelta(context.Context, string, string, string, string) (Message, error)
	RecordToolCall(context.Context, string, ToolCallInput) (string, error)
	FinalizeAssistantMessage(context.Context, string, string, bool, []SourceInput) (Message, error)
	SetAttemptOutcome(context.Context, string, string, AttemptOutcome) (ResponseAttempt, error)
	GetRunAttempt(context.Context, string, string, string) (ResponseAttempt, error)
}

type GatewayBridge interface {
	Start(context.Context, GatewayRunRequest) (io.ReadCloser, error)
	Cancel(context.Context, string) error
	Decide(context.Context, string, string, bool) error
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
}

func NewRunHandler(store RunStore, profiles *ProfileRegistry, sources RunSourceChecker, gateway GatewayBridge, signer *CapabilitySigner) *RunHandler {
	return &RunHandler{store: store, profiles: profiles, sources: sources, gateway: gateway, signer: signer}
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
	visible := FilterResumeState(ctx, state, dependencies, func(ctx context.Context, source SourceRecord) error {
		if h.sources == nil {
			return ErrKnowledgeAccessDenied
		}
		return h.sources.CheckSourceWithGroups(ctx, userID, profile.AllowedKnowledgeStores, profile.AllowedDocumentGroups, source)
	})
	storeIDs, err := h.store.ListGrantedStoreIDs(ctx, userID, profile.AllowedKnowledgeStores)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not check knowledge access"})
	}
	if len(storeIDs) == 0 {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "no granted knowledge store for this service"})
	}
	if len(storeIDs) > 32 {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "knowledge scope is too broad"})
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
	assistant, err := h.store.CreateMessage(ctx, userID, CreateMessageInput{ConversationID: state.Conversation.ID, AttemptID: &attemptID, SequenceNo: nextSequence + 1, Role: "assistant", Status: "streaming"})
	if err != nil {
		logger.Error("create agent answer failed", "run_id", attempt.ID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not start answer"})
	}
	claims := RunCapabilityClaims{UserID: userID, ProfileSlug: profile.Slug, ProfileVersion: profile.Version, RunID: attempt.ID,
		AllowedTools: append([]string(nil), profile.AllowedTools...), KnowledgeStoreIDs: storeIDs,
		DocumentGroups: append([]string(nil), profile.AllowedDocumentGroups...), MaxEvidenceBytes: profile.Limits.MaxEvidenceBytes}
	capability, err := h.signer.Mint(claims, min(profile.Limits.MaxElapsed+30*time.Second, maxCapabilityLifetime))
	if err != nil {
		logger.Error("mint agent capability failed", "run_id", attempt.ID, "error", err)
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "run authorization unavailable"})
	}
	history := make([]GatewayHistoryMessage, 0, min(100, len(visible.Messages)))
	for _, message := range visible.Messages {
		if message.Status != "complete" || (message.Role != "user" && message.Role != "assistant") {
			continue
		}
		if len(message.Content) > 16000 {
			continue
		}
		history = append(history, GatewayHistoryMessage{Role: message.Role, Content: message.Content})
	}
	if len(history) > 100 {
		history = history[len(history)-100:]
	}
	request := GatewayRunRequest{RunID: attempt.ID, ConversationID: state.Conversation.ID, UserID: userID,
		Message: strings.TrimSpace(body.Message), Capability: capability, Profile: MapGatewayProfile(profile, permission), History: history}
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
	return nil
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
