package diaryhandler

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

var logger = loggerutil.CreateDefaultLogger("CWB_DIARY_001")

var reYear  = regexp.MustCompile(`^\d{4}$`)
var reMonth = regexp.MustCompile(`^\d{2}$`)
var reDay   = regexp.MustCompile(`^\d{2}$`)

// DiaryEntry is the structure stored on disk and returned by the API.
type DiaryEntry struct {
	ID        string   `json:"id"`
	CreatedAt string   `json:"createdAt"`
	Topic     string   `json:"topic"`
	URLs      []string `json:"urls"`
	Keywords  []string `json:"keywords"`
	Source    string   `json:"source"`
	Content   string   `json:"content"`
}

func diaryHomeDir() (string, error) {
	dir := os.Getenv("DIARY_HOME_DIR")
	if dir == "" {
		return "", fmt.Errorf("DIARY_HOME_DIR environment variable is not set")
	}
	return dir, nil
}

func entryFilePath(base string, entry DiaryEntry) (string, error) {
	t, err := time.Parse(time.RFC3339Nano, entry.CreatedAt)
	if err != nil {
		return "", fmt.Errorf("invalid createdAt: %w", err)
	}
	year  := t.UTC().Format("2006")
	month := t.UTC().Format("01")
	day   := t.UTC().Format("02")
	return filepath.Join(base, year, month, day, entry.ID+".json"), nil
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// listAllEntries scans DIARY_HOME_DIR/{year}/{month}/{day}/*.json and returns
// all entries sorted newest-first.
func listAllEntries(base string) ([]DiaryEntry, error) {
	entries := []DiaryEntry{}

	yearDirs, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, err
	}

	for _, yd := range yearDirs {
		if !yd.IsDir() || !reYear.MatchString(yd.Name()) {
			continue
		}
		yearPath := filepath.Join(base, yd.Name())

		monthDirs, err := os.ReadDir(yearPath)
		if err != nil {
			continue
		}
		for _, md := range monthDirs {
			if !md.IsDir() || !reMonth.MatchString(md.Name()) {
				continue
			}
			monthPath := filepath.Join(yearPath, md.Name())

			dayDirs, err := os.ReadDir(monthPath)
			if err != nil {
				continue
			}
			for _, dd := range dayDirs {
				if !dd.IsDir() || !reDay.MatchString(dd.Name()) {
					continue
				}
				dayPath := filepath.Join(monthPath, dd.Name())

				files, err := os.ReadDir(dayPath)
				if err != nil {
					continue
				}
				for _, f := range files {
					if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
						continue
					}
					raw, err := os.ReadFile(filepath.Join(dayPath, f.Name()))
					if err != nil {
						logger.Warn("failed to read diary file", "file", f.Name(), "error", err)
						continue
					}
					var e DiaryEntry
					if err := json.Unmarshal(raw, &e); err != nil {
						logger.Warn("failed to parse diary file", "file", f.Name(), "error", err)
						continue
					}
					entries = append(entries, e)
				}
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt > entries[j].CreatedAt
	})
	return entries, nil
}

// findEntryFile walks DIARY_HOME_DIR looking for {id}.json.
func findEntryFile(base, id string) (string, error) {
	yearDirs, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	for _, yd := range yearDirs {
		if !yd.IsDir() || !reYear.MatchString(yd.Name()) {
			continue
		}
		monthDirs, _ := os.ReadDir(filepath.Join(base, yd.Name()))
		for _, md := range monthDirs {
			if !md.IsDir() || !reMonth.MatchString(md.Name()) {
				continue
			}
			dayDirs, _ := os.ReadDir(filepath.Join(base, yd.Name(), md.Name()))
			for _, dd := range dayDirs {
				if !dd.IsDir() || !reDay.MatchString(dd.Name()) {
					continue
				}
				candidate := filepath.Join(base, yd.Name(), md.Name(), dd.Name(), id+".json")
				if _, err := os.Stat(candidate); err == nil {
					return candidate, nil
				}
			}
		}
	}
	return "", nil
}

// GET /api/v1/diary
func List(c echo.Context) error {
	base, err := diaryHomeDir()
	if err != nil {
		logger.Error("[diary] missing DIARY_HOME_DIR", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	entries, err := listAllEntries(base)
	if err != nil {
		logger.Error("[diary] failed to list entries", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	logger.Info("[diary] listed entries", "count", len(entries))
	return c.JSON(http.StatusOK, entries)
}

// POST /api/v1/diary
func Create(c echo.Context) error {
	base, err := diaryHomeDir()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	var body DiaryEntry
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	entry := DiaryEntry{
		ID:        newUUID(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Topic:     body.Topic,
		URLs:      body.URLs,
		Keywords:  body.Keywords,
		Source:    body.Source,
		Content:   body.Content,
	}
	if entry.URLs == nil {
		entry.URLs = []string{}
	}
	if entry.Keywords == nil {
		entry.Keywords = []string{}
	}

	filePath, err := entryFilePath(base, entry)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		logger.Error("[diary] failed to create directory", "path", filePath, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	raw, _ := json.MarshalIndent(entry, "", "  ")
	if err := os.WriteFile(filePath, raw, 0o644); err != nil {
		logger.Error("[diary] failed to write entry", "path", filePath, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	logger.Info("[diary] created entry", "id", entry.ID, "topic", entry.Topic)
	return c.JSON(http.StatusCreated, entry)
}

// GET /api/v1/diary/:id
func Get(c echo.Context) error {
	id := c.Param("id")
	base, err := diaryHomeDir()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	filePath, err := findEntryFile(base, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if filePath == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	var entry DiaryEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "malformed entry"})
	}
	return c.JSON(http.StatusOK, entry)
}

// PUT /api/v1/diary/:id
func Update(c echo.Context) error {
	id := c.Param("id")
	base, err := diaryHomeDir()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	filePath, err := findEntryFile(base, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if filePath == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	var existing DiaryEntry
	if err := json.Unmarshal(raw, &existing); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "malformed entry"})
	}

	var body DiaryEntry
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	existing.Topic    = body.Topic
	existing.URLs     = body.URLs
	existing.Keywords = body.Keywords
	existing.Source   = body.Source
	existing.Content  = body.Content
	if existing.URLs == nil {
		existing.URLs = []string{}
	}
	if existing.Keywords == nil {
		existing.Keywords = []string{}
	}

	updated, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filePath, updated, 0o644); err != nil {
		logger.Error("[diary] failed to update entry", "id", id, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	logger.Info("[diary] updated entry", "id", id, "topic", existing.Topic)
	return c.JSON(http.StatusOK, existing)
}

// DELETE /api/v1/diary/:id
func Delete(c echo.Context) error {
	id := c.Param("id")
	base, err := diaryHomeDir()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	filePath, err := findEntryFile(base, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if filePath == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}

	if err := os.Remove(filePath); err != nil {
		logger.Error("[diary] failed to delete entry", "id", id, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	logger.Info("[diary] deleted entry", "id", id)
	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}
