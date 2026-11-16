package deployment

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/buildinfo"
)

// DeployUnit is the systemd unit that runs deploy-runner on the deployment machine.
const DeployUnit = "chenweb-deploy"

type processDef struct {
	id         string
	unit       string // systemd unit on the deployment machine; "" = none
	logSource  string // System Monitor source id
	executable string // buildinfo executable; "" = not a Go service
}

// processDefs lists the services in page order. Entries without a unit have log
// sources only on the build machine.
var processDefs = []processDef{
	{"chenweb", "chenweb", "chenweb", "server"},
	{"doc-processor", "doc-processor", "doc-processor", "doc-processor"},
	{"doc-service", "doc-service", "doc-service", "doc-service"},
	{"parser-result-converter", "parser-result-converter", "parser-result-converter", "parser-result-converter"},
	{"kratos", "kratos", "kratos", ""},
	{"nats", "nats-server", "nats", ""},
	{"pdf-parser", "pdf-parser", "pdf-parser", ""},
	{"caddy", "", "caddy", ""},
	{"deepseek-watcher", "", "deepseek-watcher", ""},
}

// Process is one row of the page's process list.
type Process struct {
	ID            string `json:"id"`
	Unit          string `json:"unit,omitempty"`
	LogSource     string `json:"log_source"`
	ActiveState   string `json:"active_state,omitempty"`
	SubState      string `json:"sub_state,omitempty"`
	PID           int    `json:"pid,omitempty"`
	UptimeSeconds int64  `json:"uptime_seconds,omitempty"`
	BuildNumber   string `json:"build_number,omitempty"`
	BuildStatus   string `json:"build_status,omitempty"` // running | unreported; "" for non-Go services
	CanRestart    bool   `json:"can_restart"`
}

// runCommand runs a command and returns its combined output. Tests replace it.
var runCommand = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// readBootUptime returns seconds since boot from /proc/uptime (Linux only).
var readBootUptime = func() (float64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, os.ErrInvalid
	}
	return strconv.ParseFloat(fields[0], 64)
}

func findProcess(id string) (processDef, bool) {
	for _, d := range processDefs {
		if d.id == id {
			return d, true
		}
	}
	return processDef{}, false
}

// unitState fills a process's systemd state. ActiveEnterTimestampMonotonic is used for
// the uptime because the box's systemd (237) prints wall-clock timestamps with a zone
// abbreviation Go cannot parse reliably.
func unitState(ctx context.Context, p *Process) {
	out, err := runCommand(ctx, "", "systemctl", "show", "-p", "ActiveState", "-p", "SubState", "-p", "MainPID",
		"-p", "ActiveEnterTimestampMonotonic", p.Unit)
	if err != nil {
		p.ActiveState = "unknown"
		return
	}
	var monotonicMicros int64
	for _, line := range strings.Split(string(out), "\n") {
		key, value, _ := strings.Cut(strings.TrimSpace(line), "=")
		switch key {
		case "ActiveState":
			p.ActiveState = value
		case "SubState":
			p.SubState = value
		case "MainPID":
			p.PID, _ = strconv.Atoi(value)
		case "ActiveEnterTimestampMonotonic":
			monotonicMicros, _ = strconv.ParseInt(value, 10, 64)
		}
	}
	if p.ActiveState == "active" && monotonicMicros > 0 {
		if boot, err := readBootUptime(); err == nil {
			if up := int64(boot - float64(monotonicMicros)/1e6); up >= 0 {
				p.UptimeSeconds = up
			}
		}
	}
}

// ListProcesses returns the services for this machine's role: systemd units with their
// state on the deployment machine, every log source elsewhere.
func ListProcesses(ctx context.Context, role string) []Process {
	var executables []string
	for _, d := range processDefs {
		if d.executable != "" && d.executable != "server" {
			executables = append(executables, d.executable)
		}
	}
	builds := map[string]buildinfo.Process{}
	for _, b := range buildinfo.ListProcesses(ctx, executables) {
		builds[b.Executable] = b
	}
	if self, err := buildinfo.CurrentProcess("server"); err == nil {
		builds["server"] = self
	} else {
		builds["server"] = buildinfo.Process{Executable: "server", Status: "unreported"}
	}

	out := []Process{}
	for _, d := range processDefs {
		if role == RoleDeploy && d.unit == "" {
			continue
		}
		p := Process{ID: d.id, LogSource: d.logSource}
		if d.executable != "" {
			b := builds[d.executable]
			p.BuildNumber, p.BuildStatus = b.BuildNumber, b.Status
			if p.BuildStatus == "" {
				p.BuildStatus = "unreported"
			}
		}
		if role == RoleDeploy {
			p.Unit = d.unit
			p.CanRestart = true
			unitState(ctx, &p)
		}
		out = append(out, p)
	}
	return out
}

// RestartUnit restarts one unit through gui's passwordless sudoers rule.
func RestartUnit(ctx context.Context, unit string) ([]byte, error) {
	return runCommand(ctx, "", "sudo", "-n", "/bin/systemctl", "restart", unit)
}

// StartDeployUnit starts chenweb-deploy, which claims the queued deployment.
func StartDeployUnit(ctx context.Context) ([]byte, error) {
	return runCommand(ctx, "", "sudo", "-n", "/bin/systemctl", "start", DeployUnit)
}

// DeployUnitActive reports whether chenweb-deploy is running or starting.
func DeployUnitActive() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, _ := runCommand(ctx, "", "systemctl", "is-active", DeployUnit)
	state := strings.TrimSpace(string(out))
	return state == "active" || state == "activating"
}
