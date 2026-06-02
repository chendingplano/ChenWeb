package kbhandler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// wikiCounts holds the corpus-wide totals rendered around the SemOS hero image
// in the Deep Wiki entrance (Panel A).
type wikiCounts struct {
	Documents           int64 `json:"documents"`
	ContentSegments     int64 `json:"content_segments"`
	Topics              int64 `json:"topics"`
	SemanticProjections int64 `json:"semantic_projections"`
	Metrics             int64 `json:"metrics"`
	Provisions          int64 `json:"provisions"`
	PartsComponents     int64 `json:"parts_components"`
	Scenes              int64 `json:"scenes"`
	Entities            int64 `json:"entities"`
	Relations           int64 `json:"relations"`
}

// wikiRecentDoc is one row in the "Recent Adds" and "Recent Edits" lists (Panel C).
type wikiRecentDoc struct {
	ID    int64      `json:"id"`
	Title string     `json:"title"`
	Type  string     `json:"type"`
	Time  *time.Time `json:"time"`
}

// wikiProcessed is one row in the "Recent Processed" list (Panel C).
type wikiProcessed struct {
	RecordID  *int64    `json:"record_id"`
	Title     string    `json:"title"`
	Processor string    `json:"processor"`
	Time      time.Time `json:"time"`
}

// wikiError is one row in the "Errors" list (Panel C).
type wikiError struct {
	RecordID  *int64    `json:"record_id"`
	Title     string    `json:"title"`
	Processor string    `json:"processor"`
	Message   string    `json:"message"`
	Time      time.Time `json:"time"`
}

type wikiOverviewResponse struct {
	Status    bool            `json:"status"`
	Counts    wikiCounts      `json:"counts"`
	Adds      []wikiRecentDoc `json:"recent_adds"`
	Edits     []wikiRecentDoc `json:"recent_edits"`
	Processed []wikiProcessed `json:"recent_processed"`
	Errors    []wikiError     `json:"errors"`
}

// recentLimit caps each Panel C activity list. Kept small so the entrance loads
// fast and the lists stay glanceable rather than becoming a full feed.
const recentLimit = 8

// WikiOverview handles GET /api/v1/kb/wiki-overview.
//
// It aggregates the corpus-wide artifact counts and the most recent activity
// (adds, edits, processed, errors) that power the SemOS Deep Wiki entrance page.
// Each sub-query is isolated: a single failing count or list degrades to its
// zero value rather than failing the whole response, so the entrance always
// renders.
func WikiOverview(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_WIKI_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "database handle unavailable (CWB_KB_WIKI_010)",
		})
	}

	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to resolve kb input table (CWB_KB_WIKI_011)",
		})
	}

	counts := wikiCounts{
		Documents:           countRows(db, logger, "SELECT COUNT(*) FROM "+inputTable),
		ContentSegments:     countRows(db, logger, "SELECT COUNT(*) FROM kb.chunks"),
		Topics:              countRows(db, logger, "SELECT COUNT(*) FROM kb.topics"),
		SemanticProjections: countRows(db, logger, "SELECT COUNT(*) FROM kb.semantic_projections"),
		Metrics:             countRows(db, logger, "SELECT COUNT(*) FROM kb.metrics"),
		Provisions:          countRows(db, logger, "SELECT COUNT(*) FROM kb.provisions"),
		PartsComponents:     countRows(db, logger, "SELECT COUNT(*) FROM kb.products"),
		Scenes:              countRows(db, logger, "SELECT COUNT(*) FROM kb.scene_objects"),
		Entities:            countRows(db, logger, "SELECT COUNT(*) FROM kb.entities"),
		Relations:           countRows(db, logger, "SELECT COUNT(*) FROM kb.relations"),
	}

	resp := wikiOverviewResponse{
		Status:    true,
		Counts:    counts,
		Adds:      queryRecentDocs(db, logger, inputTable, "create_time"),
		Edits:     queryRecentDocs(db, logger, inputTable, "modify_time"),
		Processed: queryRecentProcessed(db, logger, inputTable),
		Errors:    queryRecentErrors(db, logger, inputTable),
	}

	return c.JSON(http.StatusOK, resp)
}

// countRows runs a COUNT(*) query and returns 0 on any error so one missing or
// empty table never blocks the rest of the overview.
func countRows(db *sql.DB, logger interface{ Warn(string, ...any) }, query string) int64 {
	var n int64
	if err := db.QueryRow(query).Scan(&n); err != nil {
		logger.Warn("wiki overview count failed", "query", query, "err", err)
		return 0
	}
	return n
}

