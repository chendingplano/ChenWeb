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

const announcementsHardCap = 100
const announcementsAdminHardCap = 500

// Announcement is one localized announcement row for /semos/workspace.
type Announcement struct {
	GroupID    int64  `json:"group_id"`
	OccurredAt string `json:"occurred_at"`
	Importance string `json:"importance"`
	Text       string `json:"text"`
}

// AnnouncementAdmin is one logical announcement with every configured
// language's text, for /semos/admin/announcements.
type AnnouncementAdmin struct {
	GroupID      int64             `json:"group_id"`
	OccurredAt   string            `json:"occurred_at"`
	Importance   string            `json:"importance"`
	Translations map[string]string `json:"translations"`
}

type announcementRequest struct {
	OccurredAt   string            `json:"occurred_at"`
	Importance   string            `json:"importance"`
	Translations map[string]string `json:"translations"`
}

// ListAnnouncements serves the locale-filtered, capped announcements list.
// Endpoint: GET /api/v1/workspace/announcements?lang=<code>
func ListAnnouncements(c echo.Context) error {
	lang := normalizeLang(c.QueryParam("lang"))
	limit := appconfig.GetAnnouncementsMax()
	if limit <= 0 || limit > announcementsHardCap {
		limit = announcementsHardCap
	}

	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), `
		SELECT group_id, occurred_at, importance, announcement_text
		FROM kb.site_announcements
		WHERE lang = $1
		ORDER BY occurred_at DESC
		LIMIT $2`, lang, limit)
	if err != nil {
		log.Printf("***** Alarm: list announcements failed lang=%q: %v (CWB_WSL_001)", lang, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load announcements"})
	}
	defer rows.Close()

	out := []Announcement{}
	for rows.Next() {
		var a Announcement
		var occurredAt time.Time
		if err := rows.Scan(&a.GroupID, &occurredAt, &a.Importance, &a.Text); err != nil {
			log.Printf("***** Alarm: scan announcement failed: %v (CWB_WSL_002)", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load announcements"})
		}
		a.OccurredAt = occurredAt.UTC().Format(time.RFC3339)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		log.Printf("***** Alarm: list announcements rows error: %v (CWB_WSL_003)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load announcements"})
	}

	return c.JSON(http.StatusOK, map[string]any{"announcements": out})
}

// ListAnnouncementsAdmin serves every announcement with all of its
// translations grouped together, for /semos/admin/announcements.
// Endpoint: GET /api/v1/workspace/announcements/admin
func ListAnnouncementsAdmin(c echo.Context) error {
	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), `
		SELECT group_id, lang, occurred_at, importance, announcement_text
		FROM kb.site_announcements
		ORDER BY occurred_at DESC, group_id DESC
		LIMIT $1`, announcementsAdminHardCap)
	if err != nil {
		log.Printf("***** Alarm: list announcements (admin) failed: %v (CWB_WSL_040)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load announcements"})
	}
	defer rows.Close()

	order := []int64{}
	byGroup := map[int64]*AnnouncementAdmin{}
	for rows.Next() {
		var groupID int64
		var lang, importance, text string
		var occurredAt time.Time
		if err := rows.Scan(&groupID, &lang, &occurredAt, &importance, &text); err != nil {
			log.Printf("***** Alarm: scan announcement (admin) failed: %v (CWB_WSL_041)", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load announcements"})
		}
		a, ok := byGroup[groupID]
		if !ok {
			a = &AnnouncementAdmin{
				GroupID:      groupID,
				OccurredAt:   occurredAt.UTC().Format(time.RFC3339),
				Importance:   importance,
				Translations: map[string]string{},
			}
			byGroup[groupID] = a
			order = append(order, groupID)
		}
		a.Translations[lang] = text
	}
	if err := rows.Err(); err != nil {
		log.Printf("***** Alarm: list announcements (admin) rows error: %v (CWB_WSL_042)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load announcements"})
	}

	out := make([]*AnnouncementAdmin, 0, len(order))
	for _, id := range order {
		out = append(out, byGroup[id])
	}
	return c.JSON(http.StatusOK, map[string]any{"announcements": out})
}

