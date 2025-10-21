package Auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

var githubOauthConfig = &oauth2.Config{
	ClientID:     os.Getenv("GITHUB_OAUTH_CLIENT_ID"),
	ClientSecret: os.Getenv("GITHUB_OAUTH_CLIENT_SECRET"),
	RedirectURL:  "http://localhost:8080/auth/github/callback",
	Scopes:       []string{"user:email"},
	Endpoint:     github.Endpoint,
}

var githubOauthStateString = "random-github-state"

func HandleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	url := githubOauthConfig.AuthCodeURL(githubOauthStateString)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func HandleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("state") != githubOauthStateString {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	code := r.FormValue("code")
	token, err := githubOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		http.Error(w, "code exchange failed", http.StatusInternalServerError)
		return
	}

	client := githubOauthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		http.Error(w, "failed to get user info", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var user struct {
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		http.Error(w, "failed to decode user info", http.StatusInternalServerError)
		return
	}

	redirectURL := fmt.Sprintf("http://localhost:8080/sidebar-01?name=%s", user.Login)
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}
