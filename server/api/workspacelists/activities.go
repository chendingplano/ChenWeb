package workspacelists

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

const recentActivitiesCap = 20
const recentActivitiesAdminHardCap = 500

// RecentActivity is one localized recent-activity row for /semos/workspace.
type RecentActivity struct {
	GroupID      int64  `json:"group_id"`
	OccurredAt   string `json:"occurred_at"`
	ActivityType string `json:"activity_type"`
	Text         string `json:"text"`
}

// RecentActivityAdmin is one logical activity entry with every configured
// language's text, for /semos/admin/recent-activities.
type RecentActivityAdmin struct {
	GroupID      int64             `json:"group_id"`
	OccurredAt   string            `json:"occurred_at"`
	ActivityType string            `json:"activity_type"`
	Translations map[string]string `json:"translations"`
}

type activityRequest struct {
	OccurredAt   string            `json:"occurred_at"`
	ActivityType string            `json:"activity_type"`
	Translations map[string]string `json:"translations"`
}

// ListRecentActivities serves the locale-filtered, fixed-cap recent
// activities list.
// Endpoint: GET /api/v1/workspace/recent-activities?lang=<code>
func ListRecentActivities(c echo.Context) error {
	lang := normalizeLang(c.QueryParam("lang"))

	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), `
		SELECT group_id, occurred_at, activity_type, activity_text
		FROM kb.recent_activities
		WHERE lang = $1
		ORDER BY occurred_at DESC
		LIMIT $2`, lang, recentActivitiesCap)
	if err != nil {
		log.Printf("***** Alarm: list recent activities failed lang=%q: %v (CWB_WSL_101)", lang, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load recent activities"})
	}
	defer rows.Close()

	out := []RecentActivity{}
	for rows.Next() {
		var a RecentActivity
		var occurredAt time.Time
		if err := rows.Scan(&a.GroupID, &occurredAt, &a.ActivityType, &a.Text); err != nil {
			log.Printf("***** Alarm: scan recent activity failed: %v (CWB_WSL_102)", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load recent activities"})
		}
		a.OccurredAt = occurredAt.UTC().Format(time.RFC3339)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		log.Printf("***** Alarm: list recent activities rows error: %v (CWB_WSL_103)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load recent activities"})
	}

	return c.JSON(http.StatusOK, map[string]any{"recent_activities": out})
}

// ListRecentActivitiesAdmin serves every activity entry with all of its
// translations grouped together, for /semos/admin/recent-activities.
// Endpoint: GET /api/v1/workspace/recent-activities/admin
func ListRecentActivitiesAdmin(c echo.Context) error {
	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), `
		SELECT group_id, lang, occurred_at, activity_type, activity_text
		FROM kb.recent_activities
		ORDER BY occurred_at DESC, group_id DESC
		LIMIT $1`, recentActivitiesAdminHardCap)
	if err != nil {
		log.Printf("***** Alarm: list recent activities (admin) failed: %v (CWB_WSL_140)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load recent activities"})
	}
	defer rows.Close()

	order := []int64{}
	byGroup := map[int64]*RecentActivityAdmin{}
	for rows.Next() {
		var groupID int64
		var lang, activityType, text string
		var occurredAt time.Time
		if err := rows.Scan(&groupID, &lang, &occurredAt, &activityType, &text); err != nil {
			log.Printf("***** Alarm: scan recent activity (admin) failed: %v (CWB_WSL_141)", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load recent activities"})
		}
		a, ok := byGroup[groupID]
		if !ok {
			a = &RecentActivityAdmin{
				GroupID:      groupID,
				OccurredAt:   occurredAt.UTC().Format(time.RFC3339),
				ActivityType: activityType,
				Translations: map[string]string{},
			}
			byGroup[groupID] = a
			order = append(order, groupID)
		}
		a.Translations[lang] = text
	}
	if err := rows.Err(); err != nil {
		log.Printf("***** Alarm: list recent activities (admin) rows error: %v (CWB_WSL_142)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load recent activities"})
	}

	out := make([]*RecentActivityAdmin, 0, len(order))
	for _, id := range order {
		out = append(out, byGroup[id])
	}
	return c.JSON(http.StatusOK, map[string]any{"recent_activities": out})
}

