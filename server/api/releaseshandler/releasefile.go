package releaseshandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// File is release.json: one release and the whole history up to and including it. The
// build writes it into the archive and next to the archive on the relay; the deploy
// machine imports it.
type File struct {
	Project        string    `json:"project"`
	Version        string    `json:"version"`
	BuildNumber    string    `json:"build_number"`
	ReleaseDate    string    `json:"release_date"`
	GitRev         string    `json:"git_rev"`
	GitDirty       bool      `json:"git_dirty"`
	SharedGitRev   string    `json:"shared_git_rev"`
	SharedGitDirty bool      `json:"shared_git_dirty"`
	History        []Release `json:"history"`
}

// BuildFile makes the release file for the building release v. In the file that release
// is shown as released today, because the file ships only if the build commits it.
func BuildFile(ctx context.Context, q querier, project string, v Version, today time.Time) (File, error) {
	all, err := History(ctx, q, project)
	if err != nil {
		return File{}, err
	}
	f := File{Project: project, Version: v.String(), History: []Release{}}
	found := false
	for _, r := range all {
		if r.V.Compare(v) > 0 {
			continue
		}
		if r.V == v {
			if r.Status != StatusBuilding {
				return File{}, fmt.Errorf("%w: %s is not building", ErrConflict, v)
			}
			found = true
			r.Status = StatusReleased
			r.ReleaseDate = today.Format("2006-01-02")
			f.BuildNumber, f.ReleaseDate = r.BuildNumber, r.ReleaseDate
			f.GitRev, f.GitDirty, f.SharedGitRev, f.SharedGitDirty = r.GitRev, r.GitDirty, r.SharedGitRev, r.SharedGitDirty
		} else if r.Status != StatusReleased {
			continue
		}
		f.History = append(f.History, r)
	}
	if !found {
		return File{}, fmt.Errorf("%w: no building release %s", ErrNotFound, v)
	}
	return f, nil
}

// WriteFile saves f as JSON.
func WriteFile(path string, f File) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ReadFile loads and checks a release file.
func ReadFile(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	return ParseFile(data)
}

// ParseFile decodes a release file and checks its project and versions.
func ParseFile(data []byte) (File, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("%w: release file: %v", ErrInvalid, err)
	}
	if err := validProject(f.Project); err != nil {
		return File{}, err
	}
	if _, err := ParseVersion(f.Version); err != nil {
		return File{}, err
	}
	for i := range f.History {
		v, err := ParseVersion(f.History[i].Version)
		if err != nil {
			return File{}, err
		}
		f.History[i].V = v
	}
	return f, nil
}

// Import adds the file's releases this database does not have, with their items. It
// never changes or deletes a release that exists, so imports can repeat in any order.
// It returns how many releases were added.
func Import(ctx context.Context, db *sql.DB, f File) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	added := 0
	for _, r := range f.History {
		if r.Status != StatusReleased || r.Project != f.Project {
			continue
		}
		var date any
		if r.ReleaseDate != "" {
			date = r.ReleaseDate
		}
		var id int64
		err := tx.QueryRowContext(ctx, `INSERT INTO kb.releases
			(project, major, minor, revision, kind, status, description, build_number, release_date,
			 git_rev, git_dirty, shared_git_rev, shared_git_dirty, created_by, created_at)
			VALUES ($1, $2, $3, $4, $5, 'released', $6, NULLIF($7, ''), $8::date, $9, $10, $11, $12, $13, $14)
			ON CONFLICT DO NOTHING RETURNING id`,
			r.Project, r.V.Major, r.V.Minor, r.V.Revision, r.Kind, r.Description, r.BuildNumber, date,
			r.GitRev, r.GitDirty, r.SharedGitRev, r.SharedGitDirty, r.CreatedBy, r.CreatedAt).Scan(&id)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("import %s: %w", r.Version, err)
		}
		added++
		for _, it := range r.Items {
			if _, err := tx.ExecContext(ctx, `INSERT INTO kb.release_items
				(project, release_id, item_type, component, description, issue, pull_request, notes, created_by, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
				r.Project, id, it.ItemType, it.Component, it.Description, it.Issue, it.PullRequest, it.Notes,
				it.CreatedBy, it.CreatedAt); err != nil {
				return 0, fmt.Errorf("import %s item: %w", r.Version, err)
			}
		}
	}
	return added, tx.Commit()
}
