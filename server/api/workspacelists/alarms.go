package workspacelists

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

const alarmsDefaultLimit = 20
const alarmsHardCap = 200

var validAlarmStatuses = map[string]bool{"unsolved": true, "solved": true}

// AlarmNote is one entry in an alarm/error's append-only notes array.
type AlarmNote struct {
	Time string `json:"time"`
	User string `json:"user"`
	Note string `json:"note"`
}

// Alarm is one alarms_errors row. No i18n.
type Alarm struct {
	ID         int64       `json:"id"`
	OccurredAt string      `json:"occurred_at"`
	Severity   string      `json:"severity"`
	Message    string      `json:"message"`
	Status     string      `json:"status"`
	Notes      []AlarmNote `json:"notes"`
}

type alarmPatchRequest struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

// ListAlarms serves alarms/errors ordered by most recent first, optionally
// filtered to unsolved-only.
// Endpoint: GET /api/v1/workspace/alarms?unsolved_only=true&limit=<n>
func ListAlarms(c echo.Context) error {
	limit := alarmsDefaultLimit
	if raw := c.QueryParam("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > alarmsHardCap {
		limit = alarmsHardCap
	}
	unsolvedOnly := c.QueryParam("unsolved_only") == "true"

	query := `
		SELECT id, occurred_at, severity, message, status, notes::text
		FROM alarms_errors`
	args := []any{}
	if unsolvedOnly {
		query += ` WHERE status = 'unsolved'`
	}
	query += ` ORDER BY occurred_at DESC LIMIT $1`
	args = append(args, limit)

	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), query, args...)
	if err != nil {
		log.Printf("***** Alarm: list alarms/errors failed: %v (CWB_WSL_301)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load alarms/errors"})
	}
	defer rows.Close()

	out := []Alarm{}
	for rows.Next() {
		var a Alarm
		var occurredAt time.Time
		var notesRaw string
		if err := rows.Scan(&a.ID, &occurredAt, &a.Severity, &a.Message, &a.Status, &notesRaw); err != nil {
			log.Printf("***** Alarm: scan alarm/error failed: %v (CWB_WSL_302)", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load alarms/errors"})
		}
		a.OccurredAt = occurredAt.UTC().Format(time.RFC3339)
		a.Notes = []AlarmNote{}
		if notesRaw != "" {
			if err := json.Unmarshal([]byte(notesRaw), &a.Notes); err != nil {
				log.Printf("***** Alarm: parse alarm/error notes failed id=%d: %v (CWB_WSL_303)", a.ID, err)
			}
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		log.Printf("***** Alarm: list alarms/errors rows error: %v (CWB_WSL_304)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load alarms/errors"})
	}

	return c.JSON(http.StatusOK, map[string]any{"alarms": out})
}

// UpdateAlarm updates status and/or appends a note to an alarm/error. All
// other fields (occurred_at, severity, message) are read-only and cannot be
// changed through this endpoint.
// Endpoint: PATCH /api/v1/workspace/alarms/:id
func UpdateAlarm(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}
	var req alarmPatchRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request payload"})
	}
	status := strings.TrimSpace(req.Status)
	note := strings.TrimSpace(req.Note)
	if status == "" && note == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "status or note is required"})
	}
	if status != "" && !validAlarmStatuses[status] {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "status must be 'unsolved' or 'solved'"})
	}

	rc := EchoFactory.NewFromEcho(c, "CWB_WSL_310")
	defer rc.Close()
	actor := actorEmail(rc)
	ctx := c.Request().Context()

	setClauses := []string{"updated_at = NOW()"}
	args := []any{}
	argN := 1
	if status != "" {
		setClauses = append(setClauses, "status = $"+strconv.Itoa(argN))
		args = append(args, status)
		argN++
	}
	if note != "" {
		noteJSON, err := json.Marshal(AlarmNote{
			Time: time.Now().UTC().Format(time.RFC3339),
			User: actor,
			Note: note,
		})
		if err != nil {
			log.Printf("***** Alarm: marshal alarm note failed id=%d: %v (CWB_WSL_311)", id, err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update alarm/error"})
		}
		setClauses = append(setClauses, "notes = notes || jsonb_build_array($"+strconv.Itoa(argN)+"::jsonb)")
		args = append(args, string(noteJSON))
		argN++
	}
	args = append(args, id)

	query := "UPDATE alarms_errors SET " + strings.Join(setClauses, ", ") + " WHERE id = $" + strconv.Itoa(argN)
	res, err := ApiTypes.ProjectDBHandle.ExecContext(ctx, query, args...)
	if err != nil {
		log.Printf("***** Alarm: update alarm/error failed id=%d: %v (CWB_WSL_312)", id, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update alarm/error"})
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "alarm/error not found"})
	}

	log.Printf("Updated alarm/error id=%d status=%q note_added=%v actor=%q (CWB_WSL_320)", id, status, note != "", actor)
	return c.JSON(http.StatusOK, map[string]any{"updated": true})
}
