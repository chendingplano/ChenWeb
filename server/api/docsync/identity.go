package docsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func resolveOwner(ctx context.Context, adminURL, phone string) (string, error) {
	endpoint, err := url.Parse(strings.TrimRight(adminURL, "/") + "/admin/identities")
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return "", errors.New("KRATOS_ADMIN_URL is required to resolve document owner")
	}
	suffix := strings.TrimPrefix(strings.TrimSpace(phone), "+")
	if !phoneDigits(suffix) {
		return "", errors.New("document sync remote phone must be a phone number")
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	pages := make(map[string]bool)
	var matches []string
	for token := ""; ; {
		if pages[token] {
			return "", errors.New("identity service returned a repeated page")
		}
		pages[token] = true
		args := url.Values{"page_size": {"100"}}
		if token != "" {
			args.Set("page_token", token)
		}
		endpoint.RawQuery = args.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", errors.New("could not reach local identity service")
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return "", fmt.Errorf("local identity lookup returned HTTP %d", resp.StatusCode)
		}
		var identities []struct {
			ID     string `json:"id"`
			Traits struct {
				Phone string `json:"phone"`
			} `json:"traits"`
		}
		err = json.NewDecoder(resp.Body).Decode(&identities)
		resp.Body.Close()
		if err != nil {
			return "", errors.New("identity service returned invalid identity data")
		}
		for _, identity := range identities {
			have, ok := strings.CutPrefix(identity.Traits.Phone, "+")
			if !ok || !phoneDigits(have) || identity.ID == "" {
				continue
			}
			if (strings.HasPrefix(phone, "+") && have == suffix) ||
				(!strings.HasPrefix(phone, "+") && len(have)-len(suffix) >= 1 && len(have)-len(suffix) <= 3 && strings.HasSuffix(have, suffix)) {
				matches = append(matches, identity.ID)
			}
		}
		token = nextIdentityPage(resp.Header.Values("Link"))
		if token == "" {
			break
		}
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("configured document owner matched %d local identities, expected one", len(matches))
	}
	return matches[0], nil
}

func phoneDigits(s string) bool {
	return s != "" && strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) == -1
}

func nextIdentityPage(links []string) string {
	for _, header := range links {
		for _, link := range strings.Split(header, ",") {
			target, attributes, ok := strings.Cut(strings.TrimSpace(link), ">")
			if !ok || !strings.HasPrefix(target, "<") || !strings.Contains(attributes, `rel="next"`) {
				continue
			}
			u, err := url.Parse(target[1:])
			if err == nil {
				return u.Query().Get("page_token")
			}
		}
	}
	return ""
}
