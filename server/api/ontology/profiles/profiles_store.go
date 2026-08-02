// Package profiles persists governed review profiles and their rules. Profiles
// become runtime-visible only through an activated ontology module release.
package profiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
	"github.com/lib/pq"
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

// releasedProfileColumns is activeProfileColumns for the pinned loader, which
// joins only the release row (r) and never the current activation pointer
// (ar): the release id is r.id, and there is no active-release join to read.
const releasedProfileColumns = `
	p.id, p.profile_id, p.version, p.module_id, p.status,
	COALESCE(p.title, ''), p.applicability, p.closed_dimensions,
	r.id, r.version, p.create_time, COALESCE(p.create_by, ''),
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

// PinnedRelease is one active module release pinned by the released-profile
// loader. ReleaseID and Checksum are retained as attempt inputs so a scope
// can be reproduced and verified even after activation changes.
type PinnedRelease struct {
	ModuleID  string `json:"module_id"`
	ReleaseID int64  `json:"release_id"`
	Version   string `json:"version"`
	Checksum  string `json:"content_checksum"`
}

// ReleasedProfiles is the result of one pinned load: the active module
// releases pinned at the moment of the load plus only the profiles visible to
// that knowledge store (status included_in_release on a pinned release).
type ReleasedProfiles struct {
	Releases []PinnedRelease
	Profiles []Profile
}

// txBeginner is the transaction capability the pinned loader needs. The
// store's terms.DBX surface deliberately omits it because *sql.Tx satisfies
// DBX but cannot begin a nested transaction; the loader requires a fresh
// transaction and detects the capability at the call site.
type txBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// LoadReleasedProfiles pins the currently active module releases and loads
// only the profiles visible through those pinned releases, in one short
// repeatable-read transaction (spec §6 item 2). The transaction is committed
// before the loader returns, so a classifier call can never run inside it;
// the caller retains the release ids/checksums as attempt inputs. The load
// joins the pinned release ids directly rather than rereading current
// activation, so a later activation change cannot partially change the
// selection attempt.
func (s ProfileStore) LoadReleasedProfiles(ctx context.Context) (ReleasedProfiles, error) {
	if s.DB == nil {
		return ReleasedProfiles{}, errors.New("db is nil")
	}
	begin, ok := s.DB.(txBeginner)
	if !ok {
		return ReleasedProfiles{}, errors.New("pinned profile load requires a transaction-capable connection")
	}
	// Repeatable-read: every read in the transaction observes one consistent
	// activation snapshot, and the commit ends the transaction before any
	// classifier call (§6 item 2).
	tx, err := begin.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return ReleasedProfiles{}, err
	}
	defer tx.Rollback()

	// Pin active module releases (same shape as modules.ReleaseStore's
	// LoadActiveModuleReleases) inside the transaction.
	const pinStmt = `
SELECT ar.module_id, ar.release_id, r.version, r.content_checksum
FROM kb.ontology_active_releases ar
JOIN kb.ontology_module_releases r ON r.id = ar.release_id
WHERE ar.deactivated_at IS NULL`
	rows, err := tx.QueryContext(ctx, pinStmt)
	if err != nil {
		return ReleasedProfiles{}, err
	}
	pins := make([]PinnedRelease, 0)
	for rows.Next() {
		var pin PinnedRelease
		if err := rows.Scan(&pin.ModuleID, &pin.ReleaseID, &pin.Version, &pin.Checksum); err != nil {
			rows.Close()
			return ReleasedProfiles{}, err
		}
		pins = append(pins, pin)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ReleasedProfiles{}, err
	}
	rows.Close()
	if len(pins) == 0 {
		if err := tx.Commit(); err != nil {
			return ReleasedProfiles{}, err
		}
		return ReleasedProfiles{Releases: pins, Profiles: []Profile{}}, nil
	}

	releaseIDs := make([]int64, len(pins))
	for i, p := range pins {
		releaseIDs[i] = p.ReleaseID
	}

	// Load only the profiles belonging to the pinned releases, never the
	// current activation pointer: same runtime-visibility gate as
	// ListActiveProfiles, but against the pinned ids.
	const profileStmt = `SELECT ` + releasedProfileColumns + `
FROM kb.ontology_profiles p
JOIN kb.ontology_module_releases r ON r.id = p.released_in_release_id
WHERE r.id = ANY($1::bigint[])
  AND p.status = 'included_in_release'
ORDER BY p.profile_id, p.version DESC`
	profileRows, err := tx.QueryContext(ctx, profileStmt, pq.Array(releaseIDs))
	if err != nil {
		return ReleasedProfiles{}, err
	}
	profiles := make([]Profile, 0)
	for profileRows.Next() {
		p, err := scanProfile(profileRows.Scan)
		if err != nil {
			profileRows.Close()
			return ReleasedProfiles{}, err
		}
		profiles = append(profiles, p)
	}
	if err := profileRows.Err(); err != nil {
		profileRows.Close()
		return ReleasedProfiles{}, err
	}
	profileRows.Close()

	if err := tx.Commit(); err != nil {
		return ReleasedProfiles{}, err
	}
	return ReleasedProfiles{Releases: pins, Profiles: profiles}, nil
}

// DeriveKnowledgeStore resolves the single knowledge store behind the
// reviewed documents of a deterministic scope request (spec §6 item 1).
// Knowledge-store identity comes from kb.inputs.ks_store_id, never client
// input. A mixed-store or unresolvable document set is rejected so the
// selection layer can never guess a store.
func (s ProfileStore) DeriveKnowledgeStore(ctx context.Context, documentIDs []int64) (int64, error) {
	if s.DB == nil {
		return 0, errors.New("db is nil")
	}
	// PostgreSQL returns one row per id, so a duplicated document id must not
	// be miscounted against the input length as an unresolvable document.
	unique := deduplicateIDs(documentIDs)
	if len(unique) == 0 {
		return 0, errors.New("reviewed document ids are required")
	}
	const stmt = `SELECT ks_store_id FROM kb.inputs WHERE id = ANY($1::bigint[])`
	rows, err := s.DB.QueryContext(ctx, stmt, pq.Array(unique))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	storeID := int64(0)
	seen := 0
	for rows.Next() {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		if !id.Valid || id.Int64 == 0 {
			continue // unbound document: never a store candidate
		}
		if storeID == 0 {
			storeID = id.Int64
		} else if storeID != id.Int64 {
			return 0, fmt.Errorf("reviewed documents resolve to multiple knowledge stores: cannot derive a single store for a deterministic scope")
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if seen != len(unique) {
		return 0, fmt.Errorf("%d reviewed document(s) do not resolve to a knowledge store", len(unique)-seen)
	}
	return storeID, nil
}

func deduplicateIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
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
