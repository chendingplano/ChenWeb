// Package chadsessionshandler exposes read-only access to local Chad sessions.
package chadsessionshandler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

const maxSessionFileBytes = 8 * 1024 * 1024

type Handler struct {
	root           string
	databaseBacked bool
}

type SessionSummary struct {
	ID           string  `json:"id"`
	Title        string  `json:"title,omitempty"`
	Updated      float64 `json:"updated"`
	Turns        int     `json:"turns,omitempty"`
	CWD          string  `json:"cwd,omitempty"`
	ChadVersion  string  `json:"chadVersion,omitempty"`
	ModelName    string  `json:"modelName,omitempty"`
	Mode         string  `json:"mode,omitempty"`
	CreateTime   string  `json:"createTime,omitempty"`
	MessageCount int     `json:"messageCount"`
}

type SessionMessage struct {
	Role               string          `json:"role"`
	Name               string          `json:"name,omitempty"`
	Content            json.RawMessage `json:"content,omitempty"`
	ToolCallCommand    string          `json:"toolCallCommand,omitempty"`
	ToolCallParameters json.RawMessage `json:"toolCallParameters,omitempty"`
}

type SessionDetail struct {
	ID          string           `json:"id"`
	Title       string           `json:"title,omitempty"`
	Updated     float64          `json:"updated"`
	Turns       int              `json:"turns,omitempty"`
	CWD         string           `json:"cwd,omitempty"`
	ChadVersion string           `json:"chadVersion,omitempty"`
	ModelName   string           `json:"modelName,omitempty"`
	Mode        string           `json:"mode,omitempty"`
	CreateTime  string           `json:"createTime,omitempty"`
	Meta        json.RawMessage  `json:"meta,omitempty"`
	Messages    []SessionMessage `json:"messages"`
}

type indexFile struct {
	Sessions map[string]indexEntry `json:"sessions"`
}

type indexEntry struct {
	Title   string  `json:"title"`
	Updated float64 `json:"updated"`
	Turns   int     `json:"turns"`
}

type sessionFile struct {
	CWD       string           `json:"cwd"`
	SessionID string           `json:"session_id"`
	Updated   float64          `json:"updated"`
	Meta      json.RawMessage  `json:"meta"`
	Messages  []SessionMessage `json:"messages"`
}

func New(root string) *Handler {
	return &Handler{root: root}
}

func NewDefault() (*Handler, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve Chad session home: %w", err)
	}
	handler := New(filepath.Join(home, ".chad", "sessions"))
	handler.databaseBacked = true
	return handler, nil
}

func (h *Handler) projectDB() *sql.DB {
	if !h.databaseBacked {
		return nil
	}
	return ApiTypes.ProjectDBHandle
}

func (h *Handler) ListSessions(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_CHAD_010")
	logger.Info("List Chad sessions")
	if db := h.projectDB(); db != nil {
		var count int
		if err := db.QueryRowContext(c.Request().Context(), `SELECT COUNT(*) FROM kb.chad_sessions`).Scan(&count); err == nil && count > 0 {
			return h.listDatabaseSessions(c, db)
		}
	}

	sessions, err := h.listFilesystemSessions()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c.JSON(http.StatusOK, map[string]any{"sessions": []SessionSummary{}})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error_msg": "Unable to read Chad sessions"})
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].Updated == sessions[j].Updated {
			return sessions[i].ID > sessions[j].ID
		}
		return sessions[i].Updated > sessions[j].Updated
	})
	return c.JSON(http.StatusOK, map[string]any{"sessions": sessions})
}

func (h *Handler) listFilesystemSessions() ([]SessionSummary, error) {
	entries, err := os.ReadDir(h.root)
	if err != nil {
		return nil, err
	}
	sessions := make([]SessionSummary, 0)
	for _, directory := range entries {
		if !directory.IsDir() || !validSessionID(directory.Name()) {
			continue
		}
		files, err := os.ReadDir(filepath.Join(h.root, directory.Name()))
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() || file.Name() == "index.json" || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			id := strings.TrimSuffix(file.Name(), ".json")
			if !validSessionID(id) {
				continue
			}
			summary, err := h.readSummary(id)
			if err != nil {
				continue
			}
			sessions = append(sessions, summary)
		}
	}
	return sessions, nil
}

func (h *Handler) GetSession(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_CHAD_020")
	id := c.Param("id")
	if !validSessionID(id) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error_msg": "Invalid session id"})
	}
	if db := h.projectDB(); db != nil {
		return h.getDatabaseSession(c, db, id)
	}

	detail, err := h.readDetail(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c.JSON(http.StatusNotFound, map[string]string{"error_msg": "Session not found"})
		}
		if errors.Is(err, errSessionTooLarge) {
			return c.JSON(http.StatusRequestEntityTooLarge, map[string]string{"error_msg": "Session file is too large to display"})
		}
		logger.Error("Read Chad session", "session_id", id, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error_msg": "Unable to read session"})
	}
	return c.JSON(http.StatusOK, detail)
}

