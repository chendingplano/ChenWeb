package datasync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// resolveFileDir resolves where this instance stores an item's synced files
// locally, mirroring videohandler.videoDir()'s own resolution order (env
// override, else <DATA_HOME_DIR>/<subdir>) generalized into item config so
// this package doesn't need to import videohandler (root CLAUDE.md Project
// Isolation: avoid tight coupling between non-shared code) -- see
// design.md Decision 2.
func resolveFileDir(item TableSyncItem) (string, error) {
	if item.FileDirEnv != "" {
		if v := strings.TrimSpace(os.Getenv(item.FileDirEnv)); v != "" {
			return v, nil
		}
	}
	if item.FileDirDefaultSubdir != "" {
		if home := strings.TrimSpace(os.Getenv("DATA_HOME_DIR")); home != "" {
			return filepath.Join(home, item.FileDirDefaultSubdir), nil
		}
	}
	return "", fmt.Errorf(
		"cannot resolve a storage directory for item %s: neither $%s nor DATA_HOME_DIR/%s is configured",
		item.ID, item.FileDirEnv, item.FileDirDefaultSubdir,
	)
}

// materializeFiles is the table_with_files apply step that runs ahead of
// upsertRows (design.md Decisions 2/3): for each row with a non-empty
// FileColumn value, it fetches that row's file from the source and rewrites
// the row's FileColumn in place to the file's new local path. Rows with an
// empty file column are left untouched -- no fetch is attempted.
func materializeFiles(ctx context.Context, item TableSyncItem, sourceURL, sharedSecret string, rows []Row) error {
	if item.Kind != KindTableWithFiles || len(rows) == 0 {
		return nil
	}
	dir, err := resolveFileDir(item)
	if err != nil {
		return err
	}

	for _, row := range rows {
		rawPath, ok := row[item.FileColumn]
		if !ok || isNullJSON(rawPath) {
			continue
		}
		var sourcePath string
		if err := json.Unmarshal(rawPath, &sourcePath); err != nil {
			return fmt.Errorf("decode %s for %s: %w", item.FileColumn, item.ID, err)
		}
		if sourcePath == "" {
			continue
		}

		keyValues, err := naturalKeyValues(item, row)
		if err != nil {
			return fmt.Errorf("build file key for %s: %w", item.ID, err)
		}

		localPath, err := fetchAndSaveFile(ctx, sourceURL, sharedSecret, item.ID, keyValues, dir, sourcePath)
		if err != nil {
			return fmt.Errorf("fetch file for %s: %w", item.ID, err)
		}

		encoded, err := json.Marshal(localPath)
		if err != nil {
			return err
		}
		row[item.FileColumn] = encoded
	}
	return nil
}

// naturalKeyValues decodes a row's natural-key column values, in NaturalKey
// order, as plain strings -- the shape the file endpoint's key parameter
// expects.
func naturalKeyValues(item TableSyncItem, row Row) ([]string, error) {
	values := make([]string, len(item.NaturalKey))
	for i, col := range item.NaturalKey {
		raw, ok := row[col]
		if !ok || isNullJSON(raw) {
			return nil, fmt.Errorf("natural-key column %s is empty", col)
		}
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, fmt.Errorf("decode natural-key column %s: %w", col, err)
		}
		values[i] = v
	}
	return values, nil
}

// fetchAndSaveFile downloads one row's file and saves it under dir with a
// fresh unique local name (mirrors videohandler.UploadVideo's
// "<UnixNano>_<sanitized-basename>" naming, reimplemented locally since
// UploadVideo's own naming helper isn't exported), returning the new local
// path.
func fetchAndSaveFile(ctx context.Context, sourceURL, sharedSecret, itemID string, keyValues []string, dir, sourcePath string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create storage dir %s: %w", dir, err)
	}

	body, err := fetchItemFile(ctx, sourceURL, sharedSecret, itemID, keyValues)
	if err != nil {
		return "", err
	}
	defer body.Close()

	storedName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), sanitizeFileName(filepath.Base(sourcePath)))
	destPath := filepath.Join(dir, storedName)

	dst, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", destPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, body); err != nil {
		_ = os.Remove(destPath)
		return "", fmt.Errorf("write %s: %w", destPath, err)
	}
	return destPath, nil
}

// sanitizeFileName strips path separators and keeps a filesystem-safe
// basename, mirroring videohandler's own (unexported) sanitizeFilename.
func sanitizeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "_")
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.' || r == '-' || r == '_':
			return r
		default:
			return '_'
		}
	}, name)
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "file"
	}
	return cleaned
}
