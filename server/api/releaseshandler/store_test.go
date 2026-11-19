package releaseshandler

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func newMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

var releaseCols = []string{"id", "project", "major", "minor", "revision", "kind", "status", "description",
	"build_number", "release_date", "git_rev", "git_dirty", "shared_git_rev", "shared_git_dirty",
	"created_by", "created_at", "builder_host", "builder_pid"}

func releaseRow(id int64, v Version, kind, status, host string, pid int) *sqlmock.Rows {
	return sqlmock.NewRows(releaseCols).AddRow(id, ProjectChenWeb, v.Major, v.Minor, v.Revision, kind, status, "",
		"", "", "", false, "", false, "", time.Now(), host, pid)
}

func expectLockCurrent(mock sqlmock.Sqlmock, v Version) {
	mock.ExpectExec(`INSERT INTO kb.release_current`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT major, minor, revision FROM kb.release_current WHERE project = \$1 FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"major", "minor", "revision"}).AddRow(v.Major, v.Minor, v.Revision))
}

func expectNoBuilding(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`FROM kb.releases r\s+WHERE r.project = \$1 AND r.status = 'building'`).
		WillReturnRows(sqlmock.NewRows(releaseCols))
}

func TestReserveBindsPendingItems(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	expectLockCurrent(mock, Version{1, 2, 5})
	expectNoBuilding(mock)
	mock.ExpectQuery(`INSERT INTO kb.releases AS r`).
		WithArgs(ProjectChenWeb, 1, 2, 6, "dev", "mac", 4242).
		WillReturnRows(releaseRow(10, Version{1, 2, 6}, KindBuild, StatusBuilding, "mac", 4242))
	mock.ExpectExec(`UPDATE kb.release_items SET release_id = \$1, updated_at = NOW\(\)\s+WHERE project = \$2 AND release_id IS NULL`).
		WithArgs(int64(10), ProjectChenWeb).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()
	r, err := Reserve(context.Background(), db, ProjectChenWeb, "mac", 4242, func(int) bool { return true }, "dev")
	if err != nil || r.Version != "01.02.06" {
		t.Fatalf("Reserve = %v, %v", r.Version, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReserveRevertsKilledBuild(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	expectLockCurrent(mock, Version{1, 2, 5})
	mock.ExpectQuery(`r.status = 'building'`).WillReturnRows(releaseRow(9, Version{1, 2, 6}, KindBuild, StatusBuilding, "mac", 111))
	mock.ExpectExec(`UPDATE kb.release_items SET release_id = NULL`).WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`DELETE FROM kb.releases WHERE id = \$1 AND status = 'building'`).WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO kb.releases AS r`).WithArgs(ProjectChenWeb, 1, 2, 6, "dev", "mac", 222).
		WillReturnRows(releaseRow(11, Version{1, 2, 6}, KindBuild, StatusBuilding, "mac", 222))
	mock.ExpectExec(`UPDATE kb.release_items SET release_id = \$1`).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	r, err := Reserve(context.Background(), db, ProjectChenWeb, "mac", 222, func(pid int) bool { return pid != 111 }, "dev")
	if err != nil || r.Version != "01.02.06" {
		t.Fatalf("Reserve = %v, %v", r.Version, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReserveRefusesLiveBuild(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	expectLockCurrent(mock, Version{1, 2, 5})
	mock.ExpectQuery(`r.status = 'building'`).WillReturnRows(releaseRow(9, Version{1, 2, 6}, KindBuild, StatusBuilding, "mac", 111))
	mock.ExpectRollback()
	_, err := Reserve(context.Background(), db, ProjectChenWeb, "mac", 222, func(int) bool { return true }, "dev")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestReserveRefusesAtRevisionLimit(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	expectLockCurrent(mock, Version{1, 2, 99})
	expectNoBuilding(mock)
	mock.ExpectRollback()
	_, err := Reserve(context.Background(), db, ProjectChenWeb, "mac", 1, func(int) bool { return true }, "dev")
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("err = %v, want ErrLimit", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateMinorMakesItCurrent(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	expectLockCurrent(mock, Version{1, 2, 5})
	expectNoBuilding(mock)
	mock.ExpectQuery(`INSERT INTO kb.releases AS r`).WithArgs(ProjectChenWeb, 1, 3, KindMinor, "Release management", "admin").
		WillReturnRows(releaseRow(12, Version{1, 3, 0}, KindMinor, StatusReleased, "", 0))
	mock.ExpectExec(`INSERT INTO kb.release_current .* ON CONFLICT \(project\) DO UPDATE`).
		WithArgs(ProjectChenWeb, 1, 3, 0, "").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	r, err := CreateLine(context.Background(), db, ProjectChenWeb, KindMinor, " Release management ", "admin")
	if err != nil || r.Version != "01.03.00" {
		t.Fatalf("CreateLine = %v, %v", r.Version, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateLineRefusedDuringBuild(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	expectLockCurrent(mock, Version{1, 2, 5})
	mock.ExpectQuery(`r.status = 'building'`).WillReturnRows(releaseRow(9, Version{1, 2, 6}, KindBuild, StatusBuilding, "mac", 1))
	mock.ExpectRollback()
	if _, err := CreateLine(context.Background(), db, ProjectChenWeb, KindMajor, "", "admin"); !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestCreateLineUnknownProject(t *testing.T) {
	db, _ := newMock(t)
	if _, err := CreateLine(context.Background(), db, "tax", KindMajor, "", "admin"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestUpdateReleasedItemConflicts(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`UPDATE kb.release_items AS i SET`).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT EXISTS`).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	_, err := UpdateItem(context.Background(), db, 5, Item{ItemType: "bug fix", Component: "ChenWeb", Description: "x"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestDeleteMissingItemNotFound(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectExec(`DELETE FROM kb.release_items`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	if err := DeleteItem(context.Background(), db, 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCommitSetsCurrent(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	expectLockCurrent(mock, Version{1, 2, 5})
	mock.ExpectQuery(`UPDATE kb.releases SET status = 'released'`).WithArgs(ProjectChenWeb, 1, 2, 6).
		WillReturnRows(sqlmock.NewRows([]string{"build_number"}).AddRow("20261119063000"))
	mock.ExpectExec(`INSERT INTO kb.release_current`).WithArgs(ProjectChenWeb, 1, 2, 6, "20261119063000").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := Commit(context.Background(), db, ProjectChenWeb, Version{1, 2, 6}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAbortReturnsItemsToPending(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM kb.releases`).WithArgs(ProjectChenWeb, 1, 2, 6).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10))
	mock.ExpectExec(`UPDATE kb.release_items SET release_id = NULL`).WithArgs(int64(10)).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`DELETE FROM kb.releases`).WithArgs(int64(10)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := Abort(context.Background(), db, ProjectChenWeb, Version{1, 2, 6}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAbortWithoutBuildingIsNoop(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM kb.releases`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	if err := Abort(context.Background(), db, ProjectChenWeb, Version{1, 2, 6}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateItem(t *testing.T) {
	ok := Item{ItemType: "bug fix", Component: "shared", Description: "  fix  ", Issue: "#26"}
	if err := ValidateItem(&ok); err != nil || ok.Issue != "26" || ok.Description != "fix" {
		t.Fatalf("ValidateItem = %+v, %v", ok, err)
	}
	for name, it := range map[string]Item{
		"type":        {ItemType: "chore", Component: "ChenWeb", Description: "x"},
		"component":   {ItemType: "bug fix", Component: "tax", Description: "x"},
		"description": {ItemType: "bug fix", Component: "ChenWeb", Description: " "},
		"issue":       {ItemType: "bug fix", Component: "ChenWeb", Description: "x", Issue: "ChenWeb-142"},
	} {
		if err := ValidateItem(&it); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}