// queryRecentDocs returns the most recently created or modified documents,
// ordered by the given timestamp column (create_time or modify_time).
func queryRecentDocs(db *sql.DB, logger interface{ Warn(string, ...any) }, inputTable, orderCol string) []wikiRecentDoc {
	// orderCol is a fixed internal value (create_time | modify_time), never user input.
	query := `
SELECT i.id,
       COALESCE(NULLIF(TRIM(i.title), ''), NULLIF(TRIM(i.file_name), ''), 'Untitled document') AS title,
       COALESCE(i.type, '') AS type,
       i.` + orderCol + ` AS ts
FROM ` + inputTable + ` i
ORDER BY i.` + orderCol + ` DESC
LIMIT $1`

	rows, err := db.Query(query, recentLimit)
	if err != nil {
		logger.Warn("wiki overview recent docs failed", "order_col", orderCol, "err", err)
		return []wikiRecentDoc{}
	}
	defer rows.Close()

	out := make([]wikiRecentDoc, 0, recentLimit)
	for rows.Next() {
		var d wikiRecentDoc
		var ts sql.NullTime
		if scanErr := rows.Scan(&d.ID, &d.Title, &d.Type, &ts); scanErr != nil {
			logger.Warn("wiki overview recent doc scan failed", "err", scanErr)
			continue
		}
		if ts.Valid {
			t := ts.Time
			d.Time = &t
		}
		out = append(out, d)
	}
	return out
}

// queryRecentProcessed returns the most recent successful doc-processor summary
// entries, joined to their source document title when the record still exists.
func queryRecentProcessed(db *sql.DB, logger interface{ Warn(string, ...any) }, inputTable string) []wikiProcessed {
	query := `
SELECT l.record_id,
       COALESCE(NULLIF(TRIM(i.title), ''), NULLIF(TRIM(i.file_name), ''), 'Document') AS title,
       l.doc_proc_name,
       l.create_time
FROM kb.doc_proc_logs l
LEFT JOIN ` + inputTable + ` i ON i.id = l.record_id
WHERE l.entry_type = 'doc_proc_summary'
  AND (l.errors IS NULL OR TRIM(l.errors) = '')
ORDER BY l.create_time DESC
LIMIT $1`

	rows, err := db.Query(query, recentLimit)
	if err != nil {
		logger.Warn("wiki overview recent processed failed", "err", err)
		return []wikiProcessed{}
	}
	defer rows.Close()

	out := make([]wikiProcessed, 0, recentLimit)
	for rows.Next() {
		var p wikiProcessed
		var recordID sql.NullInt64
		if scanErr := rows.Scan(&recordID, &p.Title, &p.Processor, &p.Time); scanErr != nil {
			logger.Warn("wiki overview processed scan failed", "err", scanErr)
			continue
		}
		if recordID.Valid {
			id := recordID.Int64
			p.RecordID = &id
		}
		out = append(out, p)
	}
	return out
}

// queryRecentErrors returns the most recent doc-processor log entries that
// carry an error, joined to the source document title when available.
func queryRecentErrors(db *sql.DB, logger interface{ Warn(string, ...any) }, inputTable string) []wikiError {
	query := `
SELECT l.record_id,
       COALESCE(NULLIF(TRIM(i.title), ''), NULLIF(TRIM(i.file_name), ''), 'Document') AS title,
       l.doc_proc_name,
       l.errors,
       l.create_time
FROM kb.doc_proc_logs l
LEFT JOIN ` + inputTable + ` i ON i.id = l.record_id
WHERE l.errors IS NOT NULL AND TRIM(l.errors) <> ''
ORDER BY l.create_time DESC
LIMIT $1`

	rows, err := db.Query(query, recentLimit)
	if err != nil {
		logger.Warn("wiki overview recent errors failed", "err", err)
		return []wikiError{}
	}
	defer rows.Close()

	out := make([]wikiError, 0, recentLimit)
	for rows.Next() {
		var e wikiError
		var recordID sql.NullInt64
		if scanErr := rows.Scan(&recordID, &e.Title, &e.Processor, &e.Message, &e.Time); scanErr != nil {
			logger.Warn("wiki overview error scan failed", "err", scanErr)
			continue
		}
		if recordID.Valid {
			id := recordID.Int64
			e.RecordID = &id
		}
		out = append(out, e)
	}
	return out
}
