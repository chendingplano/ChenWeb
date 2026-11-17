// process-monitor is a bounded systemd-timer task. It neither needs the
// application server nor opens a database/broker connection.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/processhealth"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func main() {
	logger := loggerutil.CreateDefaultLogger("20261116-671")
	defer logger.Close()
	var reports, stateDir, config string
	policy := processhealth.DefaultPolicy()
	flag.StringVar(&reports, "reports", "", "process health report directory")
	flag.StringVar(&stateDir, "state", "", "persistent monitor state directory")
	flag.StringVar(&config, "services", "", "root-owned service allowlist JSON file")
	flag.DurationVar(&policy.RepeatInterval, "repeat", policy.RepeatInterval, "alarm repeat interval")
	flag.DurationVar(&policy.StaleAfter, "stale", policy.StaleAfter, "health report expiry")
	flag.DurationVar(&policy.StartupGrace, "startup-grace", policy.StartupGrace, "startup allowance")
	flag.Parse()
	if reports == "" || stateDir == "" || config == "" || policy.RepeatInterval < time.Second || policy.StaleAfter < time.Second || policy.StartupGrace < time.Second {
		logger.Error("process-monitor requires directories, service allowlist and positive policy durations")
		os.Exit(2)
	}
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		logger.Error("create monitor state directory", "error", err)
		os.Exit(1)
	}
	unlock, err := lock(filepath.Join(stateDir, "monitor.lock"))
	if err != nil {
		logger.Error("lock process monitor state", "error", err)
		os.Exit(1)
	}
	defer unlock()
	services, err := readServices(config)
	if err != nil {
		logger.Error("invalid process monitor allowlist", "error", err)
		os.Exit(1)
	}
	statePath := filepath.Join(stateDir, "state.json")
	state, err := processhealth.LoadState(statePath)
	if err != nil {
		logger.Error("refusing to reset invalid persistent monitor state", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var observations []processhealth.Observation
	for _, service := range services {
		observations = append(observations, observe(ctx, reports, service))
	}
	alarms, restarts := processhealth.Evaluate(&state, observations, time.Now().UTC(), policy)
	// Persist first: monitor restart must not erase an alarm cooldown or restart
	// budget. The snapshot is also the durable source for database alarm delivery.
	if err := processhealth.SaveState(statePath, state); err != nil {
		logger.Error("persist process monitor state", "error", err)
		os.Exit(1)
	}
	if err := processhealth.AppendAlarms(filepath.Join(stateDir, "alarms.jsonl"), alarms); err != nil {
		logger.Error("append local process health alarms", "error", err)
	}
	for _, a := range alarms {
		if a.Type != "alarm" {
			logger.Info("process functional health incident closed", "outcome", a.Type, "service", a.Incident.Service, "reason", a.Incident.Reason, "incident", a.Incident.Key)
		} else {
			logger.Error("process functional health alarm", "service", a.Incident.Service, "reason", a.Incident.Reason, "resource", a.Incident.Resource, "message", a.Incident.Message, "recovery", a.Incident.Recovery, "incident", a.Incident.Key)
		}
	}
	for _, a := range restarts {
		restartCtx, restartCancel := context.WithTimeout(ctx, 15*time.Second)
		// --no-block schedules the restart without waiting through service drain.
		err := exec.CommandContext(restartCtx, "systemctl", "--no-block", "restart", a.Unit).Run()
		restartCancel()
		if err != nil {
			logger.Error("functional health restart failed", "service", a.Service, "error", err)
		} else {
			logger.Warn("functional health restart scheduled", "service", a.Service)
		}
	}
}

func readServices(path string) ([]processhealth.Service, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var services []processhealth.Service
	if err := json.Unmarshal(data, &services); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	seenUnit := map[string]bool{}
	for _, s := range services {
		// Restrict units to plain service names: no arbitrary unit type, path,
		// glob, command options or template expansion in a root command.
		unit := strings.TrimSuffix(s.Unit, ".service")
		if !validName(s.Name) || !validName(unit) || seen[s.Name] || seenUnit[unit] {
			return nil, fmt.Errorf("invalid or duplicate service allowlist entry")
		}
		seen[s.Name] = true
		seenUnit[unit] = true
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("empty service allowlist")
	}
	return services, nil
}

func validName(v string) bool {
	if v == "" || len(v) > 80 || v[0] == '-' {
		return false
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func observe(ctx context.Context, dir string, s processhealth.Service) processhealth.Observation {
	o := processhealth.Observation{Service: s}
	unitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	data, err := exec.CommandContext(unitCtx, "systemctl", "show", s.Unit, "-p", "ActiveState", "-p", "SubState", "-p", "MainPID", "-p", "ExecMainPID", "-p", "NRestarts", "-p", "ActiveEnterTimestampMonotonic").Output()
	o.UnitError = err
	var startedMicros int64
	var execPID int
	for _, line := range strings.Split(string(data), "\n") {
		key, value, _ := strings.Cut(line, "=")
		switch key {
		case "ActiveState":
			o.ActiveState = value
		case "SubState":
			o.SubState = value
		case "MainPID":
			o.PID, _ = strconv.Atoi(value)
		case "ExecMainPID":
			execPID, _ = strconv.Atoi(value)
		case "NRestarts":
			o.Restarts, _ = strconv.ParseUint(value, 10, 64)
		case "ActiveEnterTimestampMonotonic":
			startedMicros, _ = strconv.ParseInt(value, 10, 64)
		}
	}
	if o.UnitError == nil && o.ActiveState == "" {
		o.UnitError = fmt.Errorf("systemd returned no service state")
	}
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			seconds, _ := strconv.ParseFloat(fields[0], 64)
			o.RunningFor = time.Duration(seconds*float64(time.Second)) - time.Duration(startedMicros)*time.Microsecond
		}
	}
	reportPID := o.PID
	if reportPID == 0 {
		reportPID = execPID
	}
	o.Report, o.ReportError = processhealth.ReadReport(dir, s.Name, reportPID)
	return o
}
