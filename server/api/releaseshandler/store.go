package releaseshandler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ProjectChenWeb is the only project with release versions for now.
const ProjectChenWeb = "ChenWeb"

const (
	KindMajor = "major"
	KindMinor = "minor"
	KindBuild = "build"

	StatusBuilding = "building"
	StatusReleased = "released"
)

var (
	ErrInvalid  = errors.New("invalid release request")
	ErrNotFound = errors.New("not found")
	// ErrConflict means the request clashes with a running build or a released item.
	ErrConflict = errors.New("conflict")
)

var (
	itemTypes  = []string{"new feature", "improvement", "bug fix"}
	components = []string{"ChenWeb", "shared"}
)

// Item is one change recorded for a release; ReleaseID is 0 while it is pending.
type Item struct {
	ID          int64     `json:"id"`
	ReleaseID   int64     `json:"-"`
	ItemType    string    `json:"item_type"`
	Component   string    `json:"component"`
	Description string    `json:"description"`
	Issue       string    `json:"issue"`
	PullRequest string    `json:"pull_request"`
	Notes       string    `json:"notes"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// Release is a new major or minor (revision 00) or a build.
type Release struct {
	ID             int64     `json:"id"`
	Project        string    `json:"project"`
	Version        string    `json:"version"`
	Kind           string    `json:"kind"`
	Status         string    `json:"status"`
	Description    string    `json:"description"`
	BuildNumber    string    `json:"build_number"`
	ReleaseDate    string    `json:"release_date"`
	GitRev         string    `json:"git_rev"`
	GitDirty       bool      `json:"git_dirty"`
	SharedGitRev   string    `json:"shared_git_rev"`
	SharedGitDirty bool      `json:"shared_git_dirty"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	Items          []Item    `json:"items"`

	V           Version `json:"-"`
	BuilderHost string  `json:"-"`
	BuilderPID  int     `json:"-"`
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func validProject(project string) error {
	if project != ProjectChenWeb {
		return fmt.Errorf("%w: unknown project %q", ErrInvalid, project)
	}
	return nil
}

func oneOf(v string, allowed []string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// ValidateItem trims the item's text and checks its fields.
func ValidateItem(it *Item) error {
	it.ItemType = strings.TrimSpace(it.ItemType)
	it.Component = strings.TrimSpace(it.Component)
	it.Description = strings.TrimSpace(it.Description)
	it.Issue = strings.TrimPrefix(strings.TrimSpace(it.Issue), "#")
	it.PullRequest = strings.TrimSpace(it.PullRequest)
	it.Notes = strings.TrimSpace(it.Notes)
	if !oneOf(it.ItemType, itemTypes) {
		return fmt.Errorf("%w: type must be one of %s", ErrInvalid, strings.Join(itemTypes, ", "))
	}
	if !oneOf(it.Component, components) {
		return fmt.Errorf("%w: component must be ChenWeb or shared", ErrInvalid)
	}
	if it.Description == "" {
		return fmt.Errorf("%w: description is required", ErrInvalid)
	}
	for _, r := range it.Issue {
		if r < '0' || r > '9' {
			return fmt.Errorf("%w: issue must be a GitHub issue number (digits only)", ErrInvalid)
		}
	}
	return nil
}

// Current returns this machine's current version and its build number (00.00.00 when none).
func Current(ctx context.Context, q querier, project string) (Version, string, error) {
	var v Version
	var build string
	err := q.QueryRowContext(ctx, `SELECT major, minor, revision, build_number FROM kb.release_current WHERE project = $1`,
		project).Scan(&v.Major, &v.Minor, &v.Revision, &build)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, "", nil
	}
	return v, build, err
}

// lockCurrent makes sure the project's current row exists and locks it for the transaction.
func lockCurrent(ctx context.Context, tx *sql.Tx, project string) (Version, error) {
	if _, err := tx.ExecContext(ctx, `INSERT INTO kb.release_current (project, major, minor, revision)
		VALUES ($1, 0, 0, 0) ON CONFLICT (project) DO NOTHING`, project); err != nil {
		return Version{}, err
	}
	var v Version
	err := tx.QueryRowContext(ctx, `SELECT major, minor, revision FROM kb.release_current WHERE project = $1 FOR UPDATE`,
		project).Scan(&v.Major, &v.Minor, &v.Revision)
	return v, err
}

func setCurrent(ctx context.Context, q querier, project string, v Version, buildNumber string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO kb.release_current (project, major, minor, revision, build_number, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (project) DO UPDATE SET major = EXCLUDED.major, minor = EXCLUDED.minor, revision = EXCLUDED.revision,
			build_number = EXCLUDED.build_number, updated_at = NOW()`,
		project, v.Major, v.Minor, v.Revision, buildNumber)
	return err
}

// SetCurrent records the release this machine runs (deploy machine).
func SetCurrent(ctx context.Context, db *sql.DB, project string, v Version, buildNumber string) error {
	return setCurrent(ctx, db, project, v, buildNumber)
}

const releaseColumns = `r.id, r.project, r.major, r.minor, r.revision, r.kind, r.status, r.description,
	COALESCE(r.build_number, ''), COALESCE(to_char(r.release_date, 'YYYY-MM-DD'), ''), r.git_rev, r.git_dirty,
	r.shared_git_rev, r.shared_git_dirty, r.created_by, r.created_at, r.builder_host, r.builder_pid`

func scanRelease(row interface{ Scan(...any) error }) (Release, error) {
	var r Release
	err := row.Scan(&r.ID, &r.Project, &r.V.Major, &r.V.Minor, &r.V.Revision, &r.Kind, &r.Status, &r.Description,
		&r.BuildNumber, &r.ReleaseDate, &r.GitRev, &r.GitDirty, &r.SharedGitRev, &r.SharedGitDirty,
		&r.CreatedBy, &r.CreatedAt, &r.BuilderHost, &r.BuilderPID)
	r.Version = r.V.String()
	r.Items = []Item{}
	return r, err
}

const itemColumns = `i.id, COALESCE(i.release_id, 0), i.item_type, i.component, i.description, i.issue,
	i.pull_request, i.notes, i.created_by, i.created_at`

func scanItem(row interface{ Scan(...any) error }) (Item, error) {
	var it Item
	err := row.Scan(&it.ID, &it.ReleaseID, &it.ItemType, &it.Component, &it.Description, &it.Issue,
		&it.PullRequest, &it.Notes, &it.CreatedBy, &it.CreatedAt)
	return it, err
}

// History returns every release of the project with its items, newest version first.
func History(ctx context.Context, q querier, project string) ([]Release, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+releaseColumns+` FROM kb.releases r WHERE r.project = $1
		ORDER BY r.major DESC, r.minor DESC, r.revision DESC`, project)
	if err != nil {
		return nil, err
	}
	releases := []Release{}
	index := map[int64]int{}
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		index[r.ID] = len(releases)
		releases = append(releases, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = q.QueryContext(ctx, `SELECT `+itemColumns+` FROM kb.release_items i
		WHERE i.project = $1 AND i.release_id IS NOT NULL ORDER BY i.id`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		if i, ok := index[it.ReleaseID]; ok {
			releases[i].Items = append(releases[i].Items, it)
		}
	}
	return releases, rows.Err()
}

// Pending returns the items no build has taken yet, oldest first.
func Pending(ctx context.Context, q querier, project string) ([]Item, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+itemColumns+` FROM kb.release_items i
		WHERE i.project = $1 AND i.release_id IS NULL ORDER BY i.id`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func buildingRelease(ctx context.Context, q querier, project string) (*Release, error) {
	r, err := scanRelease(q.QueryRowContext(ctx, `SELECT `+releaseColumns+` FROM kb.releases r
		WHERE r.project = $1 AND r.status = 'building' ORDER BY r.id LIMIT 1`, project))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Building returns the release a build has reserved, or nil.
func Building(ctx context.Context, q querier, project string) (*Release, error) {
	return buildingRelease(ctx, q, project)
}

// CreateLine creates a new major or minor with a description and makes it current.
func CreateLine(ctx context.Context, db *sql.DB, project, kind, description, by string) (Release, error) {
	if err := validProject(project); err != nil {
		return Release{}, err
	}
	if kind != KindMajor && kind != KindMinor {
		return Release{}, fmt.Errorf("%w: kind must be major or minor", ErrInvalid)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Release{}, err
	}
	defer tx.Rollback()
	cur, err := lockCurrent(ctx, tx, project)
	if err != nil {
		return Release{}, err
	}
	if b, err := buildingRelease(ctx, tx, project); err != nil {
		return Release{}, err
	} else if b != nil {
		return Release{}, fmt.Errorf("%w: build of %s is running; wait until it finishes", ErrConflict, b.Version)
	}
	next, err := cur.NextMajor()
	if kind == KindMinor {
		next, err = cur.NextMinor()
	}
	if err != nil {
		return Release{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	r, err := scanRelease(tx.QueryRowContext(ctx, `INSERT INTO kb.releases AS r
		(project, major, minor, revision, kind, status, description, release_date, created_by)
		VALUES ($1, $2, $3, 0, $4, 'released', $5, CURRENT_DATE, $6) RETURNING `+releaseColumns,
		project, next.Major, next.Minor, kind, strings.TrimSpace(description), by))
	if err != nil {
		return Release{}, err
	}
	if err := setCurrent(ctx, tx, project, next, ""); err != nil {
		return Release{}, err
	}
	return r, tx.Commit()
}

// AddItem adds a pending item.
func AddItem(ctx context.Context, db *sql.DB, project string, it Item, by string) (Item, error) {
	if err := validProject(project); err != nil {
		return Item{}, err
	}
	if err := ValidateItem(&it); err != nil {
		return Item{}, err
	}
	return scanItem(db.QueryRowContext(ctx, `INSERT INTO kb.release_items AS i
		(project, item_type, component, description, issue, pull_request, notes, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+itemColumns,
		project, it.ItemType, it.Component, it.Description, it.Issue, it.PullRequest, it.Notes, by))
}

// itemChangeError explains why an update or delete touched no pending item.
func itemChangeError(ctx context.Context, db *sql.DB, id int64) error {
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM kb.release_items WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: item %d belongs to a build and can no longer change", ErrConflict, id)
	}
	return fmt.Errorf("%w: item %d", ErrNotFound, id)
}

