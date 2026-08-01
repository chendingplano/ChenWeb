package modules

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/profiles"
	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
	"github.com/lib/pq"
)

// Release is one immutable snapshot of a module's approved content. payload
// holds the full snapshot for reproducibility; content_checksum is its
// deterministic hash; dependency_releases pins the release version of each
// declared dependency.
type Release struct {
	ID                    int64           `json:"id"`
	ModuleID              string          `json:"module_id"`
	Version               string          `json:"version"`
	Title                 string          `json:"title"`
	Payload               json.RawMessage `json:"payload"`
	ContentChecksum       string          `json:"content_checksum"`
	DependencyReleases    json.RawMessage `json:"dependency_releases"`
	SupersededByReleaseID *int64          `json:"superseded_by_release_id"`
	ReleasedBy            string          `json:"released_by"`
	ReleasedAt            time.Time       `json:"released_at"`
}

// ActiveRelease is the activation pointer for a module: at most one row with
// deactivated_at NULL per module. Activation/rollback insert new rows and
// deactivate the previous one; nothing is deleted.
type ActiveRelease struct {
	ID             int64      `json:"id"`
	ModuleID       string     `json:"module_id"`
	ReleaseID      int64      `json:"release_id"`
	ReleaseVersion string     `json:"release_version"`
	ActivatedAt    time.Time  `json:"activated_at"`
	ActivatedBy    string     `json:"activated_by"`
	DeactivatedAt  *time.Time `json:"deactivated_at"`
}

// ReleaseStore persists module releases and the activation pointer.
type ReleaseStore struct {
	DB *sql.DB
}

const releaseColumns = `
	id, module_id, version, COALESCE(title, ''), payload, content_checksum,
	dependency_releases, superseded_by_release_id, COALESCE(released_by, ''), released_at
FROM kb.ontology_module_releases`

func scanRelease(scan func(dest ...any) error) (Release, error) {
	var (
		r            Release
		supersededBy sql.NullInt64
	)
	if err := scan(
		&r.ID, &r.ModuleID, &r.Version, &r.Title, &r.Payload, &r.ContentChecksum,
		&r.DependencyReleases, &supersededBy, &r.ReleasedBy, &r.ReleasedAt,
	); err != nil {
		return Release{}, err
	}
	if supersededBy.Valid {
		v := supersededBy.Int64
		r.SupersededByReleaseID = &v
	}
	return r, nil
}

