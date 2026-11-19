package deployment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

type commandLog struct{ calls []string }

// setup installs a user, a role, a mock database and a command recorder.
func setup(t *testing.T, user *ApiTypes.UserInfo, machineRole string) (sqlmock.Sqlmock, *commandLog) {
	t.Helper()
	oldAuth, oldRole, oldDB, oldRun, oldAlive, oldDelay := EchoFactory.DefaultAuthenticator, role, ApiTypes.ProjectDBHandle, runCommand, pidAlive, restartDelay
	t.Cleanup(func() {
		EchoFactory.DefaultAuthenticator, role, ApiTypes.ProjectDBHandle, runCommand, pidAlive, restartDelay = oldAuth, oldRole, oldDB, oldRun, oldAlive, oldDelay
	})
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) { return user, nil }
	role = func() string { return machineRole }
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ApiTypes.ProjectDBHandle = db
	log := &commandLog{}
	runCommand = func(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
		log.calls = append(log.calls, strings.Join(append([]string{name}, args...), " "))
		return []byte("active\n"), nil
	}
	pidAlive = func(int) bool { return true }
	restartDelay = time.Hour // never restart chenweb in a test
	return mock, log
}

func admin() *ApiTypes.UserInfo { return &ApiTypes.UserInfo{Admin: true, Email: "admin@example.com"} }

