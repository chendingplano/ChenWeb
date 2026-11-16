package datasync

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

var pullLogger = loggerutil.CreateDefaultLogger("CWB_DSYNC_501")

// defaultPullLimit is sized well above kb_product_names's expected row count
// (a few thousand at most), so pagination normally never triggers.
const defaultPullLimit = 5000

// changesResponse is the source's response to a target's pull request.
// next_cursor is the maximum CursorCol value among the returned rows (or the
// request's `since`, unchanged, if the page is empty) -- the target must
// persist exactly this value as its next starting point.
type changesResponse struct {
	ItemID     string `json:"item_id"`
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
	Rows       []Row  `json:"rows"`
}

// authenticatePullRequest checks the shared-secret bearer token shared by
// every endpoint in this file (mirrors shared/go/api/auth/sms_relay.go):
// constant-time compare, fails closed if unconfigured. Writes the
// appropriate JSON error response itself on failure; the caller should
// return immediately when this returns false.
func authenticatePullRequest(c echo.Context) bool {
	sharedSecret := strings.TrimSpace(os.Getenv("DATA_SYNC_SHARED_SECRET"))
	if sharedSecret == "" {
		pullLogger.Error("DATA_SYNC_SHARED_SECRET is not configured; refusing all pull requests")
		c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "data sync source is not configured",
		})
		return false
	}
	presented := strings.TrimPrefix(c.Request().Header.Get(echo.HeaderAuthorization), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(presented), []byte(sharedSecret)) != 1 {
		pullLogger.Warn("rejected data-sync request with invalid shared secret")
		c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	return true
}

// HandlePullChanges serves GET /api/internal/data-sync/items/:itemId/changes.
// It is called by a deployed target reaching out to this instance acting as
// the sync source -- there is no session/cookie auth available, so access is
// restricted with a shared-secret header instead.
func HandlePullChanges(c echo.Context) error {
	if !authenticatePullRequest(c) {
		return nil
	}

	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "project database is not initialized"})
	}

	itemID := c.Param("itemId")
	item, ok, err := ResolveItem(c.Request().Context(), ApiTypes.ProjectDBHandle, itemID)
	if err != nil {
		pullLogger.Error("failed to resolve sync item", "item_id", itemID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to resolve sync item"})
	}
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "unknown sync item: " + itemID})
	}

	since := c.QueryParam("since")
	limit := defaultPullLimit
	if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}

	resp, err := fetchChangesPage(ApiTypes.ProjectDBHandle, item, since, limit)
	if err != nil {
		pullLogger.Error("failed to fetch data-sync changes page", "item_id", itemID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch changes"})
	}
	return c.JSON(http.StatusOK, resp)
}

// fetchChangesPage runs the source-side "changed since" query for item and
// returns at most limit rows, ordered by (CursorCol, NaturalKey...) for
// deterministic pagination boundaries.
func fetchChangesPage(db *sql.DB, item TableSyncItem, since string, limit int) (changesResponse, error) {
	selectCols := make([]string, len(item.Columns))
	for i, col := range item.Columns {
		selectCols[i] = col
		ref, ok := item.referenceFor(col)
		if !ok {
			continue
		}
		expr, err := referenceSelectExpr(item, ref)
		if err != nil {
			return changesResponse{}, err
		}
		selectCols[i] = expr
	}

	var whereClauses []string
	if item.Filter != "" {
		whereClauses = append(whereClauses, "("+item.Filter+")")
	}
	whereClauses = append(whereClauses, fmt.Sprintf("%s > $1", item.CursorCol))

	orderBy := append([]string{item.CursorCol}, item.NaturalKey...)

	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT $2",
		strings.Join(selectCols, ", "),
		item.Table,
		strings.Join(whereClauses, " AND "),
		strings.Join(orderBy, ", "),
	)

	// An empty `since` means "never synced" -- bind a sentinel that Postgres
	// can parse as a timestamptz literal (unlike ""), and that is always
	// less than every real cursor value.
	sinceArg := since
	if sinceArg == "" {
		sinceArg = "-infinity"
	}
	resp := changesResponse{ItemID: item.ID, NextCursor: since, Rows: []Row{}}
	pageRows, lastCursor, err := queryChangeRows(db, item, query, sinceArg, limit)
	if err != nil {
		return changesResponse{}, err
	}
	resp.Rows = append(resp.Rows, pageRows...)
	if len(pageRows) > 0 {
		resp.NextCursor = lastCursor
	}
	resp.HasMore = len(pageRows) == limit

	// The target resumes with `CursorCol > next_cursor`, so a page must never
	// end inside a group of rows that share one cursor value: the rest of that
	// group would be skipped for good. Bulk writes make such groups routine (an
	// import or a release stamps thousands of rows with one NOW()), so a full
	// page is completed with every remaining row at its final cursor value. The
	// page may therefore exceed limit.
	if len(pageRows) == limit && lastCursor != "" {
		tieWhere := make([]string, 0, 2)
		if item.Filter != "" {
			tieWhere = append(tieWhere, "("+item.Filter+")")
		}
		tieWhere = append(tieWhere, fmt.Sprintf("%s = $1", item.CursorCol))
		tieQuery := fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s ORDER BY %s",
			strings.Join(selectCols, ", "),
			item.Table,
			strings.Join(tieWhere, " AND "),
			strings.Join(orderBy, ", "),
		)
		tieRows, _, err := queryChangeRows(db, item, tieQuery, lastCursor)
		if err != nil {
			return changesResponse{}, err
		}
		seen := make(map[string]bool, len(pageRows))
		for _, row := range pageRows {
			seen[naturalKeyID(item, row)] = true
		}
		for _, row := range tieRows {
			if !seen[naturalKeyID(item, row)] {
				resp.Rows = append(resp.Rows, row)
			}
		}
	}
	return resp, nil
}

