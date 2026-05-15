package openmetadatahandler

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const defaultPublicBasePath = "/integrations/openmetadata/"
const defaultSSOMode = "proxy-only"

var allowedSSOModes = map[string]bool{
	defaultSSOMode:      true,
	"shared-idp":        true,
	"session-bootstrap": true,
}

var resolveCurrentUser = func(c echo.Context, loc string) (*CurrentUser, bool) {
	rc := EchoFactory.NewFromEcho(c, loc)
	defer rc.Close()

	info := rc.IsAuthenticated()
	if info == nil {
		return nil, false
	}

	displayName := strings.TrimSpace(info.UserName)
	if displayName == "" {
		displayName = strings.TrimSpace(strings.TrimSpace(info.FirstName + " " + info.LastName))
	}
	if displayName == "" {
		displayName = strings.TrimSpace(info.Email)
	}

	return &CurrentUser{
		UserID:      info.UserId,
		Email:       info.Email,
		DisplayName: displayName,
	}, true
}

func GetSession(c echo.Context) error {
	user, ok := resolveCurrentUser(c, "CWB_OMD_001")
	if !ok {
		return c.JSON(http.StatusForbidden, ErrorResponse{
			Status:  false,
			Message: "login required",
		})
	}

	cfg, err := loadConfig()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Status:  false,
			Message: err.Error(),
		})
	}

	if user.DisplayName == "" {
		user.DisplayName = cfg.DisplayName
	}

	return c.JSON(http.StatusOK, SessionResponse{
		Status:        true,
		LaunchURL:     cfg.PublicBasePath,
		ProxyBasePath: cfg.PublicBasePath,
		CallbackURL:   callbackURLForRequest(c.Request(), cfg),
		DisplayName:   cfg.DisplayName,
		UserID:        user.UserID,
		SSOMode:       cfg.SSOMode,
		Capabilities: []string{
			"embedded_ui",
			"open_in_new_tab",
			"reload",
		},
		// ChenWeb is the current access gate. Full production SSO still needs
		// stronger upstream identity wiring such as shared IdP exchange or a
		// server-side OpenMetadata session bootstrap flow.
		AuthBoundaryNote: "ChenWeb gates access to the embedded proxy path; production SSO may still require shared IdP exchange or server-side OpenMetadata session bootstrap.",
	})
}

func loadConfig() (config, error) {
	upstreamURL := strings.TrimSpace(os.Getenv("OPENMETADATA_UPSTREAM_URL"))
	if upstreamURL == "" {
		return config{}, errors.New("OPENMETADATA_UPSTREAM_URL is required")
	}

	publicBasePath := normalizeBasePath(os.Getenv("OPENMETADATA_PUBLIC_BASE_PATH"))
	displayName := strings.TrimSpace(os.Getenv("OPENMETADATA_DISPLAY_NAME"))
	if displayName == "" {
		displayName = "OpenMetadata"
	}
	ssoMode, err := normalizeSSOMode(os.Getenv("OPENMETADATA_SSO_MODE"))
	if err != nil {
		return config{}, err
	}
	bearerToken, err := resolveBearerToken(ssoMode, os.Getenv("OPENMETADATA_BEARER_TOKEN"))
	if err != nil {
		return config{}, err
	}

	return config{
		UpstreamURL:    upstreamURL,
		PublicBasePath: publicBasePath,
		DisplayName:    displayName,
		SSOMode:        ssoMode,
		BearerToken:    bearerToken,
	}, nil
}

func normalizeBasePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return defaultPublicBasePath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

func normalizeSSOMode(mode string) (string, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return defaultSSOMode, nil
	}
	if !allowedSSOModes[mode] {
		return "", fmt.Errorf("OPENMETADATA_SSO_MODE must be one of: proxy-only, shared-idp, session-bootstrap")
	}
	return mode, nil
}

func resolveBearerToken(ssoMode string, token string) (string, error) {
	token = strings.TrimSpace(token)
	if ssoMode != "session-bootstrap" {
		return token, nil
	}
	if token == "" {
		return "", fmt.Errorf("OPENMETADATA_BEARER_TOKEN is required when OPENMETADATA_SSO_MODE=session-bootstrap")
	}
	return token, nil
}

func callbackURLForRequest(req *http.Request, cfg config) string {
	if cfg.SSOMode != "shared-idp" {
		return ""
	}
	scheme := requestScheme(req)
	host := req.Host
	if host == "" {
		return ""
	}
	return scheme + "://" + host + "/callback"
}

func requestScheme(req *http.Request) string {
	if forwarded := strings.TrimSpace(req.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		return forwarded
	}
	if req.URL != nil && strings.TrimSpace(req.URL.Scheme) != "" {
		return req.URL.Scheme
	}
	if req.TLS != nil {
		return "https"
	}
	return "http"
}
