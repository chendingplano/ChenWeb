package openmetadatahandler

import (
	"bytes"
	"compress/gzip"
	"html"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

const internalThemeQueryParam = "om_theme"
const internalThemeHeader = "X-ChenWeb-OpenMetadata-Theme"

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
		req.Header.Del("Accept-Encoding")
		req.Host = upstreamURL.Host
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		return rewriteHTMLResponse(resp, cfg)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "openmetadata upstream unavailable", http.StatusBadGateway)
	}

	return proxy, nil
}

func Proxy(c echo.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Status:  false,
			Message: err.Error(),
		})
	}
	if shouldPromoteAuthLogin(c.Request(), cfg) {
		return c.HTML(http.StatusOK, authLoginPromotionHTML(c.Request()))
	}

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

func RedirectAuthCallback(c echo.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Status:  false,
			Message: err.Error(),
		})
	}
	target := url.URL{
		Path:     strings.TrimSuffix(cfg.PublicBasePath, "/") + "/auth/callback",
		RawQuery: c.Request().URL.RawQuery,
	}
	return c.Redirect(http.StatusFound, target.String())
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
	propagateInternalTheme(req)
	req.URL.Path = rewritePath(req.URL.Path, cfg.PublicBasePath)
	rewriteAuthLoginRedirectURI(req, cfg, originalHost)
	removeInternalQueryParam(req, "om_top_level")
	removeInternalQueryParam(req, internalThemeQueryParam)
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

func shouldPromoteAuthLogin(req *http.Request, cfg config) bool {
	if req.URL.Query().Get("om_top_level") == "1" {
		return false
	}
	return rewritePath(req.URL.Path, cfg.PublicBasePath) == "/api/v1/auth/login"
}

func authLoginPromotionHTML(req *http.Request) string {
	target := *req.URL
	query := target.Query()
	query.Set("om_top_level", "1")
	target.RawQuery = query.Encode()
	targetURL := target.String()

	return `<!doctype html><html><head><meta charset="utf-8"><title>OpenMetadata Sign In</title></head><body><script>window.top.location.replace(` +
		strconv.Quote(targetURL) +
		`);</script><a href="` +
		html.EscapeString(targetURL) +
		`" target="_top" rel="noopener">Continue sign in</a></body></html>`
}

func removeInternalQueryParam(req *http.Request, key string) {
	query := req.URL.Query()
	if _, ok := query[key]; !ok {
		return
	}
	query.Del(key)
	req.URL.RawQuery = query.Encode()
}

func propagateInternalTheme(req *http.Request) {
	theme := normalizeInternalTheme(req.URL.Query().Get(internalThemeQueryParam))
	if theme == "" {
		req.Header.Del(internalThemeHeader)
		return
	}
	req.Header.Set(internalThemeHeader, theme)
}

func normalizeInternalTheme(theme string) string {
	switch strings.TrimSpace(strings.ToLower(theme)) {
	case "dark":
		return "dark"
	case "light":
		return "light"
	default:
		return ""
	}
}

func rewriteAuthLoginRedirectURI(req *http.Request, cfg config, originalHost string) {
	if req.URL.Path != "/api/v1/auth/login" {
		return
	}
	query := req.URL.Query()
	if query.Get("redirectUri") == "" {
		return
	}
	scheme := requestScheme(req)
	callback := scheme + "://" + originalHost + strings.TrimSuffix(cfg.PublicBasePath, "/") + "/auth/callback"
	query.Set("redirectUri", callback)
	req.URL.RawQuery = query.Encode()
}

func rewriteHTMLResponse(resp *http.Response, cfg config) error {
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "text/html") {
		return nil
	}

	bodyReader := resp.Body
	compressed := strings.EqualFold(strings.TrimSpace(resp.Header.Get("Content-Encoding")), "gzip")
	if compressed {
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return err
		}
		defer reader.Close()
		bodyReader = reader
	}

	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	theme := normalizeInternalTheme(resp.Request.Header.Get(internalThemeHeader))
	rewritten := rewriteOpenMetadataHTML(string(body), cfg.PublicBasePath, theme)
	resp.Body = io.NopCloser(bytes.NewReader([]byte(rewritten)))
	resp.ContentLength = int64(len(rewritten))
	resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	if compressed {
		resp.Header.Del("Content-Encoding")
	}
	return nil
}

func rewriteOpenMetadataHTML(html, publicBasePath string, theme string) string {
	trimmedBase := strings.TrimSuffix(publicBasePath, "/")

	replacer := strings.NewReplacer(
		"window.BASE_PATH = '/';", "window.BASE_PATH = '"+publicBasePath+"';",
		`"/assets/`, `"`+trimmedBase+`/assets/`,
		`"/images/`, `"`+trimmedBase+`/images/`,
		`"/favicons/`, `"`+trimmedBase+`/favicons/`,
		`"/manifest.json`, `"`+trimmedBase+`/manifest.json`,
		`"/favicon.png`, `"`+trimmedBase+`/favicon.png`,
		`"/favicon.ico`, `"`+trimmedBase+`/favicon.ico`,
		`'/assets/`, `'`+trimmedBase+`/assets/`,
		`'/images/`, `'`+trimmedBase+`/images/`,
		`'/favicons/`, `'`+trimmedBase+`/favicons/`,
		`'/manifest.json`, `'`+trimmedBase+`/manifest.json`,
		`'/favicon.png`, `'`+trimmedBase+`/favicon.png`,
		`'/favicon.ico`, `'`+trimmedBase+`/favicon.ico`,
	)

	rewritten := replacer.Replace(html)
	rewritten = strings.ReplaceAll(rewritten, `'`+trimmedBase+`/images/governance.png',`, "")
	rewritten = strings.ReplaceAll(rewritten, `'`+trimmedBase+`/images/data-collaboration.png',`, "")
	rewritten = strings.ReplaceAll(rewritten, `"`+trimmedBase+`/images/governance.png",`, "")
	rewritten = strings.ReplaceAll(rewritten, `"`+trimmedBase+`/images/data-collaboration.png",`, "")
	rewritten = injectThemeBootstrap(rewritten, theme)

	return rewritten
}

func injectThemeBootstrap(html string, theme string) string {
	theme = normalizeInternalTheme(theme)
	if theme == "" {
		return html
	}

	script := `<script id="chenweb-openmetadata-theme">(function(){try{var theme=` +
		strconv.Quote(theme) +
		`;var root=document.documentElement;if(theme==="dark"){window.localStorage.setItem("ui-theme","dark");root.classList.add("dark-mode");root.style.colorScheme="dark";}else{window.localStorage.removeItem("ui-theme");root.classList.remove("dark-mode");root.style.colorScheme="light";}}catch(e){}})();</script><style id="chenweb-openmetadata-theme-style">html.dark-mode,html.dark-mode body{background:#0c0e12;color:#f0f0f1;color-scheme:dark;}</style>`

	if strings.Contains(html, "</head>") {
		return strings.Replace(html, "</head>", script+"</head>", 1)
	}
	return script + html
}
