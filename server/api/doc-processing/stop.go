package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrPipelineStopped is the cause attached to a context cancelled by a user stop request.
// Callers check errors.Is(context.Cause(ctx), ErrPipelineStopped) to distinguish a
// user-requested stop from other context cancellations.
var ErrPipelineStopped = errors.New("pipeline stopped by user request")

// isCtxStopped reports whether ctx was cancelled specifically by a user stop request.
func isCtxStopped(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), ErrPipelineStopped)
}

// OnStopFunc is the callback type passed to CheckAndHandleStop.
// Implementations must:
//  1. Write proc_status = "stopped" to the appropriate kb.inputs.status entry.
//  2. Write a finish log entry to kb.doc_proc_logs.
//
// The context argument is always context.Background() — the pipeline context is
// already cancelled at the point of this call, so DB writes must not use it.
type OnStopFunc func(bgCtx context.Context)

// CheckAndHandleStop checks whether the pipeline context was cancelled by a user
// stop request. If it was, onStop is called with a background context (for DB
// writes), and the function returns true. The caller must then return
// ErrPipelineStopped immediately without doing any further LLM work.
//
// Processors should call this function at every LLM call boundary:
//
//	if CheckAndHandleStop(ctx, func(bgCtx context.Context) {
//	    s.stopAndPersistFoo(bgCtx, rec, ...)
//	}) {
//	    return ErrPipelineStopped
//	}
func CheckAndHandleStop(ctx context.Context, onStop OnStopFunc) bool {
	if !isCtxStopped(ctx) {
		return false
	}
	if onStop != nil {
		onStop(context.Background())
	}
	return true
}

// StopRequestStore reads and writes the stop-request flag for a pipeline run.
type StopRequestStore interface {
	SetStopRequested(ctx context.Context, id int64) error
	IsStopRequested(ctx context.Context, id int64) (bool, error)
	ClearStopRequested(ctx context.Context, id int64) error
	// MarkDocProcessingStopped updates the doc_processing status entry to "stopped"
	// if it is currently "running". Safe to call regardless of whether the
	// doc-processor service is active.
	MarkDocProcessingStopped(ctx context.Context, id int64) error
}

// StopRequestSQLStore implements StopRequestStore via kb.inputs.status.
type StopRequestSQLStore struct {
	DB *sql.DB
}

const stopRequestedOperation = "stop_requested"

func (s StopRequestSQLStore) SetStopRequested(ctx context.Context, id int64) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	raw, err := s.loadStatusRaw(ctx, id)
	if err != nil {
		return err
	}
	entries := decodeDocMetaStatus(raw)
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		if strings.EqualFold(strings.TrimSpace(asString(e["operation"])), stopRequestedOperation) {
			continue
		}
		out = append(out, e)
	}
	out = append(out, map[string]any{
		"operation":   stopRequestedOperation,
		"proc_status": "pending",
		"start_time":  time.Now().Format(defaultDocMetaStatusTime),
	})
	bs, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return s.updateStatus(ctx, id, string(bs))
}

func (s StopRequestSQLStore) IsStopRequested(ctx context.Context, id int64) (bool, error) {
	if s.DB == nil {
		return false, errors.New("db is nil")
	}
	raw, err := s.loadStatusRaw(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return hasStopRequested(raw), nil
}

func (s StopRequestSQLStore) ClearStopRequested(ctx context.Context, id int64) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	raw, err := s.loadStatusRaw(ctx, id)
	if err != nil {
		return err
	}
	entries := decodeDocMetaStatus(raw)
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if strings.EqualFold(strings.TrimSpace(asString(e["operation"])), stopRequestedOperation) {
			continue
		}
		out = append(out, e)
	}
	bs, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return s.updateStatus(ctx, id, string(bs))
}

// MarkDocProcessingStopped writes proc_status:"stopped" into the doc_processing
// entry only when that entry currently reads "running". It is idempotent and
// handles the case where the doc-processor service is not running.
func (s StopRequestSQLStore) MarkDocProcessingStopped(ctx context.Context, id int64) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	raw, err := s.loadStatusRaw(ctx, id)
	if err != nil {
		return err
	}
	// Only update if doc_processing is currently "running".
	isRunning := false
	for _, e := range decodeDocMetaStatus(raw) {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		ps := strings.ToLower(strings.TrimSpace(asString(e["proc_status"])))
		if op == "doc_processing" && ps == "running" {
			isRunning = true
			break
		}
	}
	if !isRunning {
		return nil
	}
	statusJSON, err := appendPipelineStatus(raw, time.Now(), "stopped", nil)
	if err != nil {
		return err
	}
	return s.updateStatus(ctx, id, statusJSON)
}

func hasStopRequested(raw string) bool {
	for _, e := range decodeDocMetaStatus(raw) {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		ps := strings.ToLower(strings.TrimSpace(asString(e["proc_status"])))
		if op == stopRequestedOperation && ps == "pending" {
			return true
		}
	}
	return false
}

func (s StopRequestSQLStore) loadStatusRaw(ctx context.Context, id int64) (string, error) {
	const q = `SELECT COALESCE(status::text, '[]') FROM kb.inputs WHERE id = $1`
	var raw string
	err := s.DB.QueryRowContext(ctx, q, id).Scan(&raw)
	return raw, err
}

func (s StopRequestSQLStore) updateStatus(ctx context.Context, id int64, statusJSON string) error {
	const stmt = `UPDATE kb.inputs SET status = $2::jsonb, modify_time = NOW() WHERE id = $1`
	_, err := s.DB.ExecContext(ctx, stmt, id, statusJSON)
	return err
}