func call(t *testing.T, handler echo.HandlerFunc, method, target, body string, params ...string) (int, map[string]any) {
	t.Helper()
	e := echo.New()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if len(params) == 2 {
		c.SetParamNames(params[0])
		c.SetParamValues(params[1])
	}
	_ = handler(c)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func expectNoActiveDeployment(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT .* FROM kb.deployments\s+WHERE status IN`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

func expectRunningDeployment(mock sqlmock.Sqlmock) {
	now := time.Now()
	mock.ExpectQuery(`SELECT .* FROM kb.deployments\s+WHERE status IN`).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "archive_name", "build_number", "previous_build_number", "status", "error_message",
			"requested_by", "runner_pid", "requested_at", "started_at", "finished_at", "release_version", "previous_release_version"}).
		AddRow(9, "runshen-deploy-20261116093000.tar.gz", "20261116093000", "", "running", "", "x", 100, now, now, nil, "", ""))
}

func TestNonAdminIsRejected(t *testing.T) {
	setup(t, &ApiTypes.UserInfo{Email: "user@example.com"}, RoleDeploy)
	if code, _ := call(t, Status, http.MethodGet, "/status", ""); code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", code)
	}
	setup(t, nil, RoleDeploy)
	if code, _ := call(t, RestartProcess, http.MethodPost, "/processes/doc-processor/restart", "", "id", "doc-processor"); code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
}

func TestStatusWithoutRole(t *testing.T) {
	setup(t, admin(), "")
	code, body := call(t, Status, http.MethodGet, "/status", "")
	if code != http.StatusOK || body["role"] != "" || body["active"] != nil {
		t.Fatalf("status = %d body = %v", code, body)
	}
}

func TestActionsRefusedForWrongRole(t *testing.T) {
	_, log := setup(t, admin(), RoleDeploy)
	if code, body := call(t, StartBuild, http.MethodPost, "/builds", `{}`); code != http.StatusBadRequest || body["code"] != codeWrongRole {
		t.Fatalf("StartBuild on deploy machine: %d %v", code, body)
	}
	setup(t, admin(), RoleBuild)
	if code, body := call(t, StartDeployment, http.MethodPost, "/deployments", `{"archive_name":"runshen-deploy-20261116093000.tar.gz"}`); code != http.StatusBadRequest || body["code"] != codeWrongRole {
		t.Fatalf("StartDeployment on build machine: %d %v", code, body)
	}
	if code, body := call(t, RestartProcess, http.MethodPost, "/processes/doc-processor/restart", "", "id", "doc-processor"); code != http.StatusBadRequest || body["code"] != codeWrongRole {
		t.Fatalf("restart on build machine: %d %v", code, body)
	}
	if len(log.calls) != 0 {
		t.Fatalf("commands ran: %v", log.calls)
	}
}

func TestStartDeploymentRejectsUnsafeArchiveName(t *testing.T) {
	_, log := setup(t, admin(), RoleDeploy)
	code, body := call(t, StartDeployment, http.MethodPost, "/deployments", `{"archive_name":"runshen-x.tar.gz;id"}`)
	if code != http.StatusBadRequest || body["code"] != codeInvalidArchive || len(log.calls) != 0 {
		t.Fatalf("%d %v %v", code, body, log.calls)
	}
}

func TestStartDeploymentConflictsWithActiveDeployment(t *testing.T) {
	mock, log := setup(t, admin(), RoleDeploy)
	expectRunningDeployment(mock)
	code, body := call(t, StartDeployment, http.MethodPost, "/deployments", `{"archive_name":"runshen-deploy-20261116093000.tar.gz"}`)
	if code != http.StatusConflict || body["code"] != codeActiveRun || len(log.calls) != 0 {
		t.Fatalf("%d %v %v", code, body, log.calls)
	}
}

func TestRestartAllowlist(t *testing.T) {
	_, log := setup(t, admin(), RoleDeploy)
	for _, id := range []string{"sshd", "caddy", "chenweb;reboot"} {
		code, body := call(t, RestartProcess, http.MethodPost, "/processes/x/restart", "", "id", id)
		if code != http.StatusBadRequest || body["code"] != codeUnknownProcess {
			t.Errorf("restart %q: %d %v", id, code, body)
		}
	}
	if len(log.calls) != 0 {
		t.Fatalf("commands ran: %v", log.calls)
	}
}

func TestRestartRefusedDuringDeployment(t *testing.T) {
	mock, log := setup(t, admin(), RoleDeploy)
	expectRunningDeployment(mock)
	code, body := call(t, RestartProcess, http.MethodPost, "/processes/doc-processor/restart", "", "id", "doc-processor")
	if code != http.StatusConflict || body["code"] != codeActiveRun || len(log.calls) != 0 {
		t.Fatalf("%d %v %v", code, body, log.calls)
	}
}

func TestRestartRunsSystemctlThroughSudo(t *testing.T) {
	mock, log := setup(t, admin(), RoleDeploy)
	expectNoActiveDeployment(mock)
	code, _ := call(t, RestartProcess, http.MethodPost, "/processes/nats/restart", "", "id", "nats")
	if code != http.StatusOK || len(log.calls) != 1 || log.calls[0] != "sudo -n /bin/systemctl restart nats-server" {
		t.Fatalf("%d %v", code, log.calls)
	}
}

func TestRestartChenwebAnswersBeforeRestarting(t *testing.T) {
	mock, log := setup(t, admin(), RoleDeploy)
	expectNoActiveDeployment(mock)
	code, body := call(t, RestartProcess, http.MethodPost, "/processes/chenweb/restart", "", "id", "chenweb")
	if code != http.StatusAccepted || body["self"] != true || len(log.calls) != 0 {
		t.Fatalf("%d %v %v", code, body, log.calls)
	}
}
func TestParseArchiveList(t *testing.T) {
	archives, err := parseArchiveList("Warning: Permanently added host\n" +
		"runshen-deploy-20261119063000.tar.gz 2234 1763533800\nrunshen-deploy-20261116093000.tar.gz 1234 1763285400\n")
	if err != nil || len(archives) != 2 || archives[0].BuildNumber != "20261119063000" || archives[1].Size != 1234 ||
		archives[1].Modified.Unix() != 1763285400 {
		t.Fatalf("parseArchiveList = %+v %v", archives, err)
	}
	if _, err := parseArchiveList("No runshen archive found"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestArchivesListsReleasesFromTheNewestReleaseFile(t *testing.T) {
	mock, log := setup(t, admin(), RoleDeploy)
	runCommand = func(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
		log.calls = append(log.calls, strings.Join(append([]string{name}, args...), " "))
		switch args[1] {
		case "--list":
			return []byte("runshen-deploy-20261119063000.tar.gz 2234 1763533800\nrunshen-deploy-20261116093000.tar.gz 1234 1763285400\n"), nil
		case "--release-file":
			return []byte(`{"project":"ChenWeb","version":"01.00.01","build_number":"20261119063000","history":[` +
				`{"project":"ChenWeb","version":"01.00.01","kind":"build","status":"released","build_number":"20261119063000",` +
				`"release_date":"2026-11-19","items":[{"item_type":"bug fix","component":"ChenWeb","description":"Fix"}]}]}`), nil
		}
		return nil, nil
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO kb.releases`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO kb.release_items`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	now := time.Now()
	mock.ExpectQuery(`FROM kb.releases r WHERE r.project`).WillReturnRows(sqlmock.NewRows([]string{"id", "project", "major", "minor",
		"revision", "kind", "status", "description", "build_number", "release_date", "git_rev", "git_dirty", "shared_git_rev",
		"shared_git_dirty", "created_by", "created_at", "builder_host", "builder_pid"}).
		AddRow(1, "ChenWeb", 1, 0, 1, "build", "released", "", "20261119063000", "2026-11-19", "", false, "", false, "", now, "", 0))
	mock.ExpectQuery(`FROM kb.release_items i`).WillReturnRows(sqlmock.NewRows([]string{"id", "release_id", "item_type",
		"component", "description", "issue", "pull_request", "notes", "created_by", "created_at"}).
		AddRow(5, 1, "bug fix", "ChenWeb", "Fix", "", "", "", "", now))
	mock.ExpectQuery(`FROM kb.release_current`).WillReturnRows(sqlmock.NewRows([]string{"major", "minor", "revision", "build_number"}))
	code, out := call(t, Archives, http.MethodGet, "/archives", "")
	if code != http.StatusOK {
		t.Fatalf("status %d %v", code, out)
	}
	archives := out["archives"].([]any)
	first, second := archives[0].(map[string]any), archives[1].(map[string]any)
	if first["release_version"] != "01.00.01" || first["item_count"] != float64(1) || second["release_version"] != "" {
		t.Fatalf("archives = %v", archives)
	}
	if out["running_release"] != "" {
		t.Fatalf("running_release = %v", out["running_release"])
	}
	if len(log.calls) != 2 || !strings.Contains(log.calls[1], "--release-file runshen-deploy-20261119063000.release.json") {
		t.Fatalf("calls = %v", log.calls)
	}
}

func TestUnitState(t *testing.T) {
	setup(t, admin(), RoleDeploy)
	runCommand = func(context.Context, string, string, ...string) ([]byte, error) {
		return []byte("ActiveState=active\nSubState=running\nMainPID=4321\nActiveEnterTimestampMonotonic=1000000000\n"), nil
	}
	oldBoot := readBootUptime
	t.Cleanup(func() { readBootUptime = oldBoot })
	readBootUptime = func() (float64, error) { return 4600, nil }
	p := Process{Unit: "doc-processor"}
	unitState(context.Background(), &p)
	if p.ActiveState != "active" || p.SubState != "running" || p.PID != 4321 || p.UptimeSeconds != 3600 {
		t.Fatalf("unitState = %+v", p)
	}
}
