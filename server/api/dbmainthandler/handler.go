package dbmainthandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

var logger = loggerutil.CreateDefaultLogger("CWB_DBMH_001")

const orphanedLabelOperation = "resolve-orphaned-ontology-term-labels"

// OrphanedLabelRow is a label row whose term_id no longer exists in the
// governed ontology term catalog.
type OrphanedLabelRow struct {
	ID         int64     `json:"id"`
	TermID     string    `json:"term_id"`
	Label      string    `json:"label"`
	Lang       string    `json:"lang"`
	LabelRole  string    `json:"label_role"`
	Status     string    `json:"status"`
	CreateTime time.Time `json:"create_time"`
	CreateBy   string    `json:"create_by"`
	ModifyTime time.Time `json:"modify_time"`
	ModifyBy   string    `json:"modify_by"`
}

type resolveOrphanedLabelsRequest struct {
	IDs       []int64 `json:"ids"`
	Query     string  `json:"q"`
	Lang      string  `json:"lang"`
	LabelRole string  `json:"label_role"`
}

func orphanedLabelsWhere(startArg int, query, lang, labelRole string) (string, []any) {
	conditions := []string{`NOT EXISTS (SELECT 1 FROM kb.ontology_terms t WHERE t.term_id = l.term_id)`}
	args := make([]any, 0, 3)
	n := startArg
	if query != "" {
		conditions = append(conditions, fmt.Sprintf(`(l.term_id ILIKE $%d OR l.label ILIKE $%d OR l.lang ILIKE $%d)`, n, n, n))
		args = append(args, "%"+query+"%")
		n++
	}
	if lang != "" {
		conditions = append(conditions, fmt.Sprintf("l.lang = $%d", n))
		args = append(args, lang)
		n++
	}
	if labelRole != "" {
		conditions = append(conditions, fmt.Sprintf("l.label_role = $%d", n))
		args = append(args, labelRole)
	}
	return strings.Join(conditions, " AND "), args
}

// ListOrphanedLabels handles GET /api/v1/admin/db/ontology-term-labels/orphans.
func ListOrphanedLabels(c echo.Context) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
	}

	query := strings.TrimSpace(c.QueryParam("q"))
	lang := strings.TrimSpace(c.QueryParam("lang"))
	labelRole := strings.TrimSpace(c.QueryParam("label_role"))
	where, args := orphanedLabelsWhere(1, query, lang, labelRole)

	var total int64
	countQ := "SELECT count(*) FROM kb.ontology_term_labels l WHERE " + where
	if err := db.QueryRowContext(c.Request().Context(), countQ, args...).Scan(&total); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	dataQ := `SELECT l.id, l.term_id, l.label, l.lang, l.label_role, l.status,
 l.create_time, COALESCE(l.create_by, ''), l.modify_time, COALESCE(l.modify_by, '')
 FROM kb.ontology_term_labels l WHERE ` + where + `
 ORDER BY l.term_id, l.lang, l.label_role, l.id`
	rows, err := db.QueryContext(c.Request().Context(), dataQ, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer rows.Close()

	results := make([]OrphanedLabelRow, 0)
	for rows.Next() {
		var row OrphanedLabelRow
		if err := rows.Scan(&row.ID, &row.TermID, &row.Label, &row.Lang, &row.LabelRole, &row.Status,
			&row.CreateTime, &row.CreateBy, &row.ModifyTime, &row.ModifyBy); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": results, "total": total})
}

// ResolveOrphanedLabels handles POST /api/v1/admin/db/ontology-term-labels/orphans/resolve.
func ResolveOrphanedLabels(c echo.Context) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
	}
	var req resolveOrphanedLabelsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	ids := uniquePositiveIDs(req.IDs)
	if len(ids) == 0 {
		return c.JSON(http.StatusOK, map[string]any{"status": true, "deleted_count": 0})
	}

	tx, err := db.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	deleteQ := `DELETE FROM kb.ontology_term_labels l
WHERE l.id IN (` + strings.Join(placeholders, ", ") + `)
  AND NOT EXISTS (SELECT 1 FROM kb.ontology_terms t WHERE t.term_id = l.term_id)`
	result, err := tx.ExecContext(c.Request().Context(), deleteQ, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	resultData, err := json.Marshal(map[string]any{
		"deleted_count": deleted,
		"ids":           ids,
		"q":             req.Query,
		"lang":          req.Lang,
		"label_role":    req.LabelRole,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if _, err := tx.ExecContext(c.Request().Context(),
		`INSERT INTO kb.db_maintenance_logs (operation, result_data) VALUES ($1, $2)`,
		orphanedLabelOperation, string(resultData)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if err := tx.Commit(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	committed = true
	return c.JSON(http.StatusOK, map[string]any{"status": true, "deleted_count": deleted})
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				result = append(result, id)
			}
		}
	}
	return result
}

// CheckKbInputsStatus counts kb.inputs rows where the status JSONB array
// contains duplicate entries for the same operation name. These duplicates
// are produced by the appendEntityRelationStatus bug (fixed in the server)
// and cause the frontend to show stale in-progress status.
func CheckKbInputsStatus(c echo.Context) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
	}

	const q = `
SELECT count(*)
FROM (
    SELECT i.id
    FROM kb.inputs i,
    LATERAL jsonb_array_elements(coalesce(i.status, '[]'::jsonb)) AS t(elem)
    WHERE i.status IS NOT NULL
      AND jsonb_array_length(i.status) > 0
    GROUP BY i.id, lower(elem->>'operation')
    HAVING count(*) > 1
) sub`

	var staleCount int64
	if err := db.QueryRowContext(c.Request().Context(), q).Scan(&staleCount); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	result := map[string]any{"stale_count": staleCount}
	if err := insertMaintenanceLog(c, "check-kb-inputs-status", result); err != nil {
		logger.Warn("failed to log maintenance operation", "op", "check-kb-inputs-status", "error", err)
	}
	return c.JSON(http.StatusOK, result)
}

