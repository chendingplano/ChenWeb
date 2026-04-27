package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

func DataHomeDir() string {
	return strings.TrimSpace(os.Getenv("DATA_HOME_DIR"))
}

func ResolveDataHomePath(path string) string {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return ""
	}
	if filepath.IsAbs(cleanPath) {
		return filepath.Clean(cleanPath)
	}
	homeDir := DataHomeDir()
	if homeDir == "" {
		return filepath.Clean(cleanPath)
	}
	return filepath.Clean(filepath.Join(homeDir, cleanPath))
}

func ResolveBackupPath(path string) string {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return ""
	}
	if filepath.IsAbs(cleanPath) {
		return filepath.Clean(cleanPath)
	}

	backupDir := strings.TrimSpace(os.Getenv("DATA_BACKUP_DIR"))
	if backupDir == "" {
		return filepath.Clean(cleanPath)
	}

	base := filepath.Dir(filepath.Clean(backupDir))
	if base == "" || base == "." {
		return filepath.Clean(cleanPath)
	}
	return filepath.Clean(filepath.Join(base, cleanPath))
}
