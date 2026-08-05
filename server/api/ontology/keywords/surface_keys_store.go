package keywords

import (
	"context"
	"fmt"
)

// SurfaceKey is one derived alternate key for a surface. Keys are derived
// data — always written alongside the surface, never authored independently.
// key_kind is one of: alnum, sorted, phonetic, initials, singular.
type SurfaceKey struct {
	SurfaceID   string `json:"surface_id"`
	KeyKind     string `json:"key_kind"`
	KeyValue    string `json:"key_value"`
	NormVersion int    `json:"norm_version"`
}

// AllowedKeyKinds mirrors the schema CHECK constraint.
var AllowedKeyKinds = map[string]bool{
	"alnum":    true,
	"sorted":   true,
	"phonetic": true,
	"initials": true,
	"singular": true,
}

// SurfaceKeyStore persists derived surface key rows.
type SurfaceKeyStore struct {
	DB DBX
}

// UpsertSurfaceKeys replaces all keys for a surface with the given set.
// Runs in a transaction: DELETE existing keys, then INSERT the new ones.
func (s SurfaceKeyStore) UpsertSurfaceKeys(ctx context.Context, surfaceID string, keys []SurfaceKey) error {
	if len(keys) == 0 {
		return nil
	}
	for _, k := range keys {
		if !AllowedKeyKinds[k.KeyKind] {
			return fmt.Errorf("invalid key_kind: %s", k.KeyKind)
		}
		if k.KeyValue == "" {
			return fmt.Errorf("key_value is required for key_kind %s", k.KeyKind)
		}
	}

	// Delete existing keys for this surface.
	if _, err := s.DB.ExecContext(ctx, `
		DELETE FROM kb.keyword_surface_keys WHERE surface_id = $1`, surfaceID); err != nil {
		return fmt.Errorf("delete surface keys for %s: %w", surfaceID, err)
	}

	// Insert new keys.
	for _, k := range keys {
		if _, err := s.DB.ExecContext(ctx, `
			INSERT INTO kb.keyword_surface_keys (surface_id, key_kind, key_value, norm_version)
			VALUES ($1, $2, $3, $4)`,
			surfaceID, k.KeyKind, k.KeyValue, k.NormVersion); err != nil {
			return fmt.Errorf("insert surface key (%s, %s): %w", surfaceID, k.KeyKind, err)
		}
	}
	return nil
}

// LookupByKeyKind finds surface keys matching a specific key kind and value.
// Used by KeywordFamily.CandidateNodes for tier 2/4 lookups.
func (s SurfaceKeyStore) LookupByKeyKind(ctx context.Context, keyKind, keyValue string, normVersion int) ([]SurfaceKey, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT sk.surface_id, sk.key_kind, sk.key_value, sk.norm_version
		FROM kb.keyword_surface_keys sk
		WHERE sk.key_kind = $1 AND sk.key_value = $2 AND sk.norm_version = $3`,
		keyKind, keyValue, normVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SurfaceKey
	for rows.Next() {
		var sk SurfaceKey
		if err := rows.Scan(&sk.SurfaceID, &sk.KeyKind, &sk.KeyValue, &sk.NormVersion); err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}