// FixKbInputsStatus deduplicates the status JSONB array on each affected
// kb.inputs row, keeping the last entry per operation name (matching the
// DISTINCT ON ordering used by the trg_sync_input_proc_status trigger).
// The trigger fires automatically for each updated row, syncing
// kb.input_proc_status and the rollup columns (parse_state, pipeline_state).
func FixKbInputsStatus(c echo.Context) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
	}

	const q = `
WITH deduped AS (
    SELECT
        i.id,
        jsonb_agg(d.elem ORDER BY d.ord) AS new_status
    FROM kb.inputs i,
    LATERAL (
        SELECT DISTINCT ON (lower(elem->>'operation'))
            elem, ord
        FROM jsonb_array_elements(coalesce(i.status, '[]'::jsonb)) WITH ORDINALITY AS t(elem, ord)
        ORDER BY lower(elem->>'operation'), ord DESC
    ) d
    WHERE i.status IS NOT NULL AND jsonb_array_length(i.status) > 1
    GROUP BY i.id
)
UPDATE kb.inputs
SET status = deduped.new_status
FROM deduped
WHERE kb.inputs.id = deduped.id
  AND kb.inputs.status IS DISTINCT FROM deduped.new_status`

	res, err := db.ExecContext(c.Request().Context(), q)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	n, _ := res.RowsAffected()

	result := map[string]any{"fixed_count": n}
	if err := insertMaintenanceLog(c, "fix-kb-inputs-status", result); err != nil {
		logger.Warn("failed to log maintenance operation", "op", "fix-kb-inputs-status", "error", err)
	}
	return c.JSON(http.StatusOK, result)
}

// MaintenanceLogRow is a single row from kb.db_maintenance_logs.
type MaintenanceLogRow struct {
	ID          int64     `json:"id"`
	Operation   string    `json:"operation"`
	ResultData  any       `json:"result_data"`
	PerformedAt time.Time `json:"performed_at"`
}

// ListMaintenanceLogs handles GET /api/v1/admin/db/maintenance-logs.
//
// Query params:
//
//	operation  - filter by exact operation name
//	date_from  - RFC3339 lower bound on performed_at (inclusive)
//	date_to    - RFC3339 upper bound on performed_at (inclusive)
//	page       - 1-based page number (default 1)
//	page_size  - rows per page (default 50, max 500)
func ListMaintenanceLogs(c echo.Context) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
	}

	page := parsePositiveInt(c.QueryParam("page"), 1)
	pageSize := parsePositiveInt(c.QueryParam("page_size"), 50)
	pageSize = min(pageSize, 500)
	offset := (page - 1) * pageSize

	operation := strings.TrimSpace(c.QueryParam("operation"))
	dateFrom := strings.TrimSpace(c.QueryParam("date_from"))
	dateTo := strings.TrimSpace(c.QueryParam("date_to"))

	var (
		args    []any
		filters []string
		argIdx  = 1
	)
	if operation != "" {
		filters = append(filters, fmt.Sprintf("operation = $%d", argIdx))
		args = append(args, operation)
		argIdx++
	}
	if dateFrom != "" {
		filters = append(filters, fmt.Sprintf("performed_at >= $%d", argIdx))
		args = append(args, dateFrom)
		argIdx++
	}
	if dateTo != "" {
		filters = append(filters, fmt.Sprintf("performed_at <= $%d", argIdx))
		args = append(args, dateTo)
		argIdx++
	}

	where := ""
	if len(filters) > 0 {
		where = "WHERE " + strings.Join(filters, " AND ")
	}

	countQ := fmt.Sprintf("SELECT count(*) FROM kb.db_maintenance_logs %s", where)
	var total int64
	if err := db.QueryRowContext(c.Request().Context(), countQ, args...).Scan(&total); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	dataQ := fmt.Sprintf(
		`SELECT id, operation, result_data, performed_at
		 FROM kb.db_maintenance_logs %s
		 ORDER BY performed_at DESC
		 LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, pageSize, offset)

	rows, err := db.QueryContext(c.Request().Context(), dataQ, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer rows.Close()

	results := make([]MaintenanceLogRow, 0)
	for rows.Next() {
		var r MaintenanceLogRow
		var resultJSON []byte
		if err := rows.Scan(&r.ID, &r.Operation, &resultJSON, &r.PerformedAt); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		if err := json.Unmarshal(resultJSON, &r.ResultData); err != nil {
			r.ResultData = string(resultJSON)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"results":   results,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// insertMaintenanceLog records a completed maintenance operation.
func insertMaintenanceLog(c echo.Context, operation string, result map[string]any) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("database unavailable")
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	_, err = db.ExecContext(
		c.Request().Context(),
		`INSERT INTO kb.db_maintenance_logs (operation, result_data) VALUES ($1, $2)`,
		operation, string(resultJSON),
	)
	return err
}

func parsePositiveInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return def
	}
	return v
}
