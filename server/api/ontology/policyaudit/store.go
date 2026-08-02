// Package policyaudit is the neutral, dependency-free P5 audit-event sink
// (spec 2026080102 section 10). It exists purely so doc-processing,
// ontology/modules, and kbhandler can all emit stable-id policy/routing
// events into the append-only kb.pipeline_policy_events log without any of
// them importing each other: this package imports nothing from those
// packages, matching the RoutingClearanceStore/PipelineGateSQLStore style
// already used in doc-processing (a plain struct wrapping *sql.DB, with
// context-taking methods and no framework dependency).
package policyaudit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// Stable event kind identifiers. Every kind here is wired by task E3; G/H
// tasks add classifier and module-promotion kinds later through this same
// Writer.
const (
	EventRuleAuthored      = "rule_authored"
	EventBindingAuthored   = "binding_authored"
	EventPolicyActivated   = "policy_activated"
	EventBindingConflict   = "binding_conflict"
	EventGateConflict      = "gate_conflict"
	EventFallbackApplied   = "fallback_applied"
	EventClearanceApproved = "clearance_approved"
	EventClearanceRevoked  = "clearance_revoked"
	EventDecisionEnforced  = "decision_enforced"
	EventDecisionShadowed  = "decision_shadowed"
)

// Event is one append-only kb.pipeline_policy_events row. Detail must stay
// content-safe -- ids, checksums, booleans, processor/pipeline names -- and
// must never carry document content, matching spec 2026080102 section 10's
// "predicate values and traces must not log document content" requirement.
type Event struct {
	Kind          string
	PolicyID      int64
	PolicyVersion int
	SubjectKind   string
	SubjectID     int64
	RunID         int64
	RecordID      int64
	Actor         string
	Detail        map[string]any
}

// Writer persists policy/routing audit events.
type Writer interface {
	WriteEvent(ctx context.Context, event Event) error
}

// SQLStore is the production Writer, backed by kb.pipeline_policy_events.
type SQLStore struct{ DB *sql.DB }

func (s SQLStore) WriteEvent(ctx context.Context, event Event) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	kind := strings.TrimSpace(event.Kind)
	if kind == "" {
		return errors.New("event kind is required")
	}
	detail := event.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	detailRaw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO kb.pipeline_policy_events
(event_kind, policy_id, policy_version, subject_kind, subject_id, run_id, record_id, actor, detail)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`,
		kind, nullIfZeroInt64(event.PolicyID), nullIfZeroInt(event.PolicyVersion), nullIfEmpty(event.SubjectKind),
		nullIfZeroInt64(event.SubjectID), nullIfZeroInt64(event.RunID), nullIfZeroInt64(event.RecordID),
		nullIfEmpty(strings.TrimSpace(event.Actor)), string(detailRaw))
	return err
}

func nullIfZeroInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullIfZeroInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}
