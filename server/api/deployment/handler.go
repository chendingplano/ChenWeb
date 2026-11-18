package deployment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/chendingplano/deepdoc/server/buildinfo"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

// Error codes the page translates.
const (
	codeWrongRole      = "wrong_role"
	codeActiveRun      = "active_run"
	codeInvalidArchive = "invalid_archive"
	codeUnknownProcess = "unknown_process"
	codeRelayFailed    = "relay_failed"
	codeStartFailed    = "start_failed"
	codeRestartFailed  = "restart_failed"
	codeNotFound       = "not_found"
	codeInternal       = "internal"
)

const recentRunLimit = 20

var errAdminCheckFailed = errors.New("admin check failed")

// role is this machine's [deployment] role; a variable so tests can set it.
var role = func() string { return strings.TrimSpace(appconfig.AppConfig.Deployment.Role) }

// chenwebDir is the ChenWeb root, where the scripts and config.toml live.
var chenwebDir = func() string {
	if dir := appconfig.GetConfigDir(); dir != "" {
		return dir
	}
	dir, _ := os.Getwd()
	return dir
}

func hostname() string { h, _ := os.Hostname(); return h }

func kindForRole(r string) string {
	switch r {
	case RoleBuild:
		return KindBuild
	case RoleDeploy:
		return KindDeploy
	}
	return ""
}

func fail(c echo.Context, status int, code, detail string) error {
	return c.JSON(status, map[string]any{"status": false, "code": code, "error_msg": detail})
}

func requireAdmin(c echo.Context, loc string) (ApiTypes.RequestContext, *ApiTypes.UserInfo, error) {
	rc := EchoFactory.NewFromEcho(c, loc)
	user := rc.IsAuthenticated()
	if user == nil {
		_ = fail(c, http.StatusUnauthorized, "unauthorized", "authentication required ("+loc+")")
		return rc, nil, errAdminCheckFailed
	}
	admin := user.Admin || user.IsOwner
	for _, r := range user.Roles {
		if r = strings.ToLower(strings.TrimSpace(r)); r == "admin" || r == "root" {
			admin = true
		}
	}
	if !admin {
		_ = fail(c, http.StatusForbidden, "forbidden", "admin access required ("+loc+")")
		return rc, nil, errAdminCheckFailed
	}
	return rc, user, nil
}

func requester(u *ApiTypes.UserInfo) string {
	if u.Email != "" {
		return u.Email
	}
	return u.UserName
}

// activeRun returns this machine's active run after failing it if its runner is gone.
func activeRun(ctx context.Context, kind string) (*Run, error) {
	return ReconcileActive(ctx, ApiTypes.ProjectDBHandle, kind, hostname(), DeployUnitActive)
}

// Status handles GET /api/v1/system-admin/deployment/status.
func Status(c echo.Context) error {
	rc, _, err := requireAdmin(c, "20261116-741")
	defer rc.Close()
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	r := role()
	kind := kindForRole(r)
	resp := map[string]any{"status": true, "role": r, "kind": kind, "active": nil, "recent": []Run{}}
	if kind == "" {
		return c.JSON(http.StatusOK, resp)
	}
	active, err := activeRun(ctx, kind)
	if err != nil {
		rc.GetLogger().Error("deployment status: active run failed", "kind", kind, "error", err)
		return fail(c, http.StatusInternalServerError, codeInternal, err.Error())
	}
	if active != nil {
		if active, err = GetRun(ctx, ApiTypes.ProjectDBHandle, kind, active.ID); err != nil {
			return fail(c, http.StatusInternalServerError, codeInternal, err.Error())
		}
		resp["active"] = active
	}
	recent, err := RecentRuns(ctx, ApiTypes.ProjectDBHandle, kind, recentRunLimit)
	if err != nil {
		rc.GetLogger().Error("deployment status: recent runs failed", "kind", kind, "error", err)
		return fail(c, http.StatusInternalServerError, codeInternal, err.Error())
	}
	resp["recent"] = recent
	return c.JSON(http.StatusOK, resp)
}

func getRun(c echo.Context, kind, loc string) error {
	rc, _, err := requireAdmin(c, loc)
	defer rc.Close()
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return fail(c, http.StatusBadRequest, codeNotFound, "invalid id")
	}
	run, err := GetRun(c.Request().Context(), ApiTypes.ProjectDBHandle, kind, id)
	if err != nil {
		rc.GetLogger().Error("deployment: get run failed", "kind", kind, "id", id, "error", err)
		return fail(c, http.StatusInternalServerError, codeInternal, err.Error())
	}
	if run == nil {
		return fail(c, http.StatusNotFound, codeNotFound, "run not found")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "run": run})
}

// GetBuild handles GET .../deployment/builds/:id.
func GetBuild(c echo.Context) error { return getRun(c, KindBuild, "20261116-742") }

