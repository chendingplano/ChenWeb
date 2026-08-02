package profiles

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Automatic-selection alarm kind. Selection alarms are persisted verbatim into
// alarms_errors' optional `kind` column (migration 20260801000021) so the
// warning is deduplicated at the database level per review scope id, exactly
// once, independent of the run_id/record_id routing-alarm indexes (migration
// 20260801000019) because a review scope is identified by its TEXT
// review_scope_id, not a BIGINT run/record id.
const SelectionAlarmKindIndeterminate = "profile_selection_indeterminate"

// SelectionAlarmSeverityWarning is the severity of every automatic-selection
// warning (spec 2026080102 section 11: automatic profile-selection
// indeterminacy is a warning, not an error -- the scope is still created and
// executable).
const SelectionAlarmSeverityWarning = "warning"

// SelectionAlarm is one automatic profile-selection operator warning. ScopeID
// carries the review scope id as the dedup correlator, and Message is the
// human-readable text persisted verbatim into alarms_errors.message (ids and
// dimension names only -- never document content).
type SelectionAlarm struct {
	Kind     string
	Severity string
	Message  string
	ScopeID  string
}

// SelectionAlarmSQLWriter targets the existing generic alarms_errors table so
// automatic-selection warnings surface alongside every other operator alarm
// on /semos/admin/alarms without a new admin surface.
type SelectionAlarmSQLWriter struct{ DB *sql.DB }

// WriteSelectionAlarm inserts one selection warning, deduplicated at the
// database level so a repeated scope creation/redelivery of the same scope id
// never writes a second row for the same alarm kind. The dedup relies on
// uq_alarms_errors_scope_id_kind (migration 20260801000021) plus ON CONFLICT
// (scope_id, kind) DO NOTHING, and is deliberately disjoint from the routing
// alarm conflict predicates: a selection warning can neither be collapsed
// into, nor blocked by, a routing alarm for the same run/record.
func (w SelectionAlarmSQLWriter) WriteSelectionAlarm(ctx context.Context, alarm SelectionAlarm) error {
	if w.DB == nil {
		return errors.New("db is nil")
	}
	if strings.TrimSpace(alarm.ScopeID) == "" {
		return errors.New("scope_id is required")
	}
	if strings.TrimSpace(alarm.Kind) == "" {
		return errors.New("kind is required")
	}
	severity := alarm.Severity
	if severity == "" {
		severity = SelectionAlarmSeverityWarning
	}
	_, err := w.DB.ExecContext(ctx, `INSERT INTO alarms_errors (severity, message, scope_id, kind) VALUES ($1,$2,$3,$4)
ON CONFLICT (scope_id, kind) WHERE scope_id IS NOT NULL AND kind IS NOT NULL DO NOTHING`, severity, alarm.Message, alarm.ScopeID, alarm.Kind)
	return err
}
