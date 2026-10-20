package codemonitor

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestCollectProjectSkipsTrackedFileDeletedFromWorkingTree(t *testing.T) {
	root := t.TempDir()
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	gitRun("init", "-q")
	if err := os.Mkdir(filepath.Join(root, "server"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "server", "keep.go"), []byte("package server\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "server", "deleted.go"), []byte("package server\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitRun("add", "server")
	if err := os.Remove(filepath.Join(root, "server", "deleted.go")); err != nil {
		t.Fatal(err)
	}

	snapshot, err := collectProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot != (Snapshot{TotalLines: 1, BackendFiles: 1}) {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestCollectProjectCountsOnlyFrontendAndBackendDirectories(t *testing.T) {
	workspace := t.TempDir()
	projectRoot := filepath.Join(workspace, "ChenWeb")
	files := map[string]string{
		"web/src/app.ts":    "one\ntwo\n",
		"web/src/view.go":   "front\n",
		"server/main.go":    "one\ntwo\nthree\n",
		"server/assets.css": "body {}\n",
		"web/outside.ts":    "ignored\n",
		"docs/readme.md":    "ignored\n",
		"tools/extra.go":    "ignored\n",
	}
	for path, content := range files {
		fullPath := filepath.Join(projectRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = projectRoot
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	gitRun("init", "-q")
	gitRun("add", ".")

	snapshot, err := collectProject(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	want := Snapshot{TotalLines: 7, FrontendFiles: 2, BackendFiles: 2}
	if snapshot != want {
		t.Fatalf("snapshot = %+v, want %+v", snapshot, want)
	}
}

func TestHandlerReturnsProjectDeltasIncludingPreviousOutOfRangeRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()

	mock.ExpectQuery(`(?s)WITH.*LAG\(total_lines\).*WHERE.*collection_date BETWEEN`).
		WillReturnRows(sqlmock.NewRows([]string{"collection_date", "total_lines", "total_lines_delta", "frontend_files", "frontend_files_delta", "backend_files", "backend_files_delta"}).
			AddRow(time.Now().Format("2006-01-02"), 150, 20, 8, 2, 5, -1))
	e := echo.New()
	req := httptest.NewRequest("GET", "/code-monitor?range=today", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	loc, _ := time.LoadLocation("America/Chicago")
	if err := Handler([]string{"server"}, loc)(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Projects []struct {
			ProjectPath string `json:"project_path"`
			Rows        []struct {
				TotalLinesDelta    int `json:"total_lines_delta"`
				FrontendFilesDelta int `json:"frontend_files_delta"`
				BackendFilesDelta  int `json:"backend_files_delta"`
			} `json:"rows"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Projects) != 1 || got.Projects[0].ProjectPath != "server" {
		t.Fatalf("projects = %s", rec.Body.String())
	}
	if len(got.Projects[0].Rows) != 1 || got.Projects[0].Rows[0].TotalLinesDelta != 20 || got.Projects[0].Rows[0].FrontendFilesDelta != 2 || got.Projects[0].Rows[0].BackendFilesDelta != -1 {
		t.Fatalf("deltas = %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInitializeCollectsImmediatelyAtSyntheticLocalTwoAMWhenEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	projectRoot := filepath.Join(root, "ChenWeb")
	if err := os.MkdirAll(filepath.Join(projectRoot, "server"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "server", "code.go"), []byte("package test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitRun := exec.Command("git", "init", "-q")
	gitRun.Dir = projectRoot
	if out, err := gitRun.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	gitRun = exec.Command("git", "add", "server/code.go")
	gitRun.Dir = projectRoot
	if out, err := gitRun.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
	local, _ := time.LoadLocation("America/Chicago")
	now := time.Date(2026, 10, 20, 10, 15, 0, 0, local)
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`INSERT INTO kb\.code_monitor`).
		WithArgs("ChenWeb", "2026-10-20", 1, 0, 1, time.Date(2026, 10, 20, 2, 0, 0, 0, local)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := (Store{DB: db}).Initialize(context.Background(), root, []string{"ChenWeb"}, now, local); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInitializeLeavesExistingHistoryForScheduledCollection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	local, _ := time.LoadLocation("America/Chicago")
	if err := (Store{DB: db}).Initialize(context.Background(), t.TempDir(), []string{"server"}, time.Now(), local); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCountPhysicalLines(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          int
	}{
		{"empty", "", 0}, {"newline terminated", "a\nb\n", 2}, {"no final newline", "a\nb", 2}, {"blank lines", "\n\n", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.go")
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := countLines(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("countLines() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestIsCountedExtension(t *testing.T) {
	for ext, want := range map[string]bool{".svelte": true, ".ts": true, ".go": true, ".sql": true, ".md": false} {
		if got := isCountedExtension(ext); got != want {
			t.Errorf("isCountedExtension(%q) = %v, want %v", ext, got, want)
		}
	}
}

func TestDateRange(t *testing.T) {
	loc, _ := time.LoadLocation("America/Chicago")
	today := time.Date(2026, 3, 31, 12, 0, 0, 0, loc)
	start, end, err := dateRange("last_6_months", today, loc)
	if err != nil {
		t.Fatal(err)
	}
	if start.Format("2006-01-02") != "2025-09-30" || end.Format("2006-01-02") != "2026-03-31" {
		t.Fatalf("range = %s..%s", start, end)
	}
	start, end, err = dateRange("last_month", today, loc)
	if err != nil {
		t.Fatal(err)
	}
	if start.Format("2006-01-02") != "2026-03-02" || end.Format("2006-01-02") != "2026-03-31" {
		t.Fatalf("30-day range = %s..%s", start, end)
	}
}

func TestNextRunDST(t *testing.T) {
	loc, _ := time.LoadLocation("America/Chicago")
	now := time.Date(2026, 3, 7, 23, 0, 0, 0, loc)
	next := nextRun(now, loc)
	if next.Hour() != 3 || next.Minute() != 0 || next.Day() != 8 {
		t.Fatalf("next run = %s", next)
	}
	if next.Sub(now) != 3*time.Hour {
		t.Fatalf("next run duration = %s", next.Sub(now))
	}
}
