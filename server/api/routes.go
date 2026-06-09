package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/Dashboard01"
	EchartData "github.com/chendingplano/deepdoc/server/api/EchartDemo"
	"github.com/chendingplano/deepdoc/server/api/agentplatformhandler"
	"github.com/chendingplano/deepdoc/server/api/aiassistanthandler"
	"github.com/chendingplano/deepdoc/server/api/buttonhandler"
	"github.com/chendingplano/deepdoc/server/api/chatterhandler"
	"github.com/chendingplano/deepdoc/server/api/confighandler"
	"github.com/chendingplano/deepdoc/server/api/custreqloghandler"
	"github.com/chendingplano/deepdoc/server/api/diaryhandler"
	"github.com/chendingplano/deepdoc/server/api/docgenhandler"
	"github.com/chendingplano/deepdoc/server/api/dspyhandler"
	"github.com/chendingplano/deepdoc/server/api/flowhandler"
	"github.com/chendingplano/deepdoc/server/api/jetstreamhandler"
	"github.com/chendingplano/deepdoc/server/api/kbhandler"
	"github.com/chendingplano/deepdoc/server/api/kehandler"
	"github.com/chendingplano/deepdoc/server/api/openmetadatahandler"
	"github.com/chendingplano/deepdoc/server/api/promptoptimizerhandler"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/loggerutil"
	authmiddleware "github.com/chendingplano/shared/go/authmiddleware"
	"github.com/labstack/echo/v4"
)

//go:embed all:webbuild
var webBuild embed.FS

const rootFaviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="16" fill="#102033"/><path fill="#5eead4" d="M18 17h28v8H27v14h19v8H18z"/><path fill="#f8fafc" d="M30 28h16v8H30z"/></svg>`

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
			path := c.Request().URL.Path
			ctx := c.Request().Context()
			reqID, _ := c.Get(string(ApiTypes.RequestIDKey)).(string)
			if reqID == "" {
				reqID, _ = ctx.Value(ApiTypes.RequestIDKey).(string)
			}
			if reqID == "" {
				reqID = ApiUtils.GenerateRequestID("e")
			}
			new_ctx := context.WithValue(ctx, ApiTypes.CallFlowKey, "CWB_RTS_072")
			new_ctx1 := context.WithValue(new_ctx, ApiTypes.RequestIDKey, reqID)

			// Create a new request with the updated context
			newReq := c.Request().WithContext(new_ctx1)
			c.SetRequest(newReq)
			c.Set(string(ApiTypes.RequestIDKey), reqID)
			rc := EchoFactory.NewFromEcho(c, "CWB_RTR_076")

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

			if path == "/callback" || path == "/auth/callback" || isLocalFaviconPath(path) {
				return next(c)
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

			// Paths not starting with "/api", "/auth", "/shared_api", "/ws",
			// or "/_" are treated as frontend routes.
			if !strings.HasPrefix(path, "/api") &&
				!strings.HasPrefix(path, "/auth") &&
				!strings.HasPrefix(path, "/integrations/openmetadata") &&
				!strings.HasPrefix(path, "/shared_api") &&
				!strings.HasPrefix(path, "/ws") &&
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

	handleLogout := func(c echo.Context) error {
		logger.Info("Handle logout")

		// Best effort: terminate Kratos session if present.
		// We forward the incoming browser cookies to Kratos and execute the logout URL.
		logoutFromKratos(c.Request(), logger)

		// Always clear app session cookie.
		c.SetCookie(&http.Cookie{
			Name:     "session_id",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   os.Getenv("ENV") == "production", // false in dev, true in prod
		})

		// Keep response format compatible with shared auth store.
		return c.JSON(http.StatusOK, map[string]string{"redirect_url": "/login"})
	}

	// Support both methods because different frontend paths currently use different verbs.
	e.DELETE("/auth/logout", handleLogout)
	e.POST("/auth/logout", handleLogout)

	// Add the endpoint '/api/config' (public endpoint for frontend to fetch config)
	e.GET("/api/config", confighandler.GetConfig)

	// Create the routing group '/api/v1'
	apiGroup := e.Group("/api/v1")
	apiGroup.Use(authmiddleware.AuthMiddleware)

	// Add the endpoint '/api/v1/health'
	apiGroup.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{"status": "ok"})
	})

	apiGroup.GET("/integrations/openmetadata/session", openmetadatahandler.GetSession)

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
	apiGroup.GET("/kb/stores", kbhandler.ListKnowledgeStores)
	apiGroup.POST("/kb/stores", kbhandler.CreateKnowledgeStore)
	apiGroup.PUT("/kb/stores/:id", kbhandler.UpdateKnowledgeStore)
	apiGroup.DELETE("/kb/stores/:id", kbhandler.DeleteKnowledgeStore)
	apiGroup.GET("/kb/inputs", kbhandler.ListInputs)
	apiGroup.GET("/kb/inputs/:id", kbhandler.GetInput)
	apiGroup.PUT("/kb/inputs/:id", kbhandler.UpdateInput)
	apiGroup.POST("/kb/inputs/upload", kbhandler.UploadInputs)
	apiGroup.POST("/kb/inputs/check-md5", kbhandler.CheckMD5s)
	apiGroup.DELETE("/kb/inputs/:id", kbhandler.DeleteInput)
	apiGroup.GET("/kb/inputs/:id/file", kbhandler.GetInputFile)
	apiGroup.POST("/kb/inputs/:id/stop", kbhandler.StopPipeline)
	apiGroup.GET("/kb/metrics", kbhandler.ListMetrics)
	apiGroup.GET("/kb/metrics/search", kbhandler.SearchMetrics)
	apiGroup.GET("/kb/metrics/:metric_id/wiki", kbhandler.GetMetricWiki)
	apiGroup.GET("/kb/search", kbhandler.SearchAllArtifacts)
	apiGroup.POST("/kb/search/backfill-embeddings", kbhandler.BackfillSearchEmbeddings)
	apiGroup.GET("/kb/summaries/search", kbhandler.SearchSummaries)
	apiGroup.GET("/kb/topics/search", kbhandler.SearchTopics)
	apiGroup.GET("/kb/scene-blocks/search", kbhandler.SearchSceneBlocks)
	apiGroup.GET("/kb/provisions/search", kbhandler.SearchProvisions)
	apiGroup.GET("/kb/products/search", kbhandler.SearchProducts)
	apiGroup.GET("/kb/inventory-items/search", kbhandler.SearchInventoryItems)
	apiGroup.PUT("/kb/metrics/:id", kbhandler.UpdateMetric)
	apiGroup.POST("/kb/metrics/extract", kbhandler.ExtractMetric)
	apiGroup.POST("/kb/metrics/save", kbhandler.SaveExtractedMetrics)
	apiGroup.GET("/kb/chunks", kbhandler.ListChunks)
	apiGroup.GET("/kb/topic-chunks", kbhandler.ListTopicChunks)
	apiGroup.GET("/kb/raw-lines", kbhandler.GetRawLines)
	apiGroup.GET("/kb/scene-blocks", kbhandler.ListSceneBlocks)
	apiGroup.GET("/kb/products", kbhandler.ListProducts)
	apiGroup.GET("/kb/inventory-items", kbhandler.ListInventoryItems)
	apiGroup.GET("/kb/semantic-projections", kbhandler.ListSemanticProjections)
	apiGroup.GET("/kb/inventory-categories", kbhandler.ListInventoryCategories)
	apiGroup.PATCH("/kb/inventory-categories/:key", kbhandler.UpdateInventoryCategory)
	apiGroup.GET("/kb/summary-graph", kbhandler.ListSummaryGraph)
	apiGroup.GET("/kb/summary-category", kbhandler.GetSummaryCategory)
	apiGroup.GET("/kb/record-summaries", kbhandler.GetRecordSummaries)
	apiGroup.GET("/kb/topic-graph", kbhandler.ListTopicGraph)
	apiGroup.GET("/kb/topic-category", kbhandler.GetTopicCategory)
	apiGroup.GET("/kb/record-topics", kbhandler.GetRecordTopics)
	apiGroup.PUT("/kb/record-topic", kbhandler.UpdateRecordTopic)
	apiGroup.GET("/kb/config", kbhandler.GetKbFrontendConfig)
	apiGroup.GET("/kb/provision-graph", kbhandler.ListProvisionGraph)
	apiGroup.GET("/kb/provision-category", kbhandler.GetProvisionCategory)
	apiGroup.GET("/kb/record-provisions", kbhandler.GetRecordProvisions)
	apiGroup.GET("/kb/metric-category", kbhandler.GetMetricCategory)
	apiGroup.GET("/kb/scene-category", kbhandler.GetSceneCategory)
	apiGroup.GET("/kb/product-category", kbhandler.GetProductCategory)
	apiGroup.GET("/kb/artifact-category-counts", kbhandler.GetArtifactCategoryCounts)
	apiGroup.GET("/kb/wiki-overview", kbhandler.WikiOverview)
	apiGroup.GET("/kb/provisions", kbhandler.ListProvisions)
	apiGroup.POST("/kb/provisions", kbhandler.CreateProvision)
	apiGroup.POST("/kb/provisions/extract", kbhandler.ExtractProvisions)
	apiGroup.POST("/kb/provisions/save", kbhandler.SaveExtractedProvisions)
	apiGroup.POST("/kb/graph-node-filter", kbhandler.FilterGraphNodes)
	apiGroup.GET("/kb/doc-structure", kbhandler.GetDocStructure)
	apiGroup.PATCH("/kb/doc-structure", kbhandler.UpdateDocStructureLine)
	apiGroup.DELETE("/kb/doc-structure", kbhandler.DeleteDocStructureLine)
	apiGroup.POST("/kb/doc-structure/split", kbhandler.SplitDocStructureLine)
	apiGroup.GET("/kb/skill-categories", kbhandler.ListSkillCategories)
	apiGroup.GET("/kb/skills", kbhandler.ListSkills)
	apiGroup.POST("/kb/skills", kbhandler.CreateSkill)
	apiGroup.PUT("/kb/skills/:id", kbhandler.UpdateSkill)
	apiGroup.DELETE("/kb/skills/:id", kbhandler.DeleteSkillRecord)
	apiGroup.GET("/kb/doc-proc-logs", kbhandler.ListDocProcLogs)
	apiGroup.DELETE("/kb/doc-proc-logs/old", kbhandler.DeleteOldDocProcLogs)
	apiGroup.GET("/jetstream/monitor", jetstreamhandler.GetMonitor)
	apiGroup.GET("/jetstream/subjects", jetstreamhandler.GetSubjects)
	apiGroup.GET("/jetstream/nats-subjects", jetstreamhandler.ListStoredSubjects)
	apiGroup.POST("/jetstream/nats-subjects", jetstreamhandler.CreateStoredSubject)
	apiGroup.PUT("/jetstream/nats-subjects/:id", jetstreamhandler.UpdateStoredSubject)
	apiGroup.DELETE("/jetstream/nats-subjects/:id", jetstreamhandler.DeleteStoredSubject)
	apiGroup.POST("/jetstream/events", jetstreamhandler.PublishEvent)

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

	// Prompt Optimizer endpoints — model registry (with encrypted API keys),
	// templates, run history, favorites, variables, and the proxied optimize/
	// iterate/test/analyze operations. All endpoints scope by user_id.
	apiGroup.GET("/prompt-optimizer/models", promptoptimizerhandler.ListModels)
	apiGroup.POST("/prompt-optimizer/models", promptoptimizerhandler.CreateModel)
	apiGroup.PUT("/prompt-optimizer/models/:id", promptoptimizerhandler.UpdateModel)
	apiGroup.DELETE("/prompt-optimizer/models/:id", promptoptimizerhandler.DeleteModel)

	apiGroup.GET("/prompt-optimizer/templates", promptoptimizerhandler.ListTemplates)
	apiGroup.GET("/prompt-optimizer/templates/:id", promptoptimizerhandler.GetTemplate)
	apiGroup.POST("/prompt-optimizer/templates", promptoptimizerhandler.CreateTemplate)
	apiGroup.PUT("/prompt-optimizer/templates/:id", promptoptimizerhandler.UpdateTemplate)
	apiGroup.DELETE("/prompt-optimizer/templates/:id", promptoptimizerhandler.DeleteTemplate)

	apiGroup.GET("/prompt-optimizer/history", promptoptimizerhandler.ListHistory)
	apiGroup.DELETE("/prompt-optimizer/history/:id", promptoptimizerhandler.DeleteHistory)

	apiGroup.GET("/prompt-optimizer/favorites", promptoptimizerhandler.ListFavorites)
	apiGroup.POST("/prompt-optimizer/favorites", promptoptimizerhandler.CreateFavorite)
	apiGroup.DELETE("/prompt-optimizer/favorites/:id", promptoptimizerhandler.DeleteFavorite)

	apiGroup.GET("/prompt-optimizer/variables", promptoptimizerhandler.ListVariables)
	apiGroup.POST("/prompt-optimizer/variables", promptoptimizerhandler.UpsertVariable)
	apiGroup.DELETE("/prompt-optimizer/variables/:id", promptoptimizerhandler.DeleteVariable)

	apiGroup.POST("/prompt-optimizer/optimize", promptoptimizerhandler.OptimizePrompt)

	// Agent Platform (home3) endpoints. See design doc:
	// KnowledgeStore/DevDocuments/DesignDocs/design-agent-platform-home3.md
	// Tables owned by this feature are defined in
	// project_migrations/20260422000001_create_agent_platform_tables.sql.
	//
	// Workspaces (caller-scoped; no :slug).
	apiGroup.GET("/ap/workspaces", agentplatformhandler.ListWorkspaces)
	apiGroup.POST("/ap/workspaces", agentplatformhandler.CreateWorkspace)

	// Agents (workspace-scoped).
	apiGroup.GET("/ap/w/:slug/agents", agentplatformhandler.ListAgents)
	apiGroup.POST("/ap/w/:slug/agents", agentplatformhandler.CreateAgent)
	apiGroup.PATCH("/ap/w/:slug/agents/:id", agentplatformhandler.UpdateAgent)
	apiGroup.DELETE("/ap/w/:slug/agents/:id", agentplatformhandler.DeleteAgent)

	// Projects (workspace-scoped).
	apiGroup.GET("/ap/w/:slug/projects", agentplatformhandler.ListProjects)
	apiGroup.POST("/ap/w/:slug/projects", agentplatformhandler.CreateProject)
	apiGroup.PATCH("/ap/w/:slug/projects/:id", agentplatformhandler.UpdateProject)
	apiGroup.DELETE("/ap/w/:slug/projects/:id", agentplatformhandler.DeleteProject)

	// Issues (workspace-scoped). bulk-move must register BEFORE /:num so Echo
	// does not match "bulk-move" as an issue_number param.
	apiGroup.GET("/ap/w/:slug/issues", agentplatformhandler.ListIssues)
	apiGroup.POST("/ap/w/:slug/issues", agentplatformhandler.CreateIssue)
	apiGroup.POST("/ap/w/:slug/issues/bulk-move", agentplatformhandler.BulkMoveIssues)
	apiGroup.GET("/ap/w/:slug/issues/:num", agentplatformhandler.GetIssue)
	apiGroup.PATCH("/ap/w/:slug/issues/:num", agentplatformhandler.UpdateIssue)

	// Comments on an issue.
	apiGroup.GET("/ap/w/:slug/issues/:num/comments", agentplatformhandler.ListComments)
	apiGroup.POST("/ap/w/:slug/issues/:num/comments", agentplatformhandler.CreateComment)

	// Task runs (M1b). `POST /issues/:num/run` enqueues; the worker pool in
	// the same package drives execution. `GET /runs/:id/events?since=<id>`
	// is poll-friendly until the M2 WebSocket lands.
	apiGroup.POST("/ap/w/:slug/issues/:num/run", agentplatformhandler.RunIssue)
	apiGroup.GET("/ap/w/:slug/issues/:num/runs", agentplatformhandler.ListIssueRuns)
	apiGroup.GET("/ap/w/:slug/runs/:id", agentplatformhandler.GetTaskRun)
	apiGroup.GET("/ap/w/:slug/runs/:id/events", agentplatformhandler.ListRunEvents)
	apiGroup.POST("/ap/w/:slug/runs/:id/cancel", agentplatformhandler.CancelTaskRun)

	// Knowledge Engineering endpoints — reads/writes RTB files under ARTIFACT_DIR/Topics/.
	apiGroup.GET("/ke/research-topics", kehandler.List)
	apiGroup.POST("/ke/research-topics", kehandler.Create)

	// Diary (My Workspace) endpoints — reads/writes JSON files under DIARY_HOME_DIR.
	apiGroup.GET("/diary", diaryhandler.List)
	apiGroup.POST("/diary", diaryhandler.Create)
	apiGroup.GET("/diary/:id", diaryhandler.Get)
	apiGroup.PUT("/diary/:id", diaryhandler.Update)
	apiGroup.DELETE("/diary/:id", diaryhandler.Delete)

	// WebSocket endpoint for the agent platform (M2). Upgrades via Gorilla
	// WebSocket inside the handler; auth is enforced by the /ws group
	// middleware, membership re-checked in the handler itself.
	wsGroup := e.Group("/ws")
	wsGroup.Use(authmiddleware.AuthMiddleware)
	wsGroup.GET("/agent-platform", agentplatformhandler.HandleAgentPlatformWS)

	e.Any("/callback", openmetadatahandler.ProxyCallback)
	e.Any("/auth/callback", openmetadatahandler.RedirectAuthCallback)
	e.GET("/favicon.ico", serveRootFavicon)
	e.GET("/favicon.png", serveRootFavicon)
	e.GET("/integrations/openmetadata/favicon.ico", serveRootFavicon)
	e.GET("/integrations/openmetadata/favicon.png", serveRootFavicon)
	e.GET("/integrations/openmetadata/favicons/*", serveRootFavicon)

	openMetadataGroup := e.Group("/integrations/openmetadata")
	openMetadataGroup.Use(authmiddleware.AuthMiddleware)
	openMetadataGroup.Any("", openmetadatahandler.Proxy)
	openMetadataGroup.Any("/", openmetadatahandler.Proxy)
	openMetadataGroup.Any("/*", openmetadatahandler.Proxy)

	// Redirects root (/) to /login (since / is public but should show login by default).
	e.GET("/", func(c echo.Context) error {
		return c.Redirect(http.StatusFound, "/login")
	})
	return nil
}

func isLocalFaviconPath(path string) bool {
	return path == "/favicon.ico" ||
		path == "/favicon.png" ||
		path == "/integrations/openmetadata/favicon.ico" ||
		path == "/integrations/openmetadata/favicon.png" ||
		strings.HasPrefix(path, "/integrations/openmetadata/favicons/")
}

func serveRootFavicon(c echo.Context) error {
	c.Response().Header().Set("Cache-Control", "public, max-age=86400")
	return c.Blob(http.StatusOK, "image/svg+xml", []byte(rootFaviconSVG))
}

func logoutFromKratos(req *http.Request, logger ApiTypes.JimoLogger) {
	cookieHeader := req.Header.Get("Cookie")
	if cookieHeader == "" {
		return
	}

	kratosPublicURL := os.Getenv("KRATOS_PUBLIC_URL")
	if kratosPublicURL == "" {
		kratosPublicURL = "http://127.0.0.1:4433"
	}

	client := &http.Client{
		Timeout: 8 * time.Second,
		// We don't need to follow redirects for logout finalization.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	flowReq, err := http.NewRequest(http.MethodGet, strings.TrimRight(kratosPublicURL, "/")+"/self-service/logout/browser", nil)
	if err != nil {
		logger.Warn("failed to build kratos logout flow request", "error", err)
		return
	}
	flowReq.Header.Set("Accept", "application/json")
	flowReq.Header.Set("Cookie", cookieHeader)

	flowResp, err := client.Do(flowReq)
	if err != nil {
		logger.Warn("failed to create kratos logout flow", "error", err)
		return
	}
	defer flowResp.Body.Close()

	if flowResp.StatusCode < 200 || flowResp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(flowResp.Body, 1024))
		logger.Warn("kratos logout flow request not successful", "status", flowResp.StatusCode, "body", string(body))
		return
	}

	var payload struct {
		LogoutURL string `json:"logout_url"`
	}
	if err := json.NewDecoder(flowResp.Body).Decode(&payload); err != nil {
		logger.Warn("failed to parse kratos logout flow response", "error", err)
		return
	}
	if payload.LogoutURL == "" {
		logger.Warn("kratos logout flow missing logout_url")
		return
	}

	logoutReq, err := http.NewRequest(http.MethodGet, payload.LogoutURL, nil)
	if err != nil {
		logger.Warn("failed to build kratos logout request", "error", err)
		return
	}
	logoutReq.Header.Set("Cookie", cookieHeader)

	logoutResp, err := client.Do(logoutReq)
	if err != nil {
		logger.Warn("failed to execute kratos logout request", "error", err)
		return
	}
	defer logoutResp.Body.Close()
}
