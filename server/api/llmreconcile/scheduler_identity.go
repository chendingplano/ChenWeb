package llmreconcile

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

type schedulerIdentity struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

// ensureSchedulerIdentity resolves the audit identity and provisions it when absent.
func ensureSchedulerIdentity(ctx context.Context) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("KRATOS_ADMIN_URL")), "/")
	if base == "" {
		return "", errors.New("KRATOS_ADMIN_URL is not set")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	lookup := func() (*schedulerIdentity, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/admin/identities?credentials_identifier=scheduler", nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("scheduler lookup returned HTTP %d", resp.StatusCode)
		}
		var identities []schedulerIdentity
		if err := json.NewDecoder(resp.Body).Decode(&identities); err != nil {
			return nil, fmt.Errorf("decode scheduler lookup: %w", err)
		}
		if len(identities) == 0 {
			return nil, nil
		}
		if len(identities) != 1 {
			return nil, errors.New("multiple scheduler identities found")
		}
		return &identities[0], nil
	}
	validate := func(identity *schedulerIdentity) (string, error) {
		if identity == nil || strings.TrimSpace(identity.ID) == "" || identity.State != "active" {
			return "", errors.New("balance capture requires an active scheduler identity")
		}
		return identity.ID, nil
	}
	identity, err := lookup()
	if err != nil {
		return "", err
	}
	if identity != nil {
		return validate(identity)
	}
	password := make([]byte, 48)
	if _, err := rand.Read(password); err != nil {
		return "", fmt.Errorf("generate scheduler credential: %w", err)
	}
	payload, err := json.Marshal(map[string]any{
		"schema_id": "default", "state": "active",
		"traits":      map[string]any{"username": "scheduler", "name": map[string]string{"first": "scheduler", "last": "-"}},
		"credentials": map[string]any{"password": map[string]any{"config": map[string]string{"password": base64.RawURLEncoding.EncodeToString(password)}}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/admin/identities", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("create scheduler identity: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		identity, err = lookup()
		if err != nil {
			return "", err
		}
		return validate(identity)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create scheduler identity returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		return "", fmt.Errorf("decode created scheduler identity: %w", err)
	}
	id, err := validate(identity)
	if err != nil {
		return "", err
	}
	logger := loggerutil.CreateDefaultLogger("20261106-731")
	defer logger.Close()
	logger.Info("created scheduler identity for LLM balance capture", "identity_id", id)
	return id, nil
}
