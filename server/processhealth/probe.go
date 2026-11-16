package processhealth

import (
	"context"
	"os"
	"strconv"
	"time"
)

// Issue is a process-specific, credential-safe diagnosis. Never pass raw client
// errors, provider responses or connection strings as its message.
type Issue struct {
	Reason, Resource, Message string
	Recovery                  Recovery
}

// CapturePanic preserves the original crash behavior while leaving a safe,
// durable diagnosis. It must be deferred directly by the service entrypoint.
func (r *Reporter) CapturePanic() {
	if value := recover(); value != nil {
		_ = r.Set(Degraded, "process_panic", "service initialization/runtime", "Service panicked; inspect its journal for the cause", Restart)
		panic(value)
	}
}

// ApplicationEnvironment validates the shared configuration loader's required
// variables before it can panic. Values are never included in the diagnosis.
func ApplicationEnvironment() *Issue {
	if os.Getenv("APP_HOST") == "" {
		return &Issue{Reason: "configuration_invalid", Resource: "APP_HOST", Message: "Required application host is not configured", Recovery: OperatorAction}
	}
	port, err := strconv.Atoi(os.Getenv("APP_PORT"))
	if err != nil || port < 1 || port > 65535 {
		return &Issue{Reason: "configuration_invalid", Resource: "APP_PORT", Message: "Required application port is missing or invalid", Recovery: OperatorAction}
	}
	return nil
}

// GoCheck refreshes health only after a functional check returns. A stuck check
// leaves a stale report instead of a false heartbeat. Checks must honor their
// context. Long document/LLM operations are not functional health checks.
func (r *Reporter) GoCheck(ctx context.Context, interval time.Duration, check func(context.Context) *Issue, reportError func(error)) {
	if r == nil || r.path == "" || interval <= 0 || check == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			checkCtx, cancel := context.WithTimeout(ctx, interval)
			issue := check(checkCtx)
			if checkCtx.Err() != nil && issue == nil {
				issue = &Issue{Reason: "health_check_timeout", Message: "Functional health check exceeded its deadline", Recovery: Restart}
			}
			cancel()
			if ctx.Err() != nil {
				return
			}
			var err error
			if issue == nil {
				err = r.Set(Healthy, "", "", "Functional checks passed", None)
			} else {
				state := Degraded
				if issue.Recovery == OperatorAction {
					state = Blocked
				}
				err = r.Set(state, issue.Reason, issue.Resource, issue.Message, issue.Recovery)
			}
			if err != nil && reportError != nil {
				reportError(err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// WritableDirectory checks the capability needed by ingest/conversion workers,
// including permissions and full/read-only filesystems. The probe is removed.
func WritableDirectory(path string) *Issue {
	issue := &Issue{Reason: "storage_unavailable", Resource: path, Message: "Required document directory is not readable and writable", Recovery: OperatorAction}
	if path == "" {
		issue.Resource = "DATA_HOME_DIR"
		return issue
	}
	if _, err := os.ReadDir(path); err != nil {
		return issue
	}
	file, err := os.CreateTemp(path, ".process-health-*")
	if err != nil {
		return issue
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return issue
	}
	if err := os.Remove(name); err != nil {
		return issue
	}
	return nil
}
