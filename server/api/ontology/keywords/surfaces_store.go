package keywords

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Surface is one keyword surface form. SurfaceID is opaque and content-derived
// (kws_<sha256[:12]> of concept_id + surface + label_role). Surface is the
// verbatim text — never only a derived key.
type Surface struct {
	SurfaceID   string    `json:"surface_id"`
	ConceptID   string    `json:"concept_id"`
	Surface     string    `json:"surface"`
	NormKey     string    `json:"norm_key"`
	NormVersion int       `json:"norm_version"`
	LabelRole   string    `json:"label_role"`
	AliasType   string    `json:"alias_type"`
	Lang        string    `json:"lang"`
	Scope       string    `json:"scope"`
	Confidence  float64   `json:"confidence"`
	Provenance  string    `json:"provenance"`
	Locked      bool      `json:"locked"`
	Evidence    *string   `json:"evidence"`
	CreateTime  time.Time `json:"create_time"`
	ModifyTime  time.Time `json:"modify_time"`
}

// AllowedLabelRoles mirrors the schema CHECK constraint.
var AllowedLabelRoles = map[string]bool{
	"pref":   true,
	"alt":    true,
	"hidden": true,
}

func validateSurface(s Surface) error {
	if s.Surface == "" {
		return fmt.Errorf("surface is required")
	}
	if s.ConceptID == "" {
		return fmt.Errorf("concept_id is required")
	}
	if s.NormKey == "" {
		return fmt.Errorf("norm_key is required")
	}
	if !AllowedLabelRoles[s.LabelRole] {
		return fmt.Errorf("invalid label_role: %s", s.LabelRole)
	}
	if s.AliasType == "" {
		return fmt.Errorf("alias_type is required")
	}
	if s.Provenance == "" {
		return fmt.Errorf("provenance is required")
	}
	return nil
}

// deriveSurfaceID produces an opaque, content-derived surface id.
func deriveSurfaceID(conceptID, surface, labelRole string) string {
	h := sha256.Sum256([]byte(conceptID + "|" + surface + "|" + labelRole))
	return "kws_" + hex.EncodeToString(h[:6])
}

// SurfaceStore persists keyword surface rows.
type SurfaceStore struct {
	DB DBX
}

const surfaceColumns = `surface_id, concept_id, surface, norm_key, norm_version, label_role, alias_type, lang, scope, confidence, provenance, locked, evidence, create_time, modify_time`

const surfaceFrom = `FROM kb.keyword_surfaces`

func scanSurface(scan func(dest ...any) error) (Surface, error) {
	var (
		s        Surface
		evidence sql.NullString
	)
	if err := scan(
		&s.SurfaceID, &s.ConceptID, &s.Surface, &s.NormKey, &s.NormVersion,
		&s.LabelRole, &s.AliasType, &s.Lang, &s.Scope, &s.Confidence,
		&s.Provenance, &s.Locked, &evidence, &s.CreateTime, &s.ModifyTime,
	); err != nil {
		return Surface{}, err
	}
	if evidence.Valid {
		s.Evidence = &evidence.String
	}
	return s, nil
}

// CreateSurface inserts a new keyword surface. SurfaceID is derived from
// concept_id + surface + label_role, making it deterministic for idempotent
// upserts.
func (s SurfaceStore) CreateSurface(ctx context.Context, sf Surface) (Surface, error) {
	sf.SurfaceID = deriveSurfaceID(sf.ConceptID, sf.Surface, sf.LabelRole)
	if sf.LabelRole == "" {
		sf.LabelRole = "pref"
	}
	if sf.Lang == "" {
		sf.Lang = "en"
	}
	if sf.Scope == "" {
		sf.Scope = "_"
	}
	if sf.Confidence == 0 {
		sf.Confidence = 1.0
	}
	if sf.NormVersion == 0 {
		sf.NormVersion = 1
	}
	if err := validateSurface(sf); err != nil {
		return Surface{}, err
	}
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO kb.keyword_surfaces
			(surface_id, concept_id, surface, norm_key, norm_version, label_role, alias_type, lang, scope, confidence, provenance, locked, evidence)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING `+surfaceColumns,
		sf.SurfaceID, sf.ConceptID, sf.Surface, sf.NormKey, sf.NormVersion,
		sf.LabelRole, sf.AliasType, sf.Lang, sf.Scope, sf.Confidence,
		sf.Provenance, sf.Locked, nullableString(func() string {
			if sf.Evidence != nil {
				return *sf.Evidence
			}
			return ""
		}()))
	return scanSurface(row.Scan)
}

// GetSurface retrieves a surface by its opaque surface_id.
func (s SurfaceStore) GetSurface(ctx context.Context, surfaceID string) (Surface, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT `+surfaceColumns+`
		`+surfaceFrom+`
		WHERE surface_id = $1`, surfaceID)
	return scanSurface(row.Scan)
}

// ListSurfacesByConcept returns all surfaces for a concept.
func (s SurfaceStore) ListSurfacesByConcept(ctx context.Context, conceptID string) ([]Surface, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+surfaceColumns+`
		`+surfaceFrom+`
		WHERE concept_id = $1
		ORDER BY label_role, surface`, conceptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Surface
	for rows.Next() {
		sf, err := scanSurface(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, sf)
	}
	return out, rows.Err()
}

// ListSurfacesByNormKey returns surfaces matching a norm_key within a scope.
// Used by KeywordFamily.CandidateNodes for tier 1 lookups.
func (s SurfaceStore) ListSurfacesByNormKey(ctx context.Context, normKey, scope string) ([]Surface, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+surfaceColumns+`
		`+surfaceFrom+`
		WHERE norm_key = $1 AND scope = $2
		ORDER BY confidence DESC`, normKey, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Surface
	for rows.Next() {
		sf, err := scanSurface(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, sf)
	}
	return out, rows.Err()
}

// UpdateSurfaceLock locks or unlocks a surface. Locked surfaces may not be
// modified by the reconciler.
func (s SurfaceStore) UpdateSurfaceLock(ctx context.Context, surfaceID string, locked bool) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE kb.keyword_surfaces
		SET locked = $2, modify_time = NOW()
		WHERE surface_id = $1`, surfaceID, locked)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("surface %s not found", surfaceID)
	}
	return nil
}
