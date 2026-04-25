package main

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/joho/godotenv"
)

func main() {
	var (
		configPath       string
		migrateOnly      bool
		verifySchemaOnly bool
	)
	flag.StringVar(&configPath, "config", "../../../config.toml", "Path to config file")
	flag.BoolVar(&migrateOnly, "migrate-only", false, "Apply migrations and exit")
	flag.BoolVar(&verifySchemaOnly, "verify-schema-only", false, "Verify parser schema and exit")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	newCtx := context.WithValue(ctx, ApiTypes.CallFlowKey, "CWB_PDF_001")
	logger := loggerutil.CreateDefaultLogger("CWB_PDF_002")
	defer logger.Close()

	_ = godotenv.Load("./.env")
	_ = godotenv.Load()

	ApiUtils.LoadLibConfig("CWB_PDF_003")
	if err := config.LoadConfig(newCtx, logger, configPath); err != nil {
		logger.Error("failed loading config", "error", err, "config", configPath)
		os.Exit(1)
	}

	config.NormalizeMigrationPaths(logger, configPath)

	if err := databaseutil.InitDB(newCtx, ApiTypes.CommonConfig); err != nil {
		logger.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer databaseutil.CloseDatabase(ApiTypes.CommonConfig)

	if err := config.RunMigrations(ctx, logger); err != nil {
		logger.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	if err := verifyResultFilenameColumn(ctx); err != nil {
		logger.Error("schema verification failed", "error", err)
		os.Exit(1)
	}
	logger.Info("schema verification passed", "table", "kb.inputs", "column", "result_filename")

	if migrateOnly || verifySchemaOnly {
		return
	}

	stagingDir := strings.TrimSpace(os.Getenv("DATA_STAGING_DIR"))
	backupDir := strings.TrimSpace(os.Getenv("DATA_BACKUP_DIR"))
	homeDir := strings.TrimSpace(os.Getenv("DATA_HOME_DIR"))
	if stagingDir == "" || backupDir == "" || homeDir == "" {
		logger.Error("staging directories not configured",
			"DATA_STAGING_DIR", stagingDir,
			"DATA_BACKUP_DIR", backupDir,
			"DATA_HOME_DIR", homeDir,
		)
		os.Exit(1)
	}

	ctx, cancelStaging := context.WithCancel(context.Background())
	defer cancelStaging()

	go runStagingThread(ctx, logger, ApiTypes.ProjectDBHandle, stagingDir, backupDir, homeDir)
	logger.Info("staging thread started", "staging_dir", stagingDir, "backup_dir", backupDir, "home_dir", homeDir)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutdown signal received; stopping")
}

func runStagingThread(ctx context.Context, logger ApiTypes.JimoLogger, db *sql.DB, stagingDir, backupDir, homeDir string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	logger.Info("staging thread started",
		"poll_interval", "10s",
		"staging_dir", stagingDir,
		"backup_dir", backupDir,
		"home_dir", homeDir,
	)

	if err := processStagingOnce(ctx, logger, db, stagingDir, backupDir, homeDir); err != nil {
		logger.Error("initial staging cycle failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("staging thread stopping", "reason", ctx.Err())
			return
		case <-ticker.C:
			if err := processStagingOnce(ctx, logger, db, stagingDir, backupDir, homeDir); err != nil {
				logger.Error("staging cycle failed", "error", err)
			}
		}
	}
}

func processStagingOnce(ctx context.Context, logger ApiTypes.JimoLogger, db *sql.DB, stagingDir, backupDir, homeDir string) error {
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		return fmt.Errorf("ensure staging dir failed: %w", err)
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("ensure backup dir failed: %w", err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		return fmt.Errorf("ensure home dir failed: %w", err)
	}

	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		return fmt.Errorf("read staging dir failed: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			logger.Warn("failed to stat staging entry", "entry", entry.Name(), "error", err)
			continue
		}

		if info.Mode()&fs.ModeType != 0 {
			logger.Info("entry ignored", "name", entry.Name())
			continue
		}

		srcPath := filepath.Join(stagingDir, entry.Name())
		backupPath, err := uniquePath(backupDir, entry.Name())
		if err != nil {
			logger.Error("failed to allocate backup path", "source", srcPath, "error", err)
			continue
		}
		homePath, shouldCopyHome, err := resolveRepoPathForStagedFile(srcPath, homeDir, entry.Name())
		if err != nil {
			logger.Error("failed to resolve home path", "source", srcPath, "error", err)
			continue
		}

		if err := copyFile(srcPath, backupPath); err != nil {
			logger.Error("failed to backup staged file", "source", srcPath, "backup", backupPath, "error", err)
			continue
		}
		if shouldCopyHome {
			if err := copyFile(srcPath, homePath); err != nil {
				logger.Error("failed to copy staged file to home", "source", srcPath, "home", homePath, "error", err)
				continue
			}
		} else {
			logger.Info("skipped copy to home; same-name file with identical md5 already exists",
				"source", srcPath,
				"home", homePath,
			)
		}

		if err := os.Remove(srcPath); err != nil {
			logger.Error("failed to remove staged source file", "source", srcPath, "error", err)
			continue
		}

		updated, err := upsertStagedInputRecord(ctx, db, filepath.Base(homePath), srcPath, homePath, backupPath)
		if err != nil {
			logger.Error("failed to insert kb.inputs row for staged file",
				"source", srcPath,
				"home", homePath,
				"backup", backupPath,
				"error", err,
			)
			continue
		}

		logger.Info("staged file ingested",
			"source", srcPath,
			"home", homePath,
			"backup", backupPath,
			"updated_existing_row", updated,
		)
	}

	return nil
}

func resolveRepoPathForStagedFile(srcPath, homeDir, fileName string) (homePath string, shouldCopy bool, err error) {
	candidate := filepath.Join(homeDir, filepath.Base(fileName))
	if _, statErr := os.Stat(candidate); statErr != nil {
		if os.IsNotExist(statErr) {
			return candidate, true, nil
		}
		return "", false, statErr
	}

	srcMD5, err := fileMD5Hex(srcPath)
	if err != nil {
		return "", false, fmt.Errorf("calculate source md5 failed: %w", err)
	}
	dstMD5, err := fileMD5Hex(candidate)
	if err != nil {
		return "", false, fmt.Errorf("calculate existing home md5 failed: %w", err)
	}

	if srcMD5 == dstMD5 {
		return candidate, false, nil
	}

	uniqueHomePath, err := uniquePath(homeDir, fileName)
	if err != nil {
		return "", false, fmt.Errorf("allocate unique home path failed: %w", err)
	}
	return uniqueHomePath, true, nil
}

func upsertStagedInputRecord(ctx context.Context, db *sql.DB, name, srcPath, homePath, backupPath string) (bool, error) {
	md5Hex, err := fileMD5Hex(homePath)
	if err != nil {
		return false, fmt.Errorf("calculate file md5 failed: %w", err)
	}

	// If upload already created a staging-path row, reuse it instead of creating a duplicate.
	const updateStmt = `
UPDATE kb.inputs
SET name = $1,
    file_name = $2,
    backup_filename = $3,
    md5 = $4,
    modify_time = NOW()
WHERE file_name = $5
  AND COALESCE(backup_filename, '') = ''
RETURNING id`

	var existingID int64
	if err := db.QueryRowContext(ctx, updateStmt, name, homePath, backupPath, md5Hex, srcPath).Scan(&existingID); err == nil {
		return true, nil
	} else if err != sql.ErrNoRows {
		return false, fmt.Errorf("update existing staged kb.inputs failed: %w", err)
	}

	status, err := json.Marshal([]any{})
	if err != nil {
		return false, fmt.Errorf("marshal default status failed: %w", err)
	}

	const insertStmt = `
INSERT INTO kb.inputs (
    name,
    type,
    file_name,
    backup_filename,
    status,
    md5
) VALUES (
    $1,
    'pdf',
    $2,
    $3,
    $4::jsonb,
    $5
)`
	_, err = db.ExecContext(ctx, insertStmt, name, homePath, backupPath, string(status), md5Hex)
	if err != nil {
		return false, fmt.Errorf("insert kb.inputs failed: %w", err)
	}
	return false, nil
}

func fileMD5Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func uniquePath(dir, fileName string) (string, error) {
	base := strings.TrimSpace(filepath.Base(fileName))
	if base == "" || base == "." || base == string(os.PathSeparator) {
		return "", fmt.Errorf("invalid file name: %q", fileName)
	}

	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = "file"
	}

	candidate := filepath.Join(dir, base)
	if _, err := os.Stat(candidate); err != nil {
		if os.IsNotExist(err) {
			return candidate, nil
		}
		return "", err
	}

	for i := 1; ; i++ {
		name := fmt.Sprintf("%s_%d%s", stem, i, ext)
		candidate = filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				return candidate, nil
			}
			return "", err
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file failed: %w", err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("create target dir failed: %w", err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create target file failed: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy file content failed: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync target file failed: %w", err)
	}
	return nil
}

/*
func firstRepoDir(repoDirs []string) string {
	for _, dir := range repoDirs {
		if strings.TrimSpace(dir) != "" {
			return dir
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
*/

func verifyResultFilenameColumn(ctx context.Context) error {
	const stmt = `
SELECT COUNT(1)
FROM information_schema.columns
WHERE table_schema = 'kb'
  AND table_name = 'inputs'
  AND column_name = 'result_filename'`

	var count int
	if err := ApiTypes.ProjectDBHandle.QueryRowContext(ctx, stmt).Scan(&count); err != nil {
		return fmt.Errorf("query schema verification failed: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("column kb.inputs.result_filename is missing")
	}
	return nil
}
