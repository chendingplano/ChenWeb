package systemmonitor

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

// restartTarget is a service this machine can restart: its directory holds the mise
// tasks <name>-stop and <name>-start, and its pid lives in <root>/.cache/<name>.pid.
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
	runMise        = func(ctx context.Context, dir string, task string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "mise", "run", task)
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}
	pidAlive = func(pid int) bool {
		err := syscall.Kill(pid, 0)
		return err == nil || err == syscall.EPERM
	}
)

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

// restartService stops the service, waits for its old process to exit and starts it again.
func restartService(ctx context.Context, root string, t restartTarget) ([]byte, error) {
	dir := filepath.Join(root, t.dir)
	old := readPID(filepath.Join(root, ".cache", t.name+".pid"))
	out, err := runMise(ctx, dir, t.name+"-stop")
	if err != nil {
		return out, err
	}
	if old > 0 {
		if err := waitGone(ctx, old); err != nil {
			return out, err
		}
	}
	startOut, err := runMise(ctx, dir, t.name+"-start")
	return append(out, startOut...), err
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