// UpdateItem changes a pending item.
func UpdateItem(ctx context.Context, db *sql.DB, id int64, it Item) (Item, error) {
	if err := ValidateItem(&it); err != nil {
		return Item{}, err
	}
	out, err := scanItem(db.QueryRowContext(ctx, `UPDATE kb.release_items AS i SET item_type = $2, component = $3,
		description = $4, issue = $5, pull_request = $6, notes = $7, updated_at = NOW()
		WHERE i.id = $1 AND i.release_id IS NULL RETURNING `+itemColumns,
		id, it.ItemType, it.Component, it.Description, it.Issue, it.PullRequest, it.Notes))
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, itemChangeError(ctx, db, id)
	}
	return out, err
}

// DeleteItem removes a pending item.
func DeleteItem(ctx context.Context, db *sql.DB, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM kb.release_items WHERE id = $1 AND release_id IS NULL`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return itemChangeError(ctx, db, id)
	}
	return nil
}

func revert(ctx context.Context, tx *sql.Tx, releaseID int64) error {
	if _, err := tx.ExecContext(ctx, `UPDATE kb.release_items SET release_id = NULL, updated_at = NOW() WHERE release_id = $1`,
		releaseID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM kb.releases WHERE id = $1 AND status = 'building'`, releaseID)
	return err
}