// CreateActivity creates one recent-activity entry, writing one row per
// configured language sharing a new group_id.
// Endpoint: POST /api/v1/workspace/recent-activities
func CreateActivity(c echo.Context) error {
	var req activityRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request payload"})
	}
	occurredAt, activityType, errMsg := normalizeActivityFields(req)
	if errMsg != "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errMsg})
	}

	rc := EchoFactory.NewFromEcho(c, "CWB_WSL_110")
	defer rc.Close()
	actor := actorEmail(rc)
	ctx := c.Request().Context()

	tx, err := ApiTypes.ProjectDBHandle.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("***** Alarm: begin tx for create recent activity failed: %v (CWB_WSL_111)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create recent activity"})
	}
	defer tx.Rollback()

	var groupID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT nextval(pg_get_serial_sequence('kb.recent_activities', 'id'))`,
	).Scan(&groupID); err != nil {
		log.Printf("***** Alarm: reserve group id for recent activity failed: %v (CWB_WSL_112)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create recent activity"})
	}

	if err := upsertActivityGroup(ctx, tx, groupID, occurredAt, activityType, req.Translations, actor); err != nil {
		log.Printf("***** Alarm: create recent activity failed group_id=%d: %v (CWB_WSL_113)", groupID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create recent activity"})
	}

	if err := tx.Commit(); err != nil {
		log.Printf("***** Alarm: commit create recent activity failed: %v (CWB_WSL_114)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create recent activity"})
	}

	log.Printf("Created recent activity group_id=%d actor=%q (CWB_WSL_220)", groupID, actor)
	return c.JSON(http.StatusOK, map[string]any{"group_id": groupID})
}

// UpdateActivity updates every locale row sharing group_id in one
// transaction.
// Endpoint: PUT /api/v1/workspace/recent-activities/:group_id
func UpdateActivity(c echo.Context) error {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid group_id"})
	}
	var req activityRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request payload"})
	}
	occurredAt, activityType, errMsg := normalizeActivityFields(req)
	if errMsg != "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errMsg})
	}

	rc := EchoFactory.NewFromEcho(c, "CWB_WSL_120")
	defer rc.Close()
	actor := actorEmail(rc)
	ctx := c.Request().Context()

	var exists bool
	if err := ApiTypes.ProjectDBHandle.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM kb.recent_activities WHERE group_id = $1)`, groupID,
	).Scan(&exists); err != nil {
		log.Printf("***** Alarm: check recent activity exists failed group_id=%d: %v (CWB_WSL_121)", groupID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update recent activity"})
	}
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "recent activity not found"})
	}

	tx, err := ApiTypes.ProjectDBHandle.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("***** Alarm: begin tx for update recent activity failed group_id=%d: %v (CWB_WSL_122)", groupID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update recent activity"})
	}
	defer tx.Rollback()

	if err := upsertActivityGroup(ctx, tx, groupID, occurredAt, activityType, req.Translations, actor); err != nil {
		log.Printf("***** Alarm: update recent activity failed group_id=%d: %v (CWB_WSL_123)", groupID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update recent activity"})
	}

	if err := tx.Commit(); err != nil {
		log.Printf("***** Alarm: commit update recent activity failed group_id=%d: %v (CWB_WSL_124)", groupID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update recent activity"})
	}

	log.Printf("Updated recent activity group_id=%d actor=%q (CWB_WSL_221)", groupID, actor)
	return c.JSON(http.StatusOK, map[string]any{"group_id": groupID})
}

// DeleteActivity removes every locale row sharing group_id.
// Endpoint: DELETE /api/v1/workspace/recent-activities/:group_id
func DeleteActivity(c echo.Context) error {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid group_id"})
	}

	rc := EchoFactory.NewFromEcho(c, "CWB_WSL_130")
	defer rc.Close()
	actor := actorEmail(rc)

	res, err := ApiTypes.ProjectDBHandle.ExecContext(c.Request().Context(),
		`DELETE FROM kb.recent_activities WHERE group_id = $1`, groupID)
	if err != nil {
		log.Printf("***** Alarm: delete recent activity failed group_id=%d: %v (CWB_WSL_131)", groupID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete recent activity"})
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "recent activity not found"})
	}

	log.Printf("Deleted recent activity group_id=%d actor=%q (CWB_WSL_222)", groupID, actor)
	return c.JSON(http.StatusOK, map[string]any{"deleted": true})
}

// normalizeActivityFields validates/defaults the non-translation fields of
// an activityRequest. The returned string is non-empty only on validation
// failure.
func normalizeActivityFields(req activityRequest) (time.Time, string, string) {
	occurredAt, errMsg := parseOccurredAt(req.OccurredAt)
	if errMsg != "" {
		return time.Time{}, "", errMsg
	}
	activityType := strings.TrimSpace(req.ActivityType)
	if activityType == "" {
		activityType = "general"
	}
	if errMsg := requireAllLanguages(req.Translations); errMsg != "" {
		return time.Time{}, "", errMsg
	}
	return occurredAt, activityType, ""
}

// upsertActivityGroup writes/overwrites one row per configured language for
// groupID, sharing occurredAt/activityType across all of them.
func upsertActivityGroup(ctx context.Context, tx *sql.Tx, groupID int64, occurredAt time.Time, activityType string, translations map[string]string, actor string) error {
	for _, lang := range appconfig.GetLanguages() {
		text := translations[lang]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO kb.recent_activities (group_id, lang, occurred_at, activity_type, activity_text, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $6)
			ON CONFLICT (group_id, lang) DO UPDATE SET
				occurred_at = EXCLUDED.occurred_at,
				activity_type = EXCLUDED.activity_type,
				activity_text = EXCLUDED.activity_text,
				updated_by = EXCLUDED.updated_by,
				updated_at = NOW()`,
			groupID, lang, occurredAt, activityType, text, nullIfEmpty(actor)); err != nil {
			return err
		}
	}
	return nil
}
