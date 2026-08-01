// Package profiles persists governed review profiles and their rules. Profiles
// become runtime-visible only through an activated ontology module release.
package profiles

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// Profile is one version of a governed review profile. ReleaseID is populated
// only for the active-release view, which prevents draft content being used by
// a normative review.
type Profile struct {
	ID               int64           `json:"id"`
	ProfileID        string          `json:"profile_id"`
	Version          int             `json:"version"`
	ModuleID         string          `json:"module_id"`
	Status           string          `json:"status"`
	Title            string          `json:"title"`
	Applicability    json.RawMessage `json:"applicability"`
	ClosedDimensions json.RawMessage `json:"closed_dimensions"`
	ReleaseID        int64           `json:"release_id"`
	ReleaseVersion   string          `json:"release_version"`
	CreateTime       time.Time       `json:"create_time"`
	CreateBy         string          `json:"create_by"`
	ModifyTime       time.Time       `json:"modify_time"`
	ModifyBy         string          `json:"modify_by"`
}

// ProfileStore reads governed profile content from the database.
type ProfileStore struct {
	DB terms.DBX
}

var profileTransitions = map[string]map[string]bool{
	"draft":               {"in_review": true, "rejected": true},
	"in_review":           {"approved": true, "rejected": true},
	"approved":            {"included_in_release": true, "superseded": true},
	"included_in_release": {"superseded": true},
}

const activeProfileColumns = `
	p.id, p.profile_id, p.version, p.module_id, p.status,
	COALESCE(p.title, ''), p.applicability, p.closed_dimensions,
	ar.release_id, r.version, p.create_time, COALESCE(p.create_by, ''),
	p.modify_time, COALESCE(p.modify_by, '')`

func scanProfile(scan func(dest ...any) error) (Profile, error) {
	var p Profile
	err := scan(
		&p.ID, &p.ProfileID, &p.Version, &p.ModuleID, &p.Status, &p.Title,
		&p.Applicability, &p.ClosedDimensions, &p.ReleaseID, &p.ReleaseVersion,
		&p.CreateTime, &p.CreateBy, &p.ModifyTime, &p.ModifyBy,
	)
	return p, err
}

func scanStagedProfile(scan func(dest ...any) error) (Profile, error) {
	var p Profile
	err := scan(
		&p.ID, &p.ProfileID, &p.Version, &p.ModuleID, &p.Status, &p.Title,
		&p.Applicability, &p.ClosedDimensions, &p.CreateTime, &p.CreateBy,
		&p.ModifyTime, &p.ModifyBy,
	)
	return p, err
}

// CreateProfile creates the initial draft version of a profile. Approval and
// inclusion in a release are separate governed transitions owned by the
// module compiler/review flow.
func (s ProfileStore) CreateProfile(ctx context.Context, p Profile) (Profile, error) {
	if s.DB == nil {
		return Profile{}, errors.New("db is nil")
	}
	if strings.TrimSpace(p.ProfileID) == "" || strings.TrimSpace(p.ModuleID) == "" {
		return Profile{}, errors.New("profile_id and module_id are required")
	}
	if p.Status == "" {
		p.Status = "draft"
	}
	if p.Status != "draft" {
		return Profile{}, errors.New("new profiles must start in draft")
	}
	if len(p.Applicability) == 0 {
		p.Applicability = json.RawMessage(`{}`)
	}
	if len(p.ClosedDimensions) == 0 {
		p.ClosedDimensions = json.RawMessage(`[]`)
	}
	const stmt = `
INSERT INTO kb.ontology_profiles
	(profile_id, version, module_id, status, title, applicability, closed_dimensions, create_by, modify_by)
VALUES ($1, 1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8)
RETURNING id, profile_id, version, module_id, status, COALESCE(title, ''),
	applicability, closed_dimensions, create_time, COALESCE(create_by, ''),
	modify_time, COALESCE(modify_by, '')`
	row := s.DB.QueryRowContext(ctx, stmt,
		strings.TrimSpace(p.ProfileID), strings.TrimSpace(p.ModuleID), p.Status,
		nullableText(p.Title), string(p.Applicability), string(p.ClosedDimensions),
		nullableText(p.CreateBy), nullableText(p.ModifyBy),
	)
	return scanStagedProfile(row.Scan)
}

func nullableText(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func (s ProfileStore) GetProfile(ctx context.Context, profileID string, version int) (Profile, error) {
	if s.DB == nil {
		return Profile{}, errors.New("db is nil")
	}
	const stmt = `SELECT id, profile_id, version, module_id, status, COALESCE(title, ''), applicability, closed_dimensions, create_time, COALESCE(create_by, ''), modify_time, COALESCE(modify_by, '') FROM kb.ontology_profiles
WHERE profile_id = $1 AND version = $2`
	return scanStagedProfile(s.DB.QueryRowContext(ctx, stmt, strings.TrimSpace(profileID), version).Scan)
}

func (s ProfileStore) TransitionStatus(ctx context.Context, profileID string, version int, next, by string) (Profile, error) {
	current, err := s.GetProfile(ctx, profileID, version)
	if err != nil {
		return Profile{}, err
	}
	if !profileTransitions[current.Status][next] {
		return Profile{}, errors.New("illegal profile status transition")
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE kb.ontology_profiles SET status = $3, modify_time = NOW(), modify_by = $4 WHERE profile_id = $1 AND version = $2`, strings.TrimSpace(profileID), version, next, nullableText(by))
	if err != nil {
		return Profile{}, err
	}
	return s.GetProfile(ctx, profileID, version)
}

// ListActiveProfiles returns only profile versions included in the module's
// current activation pointer. A raw profile id or draft status can never make
// content visible through this path.
func (s ProfileStore) ListActiveProfiles(ctx context.Context, moduleID string) ([]Profile, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	if strings.TrimSpace(moduleID) == "" {
		return nil, errors.New("module_id is required")
	}
	const stmt = `SELECT ` + activeProfileColumns + `
FROM kb.ontology_profiles p
JOIN kb.ontology_module_releases r ON r.id = p.released_in_release_id
JOIN kb.ontology_active_releases ar ON ar.release_id = r.id
WHERE ar.deactivated_at IS NULL
  AND p.module_id = $1
  AND p.status = 'included_in_release'
ORDER BY p.profile_id, p.version DESC`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(moduleID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	profiles := make([]Profile, 0)
	for rows.Next() {
		p, err := scanProfile(rows.Scan)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

// ListApprovedProfiles returns staged profile versions for the module compiler.
// This is deliberately separate from ListActiveProfiles: approval alone does
// not make content normative until CreateRelease tags it and activation points
// at that release.
func (s ProfileStore) ListApprovedProfiles(ctx context.Context, moduleID string) ([]Profile, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	if strings.TrimSpace(moduleID) == "" {
		return nil, errors.New("module_id is required")
	}
	const stmt = `SELECT
	id, profile_id, version, module_id, status, COALESCE(title, ''),
	applicability, closed_dimensions, create_time, COALESCE(create_by, ''),
	modify_time, COALESCE(modify_by, '')
FROM kb.ontology_profiles
WHERE module_id = $1 AND status = 'approved'
ORDER BY profile_id, version DESC`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(moduleID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	profiles := make([]Profile, 0)
	for rows.Next() {
		var p Profile
		if err := rows.Scan(
			&p.ID, &p.ProfileID, &p.Version, &p.ModuleID, &p.Status, &p.Title,
			&p.Applicability, &p.ClosedDimensions, &p.CreateTime, &p.CreateBy,
			&p.ModifyTime, &p.ModifyBy,
		); err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}
