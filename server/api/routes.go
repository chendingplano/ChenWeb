package api

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/Dashboard01"
	EchartData "github.com/chendingplano/deepdoc/server/api/EchartDemo"
	"github.com/chendingplano/deepdoc/server/api/aiassistanthandler"
	"github.com/chendingplano/deepdoc/server/api/buttonhandler"
	"github.com/chendingplano/deepdoc/server/api/chatterhandler"
	"github.com/chendingplano/deepdoc/server/api/confighandler"
	"github.com/chendingplano/deepdoc/server/api/docgenhandler"
	"github.com/chendingplano/deepdoc/server/api/dspyhandler"
	"github.com/chendingplano/deepdoc/server/api/flowhandler"
	"github.com/chendingplano/deepdoc/server/api/custreqloghandler"
	"github.com/chendingplano/deepdoc/server/api/kbhandler"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/loggerutil"
	authmiddleware "github.com/chendingplano/shared/go/authmiddleware"
	"github.com/labstack/echo/v4"
)

//go:embed all:webbuild
var webBuild embed.FS

func RegisterRoutes(e *echo.Echo) error {
	// This function registers all API routes and returns the Echo instance.
	// It also sets up the frontend handler, either as a reverse proxy to a
	// development server or serving embedded static files.
	// This function should be called first during server initialization.
	// Other route registrations (such as Auth) can be done afterwards.
	logger := loggerutil.CreateDefaultLogger("CWB_0206153400")
	logger.Info("Register API routes")

	is_dev := os.Getenv("APP_ENV") != "production"
	useEmbedFrontend := os.Getenv("USE_EMBED_FRONTEND") == "true"
	logger.Info("useEmbedFrontend", "useEmbedFrontend", useEmbedFrontend, "is_dev", is_dev)

	var frontendHandler http.Handler
	if !useEmbedFrontend {
		frontendURLEnv := os.Getenv("VITE_DEV_ONLY_URL")
		if frontendURLEnv == "" {
			error_msg := "missing VITE_DEV_ONLY_URL env var (CWB_RTR_046)"
			return fmt.Errorf("%s", error_msg)
		}

		frontendURL, err := url.Parse(frontendURLEnv)
		logger.Info("FrontendURL", "frontendURL", frontendURL)
		if err != nil {
			error_msg := fmt.Errorf("failed to parse VITE_DEV_ONLY_URL env %s: %w (MID_RTS_042)", frontendURLEnv, err)
			return error_msg
		}

		proxy := httputil.NewSingleHostReverseProxy(frontendURL)
		frontendHandler = proxy
	} else {
		logger.Info("Production deployment")
		webBuildFS, err := fs.Sub(webBuild, "webbuild")
		if err != nil {
			error_msg := fmt.Errorf("failed to get webbuild subtree: %w (MID_RTS_050)", err)
			return error_msg
		}
		fileServer := http.FileServerFS(webBuildFS)
		frontendHandler = fileServer
	}

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			rc := EchoFactory.NewFromEcho(c, "CWB_RTR_076")
			path := c.Request().URL.Path
			ctx := c.Request().Context()
			new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, "CWB_RTS_072")
			reqID := ApiUtils.GenerateRequestID("e")
			new_ctx1 := context.WithValue(new_ctx, ApiTypes.RequestIDKey, reqID)

			// Create a new request with the updated context
			newReq := c.Request().WithContext(new_ctx1)
			c.SetRequest(newReq)

			// Public endpoints.
			// IMPORTANT: need to load from a configuration file!
			publicPaths := map[string]bool{
				"/":               true,
				"/login":          true,
				"/example-login":  true,
				"/oauth/callback": true, // Public route for password reset and OAuth callbacks
				"/verify-2fa":     true, // 2FA verification page (user has AAL1 session but needs AAL2)
				// Add other public pages here
			}

			// Let Echo handle / so we can redirect it
			if path == "/" {
				logger.Info("Root path, let Echo handle redirect")
				return next(c)
			}

			// Other public paths should be served by frontend
			if publicPaths[path] {
				logger.Info("Public frontend URL", "path", path)
				frontendHandler.ServeHTTP(c.Response(), c.Request())
				return nil
			}

			// Paths not starting with "/api", "/auth", "/shared_api", or "/_" are treated as frontend routes
			if !strings.HasPrefix(path, "/api") &&
				!strings.HasPrefix(path, "/auth") &&
				!strings.HasPrefix(path, "/shared_api") &&
				path != "/_" &&
				!strings.HasPrefix(path, "/_/") {

				// Exclude development-related paths
				if strings.HasPrefix(path, "/node_modules/") ||
					strings.HasPrefix(path, "/@") ||
					strings.HasPrefix(path, "/src/") ||
					strings.HasPrefix(path, "/.well-known") ||
					(is_dev && strings.HasPrefix(path, "/.svelte-kit")) ||
					strings.Contains(path, "/__") {
					// Let these development assets pass through without auth check
					frontendHandler.ServeHTTP(c.Response(), c.Request())
					return nil
				}

				// Check if it's a public path
				// Protected frontend route → check auth
				// IMPORTANT: need cache!
				user_info, err := authmiddleware.IsAuthenticated(rc)
				if err != nil {
					// Redirect to login (for browser) or 401 (for API-like requests)
					logger.Info("Not logged in. Redirect to", "error", err, "path", path)
					if authmiddleware.IsHTMLRequest(c) {
						return c.Redirect(http.StatusFound, "/login")
					}
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Login required"})
				}

				if user_info != nil {
					logger.Info("Authentication passed", "user email", user_info.Email, "path", path)
				}

				frontendHandler.ServeHTTP(c.Response(), c.Request())
				return nil
			}
			return next(c)
		}
	})

	e.DELETE("/auth/logout", func(c echo.Context) error {
		// Clear the session cookie
		logger.Info("Handle logout")
		c.SetCookie(&http.Cookie{
			Name:     "session_id",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   os.Getenv("ENV") == "production", // false in dev, true in prod
		})
		return c.NoContent(http.StatusNoContent)
	})

	// Add the endpoint '/api/config' (public endpoint for frontend to fetch config)
	e.GET("/api/config", confighandler.GetConfig)

	// Create the routing group '/api/v1'
	apiGroup := e.Group("/api/v1")
	apiGroup.Use(authmiddleware.AuthMiddleware)

	// Add the endpoint '/api/v1/health'
	apiGroup.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{"status": "ok"})
	})

	// Add the endpoint 'api/v1/button-click' (testing only)
	apiGroup.POST("/button-click", buttonhandler.HandleButtonClick)

	// Add the endpoint 'api/v1/echart-data/demo-01'
	apiGroup.GET("/echart-data/demo-01", EchartData.RetrieveDataForEchartDemo01)

	// Add the endpoint 'api/v1/echart-data/demo-02'
	apiGroup.GET("/echart-data/demo-02", EchartData.RetrieveDataForEchartDemo02)

	// Add the endpoint 'api/v1/echart-data/demo-03'
	apiGroup.GET("/echart-data/demo-03", EchartData.RetrieveDataForEchartDemo03)

	// Add the endpoint 'api/v1/echart-data/demo-04'
	apiGroup.GET("/echart-data/demo-04", EchartData.RetrieveDataForEchartDemo04)

	// Add the endpoint '/api/vi/dashboard-01-data'
	apiGroup.GET("/dashboard-01-data", Dashboard01.RetrieveDataForDashboard01)

	// Add the endpoint '/api/vi/retrieve-process-data'
	apiGroup.GET("/retrieve-process-data", Dashboard01.RetrieveDataForDashboard01)

	// Flow Canvas
	apiGroup.GET("/flow-node-types", flowhandler.GetNodeTypes)
	apiGroup.GET("/flows/default", flowhandler.GetDefaultFlow)
	apiGroup.GET("/flows", flowhandler.ListFlows)
	apiGroup.POST("/flows", flowhandler.CreateFlow)
	apiGroup.GET("/flows/:id", flowhandler.GetFlow)
	apiGroup.PUT("/flows/:id", flowhandler.UpdateFlow)
	apiGroup.DELETE("/flows/:id", flowhandler.DeleteFlow)
	apiGroup.PUT("/flows/:id/default", flowhandler.SetDefaultFlow)
	apiGroup.POST("/flows/:id/fork", flowhandler.ForkFlow)
	apiGroup.POST("/flows/:id/template", flowhandler.SaveAsTemplate)

	// AI Assistant (home2) endpoints
	apiGroup.GET("/ai-assistant/dashboard", aiassistanthandler.GetDashboard)
	apiGroup.GET("/ai-assistant/agents", aiassistanthandler.GetAgents)
	apiGroup.POST("/ai-assistant/agents", aiassistanthandler.CreateAgent)
	apiGroup.GET("/ai-assistant/skills", aiassistanthandler.GetSkills)
	apiGroup.GET("/ai-assistant/applications", aiassistanthandler.GetApplications)
	apiGroup.GET("/ai-assistant/knowledge-base", aiassistanthandler.GetKnowledgeBase)
	apiGroup.GET("/ai-assistant/user-info", aiassistanthandler.GetUserInfo)
	apiGroup.GET("/ai-assistant/settings", aiassistanthandler.GetSettings)
	apiGroup.PUT("/ai-assistant/settings", aiassistanthandler.UpdateSettings)
	apiGroup.PUT("/ai-assistant/agents/:id", aiassistanthandler.UpdateAgent)
	apiGroup.DELETE("/ai-assistant/agents/:id", aiassistanthandler.DeleteAgent)
	apiGroup.POST("/ai-assistant/skills", aiassistanthandler.CreateSkill)
	apiGroup.DELETE("/ai-assistant/skills/:id", aiassistanthandler.DeleteSkill)
	apiGroup.POST("/ai-assistant/knowledge-base/documents", aiassistanthandler.ImportDocument)
	apiGroup.DELETE("/ai-assistant/knowledge-base/documents/:id", aiassistanthandler.DeleteDocument)

	// Chatter (home3 chat page) endpoints
	apiGroup.GET("/chatter/settings", chatterhandler.GetSettings)
	apiGroup.PUT("/chatter/settings", chatterhandler.UpdateSettings)
	apiGroup.GET("/chatter/prompts", chatterhandler.GetPrompts)
	apiGroup.GET("/chatter/slash-commands", chatterhandler.GetSlashCommands)
	apiGroup.GET("/chatter/sessions", chatterhandler.ListSessions)
	apiGroup.POST("/chatter/sessions", chatterhandler.CreateSession)
	apiGroup.GET("/chatter/sessions/:id/dialogs", chatterhandler.GetDialogs)
	apiGroup.POST("/chatter/sessions/:id/messages", chatterhandler.SendMessage)

	// Customer request log endpoint
	apiGroup.POST("/cust_request_logs", custreqloghandler.CreateCustRequestLog)

	// Knowledge Base (home3) endpoints
	apiGroup.GET("/kb/inputs", kbhandler.ListInputs)

	// DSPy Prompt Studio endpoints
	apiGroup.POST("/dspy/prompts", dspyhandler.CreatePrompt)
	apiGroup.GET("/dspy/prompts", dspyhandler.ListPrompts)
	apiGroup.GET("/dspy/prompts/:id", dspyhandler.GetPrompt)
	apiGroup.PUT("/dspy/prompts/:id", dspyhandler.UpdatePrompt)
	apiGroup.DELETE("/dspy/prompts/:id", dspyhandler.DeletePrompt)
	apiGroup.POST("/dspy/optimize", dspyhandler.OptimizePrompt)

	// Doc Generation endpoints
	apiGroup.POST("/docgen/jobs", docgenhandler.SubmitJob)
	apiGroup.GET("/docgen/jobs", docgenhandler.ListJobs)
	apiGroup.GET("/docgen/jobs/:id", docgenhandler.GetJob)
	apiGroup.GET("/docgen/queries", docgenhandler.ListQueries)
	apiGroup.POST("/docgen/queries", docgenhandler.CreateQuery)
	apiGroup.PUT("/docgen/queries/:id", docgenhandler.UpdateQuery)
	apiGroup.DELETE("/docgen/queries/:id", docgenhandler.DeleteQuery)
	apiGroup.GET("/docgen/templates", docgenhandler.ListTemplates)
	apiGroup.POST("/docgen/templates", docgenhandler.UploadTemplate)

	// Redirects root (/) to /login (since / is public but should show login by default).
	e.GET("/", func(c echo.Context) error {
		return c.Redirect(http.StatusFound, "/login")
	})
	return nil
}
