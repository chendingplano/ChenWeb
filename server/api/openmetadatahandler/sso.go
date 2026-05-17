package openmetadatahandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var omHTTPClient = &http.Client{Timeout: 10 * time.Second}

type omUserListResponse struct {
	Data []struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	} `json:"data"`
}

type omCreateUserRequest struct {
	Name        string `json:"name"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	IsAdmin     bool   `json:"isAdmin"`
}

// EnsureOpenMetadataUser provisions the ChenWeb user in OpenMetadata if they do
// not already exist. It is a no-op when the user is found by email.
func EnsureOpenMetadataUser(ctx context.Context, adminToken, upstreamURL string, user *CurrentUser) error {
	exists, err := omUserExists(ctx, adminToken, upstreamURL, user.Email)
	if err != nil {
		return fmt.Errorf("check openmetadata user %s: %w", user.Email, err)
	}
	if exists {
		slog.InfoContext(ctx, "openmetadata user already provisioned", "email", user.Email)
		return nil
	}
	slog.InfoContext(ctx, "provisioning openmetadata user", "email", user.Email)
	if err := omCreateUser(ctx, adminToken, upstreamURL, user); err != nil {
		return fmt.Errorf("create openmetadata user %s: %w", user.Email, err)
	}
	slog.InfoContext(ctx, "openmetadata user provisioned", "email", user.Email)
	return nil
}

func omUserExists(ctx context.Context, adminToken, upstreamURL, email string) (bool, error) {
	reqURL := upstreamURL + "/api/v1/users?fields=id,email&email=" + url.QueryEscape(email)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := omHTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("openmetadata user lookup returned %d", resp.StatusCode)
	}

	var list omUserListResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return false, fmt.Errorf("decode user list response: %w", err)
	}
	return len(list.Data) > 0, nil
}

func omCreateUser(ctx context.Context, adminToken, upstreamURL string, user *CurrentUser) error {
	body := omCreateUserRequest{
		Name:        emailLocalPart(user.Email),
		Email:       user.Email,
		DisplayName: user.DisplayName,
		IsAdmin:     false,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL+"/api/v1/users", bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := omHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("openmetadata user creation returned %d", resp.StatusCode)
	}
	return nil
}

func emailLocalPart(email string) string {
	local, _, found := strings.Cut(email, "@")
	if !found {
		return email
	}
	return local
}
