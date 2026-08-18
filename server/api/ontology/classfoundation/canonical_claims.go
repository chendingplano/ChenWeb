package classfoundation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// CanonicalClaimInput carries only identity-bearing data. Evidence is accepted
// as caller context but deliberately excluded from serialization and registry
// identity, so evidence-only changes do not create a different claim.
type CanonicalClaimInput struct {
	KeyVersion  string
	ClassTermID string
	Fields      map[string]string
	Evidence    string
	By          string
}

type canonicalClaimKey struct {
	ClassTermID string            `json:"class_term_id"`
	Fields      map[string]string `json:"fields"`
	KeyVersion  string            `json:"key_version"`
}

// SerializeCanonicalClaimKey produces a deterministic UTF-8 JSON key. The
// standard encoder orders map keys, and the outer struct locks field order.
func SerializeCanonicalClaimKey(input CanonicalClaimInput) ([]byte, error) {
	if strings.TrimSpace(input.KeyVersion) == "" || strings.TrimSpace(input.ClassTermID) == "" {
		return nil, errors.New("canonical key version and class term ID are required")
	}
	fields := input.Fields
	if fields == nil {
		fields = map[string]string{}
	}
	key, err := json.Marshal(canonicalClaimKey{
		ClassTermID: strings.TrimSpace(input.ClassTermID),
		Fields:      fields,
		KeyVersion:  strings.TrimSpace(input.KeyVersion),
	})
	if err != nil {
		return nil, fmt.Errorf("serialize canonical claim key: %w", err)
	}
	return key, nil
}

// ClaimIdentityStore is a shadow-only registry client. It never writes a
// semantic assertion or enables a metric writer; callers may compare its
// stable claim ID with legacy occurrence identity before cutover.
type ClaimIdentityStore struct {
	DB         DBX
	NewClaimID func() string
}

func (s ClaimIdentityStore) FindOrCreateShadow(ctx context.Context, input CanonicalClaimInput) (claimID string, created bool, err error) {
	if s.DB == nil {
		return "", false, errors.New("db is nil")
	}
	key, err := SerializeCanonicalClaimKey(input)
	if err != nil {
		return "", false, err
	}
	payload, err := json.Marshal(input.Fields)
	if err != nil {
		return "", false, fmt.Errorf("serialize claim identity payload: %w", err)
	}
	newID := uuid.NewString
	if s.NewClaimID != nil {
		newID = s.NewClaimID
	}
	claimID = strings.TrimSpace(newID())
	if claimID == "" {
		return "", false, errors.New("new claim ID is empty")
	}
	err = s.DB.QueryRowContext(ctx, `
INSERT INTO kb.semantic_claim_identities (
    claim_id, key_version, canonical_key, class_term_id, identity_payload, create_by
)
VALUES ($1, $2, $3, $4, $5::jsonb, $6)
ON CONFLICT (key_version, canonical_key) DO NOTHING
RETURNING claim_id`, claimID, strings.TrimSpace(input.KeyVersion), key, strings.TrimSpace(input.ClassTermID), string(payload), nullable(input.By)).Scan(&claimID)
	if err == nil {
		return claimID, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("create shadow claim identity: %w", err)
	}
	if err := s.DB.QueryRowContext(ctx, `
SELECT claim_id FROM kb.semantic_claim_identities
WHERE key_version = $1 AND canonical_key = $2`, strings.TrimSpace(input.KeyVersion), key).Scan(&claimID); err != nil {
		return "", false, fmt.Errorf("load converged shadow claim identity: %w", err)
	}
	return claimID, false, nil
}
