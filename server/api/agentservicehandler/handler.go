package agentservicehandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type ConversationHandler struct {
	store    *Store
	profiles *ProfileRegistry
	sources  *CurrentSourceAccessChecker
}

func NewConversationHandler(store *Store, profiles *ProfileRegistry, sources *CurrentSourceAccessChecker) *ConversationHandler {
	return &ConversationHandler{store: store, profiles: profiles, sources: sources}
}

func RegisterConversationRoutes(group *echo.Group, handler *ConversationHandler) {
	group.GET("", handler.ListServices)
	group.GET("/health", handler.Health)
	group.GET("/conversations", handler.ListConversations)
	group.POST("/:slug/conversations", handler.CreateConversation)
	group.GET("/conversations/:id", handler.GetConversation)
	group.DELETE("/conversations/:id", handler.DeleteConversation)
	group.POST("/conversations/:id/messages/:messageId/feedback", handler.Feedback)
}

func agentUserID(c echo.Context) (string, bool) {
	rc := EchoFactory.NewFromEcho(c, "20260915-317")
	defer rc.Close()
	user := rc.IsAuthenticated()
	if user == nil || strings.TrimSpace(user.UserId) == "" {
		return "", false
	}
	return user.UserId, true
}

func (h *ConversationHandler) ListServices(c echo.Context) error {
	userID, ok := agentUserID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "login required"})
	}
	if h == nil || h.profiles == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "agent services unavailable"})
	}
	services := make([]PiProfile, 0, len(h.profiles.active))
	for slug := range h.profiles.active {
		profile, err := h.profiles.Resolve(slug, userID)
		if err == nil {
			services = append(services, profile)
		}
	}
	return c.JSON(http.StatusOK, services)
}

func (h *ConversationHandler) Health(c echo.Context) error {
	if _, ok := agentUserID(c); !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "login required"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *ConversationHandler) CreateConversation(c echo.Context) error {
	userID, ok := agentUserID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "login required"})
	}
	if h == nil || h.store == nil || h.profiles == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "agent services unavailable"})
	}
	slug := strings.TrimSpace(c.Param("slug"))
	profile, err := h.profiles.Resolve(slug, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "service unavailable"})
	}
	var body struct {
		Title string `json:"title"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 4096))
	if err := decoder.Decode(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid conversation request"})
	}
	if len(body.Title) > 200 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "title too long"})
	}
	created, err := h.store.CreateConversation(c.Request().Context(), userID, CreateConversationInput{
		ServiceSlug: slug, ProfileSlug: profile.Slug, ProfileVersion: profile.Version, ModelName: profile.Model, Title: strings.TrimSpace(body.Title),
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not create conversation"})
	}
	return c.JSON(http.StatusCreated, created)
}

func (h *ConversationHandler) ListConversations(c echo.Context) error {
	userID, ok := agentUserID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "login required"})
	}
	if h == nil || h.store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "agent services unavailable"})
	}
	items, err := h.store.ListConversations(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not list conversations"})
	}
	return c.JSON(http.StatusOK, items)
}

func (h *ConversationHandler) GetConversation(c echo.Context) error {
	userID, ok := agentUserID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "login required"})
	}
	if h == nil || h.store == nil || h.profiles == nil || h.sources == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "agent services unavailable"})
	}
	state, err := h.store.LoadResumeState(c.Request().Context(), userID, c.Param("id"))
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
	sources, err := h.store.LoadSourceDependencies(c.Request().Context(), userID, state.Conversation.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not check saved sources"})
	}
	visible := FilterResumeState(c.Request().Context(), state, sources, func(ctx context.Context, source SourceRecord) error {
		return h.sources.CheckSourceWithGroups(ctx, userID, profile.AllowedKnowledgeStores, profile.AllowedDocumentGroups, source)
	})
	return c.JSON(http.StatusOK, visible)
}

func (h *ConversationHandler) DeleteConversation(c echo.Context) error {
	userID, ok := agentUserID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "login required"})
	}
	if h == nil || h.store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "agent services unavailable"})
	}
	err := h.store.DeleteConversation(c.Request().Context(), userID, c.Param("id"))
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "conversation not found"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not delete conversation"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *ConversationHandler) Feedback(c echo.Context) error {
	userID, ok := agentUserID(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "login required"})
	}
	if h == nil || h.store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "agent services unavailable"})
	}
	var body struct {
		Rating  string `json:"rating"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 4096)).Decode(&body); err != nil ||
		(body.Rating != "helpful" && body.Rating != "unhelpful") || len(body.Comment) > 2000 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid feedback"})
	}
	id, err := h.store.UpsertConversationFeedback(c.Request().Context(), userID, c.Param("id"), FeedbackInput{MessageID: c.Param("messageId"), Rating: body.Rating, Comment: body.Comment})
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "message not found"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "could not save feedback"})
	}
	return c.JSON(http.StatusOK, map[string]string{"id": id})
}