// GetDeployment handles GET .../deployment/deployments/:id.
func GetDeployment(c echo.Context) error { return getRun(c, KindDeploy, "20261116-743") }

// startBuildRunner starts deploy-runner in its own session, so air restarting chenweb
// (the build rewrites server/api/webbuild) does not stop it. Tests replace it.
var startBuildRunner = func(dir string, id int64) (int, error) {
	logPath := filepath.Join(dir, ".cache", fmt.Sprintf("deploy-runner-build-%d.log", id))
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return 0, err
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return 0, err
	}
	cmd := exec.Command("go", "run", "./server/cmd/deploy-runner", KindBuild, "--build-id", strconv.FormatInt(id, 10))
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, err
	}
	go func() { _ = cmd.Wait(); logFile.Close() }()
	return cmd.Process.Pid, nil
}

// StartBuild handles POST .../deployment/builds {description}.
func StartBuild(c echo.Context) error {
	rc, user, err := requireAdmin(c, "20261116-744")
	defer rc.Close()
	if err != nil {
		return err
	}
	logger := rc.GetLogger()
	if role() != RoleBuild {
		return fail(c, http.StatusBadRequest, codeWrongRole, "builds run only where [deployment] role = \"build\"")
	}
	var body struct {
		Description string `json:"description"`
	}
	_ = c.Bind(&body)
	ctx := c.Request().Context()
	if active, err := activeRun(ctx, KindBuild); err != nil {
		return fail(c, http.StatusInternalServerError, codeInternal, err.Error())
	} else if active != nil {
		return fail(c, http.StatusConflict, codeActiveRun, "a build is already running")
	}
	run := Run{
		BuildName: "chenweb", TargetMachine: "unknown",
		OSInfo: "linux/amd64", Description: strings.TrimSpace(body.Description), RequestedBy: requester(user), RunnerHost: hostname(),
	}
	id, err := CreateBuild(ctx, ApiTypes.ProjectDBHandle, run)
	if errors.Is(err, ErrActiveRun) {
		return fail(c, http.StatusConflict, codeActiveRun, "a build is already running")
	}
	if err != nil {
		logger.Error("deployment: create build failed", "error", err)
		return fail(c, http.StatusInternalServerError, codeInternal, err.Error())
	}
	pid, err := startBuildRunner(chenwebDir(), id)
	if err != nil {
		logger.Error("deployment: start build runner failed", "build_id", id, "error", err)
		_ = FailRun(context.Background(), ApiTypes.ProjectDBHandle, KindBuild, id, "start runner: "+err.Error())
		return fail(c, http.StatusInternalServerError, codeStartFailed, err.Error())
	}
	if err := SetBuildRunner(ctx, ApiTypes.ProjectDBHandle, id, pid); err != nil {
		logger.Error("deployment: record runner pid failed", "build_id", id, "error", err)
	}
	logger.Info("deployment: build started", "build_id", id, "build_number", run.BuildNumber, "pid", pid, "user", run.RequestedBy)
	return c.JSON(http.StatusAccepted, map[string]any{"status": true, "id": id, "build_number": run.BuildNumber})
}

// NextArchive handles GET .../deployment/next-archive: the newest archive on the relay.
func NextArchive(c echo.Context) error {
	rc, _, err := requireAdmin(c, "20261116-745")
	defer rc.Close()
	if err != nil {
		return err
	}
	if role() != RoleDeploy {
		return fail(c, http.StatusBadRequest, codeWrongRole, "deployments run only where [deployment] role = \"deploy\"")
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 45*time.Second)
	defer cancel()
	out, err := runCommand(ctx, chenwebDir(), "bash", "./shell_server_deploy.sh", "--newest")
	if err != nil {
		rc.GetLogger().Error("deployment: list relay archives failed", "error", err, "output", string(out))
		return fail(c, http.StatusBadGateway, codeRelayFailed, strings.TrimSpace(string(out)+"\n"+err.Error()))
	}
	name, size, modified, err := parseNewest(string(out))
	if err != nil {
		return fail(c, http.StatusBadGateway, codeRelayFailed, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "archive_name": name, "size": size,
		"modified": modified, "build_number": ArchiveBuildNumber(name)})
}

// parseNewest reads the last "<name> <size> <mtime>" line of shell_server_deploy.sh --newest.
func parseNewest(out string) (name string, size int64, modified time.Time, err error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) != 3 || !ValidArchiveName(fields[0]) {
		return "", 0, time.Time{}, fmt.Errorf("unexpected relay listing: %q", out)
	}
	size, err1 := strconv.ParseInt(fields[1], 10, 64)
	mtime, err2 := strconv.ParseInt(fields[2], 10, 64)
	if err1 != nil || err2 != nil {
		return "", 0, time.Time{}, fmt.Errorf("unexpected relay listing: %q", out)
	}
	return fields[0], size, time.Unix(mtime, 0).UTC(), nil
}

