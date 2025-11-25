package api

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/Dashboard01"
	EchartData "github.com/chendingplano/deepdoc/server/api/EchartDemo"
	"github.com/chendingplano/deepdoc/server/api/buttonhandler"
	authmiddleware "github.com/chendingplano/shared/go/auth-middleware"
	"github.com/labstack/echo/v4"
)

//go:embed all:webbuild
var webBuild embed.FS

func RegisterRoutes(e *echo.Echo) (error) {
	// This function registers all API routes and returns the Echo instance.
	// It also sets up the frontend handler, either as a reverse proxy to a
	// development server or serving embedded static files.
	// This function should be called first during server initialization.
	// Other route registrations (such as Auth) can be done afterwards.
	log.Printf("Register API routes (MID_RTS_033)")

	useEmbedFrontend := os.Getenv("USE_EMBED_FRONTEND") != ""
	log.Printf("useEmbedFrontend (MID_RTS_037):%s", os.Getenv("USE_EMBED_FRONTEND"))

	var frontendHandler http.Handler
	if !useEmbedFrontend {
		frontendURLEnv := os.Getenv("FRONTEND_URL")
		if frontendURLEnv == "" {
			frontendURLEnv = "http://localhost:5173"
		}

		frontendURL, err := url.Parse(frontendURLEnv)
		log.Printf("FrontendURL (MID_RTS_047):%s", frontendURL)
		if err != nil {
			error_msg := fmt.Errorf("failed to parse FRONTEND_URL env %s: %w (MID_RTS_042)", frontendURLEnv, err)
			log.Printf("***** Alarm:%s, original error:%+v", error_msg, err)
			return error_msg
		}

		proxy := httputil.NewSingleHostReverseProxy(frontendURL)
		frontendHandler = proxy
	} else {
		log.Printf("Production deployment (MID_RTS_057)")
		webBuildFS, err := fs.Sub(webBuild, "webbuild")
		if err != nil {
			error_msg := fmt.Errorf("failed to get webbuild subtree: %w (MID_RTS_050)", err)
			log.Printf("***** Alarm:%s, original error:%+v (MID_RTS_061)", error_msg, err)
			return error_msg
		}
		fileServer := http.FileServerFS(webBuildFS)
		frontendHandler = fileServer
	}

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path

			// Public endpoints.
			// IMPORTANT: need to load from a configuration file!
			publicPaths := map[string]bool{
            	"/":              true,
            	"/login":         true,
            	"/example-login": true,
            	// Add other public pages here
        	}

			// Let Echo handle / so we can redirect it
            if publicPaths[path] {
				log.Printf("Public URL:%s (CWB_RTS_080)", path)
            	return next(c)
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
                  strings.Contains(path, "/__") {
                   // Let these development assets pass through without auth check
                   frontendHandler.ServeHTTP(c.Response(), c.Request())
                   return nil
                }

				// Check if it's a public path
				log.Printf("Check path:%s (CWB_RTS_091)", path)

              	// Protected frontend route → check auth
				// IMPORTANT: need cache!
				// log.Printf("Auth check for path: %s (MID_RTS_095)", c.Request().URL.Path)
               	user_name, err := authmiddleware.IsAuthenticated(c, "MID_RTS_096")
				if err != nil {
                	// Redirect to login (for browser) or 401 (for API-like requests)
					log.Printf("Not logged in. err:%v, Redirect to /login, path:%s (CWB_RTS_109)", err, path)
                   	if authmiddleware.IsHTMLRequest(c) {
                    	return c.Redirect(http.StatusFound, "/login")
                   	}
                   	return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Login required"})
            	}
				log.Printf("Authentication passed, user:%s (CWB_RTS_115):%s", user_name, path)
				frontendHandler.ServeHTTP(c.Response(), c.Request())
				return nil
			}
			return next(c)
		}
	})

	e.DELETE("/auth/logout", func(c echo.Context) error {
    	// Clear the session cookie
		log.Printf("Handle logout (MID_RTS_123)")
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

	// Create the routing group '/api/v1'
	apiGroup := e.Group("/api/v1")
	apiGroup.Use(authmiddleware.AuthMiddleware)

	// Add the endpoint '/api/v1/health'
	apiGroup.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{"status": "ok"})
	})

	/*
	// Add the endpoint '/api/v1/randum' (testing only)
	apiGroup.GET("/randnum", func(c echo.Context) error {
		type randInput struct {
			Max string `query:"max"`
		}
		var inp randInput
		err := c.Bind(&inp)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "max must exist")
		}
		maxVal, err := strconv.Atoi(inp.Max)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "max must be a number")
		}
		number := rand.IntN(maxVal)
		return c.JSON(http.StatusOK, map[string]any{"number": number})
	})
	*/

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

	// Redirects root (/) to /login (since / is public but should show login by default).
	e.GET("/", func(c echo.Context) error {
    	return c.Redirect(http.StatusFound, "/sidebar-07")
	})
	return nil
}
