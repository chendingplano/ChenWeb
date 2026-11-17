package systemmonitor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/chendingplano/deepdoc/server/api/deployment"
	"github.com/chendingplano/deepdoc/server/buildinfo"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

// restartTarget is a service this machine can restart: its directory holds the mise
// tasks <name>-stop and <name>-start, and it answers build-info requests as <name>.
type restartTarget struct {
	dir  string // relative to the ChenWeb root
	name string
}

// restartTargets lists the services the monitor can restart on the dev machine. Others
// are not listed: chenweb runs under air, and kratos, nats, caddy, pdf-parser and
// deepseek-watcher have no stop/start pair that returns with a pid file.
var restartTargets = map[string]restartTarget{
	"doc-processor":           {"server/cmd/doc-processor", "doc-processor"},
	"doc-service":             {"server/cmd/doc-service", "doc-service"},
	"parser-result-converter": {"server/cmd/parser-result-converter", "parser-result-converter"},
}

// restartLocks serialises restarts per source; a second request while one runs gets 409.
var restartLocks = func() map[string]*sync.Mutex {
	locks := make(map[string]*sync.Mutex, len(restartTargets))
	for id := range restartTargets {
		locks[id] = &sync.Mutex{}
	}
	return locks
}()

var (
	restartTimeout = 3 * time.Minute
	stopWait       = 20 * time.Second
	startWait      = 2 * time.Minute // the start task compiles with `go run` first
	startupSettle  = 3 * time.Second // a worker answers build-info before it subscribes
	pollInterval   = 500 * time.Millisecond
	runMise        = func(ctx context.Context, dir string, task string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "mise", "run", task)
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}
	pidAlive = func(pid int) bool {
		err := syscall.Kill(pid, 0)
		return err == nil || err == syscall.EPERM
	}
	listInstances = func(ctx context.Context, executable string) ([]buildinfo.Process, error) {
		return buildinfo.ListInstances(ctx, executable, 750*time.Millisecond)
	}
	processName = func(pid int) string {
		out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			return ""
		}
		return string(out)
	}
	signalTerm = func(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func chenwebRoot() string {
	if dir := appconfig.GetConfigDir(); dir != "" {
		return dir
	}
	dir, _ := os.Getwd()
	return dir
}

func readPID(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return pid
}

// waitGone waits until pid has exited so the new instance does not start on a busy port.
func waitGone(ctx context.Context, pid int) error {
	deadline := time.Now().Add(stopWait)
	for pidAlive(pid) {
		if time.Now().After(deadline) {
			return context.DeadlineExceeded
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil
}

// sameExecutable reports whether ps's command name belongs to executable. Linux cuts
// the name to 15 characters, and builds in .cache carry an .exe suffix.
func sameExecutable(comm, executable string) bool {
	name := filepath.Base(strings.TrimSpace(comm))
	for _, want := range []string{executable, executable + ".exe"} {
		if name == want || (len(name) == 15 && strings.HasPrefix(want, name)) {
			return true
		}
	}
	return false
}

// stopInstances stops every instance of name running on this machine and waits for
// each to exit. Instances are found by asking them over NATS, so one started any way
// (start.sh in a terminal, mise, go run) is found, and the pid is the service binary's
// own: SIGTERM to a `go run` wrapper exits the wrapper and leaves the binary running.
// It asks again after each round, since an earlier broken restart may have left more
// than one instance.
func stopInstances(ctx context.Context, name string, out *bytes.Buffer) error {
	for range 5 {
		instances, err := listInstances(ctx, name)
		if err != nil {
			return fmt.Errorf("find running %s over NATS: %w", name, err)
		}
		if len(instances) == 0 {
			return nil
		}
		for _, p := range instances {
			if p.PID <= 0 || !sameExecutable(processName(p.PID), name) {
				return fmt.Errorf("%s answered with pid %d, which is not a %s process on this machine; stop it where it runs", name, p.PID, name)
			}
		}
		for _, p := range instances {
			if err := signalTerm(p.PID); err != nil && !errors.Is(err, syscall.ESRCH) {
				return fmt.Errorf("stop %s (pid %d): %w", name, p.PID, err)
			}
			fmt.Fprintf(out, "stopping %s (pid %d)\n", name, p.PID)
		}
		for _, p := range instances {
			if err := waitGone(ctx, p.PID); err != nil {
				return fmt.Errorf("%s (pid %d) has not exited after %s; it may still be finishing in-flight work, so try again once it exits: %w", name, p.PID, stopWait, err)
			}
		}
	}
	return fmt.Errorf("%s is still running after five stop rounds", name)
}

// waitStarted waits until the new instance answers over NATS and is still alive
// startupSettle later, since a worker answers before it subscribes and can exit then.
// The start task's pid file names the `go run` wrapper, which exits with the service.
func waitStarted(ctx context.Context, root, name string) (int, error) {
	wrapper := readPID(filepath.Join(root, ".cache", name+".pid"))
	deadline := time.Now().Add(startWait)
	for {
		instances, err := listInstances(ctx, name)
		if err == nil && len(instances) > 0 {
			pid := instances[0].PID
			if err := sleepCtx(ctx, startupSettle); err != nil {
				return 0, err
			}
			if !pidAlive(pid) {
				return 0, fmt.Errorf("%s (pid %d) exited during startup", name, pid)
			}
			return pid, nil
		}
		if wrapper > 0 && !pidAlive(wrapper) {
			return 0, fmt.Errorf("%s exited during startup", name)
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("%s did not answer within %s of starting", name, startWait)
		}
		if err := sleepCtx(ctx, pollInterval); err != nil {
			return 0, err
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// logTail returns the last lines of the start task's log without terminal colours.
func logTail(path string, lines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	all := strings.Split(strings.TrimRight(ansiEscape.ReplaceAllString(string(data), ""), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return "--- last lines of " + path + " ---\n" + strings.Join(all, "\n") + "\n"
}

// restartService stops the service, waits for its old process to exit and starts it
// again, reporting success only once the new instance is running.
func restartService(ctx context.Context, root string, t restartTarget) ([]byte, error) {
	if strings.TrimSpace(appconfig.AppConfig.Deployment.Role) == deployment.RoleDeploy {
		return deployment.RestartUnit(ctx, t.name)
	}
	var out bytes.Buffer
	if err := stopInstances(ctx, t.name, &out); err != nil {
		return out.Bytes(), err
	}
	// The stop task removes the pid file of an earlier start, so the start task does not
	// take the exiting `go run` wrapper for a running service.
	dir := filepath.Join(root, t.dir)
	for _, task := range []string{t.name + "-stop", t.name + "-start"} {
		taskOut, err := runMise(ctx, dir, task)
		out.Write(taskOut)
		if err != nil {
			return out.Bytes(), err
		}
	}
	pid, err := waitStarted(ctx, root, t.name)
	if err != nil {
		out.WriteString(logTail(filepath.Join(root, ".cache", t.name+".log"), 20))
		return out.Bytes(), err
	}
	fmt.Fprintf(&out, "%s is running (pid %d)\n", t.name, pid)
	return out.Bytes(), nil
}

// isAdmin reports whether the user may use the system monitor.
func isAdmin(user *ApiTypes.UserInfo) bool {
	admin := user.Admin || user.IsOwner
	for _, role := range user.Roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "admin" || role == "root" {
			admin = true
		}
	}
	return admin
}

// Restart handles POST /system-admin/system-monitor/:source/restart.
func Restart(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "20261116-901")
	defer rc.Close()
	logger := rc.GetLogger()
	user := rc.IsAuthenticated()
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"status": false})
	}
	if !isAdmin(user) {
		return c.JSON(http.StatusForbidden, map[string]any{"status": false})
	}
	id := c.Param("source")
	target, ok := restartTargets[id]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "this service cannot be restarted from the monitor"})
	}
	lock := restartLocks[id]
	if !lock.TryLock() {
		return c.JSON(http.StatusConflict, map[string]any{"status": false, "error_msg": "a restart is already in progress"})
	}
	defer lock.Unlock()

	who := user.Email
	if who == "" {
		who = user.UserName
	}
	logger.Info("system monitor: restarting service", "service", id, "user", who)
	ctx, cancel := context.WithTimeout(c.Request().Context(), restartTimeout)
	defer cancel()
	out, err := restartService(ctx, chenwebRoot(), target)
	if err != nil {
		logger.Error("system monitor: restart failed", "service", id, "user", who, "error", err, "output", string(out))
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error_msg": strings.TrimSpace(err.Error() + "\n" + string(out))})
	}
	logger.Info("system monitor: restarted service", "service", id, "user", who)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "output": strings.TrimSpace(string(out))})
}
