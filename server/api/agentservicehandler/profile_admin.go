package agentservicehandler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const profileAdminBodyLimit = 64 << 10

type ProfileAdminHandler struct {
	registry      *ProfileRegistry
	store         ProfileVersionStore
	promptDir     string
	gatewayURL    string
	gatewaySecret string
	httpClient    *http.Client
	now           func() time.Time
	// harnesses, when set, rejects versions their harness cannot run.
	harnesses *HarnessRegistry
}

// WithHarnesses makes activation check each new version against the
// registered harness adapters.
func (h *ProfileAdminHandler) WithHarnesses(harnesses *HarnessRegistry) *ProfileAdminHandler {
	h.harnesses = harnesses
	return h
}

func NewProfileAdminHandler(registry *ProfileRegistry, store ProfileVersionStore, promptDir, gatewayURL, gatewaySecret string) *ProfileAdminHandler {
	if strings.TrimSpace(gatewayURL) == "" {
		gatewayURL = "http://127.0.0.1:4317"
	}
	return &ProfileAdminHandler{
		registry: registry, store: store, promptDir: promptDir,
		gatewayURL: gatewayURL, gatewaySecret: gatewaySecret,
		httpClient: &http.Client{Timeout: 3 * time.Second}, now: time.Now,
	}
}

func RegisterProfileAdminRoutes(group *echo.Group, handler *ProfileAdminHandler) {
	group.GET("", handler.GetState)
	group.PUT("/profiles/:slug", handler.UpdateProfile)
	group.POST("/initialize", handler.Initialize)
	group.POST("/gateway-check", handler.CheckGateway)
}

func (h *ProfileAdminHandler) GetState(c echo.Context) error {
	rc, ok := profileAdminAuth(c, "20261018-401")
	if !ok {
		return nil
	}
	defer rc.Close()
	if h == nil || h.registry == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "profile service unavailable"})
	}
	profiles := make([]adminProfile, 0, 2)
	degraded := !h.registry.VersionStoreReady()
	if !degraded {
		if h.store == nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "profile state unavailable"})
		}
		active, err := h.store.ListActive(c.Request().Context())
		if err != nil {
			rc.GetLogger().Error("load active Pi profiles failed", "error", err)
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "profile state unavailable"})
		}
		for _, profile := range active {
			profiles = append(profiles, toAdminProfile(profile))
		}
	} else {
		for _, slug := range []string{"knowledge-guide", "problem-diagnostics"} {
			version := h.registry.active[slug]
			if profile, ok := h.registry.versions[slug][version]; ok {
				profiles = append(profiles, toAdminProfile(profile))
			}
		}
	}
	return c.JSON(http.StatusOK, map[string]any{
		"gateway_url": safeGatewayURL(h.gatewayURL), "initialized": !degraded, "profiles": profiles,
		"prompt_choices": map[string][]string{
			"knowledge-guide":     {"prompt-agent-knowledge-guide-v1.md"},
			"problem-diagnostics": {"prompt-agent-problem-diagnosis-v1.md"},
		},
	})
}

func safeGatewayURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

type profileUpdateRequest struct {
	Slug                   string             `json:"slug"`
	FriendlyName           string             `json:"friendly_name"`
	Description            string             `json:"description"`
	Provider               string             `json:"provider"`
	Model                  string             `json:"model"`
	ThinkingLevel          string             `json:"thinking_level"`
	PromptFile             string             `json:"prompt_file"`
	AllowedKnowledgeStores []string           `json:"allowed_knowledge_stores"`
	AllowedDocumentGroups  []string           `json:"allowed_document_groups"`
	PermissionDefault      string             `json:"permission_default"`
	Enabled                bool               `json:"enabled"`
	PilotUsers             []string           `json:"pilot_users"`
	ExpectedActiveVersion  string             `json:"expected_active_version"`
	Limits                 profileAdminLimits `json:"limits"`
}

type profileAdminLimits struct {
	MaxToolCalls              int `json:"max_tool_calls"`
	MaxElapsed                int `json:"max_elapsed"`
	MaxOutputTokens           int `json:"max_output_tokens"`
	MaxEvidenceBytes          int `json:"max_evidence_bytes"`
	MaxHistoryTokens          int `json:"max_history_tokens"`
	MaxHistoryToolResultBytes int `json:"max_history_tool_result_bytes"`
}

type adminProfile struct {
	AgentServiceProfile
	Limits     profileAdminLimits `json:"limits"`
	PilotUsers []string           `json:"pilot_users"`
}

func toAdminProfile(profile AgentServiceProfile) adminProfile {
	return adminProfile{AgentServiceProfile: profile, Limits: profileAdminLimits{
		MaxToolCalls: profile.Limits.MaxToolCalls, MaxElapsed: int(profile.Limits.MaxElapsed / time.Second),
		MaxOutputTokens: profile.Limits.MaxOutputTokens, MaxEvidenceBytes: profile.Limits.MaxEvidenceBytes,
		MaxHistoryTokens:          profile.Limits.MaxHistoryTokens,
		MaxHistoryToolResultBytes: profile.Limits.MaxHistoryToolResultBytes,
	}, PilotUsers: append([]string(nil), profile.PilotUsers...)}
}

