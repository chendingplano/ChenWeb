// Package processhealthalarms projects the independent monitor's durable incidents
// into the existing operator alarm list. Supervision never depends on this bridge.
package processhealthalarms

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/chendingplano/deepdoc/server/processhealth"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

const upsertIncident = `INSERT INTO alarms_errors
 (severity,message,status,scope_id,kind,occurred_at,updated_at)
 VALUES ($1,$2,$3,$4,'process_health',$5,$6)
 ON CONFLICT (scope_id,kind) WHERE scope_id IS NOT NULL AND kind IS NOT NULL
 DO UPDATE SET severity=EXCLUDED.severity,message=EXCLUDED.message,
 status=EXCLUDED.status,updated_at=EXCLUDED.updated_at
 WHERE alarms_errors.updated_at < EXCLUDED.updated_at`

// Sync is idempotent, including after either process restarts. Incident onset is
// part of the scope so a later recurrence retains a separate alarm history.
func Sync(ctx context.Context, db *sql.DB, path string) error {
	state, err := processhealth.LoadState(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	incidents := append([]processhealth.Incident(nil), state.History...)
	for _, in := range state.Incidents {
		incidents = append(incidents, *in)
	}
	for _, in := range incidents {
		if in.LastNotified.IsZero() {
			continue
		}
		status, severity, observed := "unsolved", "error", in.LastNotified
		message := fmt.Sprintf("Process %s: %s [%s; %s] resource=%s; observations=%d", in.Service, in.Message, in.Reason, in.Recovery, in.Resource, in.Occurrences)
		if in.ResolvedAt != nil {
			status = "solved"
			severity = "info"
			observed = *in.ResolvedAt
			message = "Closed: " + message
		}
		scope := "process-health/" + in.Key + "/" + in.Since.UTC().Format(time.RFC3339Nano)
		if _, err := db.ExecContext(ctx, upsertIncident, severity, message, status, scope, in.Since, observed); err != nil {
			return err
		}
	}
	return nil
}

// Run is optional on developer machines. PostgreSQL outages leave the independent
// monitor's incidents and journal alarms intact; the next pass retries delivery.
func Run(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger) {
	path := os.Getenv("PROCESS_HEALTH_MONITOR_STATE")
	if path == "" || db == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := Sync(checkCtx, db, path)
		cancel()
		if err != nil && logger != nil {
			logger.Error("process health alarm delivery failed; local monitor incidents retained", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