// StartDeployment handles POST .../deployment/deployments {archive_name}.
func StartDeployment(c echo.Context) error {
	rc, user, err := requireAdmin(c, "20261116-746")
	defer rc.Close()
	if err != nil {
		return err
	}
	logger := rc.GetLogger()
	if role() != RoleDeploy {
		return fail(c, http.StatusBadRequest, codeWrongRole, "deployments run only where [deployment] role = \"deploy\"")
	}
	var body struct {
		ArchiveName string `json:"archive_name"`
	}
	_ = c.Bind(&body)
	if !ValidArchiveName(body.ArchiveName) {
		return fail(c, http.StatusBadRequest, codeInvalidArchive, "invalid archive name")
	}
	ctx := c.Request().Context()
	if active, err := activeRun(ctx, KindDeploy); err != nil {
		return fail(c, http.StatusInternalServerError, codeInternal, err.Error())
	} else if active != nil {
		return fail(c, http.StatusConflict, codeActiveRun, "a deployment is already queued or running")
	}
	previous := ""
	if self, err := buildinfo.CurrentProcess("server"); err == nil {
		previous = self.BuildNumber
	}
	run := Run{ArchiveName: body.ArchiveName, BuildNumber: ArchiveBuildNumber(body.ArchiveName),
		PreviousBuildNumber: previous, RequestedBy: requester(user)}
	id, err := CreateDeployment(ctx, ApiTypes.ProjectDBHandle, run)
	if errors.Is(err, ErrActiveRun) {
		return fail(c, http.StatusConflict, codeActiveRun, "a deployment is already queued or running")
	}
	if err != nil {
		logger.Error("deployment: create deployment failed", "error", err)
		return fail(c, http.StatusInternalServerError, codeInternal, err.Error())
	}
	startCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if out, err := StartDeployUnit(startCtx); err != nil {
		msg := strings.TrimSpace("start " + DeployUnit + ": " + err.Error() + "\n" + string(out))
		logger.Error("deployment: start deploy unit failed", "deployment_id", id, "error", err, "output", string(out))
		_ = FailRun(context.Background(), ApiTypes.ProjectDBHandle, KindDeploy, id, msg)
		return fail(c, http.StatusInternalServerError, codeStartFailed, msg)
	}
	logger.Info("deployment: deployment queued", "deployment_id", id, "archive", run.ArchiveName,
		"build_number", run.BuildNumber, "previous_build_number", previous, "user", run.RequestedBy)
	return c.JSON(http.StatusAccepted, map[string]any{"status": true, "id": id})
}

// Processes handles GET .../deployment/processes.
func Processes(c echo.Context) error {
	rc, _, err := requireAdmin(c, "20261116-747")
	defer rc.Close()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 15*time.Second)
	defer cancel()
	return c.JSON(http.StatusOK, map[string]any{"status": true, "role": role(), "processes": ListProcesses(ctx, role())})
}

// restartDelay lets the response reach the browser before chenweb restarts itself.
var restartDelay = 500 * time.Millisecond

// RestartProcess handles POST .../deployment/processes/:id/restart.
func RestartProcess(c echo.Context) error {
	rc, user, err := requireAdmin(c, "20261116-748")
	defer rc.Close()
	if err != nil {
		return err
	}
	logger := rc.GetLogger()
	if role() != RoleDeploy {
		return fail(c, http.StatusBadRequest, codeWrongRole, "restart is available only where [deployment] role = \"deploy\"")
	}
	def, ok := findProcess(c.Param("id"))
	if !ok || def.unit == "" {
		return fail(c, http.StatusBadRequest, codeUnknownProcess, "unknown process")
	}
	ctx := c.Request().Context()
	if active, err := activeRun(ctx, KindDeploy); err != nil {
		return fail(c, http.StatusInternalServerError, codeInternal, err.Error())
	} else if active != nil {
		return fail(c, http.StatusConflict, codeActiveRun, "a deployment is in progress")
	}
	who := requester(user)
	if def.unit == "chenweb" {
		logger.Info("deployment: restarting chenweb (self)", "unit", def.unit, "user", who)
		go func() {
			time.Sleep(restartDelay)
			restartCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			// systemd stops this process (and this goroutine) as part of the restart.
			if out, err := RestartUnit(restartCtx, def.unit); err != nil {
				logger.Error("deployment: restart chenweb failed", "error", err, "output", string(out))
			}
		}()
		return c.JSON(http.StatusAccepted, map[string]any{"status": true, "self": true})
	}
	restartCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	out, err := RestartUnit(restartCtx, def.unit)
	if err != nil {
		logger.Error("deployment: restart failed", "unit", def.unit, "user", who, "error", err, "output", string(out))
		return fail(c, http.StatusInternalServerError, codeRestartFailed, strings.TrimSpace(err.Error()+"\n"+string(out)))
	}
	logger.Info("deployment: restarted", "unit", def.unit, "user", who)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "self": false})
}