func (h *ProfileAdminHandler) UpdateProfile(c echo.Context) error {
	rc, ok := profileAdminAuth(c, "20261018-402")
	if !ok {
		return nil
	}
	defer rc.Close()
	if h == nil || h.registry == nil || h.store == nil || !h.registry.VersionStoreReady() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "profile store is not initialized"})
	}
	var request profileUpdateRequest
	if err := decodeProfileAdminJSON(c, &request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	candidate := ProfileCandidate{
		Slug: strings.TrimSpace(c.Param("slug")), FriendlyName: request.FriendlyName,
		Description: request.Description, Provider: request.Provider, Model: request.Model,
		ThinkingLevel: request.ThinkingLevel, PromptFile: request.PromptFile,
		AllowedKnowledgeStores: request.AllowedKnowledgeStores, AllowedDocumentGroups: request.AllowedDocumentGroups,
		PermissionDefault: request.PermissionDefault, Enabled: request.Enabled, PilotUsers: request.PilotUsers,
	}
	maxElapsed, err := profileElapsedDuration(request.Limits.MaxElapsed)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	candidate.Limits = ProfileLimits{
		MaxToolCalls: request.Limits.MaxToolCalls, MaxElapsed: maxElapsed,
		MaxOutputTokens: request.Limits.MaxOutputTokens, MaxEvidenceBytes: request.Limits.MaxEvidenceBytes,
		MaxHistoryTokens:          request.Limits.MaxHistoryTokens,
		MaxHistoryToolResultBytes: request.Limits.MaxHistoryToolResultBytes,
	}
	if strings.TrimSpace(request.ExpectedActiveVersion) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "expected_active_version is required"})
	}
	validated, err := ValidateProfileCandidate(h.promptDir, candidate)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if h.harnesses != nil {
		if err := h.harnesses.CheckProfile(validated.Profile); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
	}
	user := rc.IsAuthenticated()
	if user == nil || strings.TrimSpace(user.UserId) == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}
	activated, previous, err := h.store.ActivateVersion(c.Request().Context(), validated.Profile, request.ExpectedActiveVersion, user.UserId)
	if errors.Is(err, ErrProfileVersionConflict) {
		return c.JSON(http.StatusConflict, map[string]string{"error": "active profile version changed; reload before saving", "active_version": previous})
	}
	if err != nil {
		rc.GetLogger().Error("activate Pi profile version failed", "slug", validated.Profile.Slug, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "profile update failed"})
	}
	return c.JSON(http.StatusOK, map[string]any{"profile": toAdminProfile(activated), "previous_version": previous})
}

func profileElapsedDuration(seconds int) (time.Duration, error) {
	if seconds < 1 || seconds > 600 {
		return 0, errors.New("max_elapsed must be between 1 and 600 seconds")
	}
	return time.Duration(seconds) * time.Second, nil
}

func (h *ProfileAdminHandler) Initialize(c echo.Context) error {
	rc, ok := profileAdminAuth(c, "20261018-403")
	if !ok {
		return nil
	}
	defer rc.Close()
	if h == nil || h.registry == nil || h.store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "profile service unavailable"})
	}
	if err := h.registry.EnsureVersionStoreSeeded(c.Request().Context()); err != nil {
		rc.GetLogger().Error("initialize Pi profile store failed", "error", err)
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "profile store initialization failed"})
	}
	profiles, err := h.store.ListActive(c.Request().Context())
	if err != nil {
		rc.GetLogger().Error("read initialized Pi profiles failed", "error", err)
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "profile store initialized but state is unavailable"})
	}
	adminProfiles := make([]adminProfile, 0, len(profiles))
	for _, profile := range profiles {
		adminProfiles = append(adminProfiles, toAdminProfile(profile))
	}
	return c.JSON(http.StatusOK, map[string]any{"initialized": true, "profiles": adminProfiles})
}

type gatewayCheckResponse struct {
	Status    string    `json:"status"`
	CheckedAt time.Time `json:"checked_at"`
	ElapsedMS int64     `json:"elapsed_ms"`
}

func (h *ProfileAdminHandler) CheckGateway(c echo.Context) error {
	rc, ok := profileAdminAuth(c, "20261018-404")
	if !ok {
		return nil
	}
	defer rc.Close()
	clock := time.Now
	if h != nil && h.now != nil {
		clock = h.now
	}
	checkedAt := clock().UTC()
	started := time.Now()
	response := gatewayCheckResponse{Status: "unavailable", CheckedAt: checkedAt}
	if h == nil || strings.TrimSpace(h.gatewayURL) == "" || strings.TrimSpace(h.gatewaySecret) == "" {
		return c.JSON(http.StatusOK, response)
	}
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(h.gatewayURL), "/"))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return c.JSON(http.StatusOK, response)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/health"
	base.RawQuery = ""
	base.Fragment = ""
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+h.gatewaySecret)
		client := h.httpClient
		if client == nil {
			client = &http.Client{Timeout: 3 * time.Second}
		}
		resp, requestErr := client.Do(req)
		if requestErr == nil {
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				response.Status = "healthy"
			}
			_ = resp.Body.Close()
		} else {
			rc.GetLogger().Warn("Pi gateway health check failed", "error", requestErr)
		}
	}
	response.ElapsedMS = time.Since(started).Milliseconds()
	return c.JSON(http.StatusOK, response)
}

func profileAdminAuth(c echo.Context, loc string) (ApiTypes.RequestContext, bool) {
	rc := EchoFactory.NewFromEcho(c, loc)
	user := rc.IsAuthenticated()
	if user == nil {
		_ = c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		rc.Close()
		return rc, false
	}
	admin := user.Admin || user.IsOwner
	if !admin {
		for _, role := range user.Roles {
			role = strings.ToLower(strings.TrimSpace(role))
			if slices.Contains([]string{"admin", "root"}, role) {
				admin = true
				break
			}
		}
	}
	if !admin {
		_ = c.JSON(http.StatusForbidden, map[string]string{"error": "admin access required"})
		rc.Close()
		return rc, false
	}
	return rc, true
}

func decodeProfileAdminJSON(c echo.Context, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, profileAdminBodyLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
