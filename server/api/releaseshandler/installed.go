package releaseshandler

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"path/filepath"
)

// RoleDeploy is the [deployment] role of the machine that runs the deployed build.
const RoleDeploy = "deploy"

// InstalledFile is where the deploy script installs the archive's release.json.
func InstalledFile(chenwebDir string) string {
	return filepath.Join(chenwebDir, "release", "release.json")
}

// ImportInstalled runs at server start on the deploy machine: it imports the installed
// release.json and records its release as the one this machine runs. It returns the
// running version, or "" when the machine is not the deploy machine or no file is
// installed (a build from before release versioning, which has no current release).
func ImportInstalled(ctx context.Context, db *sql.DB, chenwebDir, machineRole string) (string, int, error) {
	if machineRole != RoleDeploy {
		return "", 0, nil
	}
	f, err := ReadFile(InstalledFile(chenwebDir))
	if errors.Is(err, fs.ErrNotExist) {
		return "", 0, ClearCurrent(ctx, db, ProjectChenWeb)
	}
	if err != nil {
		return "", 0, err
	}
	added, err := Import(ctx, db, f)
	if err != nil {
		return "", 0, err
	}
	v, _ := ParseVersion(f.Version)
	if err := SetCurrent(ctx, db, f.Project, v, f.BuildNumber); err != nil {
		return "", added, err
	}
	return f.Version, added, nil
}
