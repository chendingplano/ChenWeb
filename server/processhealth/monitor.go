package processhealth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Service struct {
	Name           string `json:"name"`
	Unit           string `json:"unit"`
	RequiresReport bool   `json:"requires_report"`
}

type Observation struct {
	Service     Service
	ActiveState string
	SubState    string
	PID         int
	Restarts    uint64
	RunningFor  time.Duration
	Report      Report
	ReportError error
	UnitError   error
}

type Incident struct {
	Key          string     `json:"key"`
	Service      string     `json:"service"`
	Reason       string     `json:"reason"`
	Resource     string     `json:"resource,omitempty"`
	Message      string     `json:"message"`
	Recovery     Recovery   `json:"recovery"`
	Since        time.Time  `json:"since"`
	LastObserved time.Time  `json:"last_observed"`
	LastNotified time.Time  `json:"last_notified"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	Occurrences  uint64     `json:"occurrences"`
}

// MonitorState is also the read-only bridge contract for the web application's
// alarms table. Resolved incidents remain present so a bridge can close them.
type MonitorState struct {
	Version    int                    `json:"version"`
	Incidents  map[string]*Incident   `json:"incidents"`
	History    []Incident             `json:"history,omitempty"`
	Restarts   map[string][]time.Time `json:"restarts"`
	CrashLoops map[string]CrashSample `json:"crash_loops"`
}

type CrashSample struct {
	Baseline    uint64    `json:"baseline"`
	LastCount   uint64    `json:"last_count"`
	WindowStart time.Time `json:"window_start"`
	Loop        bool      `json:"loop"`
}

type Alarm struct {
	Type     string    `json:"type"` // alarm, recovered or superseded
	At       time.Time `json:"at"`
	Incident Incident  `json:"incident"`
}

type RestartAction struct {
	Service string
	Unit    string
}

type Policy struct {
	RepeatInterval time.Duration
	StaleAfter     time.Duration
	StartupGrace   time.Duration
	RestartWindow  time.Duration
	MaxRestarts    int
}

func DefaultPolicy() Policy {
	return Policy{RepeatInterval: time.Minute, StaleAfter: time.Minute, StartupGrace: 2 * time.Minute, RestartWindow: 15 * time.Minute, MaxRestarts: 3}
}

func NewMonitorState() MonitorState {
	return MonitorState{Version: 1, Incidents: map[string]*Incident{}, Restarts: map[string][]time.Time{}, CrashLoops: map[string]CrashSample{}}
}

func LoadState(path string) (MonitorState, error) {
	s := NewMonitorState()
	err := readJSON(path, 32*1024*1024, &s)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if s.Version != 1 || s.Incidents == nil || s.Restarts == nil {
		return s, fmt.Errorf("invalid process monitor state")
	}
	if s.CrashLoops == nil {
		s.CrashLoops = map[string]CrashSample{}
	}
	for key, i := range s.Incidents {
		if i == nil || i.Key != key || !serviceName.MatchString(i.Service) || i.Since.IsZero() {
			return s, fmt.Errorf("invalid process monitor incident")
		}
	}
	return s, nil
}

func SaveState(path string, s MonitorState) error { return writeJSON(path, s) }

func incidentKey(service, reason, resource string) string {
	h := sha256.Sum256([]byte(service + "\x00" + reason + "\x00" + resource))
	return hex.EncodeToString(h[:16])
}

// Evaluate has no side effects beyond updating s. Persist the returned state
// before issuing restart actions or delivering alarms. This makes restart budgets
// and notification throttles survive command crashes/restarts.
func Evaluate(s *MonitorState, observations []Observation, now time.Time, policy Policy) ([]Alarm, []RestartAction) {
	var alarms []Alarm
	var actions []RestartAction
	for _, o := range observations {
		// Inactive services were deliberately stopped. Do not start them or close
		// their open incident merely because maintenance masks the condition.
		if o.UnitError == nil && o.ActiveState == "inactive" {
			continue
		}
		reason, resource, message, recovery, skip := assess(o, now, policy)
		// Repeated auto-restarts must not perpetually reset the startup grace.
		// Detect systemd's restart counter independently of application reporting.
		if o.UnitError == nil && trackCrashLoop(s, o, now, policy) && recovery != OperatorAction {
			reason, resource, message, recovery, skip = "process_restart_loop", "", "Service repeatedly exits before becoming functionally stable; operator attention required", OperatorAction, false
		}
		if skip {
			continue
		}
		key := ""
		if reason != "" {
			key = incidentKey(o.Service.Name, reason, resource)
		}
		for oldKey, old := range s.Incidents {
			if old.Service == o.Service.Name && old.ResolvedAt == nil && oldKey != key {
				t := now
				old.ResolvedAt = &t
				eventType := "recovered"
				if reason != "" {
					eventType = "superseded"
				}
				alarms = append(alarms, Alarm{Type: eventType, At: now, Incident: *old})
			}
		}
		if reason == "" {
			continue
		}
		i := s.Incidents[key]
		if i == nil || i.ResolvedAt != nil {
			if i != nil {
				s.History = append(s.History, *i)
			}
			i = &Incident{Key: key, Service: o.Service.Name, Reason: reason, Resource: resource, Message: message, Recovery: recovery, Since: now}
			s.Incidents[key] = i
		}
		i.LastObserved, i.Message, i.Recovery = now, message, recovery
		var recent []time.Time
		if recovery == Restart && o.ActiveState == "active" {
			for _, t := range s.Restarts[o.Service.Name] {
				if now.Sub(t) < policy.RestartWindow {
					recent = append(recent, t)
				}
			}
			s.Restarts[o.Service.Name] = recent
			if len(recent) >= policy.MaxRestarts {
				i.Recovery = OperatorAction
				i.Message += "; automatic restart budget exhausted; operator attention required"
			}
		}
		if i.LastNotified.IsZero() || now.Sub(i.LastNotified) >= policy.RepeatInterval {
			i.LastNotified = now
			i.Occurrences++
			alarms = append(alarms, Alarm{Type: "alarm", At: now, Incident: *i})
		}
		// systemd already owns failed-process restarts. Only restart an active
		// process when its functional report permits it, or the report is stale.
		if recovery != Restart || o.ActiveState != "active" {
			continue
		}
		if len(recent) >= policy.MaxRestarts {
			continue
		}
		if len(recent) > 0 && now.Sub(recent[len(recent)-1]) < policy.RepeatInterval {
			continue
		}
		s.Restarts[o.Service.Name] = append(recent, now)
		actions = append(actions, RestartAction{Service: o.Service.Name, Unit: o.Service.Unit})
	}
	return alarms, actions
}

func trackCrashLoop(s *MonitorState, o Observation, now time.Time, p Policy) bool {
	if s.CrashLoops == nil {
		s.CrashLoops = map[string]CrashSample{}
	}
	c, exists := s.CrashLoops[o.Service.Name]
	if !exists || o.Restarts < c.LastCount {
		c = CrashSample{Baseline: o.Restarts, LastCount: o.Restarts, WindowStart: now}
		// The first monitor observation can happen after a startup crash loop.
		if o.Restarts >= 3 {
			c.Baseline = o.Restarts - 3
			c.Loop = true
		}
	}
	validHealthy := !o.Service.RequiresReport || (o.ReportError == nil && o.Report.Validate() == nil && o.Report.PID == o.PID && o.Report.State == Healthy && now.Sub(o.Report.ObservedAt) <= p.StaleAfter)
	if o.ActiveState == "active" && o.RunningFor >= p.StartupGrace && validHealthy {
		c = CrashSample{Baseline: o.Restarts, LastCount: o.Restarts, WindowStart: now}
	} else {
		if now.Sub(c.WindowStart) > 5*time.Minute {
			c.Baseline = c.LastCount
			c.WindowStart = now
		}
		c.LastCount = o.Restarts
		if o.Restarts >= c.Baseline+3 {
			c.Loop = true
		}
	}
	s.CrashLoops[o.Service.Name] = c
	return c.Loop
}

func assess(o Observation, now time.Time, p Policy) (reason, resource, message string, recovery Recovery, skip bool) {
	if o.UnitError != nil {
		return "service_state_unavailable", "", "Unable to inspect systemd service state", OperatorAction, false
	}
	r := o.Report
	valid := o.ReportError == nil && r.Validate() == nil && r.Service == o.Service.Name && !r.ObservedAt.After(now.Add(5*time.Second))
	// Retain operator-action startup faults even if systemd already started
	// another PID. A new report from the service supersedes this evidence.
	if valid && (r.State == Blocked || r.State == Degraded) && r.Recovery == OperatorAction {
		return r.Reason, r.Resource, r.Message, OperatorAction, false
	}
	if o.ActiveState == "failed" {
		return "process_failed", "", "Service failed; inspect service journal and configuration", OperatorAction, false
	}
	if o.ActiveState == "activating" || o.ActiveState == "deactivating" {
		return "", "", "", None, true
	}
	if o.ActiveState != "active" {
		return "unexpected_service_state", "", "Service is not running", OperatorAction, false
	}
	if !o.Service.RequiresReport {
		return "", "", "", None, false
	}
	current := valid && r.PID == o.PID
	if (!current || r.State == Starting) && o.RunningFor < p.StartupGrace {
		return "", "", "", None, true
	}
	if !current {
		return "health_report_unavailable", "", "Functional health report is missing, invalid, or belongs to another process", Restart, false
	}
	if now.Sub(r.ObservedAt) > p.StaleAfter {
		return "health_report_stale", "", "Functional health report stopped updating", Restart, false
	}
	switch r.State {
	case Healthy, Waiting:
		return "", "", "", None, false
	case Starting:
		return "startup_timeout", "", "Service did not complete initialization within its startup allowance", Restart, false
	default:
		return r.Reason, r.Resource, r.Message, r.Recovery, false
	}
}

// AppendAlarms keeps a local durable fallback when the application/database is
// unavailable. Log rotation is an installation concern.
func AppendAlarms(path string, alarms []Alarm) error {
	if len(alarms) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, a := range alarms {
		if err := json.NewEncoder(f).Encode(a); err != nil {
			return err
		}
	}
	return f.Sync()
}