// CreateAnnouncement creates one announcement, writing one row per
// configured language sharing a new group_id.
// Endpoint: POST /api/v1/workspace/announcements
func CreateAnnouncement(c echo.Context) error {
	var req announcementRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request payload"})
	}
	occurredAt, importance, errMsg := normalizeAnnouncementFields(req)
	if errMsg != "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errMsg})
	}

	rc := EchoFactory.NewFromEcho(c, "CWB_WSL_010")
	defer rc.Close()
	actor := actorEmail(rc)
	ctx := c.Request().Context()

	tx, err := ApiTypes.ProjectDBHandle.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("***** Alarm: begin tx for create announcement failed: %v (CWB_WSL_011)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create announcement"})
	}
	defer tx.Rollback()

	var groupID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT nextval(pg_get_serial_sequence('kb.site_announcements', 'id'))`,
	).Scan(&groupID); err != nil {
		log.Printf("***** Alarm: reserve group id for announcement failed: %v (CWB_WSL_012)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create announcement"})
	}

	if err := upsertAnnouncementGroup(ctx, tx, groupID, occurredAt, importance, req.Translations, actor); err != nil {
		log.Printf("***** Alarm: create announcement failed group_id=%d: %v (CWB_WSL_013)", groupID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create announcement"})
	}

	if err := tx.Commit(); err != nil {
		log.Printf("***** Alarm: commit create announcement failed: %v (CWB_WSL_014)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create announcement"})
	}

	log.Printf("Created announcement group_id=%d actor=%q (CWB_WSL_210)", groupID, actor)
	return c.JSON(http.StatusOK, map[string]any{"group_id": groupID})
}

// UpdateAnnouncement updates every locale row sharing group_id in one
// transaction.
// Endpoint: PUT /api/v1/workspace/announcements/:group_id
func UpdateAnnouncement(c echo.Context) error {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid group_id"})
	}
	var req announcementRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request payload"})
	}
	occurredAt, importance, errMsg := normalizeAnnouncementFields(req)
	if errMsg != "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": errMsg})
	}

	rc := EchoFactory.NewFromEcho(c, "CWB_WSL_020")
	defer rc.Close()
	actor := actorEmail(rc)
	ctx := c.Request().Context()

	var exists bool
	if err := ApiTypes.ProjectDBHandle.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM kb.site_announcements WHERE group_id = $1)`, groupID,
	).Scan(&exists); err != nil {
		log.Printf("***** Alarm: check announcement exists failed group_id=%d: %v (CWB_WSL_021)", groupID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update announcement"})
	}
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "announcement not found"})
	}

	tx, err := ApiTypes.ProjectDBHandle.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("***** Alarm: begin tx for update announcement failed group_id=%d: %v (CWB_WSL_022)", groupID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update announcement"})
	}
	defer tx.Rollback()

	if err := upsertAnnouncementGroup(ctx, tx, groupID, occurredAt, importance, req.Translations, actor); err != nil {
		log.Printf("***** Alarm: update announcement failed group_id=%d: %v (CWB_WSL_023)", groupID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update announcement"})
	}

	if err := tx.Commit(); err != nil {
		log.Printf("***** Alarm: commit update announcement failed group_id=%d: %v (CWB_WSL_024)", groupID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update announcement"})
	}

	log.Printf("Updated announcement group_id=%d actor=%q (CWB_WSL_211)", groupID, actor)
	return c.JSON(http.StatusOK, map[string]any{"group_id": groupID})
}

// DeleteAnnouncement removes every locale row sharing group_id.
// Endpoint: DELETE /api/v1/workspace/announcements/:group_id
func DeleteAnnouncement(c echo.Context) error {
	groupID, err := strconv.ParseInt(c.Param("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid group_id"})
	}

	rc := EchoFactory.NewFromEcho(c, "CWB_WSL_030")
	defer rc.Close()
	actor := actorEmail(rc)

	res, err := ApiTypes.ProjectDBHandle.ExecContext(c.Request().Context(),
		`DELETE FROM kb.site_announcements WHERE group_id = $1`, groupID)
	if err != nil {
		log.Printf("***** Alarm: delete announcement failed group_id=%d: %v (CWB_WSL_031)", groupID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete announcement"})
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "announcement not found"})
	}

	log.Printf("Deleted announcement group_id=%d actor=%q (CWB_WSL_212)", groupID, actor)
	return c.JSON(http.StatusOK, map[string]any{"deleted": true})
}

// normalizeAnnouncementFields validates/defaults the non-translation fields
// of an announcementRequest. The returned string is non-empty only on
// validation failure.
func normalizeAnnouncementFields(req announcementRequest) (time.Time, string, string) {
	occurredAt, errMsg := parseOccurredAt(req.OccurredAt)
	if errMsg != "" {
		return time.Time{}, "", errMsg
	}
	importance := strings.TrimSpace(req.Importance)
	if importance == "" {
		importance = "normal"
	}
	if errMsg := requireAllLanguages(req.Translations); errMsg != "" {
		return time.Time{}, "", errMsg
	}
	return occurredAt, importance, ""
}

// upsertAnnouncementGroup writes/overwrites one row per configured language
// for groupID, sharing occurredAt/importance across all of them.
func upsertAnnouncementGroup(ctx context.Context, tx *sql.Tx, groupID int64, occurredAt time.Time, importance string, translations map[string]string, actor string) error {
	for _, lang := range appconfig.GetLanguages() {
		text := translations[lang]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO kb.site_announcements (group_id, lang, occurred_at, importance, announcement_text, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $6)
			ON CONFLICT (group_id, lang) DO UPDATE SET
				occurred_at = EXCLUDED.occurred_at,
				importance = EXCLUDED.importance,
				announcement_text = EXCLUDED.announcement_text,
				updated_by = EXCLUDED.updated_by,
				updated_at = NOW()`,
			groupID, lang, occurredAt, importance, text, nullIfEmpty(actor)); err != nil {
			return err
		}
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
