package deployment

import (
	"context"
	"database/sql"
	"errors"
	"syscall"
	"time"
)

// claimGrace is how long a run may wait for its runner to start or claim it.
const claimGrace = 30 * time.Second

var (
	nowFunc  = time.Now
	pidAlive = func(pid int) bool {
		err := syscall.Kill(pid, 0)
		return err == nil || errors.Is(err, syscall.EPERM)
	}
)

// runnerGone reports whether an active run's runner has died without reporting.
// host is this machine's hostname; unitActive reports whether chenweb-deploy is active.
func runnerGone(r Run, host string, unitActive func() bool) bool {
	waitedTooLong := func(since *time.Time) bool { return since != nil && nowFunc().Sub(*since) > claimGrace }
	switch r.Status {
	case StatusQueued:
		return waitedTooLong(r.RequestedAt) && !unitActive()
	case StatusRunning:
		if r.RunnerPID == 0 {
			return waitedTooLong(r.StartedAt)
		}
		if r.RunnerHost != "" && r.RunnerHost != host {
			return false // another machine's runner; nothing to check here
		}
		return !pidAlive(r.RunnerPID)
	}
	return false
}

// ReconcileActive returns the active run of this kind after failing it if its runner is
// gone (then it returns nil).
func ReconcileActive(ctx context.Context, db *sql.DB, kind, host string, unitActive func() bool) (*Run, error) {
	r, err := ActiveRun(ctx, db, kind)
	if err != nil || r == nil {
		return r, err
	}
	if !runnerGone(*r, host, unitActive) {
		return r, nil
	}
	if err := FailRun(ctx, db, kind, r.ID, ErrRunnerExited); err != nil {
		return nil, err
	}
	return nil, nil
}