// CreateRelease builds an immutable release from the module's approved
// content in one transaction: validate -> snapshot -> checksum -> pin
// dependencies -> insert -> tag the included content rows -> supersede older
// releases. Any validation failure rolls the transaction back and leaves the
// previous active release untouched (spec §16.3 items 5 and 7).
func (s ReleaseStore) CreateRelease(ctx context.Context, moduleID, version, by string) (Release, error) {
	if s.DB == nil {
		return Release{}, errors.New("db is nil")
	}
	if strings.TrimSpace(version) == "" {
		return Release{}, errors.New("version is required")
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return Release{}, err
	}
	defer tx.Rollback()

	snap, pins, err := s.validateAndBuildSnapshot(ctx, tx, strings.TrimSpace(moduleID), strings.TrimSpace(version))
	if err != nil {
		return Release{}, err
	}

	payload, err := json.Marshal(snap)
	if err != nil {
		return Release{}, err
	}
	checksum, err := Checksum(payload)
	if err != nil {
		return Release{}, err
	}
	depPins, err := json.Marshal(pins)
	if err != nil {
		return Release{}, err
	}

	var releaseID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO kb.ontology_module_releases
	(module_id, version, title, payload, content_checksum, dependency_releases, released_by)
VALUES ($1, $2, $3, $4::jsonb, $5, $6::jsonb, $7)
RETURNING id`,
		strings.TrimSpace(moduleID), strings.TrimSpace(version), nullableString(snapTitle(snap)),
		string(payload), checksum, string(depPins), nullableString(by),
	).Scan(&releaseID)
	if err != nil {
		return Release{}, fmt.Errorf("insert release: %w", err)
	}

	if err := tagContentRows(ctx, tx, releaseID, snap); err != nil {
		return Release{}, err
	}

	// Approved candidates whose content this release includes become
	// included_in_release. This is the ONLY path that activates a candidate
	// (spec §16.3 item 3: approved candidates remain inactive until included
	// in a module release).
	if _, err := tx.ExecContext(ctx, `
UPDATE kb.ontology_candidates
SET status = 'included_in_release', modify_time = NOW()
WHERE id IN (
	SELECT DISTINCT source_candidate_id FROM kb.ontology_terms WHERE released_in_release_id = $1 AND source_candidate_id IS NOT NULL
	UNION
	SELECT DISTINCT source_candidate_id FROM kb.ontology_term_labels WHERE released_in_release_id = $1 AND source_candidate_id IS NOT NULL
	UNION
	SELECT DISTINCT source_candidate_id FROM kb.ontology_axioms WHERE released_in_release_id = $1 AND source_candidate_id IS NOT NULL
	UNION
	SELECT DISTINCT source_candidate_id FROM kb.ontology_mappings WHERE released_in_release_id = $1 AND source_candidate_id IS NOT NULL
)`, releaseID); err != nil {
		return Release{}, err
	}

	// The new release supersedes any prior release of the same module.
	if _, err := tx.ExecContext(ctx, `
UPDATE kb.ontology_module_releases
SET superseded_by_release_id = $1, modify_time = NOW()
WHERE module_id = $2 AND id <> $1 AND superseded_by_release_id IS NULL`,
		releaseID, strings.TrimSpace(moduleID)); err != nil {
		return Release{}, err
	}

	if err := tx.Commit(); err != nil {
		return Release{}, err
	}
	return s.GetReleaseByID(ctx, releaseID)
}

func snapTitle(snap snapshot) string {
	if len(snap.Terms) == 0 {
		return ""
	}
	return fmt.Sprintf("module %s release %s (%d terms)", snap.ModuleID, snap.Version, len(snap.Terms))
}

func tagContentRows(ctx context.Context, tx *sql.Tx, releaseID int64, snap snapshot) error {
	tag := func(table string, ids []int64) error {
		if len(ids) == 0 {
			return nil
		}
		_, err := tx.ExecContext(ctx,
			"UPDATE "+table+" SET status = 'included_in_release', released_in_release_id = $1, modify_time = NOW() WHERE id = ANY($2::bigint[])",
			releaseID, pq.Array(ids),
		)
		return err
	}
	if err := tag("kb.ontology_terms", idsOfTerms(snap.Terms)); err != nil {
		return err
	}
	if err := tag("kb.ontology_term_labels", idsOfLabels(snap.Labels)); err != nil {
		return err
	}
	if err := tag("kb.ontology_axioms", idsOfAxioms(snap.Axioms)); err != nil {
		return err
	}
	if err := tag("kb.ontology_mappings", idsOfMappings(snap.Mappings)); err != nil {
		return err
	}
	if err := tag("kb.ontology_profiles", idsOfProfiles(snap.Profiles)); err != nil {
		return err
	}
	return tag("kb.ontology_profile_rules", idsOfProfileRules(snap.ProfileRules))
}

func idsOfTerms(ts []terms.Term) []int64 {
	out := make([]int64, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}

func idsOfLabels(ls []terms.TermLabel) []int64 {
	out := make([]int64, 0, len(ls))
	for _, l := range ls {
		out = append(out, l.ID)
	}
	return out
}

func idsOfAxioms(as []terms.Axiom) []int64 {
	out := make([]int64, 0, len(as))
	for _, a := range as {
		out = append(out, a.ID)
	}
	return out
}

func idsOfMappings(ms []terms.Mapping) []int64 {
	out := make([]int64, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

func idsOfProfiles(ps []profiles.Profile) []int64 {
	out := make([]int64, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

func idsOfProfileRules(rs []profiles.ProfileRule) []int64 {
	out := make([]int64, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

// Validate runs the release-time gate for a module without writing anything:
// the module must exist, hold at least one approved term, have acyclic
// registered dependencies with pinnable releases, and every axiom/mapping
// reference must resolve. It is the compiler's `validate` subcommand.
func (s ReleaseStore) Validate(ctx context.Context, moduleID string) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	_, _, err := s.validateAndBuildSnapshot(ctx, s.DB, moduleID, "")
	return err
}

// GetReleaseByID returns a release by id.
func (s ReleaseStore) GetReleaseByID(ctx context.Context, id int64) (Release, error) {
	if s.DB == nil {
		return Release{}, errors.New("db is nil")
	}
	const stmt = `SELECT ` + releaseColumns + `
WHERE id = $1`
	row := s.DB.QueryRowContext(ctx, stmt, id)
	return scanRelease(row.Scan)
}

// GetRelease returns a release by module_id + version.
func (s ReleaseStore) GetRelease(ctx context.Context, moduleID, version string) (Release, error) {
	if s.DB == nil {
		return Release{}, errors.New("db is nil")
	}
	const stmt = `SELECT ` + releaseColumns + `
WHERE module_id = $1 AND version = $2`
	row := s.DB.QueryRowContext(ctx, stmt, strings.TrimSpace(moduleID), strings.TrimSpace(version))
	return scanRelease(row.Scan)
}

// ListReleases returns a module's releases, newest version first.
func (s ReleaseStore) ListReleases(ctx context.Context, moduleID string) ([]Release, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	stmt := `SELECT ` + releaseColumns + `
WHERE ($1 = '' OR module_id = $1)
ORDER BY module_id, version DESC`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(moduleID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Release, 0)
	for rows.Next() {
		r, err := scanRelease(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Activate makes a release the module's active release. The current active
// pointer is deactivated and the target is activated in one transaction; the
// partial unique index (one active per module) is the real enforcement.
func (s ReleaseStore) Activate(ctx context.Context, moduleID string, releaseID int64, by string) (ActiveRelease, error) {
	if s.DB == nil {
		return ActiveRelease{}, errors.New("db is nil")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return ActiveRelease{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
UPDATE kb.ontology_active_releases
SET deactivated_at = NOW(), deactivated_by = $2
WHERE module_id = $1 AND deactivated_at IS NULL`,
		strings.TrimSpace(moduleID), nullableString(by)); err != nil {
		return ActiveRelease{}, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `
INSERT INTO kb.ontology_active_releases (module_id, release_id, activated_at, activated_by)
VALUES ($1, $2, NOW(), $3)
RETURNING id`,
		strings.TrimSpace(moduleID), releaseID, nullableString(by),
	).Scan(&id); err != nil {
		return ActiveRelease{}, fmt.Errorf("activate release: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ActiveRelease{}, err
	}
	return s.GetActiveRelease(ctx, strings.TrimSpace(moduleID))
}

