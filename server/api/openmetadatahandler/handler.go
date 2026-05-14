package openmetadatahandler

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const defaultPublicBasePath = "/integrations/openmetadata/"

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
		DisplayName:   cfg.DisplayName,
		UserID:        user.UserID,
		Capabilities: []string{
			"embedded_ui",
			"open_in_new_tab",
			"reload",
		},
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

	return config{
		UpstreamURL:    upstreamURL,
		PublicBasePath: publicBasePath,
		DisplayName:    displayName,
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
