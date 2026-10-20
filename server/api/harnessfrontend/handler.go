package harnessfrontend

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

var (
	sessionIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	permissionModes  = []string{"default", "acceptEdits", "plan"}
	// eventTypes are the protocol-v1 turn events relayed to the browser.
	eventTypes = []string{"session", "text", "tool_use", "tool_result", "permission_request", "status", "result", "error"}
)

// maxEventBytes bounds one event line; a tool_use can carry a whole file.
const maxEventBytes = 16 << 20

type Handler struct{ registry *Registry }

func NewHandler(registry *Registry) *Handler { return &Handler{registry: registry} }

// RegisterRoutes adds the front-end routes. They are admin-only: a harness
// such as Claude Code has shell and file access as the server's OS user.
func RegisterRoutes(group *echo.Group, h *Handler) {
	group.GET("", h.ListHarnesses)
	group.GET("/:harness/sessions", h.ListSessions)
	group.GET("/:harness/sessions/:sessionId/messages", h.SessionMessages)
	group.POST("/:harness/turns", h.Turn)
	group.POST("/:harness/sessions/:sessionId/interrupt", h.Interrupt)
	group.POST("/:harness/sessions/:sessionId/permissions/:requestId", h.Decide)
}

func adminOnly(c echo.Context, loc string) (ApiTypes.RequestContext, bool) {
	rc := EchoFactory.NewFromEcho(c, loc)
	user := rc.IsAuthenticated()
	if user == nil {
		_ = c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		rc.Close()
		return rc, false
	}
	admin := user.Admin || user.IsOwner
	for _, role := range user.Roles {
		role = strings.ToLower(strings.TrimSpace(role))
		admin = admin || role == "admin" || role == "root"
	}
	if !admin {
		_ = c.JSON(http.StatusForbidden, map[string]string{"error": "admin access required"})
		rc.Close()
		return rc, false
	}
	return rc, true
}

// host authorizes the caller and resolves the :harness parameter. On failure
// it has already written the response.
func (h *Handler) host(c echo.Context, loc string) (ApiTypes.RequestContext, *Host, bool) {
	rc, ok := adminOnly(c, loc)
	if !ok {
		return nil, nil, false
	}
	host, found := h.registry.Get(c.Param("harness"))
	if !found {
		_ = c.JSON(http.StatusNotFound, map[string]string{"error": "harness not configured"})
		rc.Close()
		return nil, nil, false
	}
	return rc, host, true
}

func (h *Handler) ListHarnesses(c echo.Context) error {
	rc, ok := adminOnly(c, "20261020-611")
	if !ok {
		return nil
	}
	defer rc.Close()
	return c.JSON(http.StatusOK, map[string]any{"harnesses": h.registry.Names()})
}

func pageParams(c echo.Context, defaultLimit int) (int, int) {
	limit, err := strconv.Atoi(c.QueryParam("limit"))
	if err != nil || limit <= 0 {
		limit = defaultLimit
	}
	offset, err := strconv.Atoi(c.QueryParam("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (h *Handler) passThrough(c echo.Context, rc ApiTypes.RequestContext, host *Host, action string, response Response, err error) error {
	if err != nil {
		rc.GetLogger().Error("harness gateway request failed", "harness", host.Name(), "action", action, "error", err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "harness unavailable"})
	}
	return c.JSONBlob(response.Status, response.Body)
}

func (h *Handler) ListSessions(c echo.Context) error {
	rc, host, ok := h.host(c, "20261020-612")
	if !ok {
		return nil
	}
	defer rc.Close()
	limit, offset := pageParams(c, 50)
	response, err := host.ListSessions(c.Request().Context(), limit, offset)
	return h.passThrough(c, rc, host, "list_sessions", response, err)
}

func (h *Handler) SessionMessages(c echo.Context) error {
	rc, host, ok := h.host(c, "20261020-613")
	if !ok {
		return nil
	}
	defer rc.Close()
	sessionID := c.Param("sessionId")
	if !sessionIDPattern.MatchString(sessionID) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid session id"})
	}
	limit, offset := pageParams(c, 200)
	response, err := host.SessionMessages(c.Request().Context(), sessionID, limit, offset)
	return h.passThrough(c, rc, host, "session_messages", response, err)
}

