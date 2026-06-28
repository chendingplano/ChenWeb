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
