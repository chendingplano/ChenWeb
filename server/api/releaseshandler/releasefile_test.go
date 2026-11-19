package releaseshandler

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestBuildFileShowsBuildingReleaseAsReleased(t *testing.T) {
	db, mock := newMock(t)
	rows := sqlmock.NewRows(releaseCols).
		AddRow(3, ProjectChenWeb, 1, 2, 6, KindBuild, StatusBuilding, "", "20261119063000", "", "abc", false, "def", true, "dev", time.Now(), "mac", 1).
		AddRow(2, ProjectChenWeb, 1, 2, 5, KindBuild, StatusReleased, "", "20261118063000", "2026-11-18", "", false, "", false, "dev", time.Now(), "", 0)
	mock.ExpectQuery(`FROM kb.releases r WHERE r.project = \$1`).WillReturnRows(rows)
	mock.ExpectQuery(`FROM kb.release_items i`).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "release_id", "item_type", "component", "description", "issue", "pull_request", "notes", "created_by", "created_at"}).
		AddRow(7, 3, "bug fix", "ChenWeb", "Fix", "142", "", "", "dev", time.Now()))
	f, err := BuildFile(context.Background(), db, ProjectChenWeb, Version{1, 2, 6}, time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != "01.02.06" || f.BuildNumber != "20261119063000" || f.ReleaseDate != "2026-11-19" || !f.SharedGitDirty {
		t.Fatalf("file header = %+v", f)
	}
	if len(f.History) != 2 || f.History[0].Status != StatusReleased || len(f.History[0].Items) != 1 {
		t.Fatalf("history = %+v", f.History)
	}
}

func TestFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release.json")
	in := File{Project: ProjectChenWeb, Version: "01.02.06", History: sampleHistory()}
	if err := WriteFile(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.History) != len(in.History) || out.History[3].V != (Version{1, 2, 6}) || len(out.History[3].Items) != 3 {
		t.Fatalf("round trip lost data: %+v", out.History)
	}
	if _, err := ParseFile([]byte(`{"project":"tax","version":"01.00.00"}`)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tax file: %v", err)
	}
}

func TestImportSkipsExistingReleases(t *testing.T) {
	db, mock := newMock(t)
	f := File{Project: ProjectChenWeb, Version: "01.02.06", History: []Release{
		rel(Version{1, 2, 6}, KindBuild, "", Item{ItemType: "bug fix", Component: "ChenWeb", Description: "Fix"}),
		rel(Version{1, 2, 5}, KindBuild, "", Item{ItemType: "bug fix", Component: "ChenWeb", Description: "Old"}),
	}}
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO kb.releases`).WithArgs(sqlmock.AnyArg(), 1, 2, 6, sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(40))
	mock.ExpectExec(`INSERT INTO kb.release_items`).WithArgs(ProjectChenWeb, int64(40), "bug fix", "ChenWeb", "Fix",
		"", "", "", "", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	// 01.02.05 already exists: ON CONFLICT DO NOTHING returns no row and its items are not touched.
	mock.ExpectQuery(`INSERT INTO kb.releases`).WillReturnError(sql.ErrNoRows)
	mock.ExpectCommit()
	added, err := Import(context.Background(), db, f)
	if err != nil || added != 1 {
		t.Fatalf("Import = %d, %v", added, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
