package processhealth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func exampleObservation(now time.Time) Observation {
	return Observation{Service: Service{Name: "worker", Unit: "worker.service", RequiresReport: true}, ActiveState: "active", PID: 123, RunningFor: 10 * time.Minute,
		Report: Report{Service: "worker", PID: 123, Generation: "123-generation", State: Healthy, Recovery: None, Since: now.Add(-time.Minute), ObservedAt: now}}
}

func TestOperatorFaultIsThrottledPersistentlyAndNeverRestarted(t *testing.T) {
	now := time.Now().UTC()
	o := exampleObservation(now)
	o.Report.State = Blocked
	o.Report.Reason = "invalid_configuration"
	o.Report.Resource = "PROMPT_VAR"
	o.Report.Recovery = OperatorAction
	s := NewMonitorState()
	p := DefaultPolicy()
	alarms, actions := Evaluate(&s, []Observation{o}, now, p)
	if len(alarms) != 1 || len(actions) != 0 {
		t.Fatalf("first observation alarms=%d restarts=%d", len(alarms), len(actions))
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveState(path, s); err != nil {
		t.Fatal(err)
	}
	s, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	// A new process ID from a restart loop must not hide the operator fault.
	o.PID = 456
	alarms, actions = Evaluate(&s, []Observation{o}, now.Add(59*time.Second), p)
	if len(alarms) != 0 || len(actions) != 0 {
		t.Fatal("restart reset alarm throttle or restarted operator fault")
	}
	alarms, actions = Evaluate(&s, []Observation{o}, now.Add(time.Minute), p)
	if len(alarms) != 1 || len(actions) != 0 {
		t.Fatal("expected one repeated operator alarm")
	}
}

func TestRecoveryOnceAndRecurrencePreservesHistory(t *testing.T) {
	now := time.Now().UTC()
	o := exampleObservation(now)
	s := NewMonitorState()
	p := DefaultPolicy()
	o.Report.State = Blocked
	o.Report.Reason = "required_asset_missing"
	o.Report.Recovery = OperatorAction
	Evaluate(&s, []Observation{o}, now, p)
	o.Report.State = Healthy
	o.Report.Reason = ""
	o.Report.Recovery = None
	o.Report.ObservedAt = now.Add(time.Minute)
	a, _ := Evaluate(&s, []Observation{o}, now.Add(time.Minute), p)
	if len(a) != 1 || a[0].Type != "recovered" {
		t.Fatalf("expected recovery: %+v", a)
	}
	a, _ = Evaluate(&s, []Observation{o}, now.Add(time.Minute), p)
	if len(a) != 0 {
		t.Fatal("repeated recovery")
	}
	o.Report.State = Blocked
	o.Report.Reason = "required_asset_missing"
	o.Report.Recovery = OperatorAction
	a, _ = Evaluate(&s, []Observation{o}, now.Add(2*time.Minute), p)
	if len(a) != 1 || len(s.History) != 1 || s.History[0].ResolvedAt == nil {
		t.Fatal("recurrence lost previous closed incident")
	}
}

func TestMaintenanceAndIntentionalWaitingDoNotRestart(t *testing.T) {
	now := time.Now().UTC()
	p := DefaultPolicy()
	for _, state := range []string{"inactive", "active"} {
		o := exampleObservation(now)
		o.ActiveState = state
		o.Report.State = Waiting
		o.Report.Reason = "offpeak_window"
		s := NewMonitorState()
		a, r := Evaluate(&s, []Observation{o}, now, p)
		if len(a) != 0 || len(r) != 0 {
			t.Fatalf("%s waiting restarted or alarmed", state)
		}
	}
}

func TestStaleReportsBoundRestartBudgetAcrossRuns(t *testing.T) {
	now := time.Now().UTC()
	o := exampleObservation(now)
	o.Report.ObservedAt = now.Add(-2 * time.Minute)
	o.Report.Since = o.Report.ObservedAt
	s := NewMonitorState()
	p := DefaultPolicy()
	for n := 0; n < 5; n++ {
		_, r := Evaluate(&s, []Observation{o}, now.Add(time.Duration(n)*time.Minute), p)
		want := 1
		if n >= p.MaxRestarts {
			want = 0
		}
		if len(r) != want {
			t.Fatalf("iteration %d restarts %d want %d", n, len(r), want)
		}
		if n >= p.MaxRestarts {
			for _, in := range s.Incidents {
				if in.ResolvedAt == nil && in.Recovery != OperatorAction {
					t.Fatal("exhausted budget did not request operator action")
				}
			}
		}
	}
}

func TestInvalidReportStartupGraceAndSystemdFailure(t *testing.T) {
	now := time.Now().UTC()
	o := exampleObservation(now)
	o.ReportError = errors.New("malformed json")
	o.RunningFor = time.Second
	s := NewMonitorState()
	p := DefaultPolicy()
	a, r := Evaluate(&s, []Observation{o}, now, p)
	if len(a) != 0 || len(r) != 0 {
		t.Fatal("startup grace not respected")
	}
	o.RunningFor = 3 * time.Minute
	a, r = Evaluate(&s, []Observation{o}, now, p)
	if len(a) != 1 || len(r) != 1 || a[0].Incident.Reason != "health_report_unavailable" {
		t.Fatal("invalid report not detected")
	}
	o.ActiveState = "failed"
	a, r = Evaluate(&s, []Observation{o}, now.Add(time.Minute), p)
	if len(r) != 0 {
		t.Fatal("monitor competed with systemd for crashed process")
	}
	for _, event := range a {
		if event.Type == "recovered" {
			t.Fatal("changed failure wrongly reported recovered")
		}
	}
}

func TestCorruptedStateIsNotSilentlyReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(path); err == nil {
		t.Fatal("invalid state reset restart budgets")
	}
}

func TestRepeatedStartupCrashDoesNotResetGraceForever(t *testing.T) {
	now := time.Now().UTC()
	o := exampleObservation(now)
	o.ActiveState = "activating"
	o.SubState = "auto-restart"
	o.ReportError = os.ErrNotExist
	o.RunningFor = 0
	s := NewMonitorState()
	p := DefaultPolicy()
	for n := 0; n < 4; n++ {
		o.Restarts = uint64(n)
		a, r := Evaluate(&s, []Observation{o}, now.Add(time.Duration(n)*10*time.Second), p)
		if len(r) != 0 {
			t.Fatal("restart loop triggered competing monitor restart")
		}
		if n < 3 && len(a) != 0 {
			t.Fatal("premature restart loop alarm")
		}
		if n == 3 && (len(a) != 1 || a[0].Incident.Reason != "process_restart_loop") {
			t.Fatalf("startup loop hidden by grace: %+v", a)
		}
	}
	// A running PID for a moment is not recovery. Only stable healthy work is.
	o.ActiveState = "active"
	o.RunningFor = time.Second
	a, _ := Evaluate(&s, []Observation{o}, now.Add(40*time.Second), p)
	for _, event := range a {
		if event.Type == "recovered" {
			t.Fatal("brief startup falsely recovered crash loop")
		}
	}
	o = exampleObservation(now.Add(3 * time.Minute))
	o.Restarts = 3
	a, _ = Evaluate(&s, []Observation{o}, now.Add(3*time.Minute), p)
	if len(a) != 1 || a[0].Type != "recovered" {
		t.Fatalf("stable healthy process did not recover: %+v", a)
	}
}
