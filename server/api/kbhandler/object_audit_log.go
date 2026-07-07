package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

const (
	objectAuditActionResolveObjectID = "resolve_object_id"
	objectAuditActionEditFields      = "edit_fields"
)

// logObjectAudit is a best-effort insert into kb.object_audit_log recording
// one successful PATCH against kb.artifact_objects or kb.object_nodes made
// from the Resolve Ambiguous Objects admin page. Failures are logged and
// swallowed so a logging problem never fails the caller's PATCH response,
// matching the docactivity.Log precedent used by the doc-structure editor
// (server/api/docactivity/activity.go).
func logObjectAudit(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger,
	tableName, rowKey, action, actor string, payload map[string]json.RawMessage) {
	changes, err := json.Marshal(payload)
	if err != nil {
		logger.Warn("marshal object audit changes failed", "table_name", tableName, "row_key", rowKey, "err", err)
		return
	}
	var actorArg any
	if actor != "" {
		actorArg = actor
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)`,
		tableName, rowKey, action, string(changes), actorArg)
	if err != nil {
		logger.Warn("insert object audit log failed", "table_name", tableName, "row_key", rowKey, "err", err)
		return
	}
	logger.Info("object audit logged", "table_name", tableName, "row_key", rowKey, "action", action)
}
