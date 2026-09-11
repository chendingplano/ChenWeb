// Package chadsessionshandler exposes read-only access to local Chad sessions.
package chadsessionshandler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

const maxSessionFileBytes = 8 * 1024 * 1024

type Handler struct {
	root string
}

type SessionSummary struct {
	ID           string  `json:"id"`
	Title        string  `json:"title,omitempty"`
	Updated      float64 `json:"updated"`
	Turns        int     `json:"turns,omitempty"`
	CWD          string  `json:"cwd,omitempty"`
	MessageCount int     `json:"messageCount"`
}

type SessionMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content,omitempty"`
}

type SessionDetail struct {
	ID       string           `json:"id"`
	Title    string           `json:"title,omitempty"`
	Updated  float64          `json:"updated"`
	Turns    int              `json:"turns,omitempty"`
	CWD      string           `json:"cwd,omitempty"`
	Meta     json.RawMessage  `json:"meta,omitempty"`
	Messages []SessionMessage `json:"messages"`
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
	return New(filepath.Join(home, ".chad", "sessions")), nil
}

func (h *Handler) ListSessions(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_CHAD_010")
	logger.Info("List Chad sessions")

	entries, err := os.ReadDir(h.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c.JSON(http.StatusOK, map[string]any{"sessions": []SessionSummary{}})
		}
		logger.Error("Read Chad session directory", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error_msg": "Unable to read Chad sessions"})
	}

	sessions := make([]SessionSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validSessionID(entry.Name()) {
			continue
		}
		summary, err := h.readSummary(entry.Name())
		if err != nil {
			logger.Warn("Skip invalid Chad session", "session_id", entry.Name(), "error", err)
			continue
		}
		sessions = append(sessions, summary)
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].Updated == sessions[j].Updated {
			return sessions[i].ID > sessions[j].ID
		}
		return sessions[i].Updated > sessions[j].Updated
	})
	return c.JSON(http.StatusOK, map[string]any{"sessions": sessions})
}

func (h *Handler) GetSession(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_CHAD_020")
	id := c.Param("id")
	if !validSessionID(id) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error_msg": "Invalid session id"})
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
	dir := filepath.Join(h.root, id)
	if info, err := os.Stat(dir); err != nil {
		return SessionDetail{}, err
	} else if !info.IsDir() {
		return SessionDetail{}, os.ErrNotExist
	}

	var index indexFile
	if err := readJSON(filepath.Join(dir, "index.json"), &index); err != nil {
		return SessionDetail{}, err
	}
	contentPath, err := newestSessionFile(dir)
	if err != nil {
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

func newestSessionFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") && entry.Name() != "index.json" {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return "", os.ErrNotExist
	}
	sort.Strings(names)
	return filepath.Join(dir, names[len(names)-1]), nil
}

func validSessionID(id string) bool {
	return id != "" && id != "." && id != ".." && filepath.Base(id) == id && !strings.ContainsAny(id, `/\\`)
}
