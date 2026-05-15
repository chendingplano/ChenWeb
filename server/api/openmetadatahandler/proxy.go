package openmetadatahandler

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"
)

func NewProxy() (http.Handler, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	upstreamURL, err := url.Parse(cfg.UpstreamURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		rewriteRequest(req, cfg)
		req.Host = upstreamURL.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "openmetadata upstream unavailable", http.StatusBadGateway)
	}

	return proxy, nil
}

func Proxy(c echo.Context) error {
	proxy, err := NewProxy()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Status:  false,
			Message: err.Error(),
		})
	}

	proxy.ServeHTTP(c.Response(), c.Request())
	return nil
}

func ProxyCallback(c echo.Context) error {
	proxy, err := NewProxy()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Status:  false,
			Message: err.Error(),
		})
	}

	cfg, err := loadConfig()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Status:  false,
			Message: err.Error(),
		})
	}

	rewriteCallbackRequest(c.Request(), cfg)
	proxy.ServeHTTP(c.Response(), c.Request())
	return nil
}

func rewritePath(requestPath, publicBasePath string) string {
	trimmedBase := strings.TrimSuffix(publicBasePath, "/")
	rewritten := strings.TrimPrefix(requestPath, trimmedBase)
	if rewritten == "" || rewritten == trimmedBase {
		return "/"
	}
	if !strings.HasPrefix(rewritten, "/") {
		rewritten = "/" + rewritten
	}
	return rewritten
}

func rewriteRequest(req *http.Request, cfg config) {
	originalHost := req.Host
	req.URL.Path = rewritePath(req.URL.Path, cfg.PublicBasePath)
	applyForwardedHeaders(req, cfg, originalHost)
	if cfg.SSOMode == "session-bootstrap" && cfg.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.BearerToken)
	}
}

func rewriteCallbackRequest(req *http.Request, cfg config) {
	originalHost := req.Host
	req.URL.Path = "/callback"
	applyForwardedHeaders(req, cfg, originalHost)
}

func applyForwardedHeaders(req *http.Request, cfg config, originalHost string) {
	req.Header.Set("X-Forwarded-Host", originalHost)
	req.Header.Set("X-Forwarded-Proto", requestScheme(req))
	req.Header.Set("X-Forwarded-Prefix", strings.TrimSuffix(cfg.PublicBasePath, "/"))
}
