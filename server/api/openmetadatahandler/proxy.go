package openmetadatahandler

import (
	"bytes"
	"compress/gzip"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

const internalThemeQueryParam = "om_theme"
const internalThemeHeader = "X-ChenWeb-OpenMetadata-Theme"

// ssoLog emits a structured trace line for the OpenMetadata SSO flow. All
// lines share the "OMD-SSO" component so they can be grepped together.
func ssoLog(msg string, args ...any) {
	slog.Info("[OMD-SSO] "+msg, args...)
}

// isAuthRelevantPath reports whether an upstream path is part of the
// OpenMetadata authentication / session decision, so we can trace it loudly.
func isAuthRelevantPath(p string) bool {
	authMarkers := []string{
		"/api/v1/users/loggedInUser",
		"/api/v1/system/config/auth",
		"/api/v1/system/config/jwks",
		"/api/v1/auth/",
		"/auth/",
		"/callback",
		"/app-worker.js",
	}
	for _, m := range authMarkers {
		if strings.Contains(p, m) {
			return true
		}
	}
	return false
}

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
		incomingPath := req.URL.Path
		originalDirector(req)
		rewriteRequest(req, cfg)
		req.Header.Del("Accept-Encoding")
		req.Host = upstreamURL.Host

		authHeader := req.Header.Get("Authorization")
		injected := ""
		if authHeader != "" {
			injected = "Bearer(len=" + strconv.Itoa(len(authHeader)) + ")"
		}
		if isAuthRelevantPath(req.URL.Path) {
			ssoLog("proxy -> upstream (auth-relevant)",
				"sso_mode", cfg.SSOMode,
				"incoming_path", incomingPath,
				"upstream_path", req.URL.Path,
				"authorization", injected,
			)
		} else {
			slog.Debug("[OMD-SSO] proxy -> upstream",
				"incoming_path", incomingPath,
				"upstream_path", req.URL.Path,
				"authorization", injected,
			)
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.Request != nil && resp.Request.URL != nil && isAuthRelevantPath(resp.Request.URL.Path) {
			ssoLog("upstream -> proxy (auth-relevant)",
				"upstream_path", resp.Request.URL.Path,
				"status", resp.StatusCode,
				"content_type", resp.Header.Get("Content-Type"),
			)
		}
		return rewriteHTMLResponse(resp, cfg)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		ssoLog("upstream error", "path", r.URL.Path, "error", err.Error())
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
	ssoLog("incoming proxy request",
		"sso_mode", cfg.SSOMode,
		"method", c.Request().Method,
		"path", c.Request().URL.Path,
		"has_admin_token", cfg.AdminToken != "",
	)

	if shouldPromoteAuthLogin(c.Request(), cfg) {
		ssoLog("promoting embedded auth/login to top-level", "path", c.Request().URL.Path)
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
	switch cfg.SSOMode {
	case "session-bootstrap":
		if cfg.BearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.BearerToken)
		}
	case "token-bridge":
		if cfg.AdminToken != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.AdminToken)
		}
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
	rewritten := rewriteOpenMetadataHTML(string(body), cfg.PublicBasePath, theme, cfg.AdminToken)

	upstreamPath := ""
	if resp.Request != nil && resp.Request.URL != nil {
		upstreamPath = resp.Request.URL.Path
	}
	ssoLog("rewrote HTML response",
		"upstream_path", upstreamPath,
		"sso_mode", cfg.SSOMode,
		"original_bytes", len(body),
		"rewritten_bytes", len(rewritten),
		"bootstrap_injected", cfg.SSOMode == "token-bridge" && cfg.AdminToken != "",
		"admin_token_len", len(cfg.AdminToken),
	)

	resp.Body = io.NopCloser(bytes.NewReader([]byte(rewritten)))
	resp.ContentLength = int64(len(rewritten))
	resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	if compressed {
		resp.Header.Del("Content-Encoding")
	}
	return nil
}

func rewriteOpenMetadataHTML(html, publicBasePath, theme, adminToken string) string {
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
	rewritten = injectSessionBootstrap(rewritten, adminToken)

	return rewritten
}

