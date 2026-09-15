package agentservicehandler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrCapabilityInvalid     = errors.New("invalid run capability")
	ErrCapabilityExpired     = errors.New("run capability expired")
	ErrCapabilityRunMismatch = errors.New("run capability belongs to another run")
	ErrCapabilityToolDenied  = errors.New("tool is not allowed by run capability")
)

const maxCapabilityLifetime = 10 * time.Minute

type RunCapabilityClaims struct {
	UserID            string   `json:"user_id"`
	ProfileSlug       string   `json:"profile_slug"`
	ProfileVersion    string   `json:"profile_version"`
	RunID             string   `json:"run_id"`
	AllowedTools      []string `json:"allowed_tools"`
	KnowledgeStoreIDs []string `json:"knowledge_store_ids"`
	DocumentGroups    []string `json:"document_groups,omitempty"`
	DocumentIDs       []string `json:"document_ids,omitempty"`
	MaxEvidenceBytes  int      `json:"max_evidence_bytes"`
	ExpiresAtUnix     int64    `json:"expires_at"`
}

type CapabilitySigner struct {
	secret []byte
	now    func() time.Time
}

func NewCapabilitySigner(secret []byte, now func() time.Time) (*CapabilitySigner, error) {
	if len(secret) < 32 || now == nil {
		return nil, ErrCapabilityInvalid
	}
	return &CapabilitySigner{secret: append([]byte(nil), secret...), now: now}, nil
}

func (s *CapabilitySigner) Mint(claims RunCapabilityClaims, lifetime time.Duration) (string, error) {
	if !validClaims(claims) || lifetime <= 0 || lifetime > maxCapabilityLifetime {
		return "", ErrCapabilityInvalid
	}
	claims.ExpiresAtUnix = s.now().Add(lifetime).Unix()
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", ErrCapabilityInvalid
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(s.sign(encoded)), nil
}

func (s *CapabilitySigner) Verify(token, expectedRunID, tool string) (RunCapabilityClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return RunCapabilityClaims{}, ErrCapabilityInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, s.sign(parts[0])) {
		return RunCapabilityClaims{}, ErrCapabilityInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return RunCapabilityClaims{}, ErrCapabilityInvalid
	}
	var claims RunCapabilityClaims
	if json.Unmarshal(payload, &claims) != nil || !validClaims(claims) || claims.ExpiresAtUnix <= 0 {
		return RunCapabilityClaims{}, ErrCapabilityInvalid
	}
	if !s.now().Before(time.Unix(claims.ExpiresAtUnix, 0)) {
		return RunCapabilityClaims{}, ErrCapabilityExpired
	}
	if claims.RunID != strings.TrimSpace(expectedRunID) {
		return RunCapabilityClaims{}, ErrCapabilityRunMismatch
	}
	if !stringIn(claims.AllowedTools, tool) {
		return RunCapabilityClaims{}, ErrCapabilityToolDenied
	}
	return claims, nil
}

func (s *CapabilitySigner) sign(payload string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func validClaims(c RunCapabilityClaims) bool {
	return strings.TrimSpace(c.UserID) != "" && strings.TrimSpace(c.ProfileSlug) != "" &&
		strings.TrimSpace(c.ProfileVersion) != "" && strings.TrimSpace(c.RunID) != "" &&
		len(c.AllowedTools) > 0 && len(c.KnowledgeStoreIDs) > 0 &&
		c.MaxEvidenceBytes > 0 && c.MaxEvidenceBytes <= MaxToolResponseBytes
}

func stringIn(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