// Rollback re-points the active pointer at an older release (by version).
// Nothing is deleted; history is preserved.
func (s ReleaseStore) Rollback(ctx context.Context, moduleID, version, by string) (ActiveRelease, error) {
	if s.DB == nil {
		return ActiveRelease{}, errors.New("db is nil")
	}
	release, err := s.GetRelease(ctx, moduleID, version)
	if err != nil {
		return ActiveRelease{}, fmt.Errorf("release %s@%s not found", moduleID, version)
	}
	return s.Activate(ctx, moduleID, release.ID, by)
}

// GetActiveRelease returns the currently active release of a module.
func (s ReleaseStore) GetActiveRelease(ctx context.Context, moduleID string) (ActiveRelease, error) {
	if s.DB == nil {
		return ActiveRelease{}, errors.New("db is nil")
	}
	const stmt = `
SELECT ar.id, ar.module_id, ar.release_id, r.version, ar.activated_at,
	COALESCE(ar.activated_by, ''), ar.deactivated_at
FROM kb.ontology_active_releases ar
JOIN kb.ontology_module_releases r ON r.id = ar.release_id
WHERE ar.module_id = $1 AND ar.deactivated_at IS NULL`
	var (
		ar            ActiveRelease
		deactivatedAt sql.NullTime
	)
	if err := s.DB.QueryRowContext(ctx, stmt, strings.TrimSpace(moduleID)).Scan(
		&ar.ID, &ar.ModuleID, &ar.ReleaseID, &ar.ReleaseVersion, &ar.ActivatedAt,
		&ar.ActivatedBy, &deactivatedAt,
	); err != nil {
		return ActiveRelease{}, err
	}
	if deactivatedAt.Valid {
		t := deactivatedAt.Time
		ar.DeactivatedAt = &t
	}
	return ar, nil
}

// activeReleaseVersion returns the version of the module's active release via
// the given queryer (a transaction during release building).
func (s ReleaseStore) activeReleaseVersion(ctx context.Context, db terms.DBX, moduleID string) (string, error) {
	var version string
	err := db.QueryRowContext(ctx, `
SELECT r.version
FROM kb.ontology_active_releases ar
JOIN kb.ontology_module_releases r ON r.id = ar.release_id
WHERE ar.module_id = $1 AND ar.deactivated_at IS NULL`,
		strings.TrimSpace(moduleID)).Scan(&version)
	return version, err
}

// latestReleaseVersion returns the newest release version of a module.
func (s ReleaseStore) latestReleaseVersion(ctx context.Context, db terms.DBX, moduleID string) (string, error) {
	var version string
	err := db.QueryRowContext(ctx, `
SELECT version
FROM kb.ontology_module_releases
WHERE module_id = $1
ORDER BY version DESC
LIMIT 1`, strings.TrimSpace(moduleID)).Scan(&version)
	return version, err
}
