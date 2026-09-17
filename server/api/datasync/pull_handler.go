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
	copy(selectCols, item.Columns)

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
	rows, err := db.Query(query, sinceArg, limit)
	if err != nil {
		return changesResponse{}, fmt.Errorf("query changes for %s: %w", item.ID, err)
	}
	defer rows.Close()

	resp := changesResponse{ItemID: item.ID, NextCursor: since, Rows: []Row{}}
	rowCount := 0
	for rows.Next() {
		row, cursorValue, err := scanRow(rows, item)
		if err != nil {
			return changesResponse{}, fmt.Errorf("scan row for %s: %w", item.ID, err)
		}
		resp.Rows = append(resp.Rows, row)
		resp.NextCursor = cursorValue
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return changesResponse{}, fmt.Errorf("iterate changes for %s: %w", item.ID, err)
	}
	resp.HasMore = rowCount == limit
	return resp, nil
}

// scanRow scans one row of item.Columns into a Row and also returns the raw
// text value of the cursor column (for tracking NextCursor without a JSON
// round trip).
func scanRow(rows *sql.Rows, item TableSyncItem) (Row, string, error) {
	dests := make([]any, len(item.Columns))
	jsonDests := make([]json.RawMessage, len(item.Columns))
	strDests := make([]sql.NullString, len(item.Columns))
	for i, col := range item.Columns {
		if item.isJSONColumn(col) {
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
		if item.isJSONColumn(col) {
			if len(jsonDests[i]) == 0 {
				value = nullJSON
			} else {
				value = jsonDests[i]
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
