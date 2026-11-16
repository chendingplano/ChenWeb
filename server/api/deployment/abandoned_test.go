package deployment

import (
	"testing"
	"time"
)

func TestRunnerGone(t *testing.T) {
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	oldNow, oldAlive := nowFunc, pidAlive
	t.Cleanup(func() { nowFunc, pidAlive = oldNow, oldAlive })
	nowFunc = func() time.Time { return now }
	pidAlive = func(pid int) bool { return pid == 100 }

	recent, old := now.Add(-5*time.Second), now.Add(-time.Minute)
	active := func() bool { return true }
	inactive := func() bool { return false }

	cases := []struct {
		name string
		run  Run
		unit func() bool
		want bool
	}{
		{"live runner", Run{Status: StatusRunning, RunnerPID: 100, RunnerHost: "mac", StartedAt: &old}, inactive, false},
		{"dead runner", Run{Status: StatusRunning, RunnerPID: 200, RunnerHost: "mac", StartedAt: &old}, inactive, true},
		{"dead runner, no host recorded", Run{Status: StatusRunning, RunnerPID: 200, StartedAt: &old}, inactive, true},
		{"runner on another host", Run{Status: StatusRunning, RunnerPID: 200, RunnerHost: "other", StartedAt: &old}, inactive, false},
		{"runner not started yet", Run{Status: StatusRunning, StartedAt: &recent}, inactive, false},
		{"runner never started", Run{Status: StatusRunning, StartedAt: &old}, inactive, true},
		{"queued, waiting", Run{Status: StatusQueued, RequestedAt: &recent}, inactive, false},
		{"queued, unit starting", Run{Status: StatusQueued, RequestedAt: &old}, active, false},
		{"queued, never claimed", Run{Status: StatusQueued, RequestedAt: &old}, inactive, true},
		{"finished", Run{Status: StatusFailed, RunnerPID: 200}, inactive, false},
	}
	for _, c := range cases {
		if got := runnerGone(c.run, "mac", c.unit); got != c.want {
			t.Errorf("%s: runnerGone = %v, want %v", c.name, got, c.want)
		}
	}
}
