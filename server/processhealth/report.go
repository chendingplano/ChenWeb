// Package processhealth publishes functional health without depending on the
// database, message broker or application API. Each process decides its health.
package processhealth

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

type State string
type Recovery string

const (
	ExitOperatorAction          = 78
	Starting           State    = "starting"
	Healthy            State    = "healthy"
	Waiting            State    = "waiting"
	Degraded           State    = "degraded"
	Blocked            State    = "blocked"
	None               Recovery = "none"
	Restart            Recovery = "restart"
	OperatorAction     Recovery = "operator_action"
)

type Report struct {
	Service    string    `json:"service"`
	PID        int       `json:"pid"`
	Generation string    `json:"generation"`
	State      State     `json:"state"`
	Reason     string    `json:"reason,omitempty"`
	Resource   string    `json:"resource,omitempty"`
	Message    string    `json:"message,omitempty"`
	Recovery   Recovery  `json:"recovery"`
	Since      time.Time `json:"since"`
	ObservedAt time.Time `json:"observed_at"`
}

var serviceName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$`)

func (r Report) Validate() error {
	if !serviceName.MatchString(r.Service) || r.PID <= 0 || r.Generation == "" || r.Since.IsZero() || r.ObservedAt.IsZero() || r.Since.After(r.ObservedAt) {
		return fmt.Errorf("invalid process health identity or timestamps")
	}
	switch r.State {
	case Starting, Healthy, Waiting, Degraded, Blocked:
	default:
		return fmt.Errorf("invalid process health state %q", r.State)
	}
	switch r.Recovery {
	case None, Restart, OperatorAction:
	default:
		return fmt.Errorf("invalid recovery policy %q", r.Recovery)
	}
	if (r.State == Blocked || r.State == Degraded) && r.Reason == "" {
		return fmt.Errorf("unhealthy report requires a stable reason")
	}
	return nil
}

type Reporter struct {
	mu     sync.Mutex
	path   string
	report Report
}

// NewReporter is disabled when PROCESS_HEALTH_DIR is unset. The caller must
// handle write failures: a service must never claim health only in memory.
func NewReporter(service string) (*Reporter, error) {
	return NewReporterAt(os.Getenv("PROCESS_HEALTH_DIR"), service)
}

func NewReporterAt(dir, service string) (*Reporter, error) {
	if !serviceName.MatchString(service) {
		return nil, fmt.Errorf("invalid service name")
	}
	now := time.Now().UTC()
	r := &Reporter{report: Report{Service: service, PID: os.Getpid(), Generation: fmt.Sprintf("%d-%d", os.Getpid(), now.UnixNano()), State: Starting, Recovery: None, Since: now, ObservedAt: now}}
	if dir != "" {
		r.path = filepath.Join(dir, fmt.Sprintf("%s-%d.json", service, r.report.PID))
	}
	return r, r.write()
}

// Set messages must be safe for operator display: never pass credentials or
// raw provider responses. Resource identifies the affected file/configuration.
func (r *Reporter) Set(state State, reason, resource, message string, recovery Recovery) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	next := r.report
	now := time.Now().UTC()
	if next.State != state || next.Reason != reason || next.Resource != resource || next.Recovery != recovery {
		next.Since = now
	}
	next.State, next.Reason, next.Resource, next.Message, next.Recovery, next.ObservedAt = state, reason, resource, message, recovery, now
	if err := next.Validate(); err != nil {
		return err
	}
	r.report = next
	return r.write()
}

func (r *Reporter) Refresh() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.report.ObservedAt = time.Now().UTC()
	return r.write()
}

func (r *Reporter) Snapshot() Report {
	if r == nil {
		return Report{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.report
}

func (r *Reporter) write() error {
	if r.path == "" {
		return nil
	}
	return writeJSON(r.path, r.report)
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".health-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0644); err == nil {
		err = json.NewEncoder(f).Encode(value)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	// Persist the directory entry as well as file contents. Monitor cooldowns
	// must survive an unexpected host restart after the atomic replacement.
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func ReadReport(dir, service string, pid int) (Report, error) {
	var r Report
	if !serviceName.MatchString(service) || pid <= 0 {
		return r, fmt.Errorf("invalid service name or pid")
	}
	err := readJSON(filepath.Join(dir, fmt.Sprintf("%s-%d.json", service, pid)), 64*1024, &r)
	if os.IsNotExist(err) {
		// Accept reports from workers that have not yet been upgraded.
		err = readJSON(filepath.Join(dir, service+".json"), 64*1024, &r)
	}
	if err != nil {
		return r, err
	}
	if err := r.Validate(); err != nil {
		return r, err
	}
	if r.Service != service || r.PID != pid {
		return r, fmt.Errorf("health report identity mismatch")
	}
	return r, nil
}

func readJSON(path string, limit int64, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return err
	}
	if int64(len(b)) > limit {
		return fmt.Errorf("process health JSON exceeds size limit")
	}
	return json.Unmarshal(b, value)
}
