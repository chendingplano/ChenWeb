// server/api/Auth/google.go
package Auth

// server/api/Auth/google.go
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/chendingplano/deepdoc/server/api/Databases"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var googleOauthConfig = &oauth2.Config{
	ClientID:     os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
	ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
	RedirectURL:  "http://localhost:8080/auth/google/callback",
	Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
	Endpoint:     google.Endpoint,
}

var oauthStateString = "random-string" // 开发阶段可用常量，生产环境请生成并验证

func HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	log.Printf("HandleGoogleLogin called (MID_GGL_010)")
	log.Printf("Google Config (MID_GGL_033):%s, secret:%s", googleOauthConfig.ClientID, googleOauthConfig.ClientSecret)
	log.Printf("Google Client ID (MID_GGL_034):%s", os.Getenv("GOOGLE_CLIENT_ID"))
	url := googleOauthConfig.AuthCodeURL(oauthStateString, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	// 验证 state
	log.Printf("HandleGoogleCallback called (MID_GGL_020)")
	if r.FormValue("state") != oauthStateString {
		http.Error(w, "invalid oauth state (MID_GGL_043)", http.StatusBadRequest)
		return
	}
	code := r.FormValue("code")
	if code == "" {
		http.Error(w, "code not found in request (MID_GGL_048)", http.StatusBadRequest)
		return
	}

	userInfo, err := getUserInfo(r.Context(), code)
	if err != nil {
		http.Error(w, "failed to get user info (MID_GGL_054): "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Generate a secure random session ID
	sessionID := generateSecureToken(32) // e.g., 256-bit random string

	// Save session in DB/cache: map sessionID → user_email (or user_id)
	err1 := Databases.SaveSession(
				"google_login",
				sessionID, 
				userInfo.Email,
				"email",
				userInfo.Email,
				time.Now().Add(24*time.Hour))
	if err1 != nil {
		http.Error(w, "failed to save session (MID_GGL_068): "+err1.Error(), http.StatusInternalServerError)
		return
	}

	// 示例：将用户信息写入 cookie（仅演示，生产请设置 Secure、SameSite、过期等）
	// 也可以在此处创建用户、发放 JWT 等
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,   // require HTTPS in production
		SameSite: http.SameSiteStrictMode,
	})

	log.Printf("User %s (%s) logged in successfully (MID_GGL_083)", userInfo.Name, userInfo.Email)

	// 重定向到前端页面（或你希望的任意 URL）
	redirectURL := fmt.Sprintf("http://localhost:8080/sidebar-01?name=%s", url.QueryEscape(userInfo.Name))
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}



func generateSecureToken(length int) string {
    bytes := make([]byte, length)
    if _, err := rand.Read(bytes); err != nil {
        panic(err)
    }
    return hex.EncodeToString(bytes)
}

// userInfo 结构体用于解码 Google 返回的用户信息
type userInfoResp struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale,omitempty"`
}

// getUserInfo: 使用 oauth2.Config.Exchange 取得 token，再用 config.Client 发请求并解析 JSON
func getUserInfo(ctx context.Context, code string) (*userInfoResp, error) {
	token, err := googleOauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("code exchange failed: %w", err)
	}

	// 使用 oauth2 client（会自动带上 access token）
	client := googleOauthConfig.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, fmt.Errorf("failed to get userinfo (MID_GGL_121): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint returned status %d (MID_GGL_126)", resp.StatusCode)
	}

	var ui userInfoResp
	if err := json.NewDecoder(resp.Body).Decode(&ui); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo: %w (MID_GGL_131)", err)
	}

	return &ui, nil
}