// Reserve starts a build's release: the next revision as 'building', with every pending
// item bound to it. A 'building' release left by a dead build script on this host is
// reverted first; one held by a live script is a conflict.
func Reserve(ctx context.Context, db *sql.DB, project, host string, pid int, alive func(int) bool, by string) (Release, error) {
	if err := validProject(project); err != nil {
		return Release{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Release{}, err
	}
	defer tx.Rollback()
	cur, err := lockCurrent(ctx, tx, project)
	if err != nil {
		return Release{}, err
	}
	if b, err := buildingRelease(ctx, tx, project); err != nil {
		return Release{}, err
	} else if b != nil {
		if b.BuilderHost != host || alive(b.BuilderPID) {
			return Release{}, fmt.Errorf("%w: %s is already being built by process %d on %s", ErrConflict, b.Version, b.BuilderPID, b.BuilderHost)
		}
		if err := revert(ctx, tx, b.ID); err != nil {
			return Release{}, err
		}
	}
	next, err := cur.NextRevision()
	if err != nil {
		return Release{}, err
	}
	r, err := scanRelease(tx.QueryRowContext(ctx, `INSERT INTO kb.releases AS r
		(project, major, minor, revision, kind, status, created_by, builder_host, builder_pid)
		VALUES ($1, $2, $3, $4, 'build', 'building', $5, $6, $7) RETURNING `+releaseColumns,
		project, next.Major, next.Minor, next.Revision, by, host, pid))
	if err != nil {
		return Release{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE kb.release_items SET release_id = $1, updated_at = NOW()
		WHERE project = $2 AND release_id IS NULL`, r.ID, project); err != nil {
		return Release{}, err
	}
	return r, tx.Commit()
}

// BuildInfo is what the build records on its release before the notes are written.
type BuildInfo struct {
	BuildNumber    string
	Description    string
	GitRev         string
	GitDirty       bool
	SharedGitRev   string
	SharedGitDirty bool
}

// SetBuildInfo records the build number, description and git revisions on a building release.
func SetBuildInfo(ctx context.Context, db *sql.DB, project string, v Version, b BuildInfo) error {
	res, err := db.ExecContext(ctx, `UPDATE kb.releases SET build_number = NULLIF($5, ''), description = $6,
		git_rev = $7, git_dirty = $8, shared_git_rev = $9, shared_git_dirty = $10
		WHERE project = $1 AND major = $2 AND minor = $3 AND revision = $4 AND status = 'building'`,
		project, v.Major, v.Minor, v.Revision, b.BuildNumber, strings.TrimSpace(b.Description),
		b.GitRev, b.GitDirty, b.SharedGitRev, b.SharedGitDirty)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: no building release %s", ErrNotFound, v)
	}
	return nil
}

// Commit releases a building release and makes it the current version.
func Commit(ctx context.Context, db *sql.DB, project string, v Version) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := lockCurrent(ctx, tx, project); err != nil {
		return err
	}
	var build string
	err = tx.QueryRowContext(ctx, `UPDATE kb.releases SET status = 'released', release_date = CURRENT_DATE,
		builder_host = '', builder_pid = 0
		WHERE project = $1 AND major = $2 AND minor = $3 AND revision = $4 AND status = 'building'
		RETURNING COALESCE(build_number, '')`, project, v.Major, v.Minor, v.Revision).Scan(&build)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: no building release %s", ErrNotFound, v)
	}
	if err != nil {
		return err
	}
	if err := setCurrent(ctx, tx, project, v, build); err != nil {
		return err
	}
	return tx.Commit()
}

// Abort removes a building release and returns its items to pending. It does nothing
// when that release is not building (already aborted, or never reserved).
func Abort(ctx context.Context, db *sql.DB, project string, v Version) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM kb.releases
		WHERE project = $1 AND major = $2 AND minor = $3 AND revision = $4 AND status = 'building' FOR UPDATE`,
		project, v.Major, v.Minor, v.Revision).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := revert(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// VersionForBuild returns the released version built as buildNumber, or "" when unknown.
func VersionForBuild(ctx context.Context, q querier, project, buildNumber string) (string, error) {
	if buildNumber == "" {
		return "", nil
	}
	var v Version
	err := q.QueryRowContext(ctx, `SELECT major, minor, revision FROM kb.releases
		WHERE project = $1 AND build_number = $2 AND status = 'released'`, project, buildNumber).Scan(&v.Major, &v.Minor, &v.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v.String(), nil
}

// ClearCurrent forgets this machine's current release (deploy machine running a build
// from before release versioning).
func ClearCurrent(ctx context.Context, db *sql.DB, project string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM kb.release_current WHERE project = $1`, project)
	return err
}