// queryChangeRows runs one change query and scans every row, also returning
// the cursor value of the last row.
func queryChangeRows(db *sql.DB, item TableSyncItem, query string, args ...any) ([]Row, string, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("query changes for %s: %w", item.ID, err)
	}
	defer rows.Close()

	var (
		out        []Row
		lastCursor string
	)
	for rows.Next() {
		row, cursorValue, err := scanRow(rows, item)
		if err != nil {
			return nil, "", fmt.Errorf("scan row for %s: %w", item.ID, err)
		}
		out = append(out, row)
		lastCursor = cursorValue
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate changes for %s: %w", item.ID, err)
	}
	return out, lastCursor, nil
}

// naturalKeyID identifies a scanned row by its natural-key values, as sent on
// the wire.
func naturalKeyID(item TableSyncItem, row Row) string {
	parts := make([]string, len(item.NaturalKey))
	for i, col := range item.NaturalKey {
		parts[i] = string(row[col])
	}
	return strings.Join(parts, "\x00")
}

// referenceSelectExpr selects a reference column's parent natural key as a
// JSON array of text values, in the parent's NaturalKey order, in place of the
// local surrogate id.
func referenceSelectExpr(item TableSyncItem, ref SyncReference) (string, error) {
	parent, ok := ItemByID(ref.ItemID)
	if !ok {
		return "", fmt.Errorf("reference %s.%s: parent item %q is not a compiled item", item.ID, ref.Column, ref.ItemID)
	}
	parts := make([]string, len(parent.NaturalKey))
	for i, col := range parent.NaturalKey {
		parts[i] = fmt.Sprintf("p.%s::text", col)
	}
	return fmt.Sprintf(
		"(SELECT json_build_array(%s) FROM %s p WHERE p.id = %s.%s)",
		strings.Join(parts, ", "),
		parent.Table,
		item.Table,
		ref.Column,
	), nil
}

// scanRow scans one row of item.Columns into a Row and also returns the raw
// text value of the cursor column (for tracking NextCursor without a JSON
// round trip).
func scanRow(rows *sql.Rows, item TableSyncItem) (Row, string, error) {
	dests := make([]any, len(item.Columns))
	// []byte, not json.RawMessage: database/sql cannot scan a NULL into a
	// *json.RawMessage, but a nil []byte is how a NULL jsonb value (or a NULL
	// reference) arrives here, and is turned into JSON null below.
	jsonDests := make([][]byte, len(item.Columns))
	strDests := make([]sql.NullString, len(item.Columns))
	for i, col := range item.Columns {
		if _, isRef := item.referenceFor(col); isRef || item.isJSONColumn(col) {
			dests[i] = &jsonDests[i]
		} else {
			dests[i] = &strDests[i]
		}
	}
	if err := rows.Scan(dests...); err != nil {
		return nil, "", err
	}

	row := make(Row, len(item.Columns))
	var cursorValue string
	for i, col := range item.Columns {
		var value json.RawMessage
		if _, isRef := item.referenceFor(col); isRef || item.isJSONColumn(col) {
			if len(jsonDests[i]) == 0 {
				if item.isJSONColumn(col) {
					// SQL NULL in a JSON column is sent as an absent key, so it
					// stays distinguishable from the jsonb value `null`, which
					// is sent as the JSON literal null (see rowArgs).
					continue
				}
				value = nullJSON
			} else {
				value = json.RawMessage(jsonDests[i])
			}
		} else {
			if strDests[i].Valid {
				encoded, err := json.Marshal(strDests[i].String)
				if err != nil {
					return nil, "", err
				}
				value = encoded
				if col == item.CursorCol {
					cursorValue = strDests[i].String
				}
			} else {
				value = nullJSON
			}
		}
		row[col] = value
	}
	return row, cursorValue, nil
}