// sessionBootstrapJS writes the admin token into every storage layer that
// OpenMetadata's auth provider consults, and traces each step to the browser
// console under the [ChenWeb-OM-SSO] prefix. OM keeps the OIDC token at
// app_state.primary, read via a Service Worker that caches it in memory; we
// therefore (1) write IndexedDB, (2) write localStorage as fallback, and
// (3) message the live Service Worker so its in-memory cache cannot shadow us.
const sessionBootstrapJS = `(function(){
var LOG='[ChenWeb-OM-SSO]';
function log(){try{console.log.apply(console,[LOG].concat([].slice.call(arguments)));}catch(e){}}
function err(){try{console.error.apply(console,[LOG].concat([].slice.call(arguments)));}catch(e){}}
var t=__OMD_ADMIN_TOKEN__;
var s=JSON.stringify({primary:t});
var DB='AppDataStore',ST='keyValueStore',K='app_state';
log('bootstrap start; token length =',t?t.length:0,'; path =',location.pathname);
try{localStorage.setItem(K,s);log('localStorage app_state set OK');}catch(e){err('localStorage set failed',e);}
function writeIDB(cb){
try{var r=indexedDB.open(DB,1);
r.onupgradeneeded=function(e){var d=e.target.result;if(!d.objectStoreNames.contains(ST)){d.createObjectStore(ST);}};
r.onsuccess=function(){try{var db=r.result;var tx=db.transaction([ST],'readwrite');tx.objectStore(ST).put(s,K);
tx.oncomplete=function(){log('IndexedDB app_state write OK');if(cb)cb();};
tx.onerror=function(){err('IndexedDB tx error',tx.error);};}catch(e){err('IndexedDB put failed',e);}};
r.onerror=function(){err('IndexedDB open failed',r.error);};}catch(e){err('IndexedDB unavailable',e);}}
function verifyIDB(){
try{var r=indexedDB.open(DB,1);
r.onsuccess=function(){try{var db=r.result;var tx=db.transaction([ST],'readonly');var g=tx.objectStore(ST).get(K);
g.onsuccess=function(){var v=g.result;log('IndexedDB readback app_state =',v?(v.length>80?v.slice(0,80)+'...':v):'(empty)');};}catch(e){err('IndexedDB readback failed',e);}};}catch(e){}}
function pushToSW(){
try{
if(!('serviceWorker' in navigator)){log('no serviceWorker API');return;}
navigator.serviceWorker.ready.then(function(reg){
var target=navigator.serviceWorker.controller||reg.active;
if(!target){log('SW present but not controlling yet; IndexedDB will be read on SW init');return;}
var ch=new MessageChannel();
ch.port1.onmessage=function(ev){log('SW set ack =',JSON.stringify(ev.data));};
target.postMessage({type:'set',key:K,value:s,requestId:Date.now()},[ch.port2]);
log('posted app_state to service worker controller');
}).catch(function(e){err('serviceWorker.ready failed',e);});
}catch(e){err('pushToSW failed',e);}}
if(typeof indexedDB!=='undefined'){writeIDB(function(){verifyIDB();});}else{log('indexedDB undefined; localStorage fallback only');}
pushToSW();
try{if(navigator.serviceWorker){navigator.serviceWorker.addEventListener('controllerchange',function(){log('controllerchange -> re-push to SW');pushToSW();});}}catch(e){}
})();`

func injectSessionBootstrap(html, adminToken string) string {
	if adminToken == "" {
		return html
	}

	body := strings.Replace(sessionBootstrapJS, "__OMD_ADMIN_TOKEN__", strconv.Quote(adminToken), 1)
	script := `<script id="chenweb-om-sso">` + body + `</script>` + hideLogoutStyle

	if strings.Contains(html, "</head>") {
		return strings.Replace(html, "</head>", script+"</head>", 1)
	}
	return script + html
}

// hideLogoutStyle hides OpenMetadata's left-sidebar Logout control. In
// token-bridge SSO the ChenWeb session owns the identity, so an in-frame
// logout would only drop the embedded app into a broken state. This is
// scoped to token-bridge mode (it travels with the bootstrap injection),
// so a standalone OpenMetadata still shows Logout normally.
const hideLogoutStyle = `<style id="chenweb-om-hide-logout">` +
	`[data-testid="app-bar-item-logout"]{display:none !important;}` +
	`li:has([data-testid="app-bar-item-logout"]){display:none !important;}` +
	`</style>`

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