func (h *Handler) listDatabaseSessions(c echo.Context, db *sql.DB) error {
	rows, err := db.QueryContext(c.Request().Context(), `
		SELECT s.session_id, s.title, s.updated, s.turns, s.directory,
		       s.chad_version, s.model_name, s.mode, s.create_time,
		       COUNT(m.id)
		FROM kb.chad_sessions s
		LEFT JOIN kb.chad_messages m ON m.session_id = s.session_id
		GROUP BY s.id
		ORDER BY s.updated DESC, s.session_id DESC`)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error_msg": "Unable to read Chad sessions from database"})
	}
	defer rows.Close()
	sessions := make([]SessionSummary, 0)
	for rows.Next() {
		var session SessionSummary
		var createTime time.Time
		if err := rows.Scan(&session.ID, &session.Title, &session.Updated, &session.Turns,
			&session.CWD, &session.ChadVersion, &session.ModelName, &session.Mode,
			&createTime, &session.MessageCount); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error_msg": "Unable to decode Chad sessions"})
		}
		session.CreateTime = createTime.UTC().Format(time.RFC3339)
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error_msg": "Unable to read Chad sessions from database"})
	}
	return c.JSON(http.StatusOK, map[string]any{"sessions": sessions})
}

func (h *Handler) getDatabaseSession(c echo.Context, db *sql.DB, id string) error {
	var detail SessionDetail
	var createTime time.Time
	if err := db.QueryRowContext(c.Request().Context(), `
		SELECT session_id, title, updated, turns, directory, chad_version,
		       model_name, mode, create_time, meta
		FROM kb.chad_sessions WHERE session_id = $1`, id).Scan(
		&detail.ID, &detail.Title, &detail.Updated, &detail.Turns, &detail.CWD,
		&detail.ChadVersion, &detail.ModelName, &detail.Mode, &createTime,
		&detail.Meta); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if legacy, legacyErr := h.readDetail(id); legacyErr == nil {
				return c.JSON(http.StatusOK, legacy)
			}
			return c.JSON(http.StatusNotFound, map[string]string{"error_msg": "Session not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error_msg": "Unable to read session from database"})
	}
	detail.CreateTime = createTime.UTC().Format(time.RFC3339)
	rows, err := db.QueryContext(c.Request().Context(), `
		SELECT message_json, tool_call_command, tool_call_parameters
		FROM kb.chad_messages WHERE session_id = $1 ORDER BY message_index`, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error_msg": "Unable to read Chad messages"})
	}
	defer rows.Close()
	detail.Messages = make([]SessionMessage, 0)
	for rows.Next() {
		var raw, parameters []byte
		var command string
		if err := rows.Scan(&raw, &command, &parameters); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error_msg": "Unable to decode Chad messages"})
		}
		var message SessionMessage
		if err := json.Unmarshal(raw, &message); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error_msg": "Unable to decode Chad message"})
		}
		message.ToolCallCommand = command
		if len(parameters) > 0 {
			message.ToolCallParameters = json.RawMessage(parameters)
		}
		detail.Messages = append(detail.Messages, message)
	}
	return c.JSON(http.StatusOK, detail)
}

func (h *Handler) readSummary(id string) (SessionSummary, error) {
	detail, err := h.readDetail(id)
	if err != nil {
		return SessionSummary{}, err
	}
	return SessionSummary{
		ID:           id,
		Title:        detail.Title,
		Updated:      detail.Updated,
		Turns:        detail.Turns,
		CWD:          detail.CWD,
		MessageCount: len(detail.Messages),
	}, nil
}

func (h *Handler) readDetail(id string) (SessionDetail, error) {
	contentPath, err := h.findSessionFile(id)
	if err != nil {
		return SessionDetail{}, err
	}
	dir := filepath.Dir(contentPath)

	var index indexFile
	if err := readJSON(filepath.Join(dir, "index.json"), &index); err != nil {
		return SessionDetail{}, err
	}
	var content sessionFile
	if err := readJSON(contentPath, &content); err != nil {
		return SessionDetail{}, err
	}
	entry, ok := index.Sessions[content.SessionID]
	if !ok && len(index.Sessions) == 1 {
		for _, candidate := range index.Sessions {
			entry = candidate
			ok = true
		}
	}
	if !ok {
		return SessionDetail{}, os.ErrNotExist
	}
	updated := entry.Updated
	if updated == 0 {
		updated = content.Updated
	}
	return SessionDetail{
		ID:       id,
		Title:    entry.Title,
		Updated:  updated,
		Turns:    entry.Turns,
		CWD:      content.CWD,
		Meta:     content.Meta,
		Messages: content.Messages,
	}, nil
}

func (h *Handler) findSessionFile(id string) (string, error) {
	entries, err := os.ReadDir(h.root)
	if err != nil {
		return "", err
	}
	filename := id + ".json"
	for _, directory := range entries {
		if !directory.IsDir() {
			continue
		}
		path := filepath.Join(h.root, directory.Name(), filename)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", os.ErrNotExist
}

var errSessionTooLarge = errors.New("session file exceeds size limit")

func readJSON(path string, target any) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() > maxSessionFileBytes {
		return errSessionTooLarge
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func validSessionID(id string) bool {
	return id != "" && id != "." && id != ".." && filepath.Base(id) == id && !strings.ContainsAny(id, `/\\`)
}
