package keywords

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
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

// SurfaceStore persists keyword surface rows. NormalizerVersion is the
// version derived keys are produced at (D6: keys = surface + version); it
// defaults to the current normalizer version.
type SurfaceStore struct {
	DB                DBX
	NormalizerVersion int
}

func (s SurfaceStore) normVersion() int {
	if s.NormalizerVersion <= 0 {
		return semid.CurrentNormalizerVersion
	}
	return s.NormalizerVersion
}

// txBeginner is implemented by handles that can open transactions (*sql.DB).
type txBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
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

// CreateSurface inserts a new keyword surface together with its derived keys
// (K1). Keys are derived server-side from the surface (D6): a caller-supplied
// norm_key that does not match the derived key is rejected, never trusted
// (K3). SurfaceID is derived from concept_id + surface + label_role, making
// it deterministic for idempotent upserts.
func (s SurfaceStore) CreateSurface(ctx context.Context, sf Surface) (Surface, error) {
	version := s.normVersion()
	ks := (semid.Normalizer{Version: version}).Normalize(sf.Surface)
	if sf.NormKey != "" && sf.NormKey != ks.Norm {
		return Surface{}, fmt.Errorf("norm_key %q does not match derived key %q: keys are derived from the surface, never authored (K3/D6)", sf.NormKey, ks.Norm)
	}
	sf.NormKey = ks.Norm
	sf.NormVersion = version

	if sf.LabelRole == "" {
		sf.LabelRole = "pref"
	}
	// The id is derived after the role default: the scheme hashes
	// concept|surface|role, so the hashed role must be the stored role
	// (§20.2 "surface_id hashed before role defaulting").
	sf.SurfaceID = deriveSurfaceID(sf.ConceptID, sf.Surface, sf.LabelRole)
	if sf.Lang == "" {
		sf.Lang = "en"
	}
	if sf.Scope == "" {
		sf.Scope = "_"
	}
	if sf.Confidence == 0 {
		sf.Confidence = 1.0
	}
	if err := validateSurface(sf); err != nil {
		return Surface{}, err
	}

	keys := derivedSurfaceKeys(ks, sf.SurfaceID, version)

	// The surface row and its derived keys are written together (K1), in a
	// transaction when the handle supports it.
	if beginner, ok := s.DB.(txBeginner); ok {
		tx, err := beginner.BeginTx(ctx, nil)
		if err != nil {
			return Surface{}, err
		}
		created, err := insertSurfaceRow(ctx, tx, sf)
		if err != nil {
			_ = tx.Rollback()
			return Surface{}, err
		}
		if err := (SurfaceKeyStore{DB: tx}).UpsertSurfaceKeys(ctx, created.SurfaceID, keys); err != nil {
			_ = tx.Rollback()
			return Surface{}, err
		}
		if err := tx.Commit(); err != nil {
			return Surface{}, err
		}
		return created, nil
	}

	created, err := insertSurfaceRow(ctx, s.DB, sf)
	if err != nil {
		return Surface{}, err
	}
	if err := (SurfaceKeyStore{DB: s.DB}).UpsertSurfaceKeys(ctx, created.SurfaceID, keys); err != nil {
		return Surface{}, err
	}
	return created, nil
}

func insertSurfaceRow(ctx context.Context, db DBX, sf Surface) (Surface, error) {
	row := db.QueryRowContext(ctx, `
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

// derivedSurfaceKeys maps the normalizer's key set onto stored key rows.
// The singular row is written even when it equals the norm key: it is the
// plural→singular bridge (a query "analyses" finds a stored "analysis"
// through it). Other keys equal to the norm key carry no lookup signal —
// tier 1 already matches the norm key — and are omitted.
func derivedSurfaceKeys(ks semid.KeySet, surfaceID string, version int) []SurfaceKey {
	keys := make([]SurfaceKey, 0, 5)
	add := func(kind, value string, allowNormEqual bool) {
		if value == "" || (!allowNormEqual && value == ks.Norm) {
			return
		}
		keys = append(keys, SurfaceKey{SurfaceID: surfaceID, KeyKind: kind, KeyValue: value, NormVersion: version})
	}
	add("alnum", ks.Alnum, false)
	add("sorted", ks.Sorted, false)
	// phonetic is deliberately not persisted: no tier reads it (Double
	// Metaphone is deferred, §20.1), and writing key rows nothing queries
	// is the §20.2 "dead phonetic key" defect. The schema CHECK still
	// permits the kind for when a phonetic tier lands.
	add("initials", ks.Initials, false)
	add("singular", ks.Singular, true)
	return keys
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