func (h *Handler) Interrupt(c echo.Context) error {
	rc, host, ok := h.host(c, "20261020-614")
	if !ok {
		return nil
	}
	defer rc.Close()
	sessionID := c.Param("sessionId")
	if !sessionIDPattern.MatchString(sessionID) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid session id"})
	}
	rc.GetLogger().Info("harness turn interrupt requested", "harness", host.Name(), "session_id", sessionID)
	response, err := host.Interrupt(c.Request().Context(), sessionID)
	return h.passThrough(c, rc, host, "interrupt", response, err)
}

func (h *Handler) Decide(c echo.Context) error {
	rc, host, ok := h.host(c, "20261020-615")
	if !ok {
		return nil
	}
	defer rc.Close()
	sessionID, requestID := c.Param("sessionId"), c.Param("requestId")
	if !sessionIDPattern.MatchString(sessionID) || !requestIDPattern.MatchString(requestID) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid permission request"})
	}
	var body struct {
		Behavior     string          `json:"behavior"`
		UpdatedInput json.RawMessage `json:"updated_input"`
		Message      string          `json:"message"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || (body.Behavior != "allow" && body.Behavior != "deny") || len(body.Message) > 2000 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid permission decision"})
	}
	if len(body.UpdatedInput) > 0 && !json.Valid(body.UpdatedInput) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid permission decision"})
	}
	rc.GetLogger().Info("harness permission decided", "harness", host.Name(), "session_id", sessionID, "request_id", requestID, "behavior", body.Behavior)
	response, err := host.Decide(c.Request().Context(), sessionID, requestID, Decision{Behavior: body.Behavior, UpdatedInput: body.UpdatedInput, Message: body.Message})
	return h.passThrough(c, rc, host, "decide", response, err)
}

// Turn sends one message and relays the harness's events as SSE. When the
// browser disconnects, the gateway request is cancelled and the harness
// interrupts the turn.
func (h *Handler) Turn(c echo.Context) error {
	rc, host, ok := h.host(c, "20261020-616")
	if !ok {
		return nil
	}
	defer rc.Close()
	logger := rc.GetLogger()
	var body struct {
		SessionID      string `json:"session_id"`
		Message        string `json:"message"`
		PermissionMode string `json:"permission_mode"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 512*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || strings.TrimSpace(body.Message) == "" || len(body.Message) > 100_000 ||
		(body.SessionID != "" && !sessionIDPattern.MatchString(body.SessionID)) ||
		(body.PermissionMode != "" && !slices.Contains(permissionModes, body.PermissionMode)) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid turn request"})
	}
	ctx := c.Request().Context()
	stream, refused, err := host.Turn(ctx, TurnRequest{SessionID: body.SessionID, Message: body.Message, PermissionMode: body.PermissionMode})
	if err != nil {
		logger.Error("harness turn start failed", "harness", host.Name(), "session_id", body.SessionID, "error", err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "harness unavailable"})
	}
	if stream == nil {
		return c.JSONBlob(refused.Status, refused.Body)
	}
	defer stream.Close()
	logger.Info("harness turn started", "harness", host.Name(), "session_id", body.SessionID)
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream; charset=utf-8")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-store")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 64*1024), maxEventBytes)
	status := "ended"
	for scanner.Scan() {
		line := scanner.Bytes()
		var event struct {
			Type      string `json:"type"`
			SessionID string `json:"sessionId"`
			Status    string `json:"status"`
		}
		if err := json.Unmarshal(line, &event); err != nil || !slices.Contains(eventTypes, event.Type) {
			// Newer gateways may add event types; skip what this version does not know.
			continue
		}
		if event.Type == "result" {
			status = event.Status
			logger.Info("harness turn finished", "harness", host.Name(), "session_id", event.SessionID, "status", event.Status)
		}
		if _, err := fmt.Fprintf(c.Response(), "event: %s\ndata: %s\n\n", event.Type, line); err != nil {
			logger.Info("harness turn client disconnected", "harness", host.Name(), "session_id", body.SessionID)
			return nil
		}
		c.Response().Flush()
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		logger.Warn("harness turn stream failed", "harness", host.Name(), "session_id", body.SessionID, "status", status, "error", err)
		_, _ = fmt.Fprintf(c.Response(), "event: error\ndata: %s\n\n", `{"type":"error","message":"The harness stream was interrupted."}`)
		c.Response().Flush()
	}
	return nil
}
