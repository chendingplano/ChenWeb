package main

import (
	"archive/zip"
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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
)

const (
	stagingWatchDebounce    = 500 * time.Millisecond
	stagingFallbackInterval = 10 * time.Second
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

	_ = godotenv.Load("../../../.env")
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

	publisher, err := newStageEventPublisher(logger)
	if err != nil {
		logger.Error("failed to initialize JetStream publisher; stage events disabled", "error", err)
	}
	if publisher != nil {
		defer publisher.Close()
	}

	go runStagingThread(ctx, logger, ApiTypes.ProjectDBHandle, stagingDir, backupDir, homeDir, publisher)
	logger.Info("staging thread started", "staging_dir", stagingDir, "backup_dir", backupDir, "home_dir", homeDir)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutdown signal received; stopping")
}

func runStagingThread(ctx context.Context, logger ApiTypes.JimoLogger, db *sql.DB, stagingDir, backupDir, homeDir string, publisher *stageEventPublisher) {
	logger.Info("staging thread started",
		"fallback_interval", stagingFallbackInterval.String(),
		"watch_debounce", stagingWatchDebounce.String(),
		"staging_dir", stagingDir,
		"backup_dir", backupDir,
		"home_dir", homeDir,
	)

	process := func() error {
		return processStagingOnce(ctx, logger, db, stagingDir, backupDir, homeDir, publisher)
	}

	if err := process(); err != nil {
		logger.Error("initial staging cycle failed", "error", err)
	}

	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		logger.Error("ensure staging watch dir failed; falling back to polling", "staging_dir", stagingDir, "error", err)
		runStagingFallbackLoop(ctx, logger, stagingFallbackInterval, process)
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("create staging watcher failed; falling back to polling", "staging_dir", stagingDir, "error", err)
		runStagingFallbackLoop(ctx, logger, stagingFallbackInterval, process)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(stagingDir); err != nil {
		logger.Error("watch staging dir failed; falling back to polling", "staging_dir", stagingDir, "error", err)
		runStagingFallbackLoop(ctx, logger, stagingFallbackInterval, process)
		return
	}

	ticker := time.NewTicker(stagingFallbackInterval)
	defer ticker.Stop()

	runStagingEventLoop(ctx, logger, watcher.Events, watcher.Errors, ticker.C, stagingWatchDebounce, process)
}

func runStagingFallbackLoop(ctx context.Context, logger ApiTypes.JimoLogger, interval time.Duration, process func() error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("staging thread stopping", "reason", ctx.Err())
			return
		case <-ticker.C:
			if err := process(); err != nil {
				logger.Error("staging fallback cycle failed", "error", err)
			}
		}
	}
}

func runStagingEventLoop(
	ctx context.Context,
	logger ApiTypes.JimoLogger,
	events <-chan fsnotify.Event,
	watchErrors <-chan error,
	fallback <-chan time.Time,
	debounce time.Duration,
	process func() error,
) {
	var debounceTimer *time.Timer
	var debounceC <-chan time.Time
	var pendingEventCount int
	var pendingLastEvent string
	var pendingLastPath string
	defer func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
	}()

	runCycle := func(reason string) {
		if pendingEventCount > 0 {
			logger.Info("staging directory changed",
				"reason", reason,
				"event_count", pendingEventCount,
				"last_event", pendingLastEvent,
				"last_path", pendingLastPath,
			)
			pendingEventCount = 0
			pendingLastEvent = ""
			pendingLastPath = ""
		}
		if err := process(); err != nil {
			logger.Error("staging cycle failed", "reason", reason, "error", err)
		}
	}

	scheduleCycle := func(event fsnotify.Event) {
		pendingEventCount++
		pendingLastEvent = event.String()
		pendingLastPath = event.Name
		if debounce <= 0 {
			runCycle("watch_event")
			return
		}
		if debounceTimer != nil {
			if !debounceTimer.Stop() {
				select {
				case <-debounceTimer.C:
				default:
				}
			}
		}
		debounceTimer = time.NewTimer(debounce)
		debounceC = debounceTimer.C
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("staging thread stopping", "reason", ctx.Err())
			return
		case event, ok := <-events:
			if !ok {
				logger.Warn("staging watcher event channel closed")
				return
			}
			if shouldProcessStagingEvent(event) {
				logger.Debug("staging watcher event", "event", event.String(), "path", event.Name)
				scheduleCycle(event)
			}
		case err, ok := <-watchErrors:
			if !ok {
				logger.Warn("staging watcher error channel closed")
				return
			}
			logger.Error("staging watcher error", "error", err)
		case <-fallback:
			runCycle("fallback_rescan")
		case <-debounceC:
			debounceTimer = nil
			debounceC = nil
			runCycle("watch_event")
		}
	}
}

func shouldProcessStagingEvent(event fsnotify.Event) bool {
	return event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) != 0
}

func processStagingOnce(ctx context.Context, logger ApiTypes.JimoLogger, db *sql.DB, stagingDir, backupDir, homeDir string, publisher *stageEventPublisher) error {
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
		recordID, updated, homePath, backupPath, fileType, err := ingestInputFile(ctx, db, homeDir, backupDir, entry.Name(), srcPath)
		if err != nil {
			logger.Error("failed to insert kb.inputs row for staged file", "source", srcPath, "error", err)
			continue
		}

		if strings.EqualFold(filepath.Ext(entry.Name()), ".zip") {
			if err := ingestZipChildren(ctx, logger, db, homeDir, backupDir, homePath, publisher); err != nil {
				logger.Error("failed to ingest zip entries", "source", srcPath, "record_id", recordID, "home", homePath, "error", err)
				continue
			}
		}

		if err := os.Remove(srcPath); err != nil {
			logger.Error("failed to remove staged source file", "source", srcPath, "error", err)
			continue
		}

		logger.Info("staged file ingested",
			"source", srcPath,
			"home", homePath,
			"backup", backupPath,
			"type", fileType,
			"record_id", recordID,
			"updated_existing_row", updated,
		)
		if publisher != nil && strings.EqualFold(fileType, "pdf") {
			if err := publisher.Publish(stageEvent{
				RecordID:   recordID,
				Type:       fileType,
				Status:     "success",
				Force:      true,
				FileFormat: fileType,
				FileName:   homePath,
			}); err != nil {
				logger.Error("failed to publish stage event", "record_id", recordID, "error", err)
			}
		}
	}

	return nil
}

func ingestInputFile(
	ctx context.Context,
	db *sql.DB,
	homeDir, backupDir, stagingName, srcPath string,
) (recordID int64, updated bool, homePath string, backupPath string, fileType string, err error) {
	fileType = detectInputType(stagingName)

	backupPath, err = uniquePath(backupDir, filepath.Base(stagingName))
	if err != nil {
		return 0, false, "", "", fileType, fmt.Errorf("allocate backup path: %w", err)
	}
	if err := copyFile(srcPath, backupPath); err != nil {
		return 0, false, "", "", fileType, fmt.Errorf("backup staged file: %w", err)
	}

	recordID, updated, err = upsertStagedInputRecord(ctx, db, filepath.Base(stagingName), srcPath, fileType)
	if err != nil {
		return 0, false, "", backupPath, fileType, err
	}

	repoDir, err := repoDirForRecord(homeDir, recordID)
	if err != nil {
		return 0, false, "", backupPath, fileType, fmt.Errorf("resolve record repo dir: %w", err)
	}
	homePath = filepath.Join(repoDir, filepath.Base(stagingName))
	if err := copyFile(srcPath, homePath); err != nil {
		return 0, false, "", backupPath, fileType, fmt.Errorf("copy staged file to home: %w", err)
	}
	if err := finalizeStagedInputRecord(ctx, db, recordID, homeDir, homePath, backupPath); err != nil {
		return 0, false, homePath, backupPath, fileType, err
	}
	return recordID, updated, homePath, backupPath, fileType, nil
}

func ingestZipChildren(
	ctx context.Context,
	logger ApiTypes.JimoLogger,
	db *sql.DB,
	homeDir, backupDir, zipHomePath string,
	publisher *stageEventPublisher,
) error {
	reader, err := zip.OpenReader(zipHomePath)
	if err != nil {
		return fmt.Errorf("open zip file: %w", err)
	}
	defer reader.Close()

	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}

		rc, err := entry.Open()
		if err != nil {
			logger.Warn("open zip entry failed", "zip", zipHomePath, "entry", entry.Name, "error", err)
			continue
		}

		tmpFile, err := os.CreateTemp("", "pdf-parser-zip-*"+filepath.Ext(entry.Name))
		if err != nil {
			_ = rc.Close()
			logger.Warn("create temp file for zip entry failed", "zip", zipHomePath, "entry", entry.Name, "error", err)
			continue
		}

		_, copyErr := io.Copy(tmpFile, rc)
		closeErr := rc.Close()
		syncErr := tmpFile.Sync()
		tmpCloseErr := tmpFile.Close()
		if copyErr != nil || closeErr != nil || syncErr != nil || tmpCloseErr != nil {
			_ = os.Remove(tmpFile.Name())
			logger.Warn("materialize zip entry failed", "zip", zipHomePath, "entry", entry.Name, "copy_error", copyErr, "close_error", closeErr, "sync_error", syncErr, "tmp_close_error", tmpCloseErr)
			continue
		}

		recordID, updated, homePath, _, fileType, err := ingestInputFile(ctx, db, homeDir, backupDir, filepath.Base(entry.Name), tmpFile.Name())
		_ = os.Remove(tmpFile.Name())
		if err != nil {
			logger.Error("failed to ingest zip child", "zip", zipHomePath, "entry", entry.Name, "error", err)
			continue
		}

		logger.Info("zip child ingested",
			"zip", zipHomePath,
			"entry", entry.Name,
			"home", homePath,
			"type", fileType,
			"record_id", recordID,
			"updated_existing_row", updated,
		)
		if publisher != nil && strings.EqualFold(fileType, "pdf") {
			if err := publisher.Publish(stageEvent{
				RecordID:   recordID,
				Type:       fileType,
				Status:     "success",
				Force:      true,
				FileFormat: fileType,
				FileName:   homePath,
			}); err != nil {
				logger.Error("failed to publish zip child stage event", "zip", zipHomePath, "entry", entry.Name, "record_id", recordID, "error", err)
			}
		}
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

func repoDirForRecord(homeDir string, recordID int64) (string, error) {
	if recordID <= 0 {
		return "", fmt.Errorf("invalid record id: %d", recordID)
	}
	groupID := recordID / 1000
	return filepath.Join(homeDir, "Artifacts", strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10)), nil
}

func upsertStagedInputRecord(ctx context.Context, db *sql.DB, staging_filename, srcPath, fileType string) (int64, bool, error) {
	md5Hex, err := fileMD5Hex(srcPath)
	if err != nil {
		return 0, false, fmt.Errorf("calculate file md5 failed: %w", err)
	}

	// If upload already created a staging-path row, reuse it instead of creating a duplicate.
	const updateStmt = `
UPDATE kb.inputs
SET staging_filename = $1,
    md5 = $2,
    modify_time = NOW()
WHERE file_name = $3
  AND COALESCE(backup_filename, '') = ''
RETURNING id`

	var existingID int64
	if err := db.QueryRowContext(ctx, updateStmt, staging_filename, md5Hex, srcPath).Scan(&existingID); err == nil {
		return existingID, true, nil
	} else if err != sql.ErrNoRows {
		return 0, false, fmt.Errorf("update existing staged kb.inputs failed: %w", err)
	}

	status, err := json.Marshal([]any{})
	if err != nil {
		return 0, false, fmt.Errorf("marshal default status failed: %w", err)
	}

	const insertStmt = `
INSERT INTO kb.inputs (
    staging_filename,
    type,
    file_name,
    backup_filename,
    status,
    md5
) VALUES (
    $1,
    $2,
    $3,
    '',
    $4::jsonb,
    $5
)
RETURNING id`
	var insertedID int64
	if err := db.QueryRowContext(ctx, insertStmt, staging_filename, fileType, srcPath, string(status), md5Hex).Scan(&insertedID); err != nil {
		return 0, false, fmt.Errorf("insert kb.inputs failed: %w", err)
	}
	return insertedID, false, nil
}

func finalizeStagedInputRecord(ctx context.Context, db *sql.DB, recordID int64, homeDir, homePath, backupPath string) error {
	relativeHomePath, err := relativePathFromHomeDir(homeDir, homePath)
	if err != nil {
		return fmt.Errorf("normalize file_name for record %d: %w", recordID, err)
	}
	relativeBackupPath, err := relativePathFromParentDir(os.Getenv("DATA_BACKUP_DIR"), backupPath)
	if err != nil {
		return fmt.Errorf("normalize backup_filename for record %d: %w", recordID, err)
	}

	const stmt = `
UPDATE kb.inputs
SET file_name = $1,
    backup_filename = $2,
    modify_time = NOW()
WHERE id = $3`
	if _, err := db.ExecContext(ctx, stmt, relativeHomePath, relativeBackupPath, recordID); err != nil {
		return fmt.Errorf("update finalized kb.inputs path failed: %w", err)
	}
	return nil
}

func detectInputType(name string) string {
	ext := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(filepath.Ext(name))), ".")
	if ext == "" {
		return "file"
	}
	return ext
}

func relativePathFromHomeDir(homeDir, fullPath string) (string, error) {
	cleanHome := filepath.Clean(strings.TrimSpace(homeDir))
	cleanPath := filepath.Clean(strings.TrimSpace(fullPath))
	if cleanHome == "" || cleanPath == "" {
		return "", fmt.Errorf("homeDir=%q fullPath=%q", homeDir, fullPath)
	}
	rel, err := filepath.Rel(cleanHome, cleanPath)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return "", fmt.Errorf("path %q is outside home dir %q", cleanPath, cleanHome)
	}
	return rel, nil
}

func relativePathFromParentDir(baseDir, fullPath string) (string, error) {
	cleanBaseDir := filepath.Clean(strings.TrimSpace(baseDir))
	if cleanBaseDir == "" {
		return "", fmt.Errorf("baseDir=%q", baseDir)
	}
	parentDir := filepath.Dir(cleanBaseDir)
	cleanPath := filepath.Clean(strings.TrimSpace(fullPath))
	if cleanPath == "" {
		return "", fmt.Errorf("fullPath=%q", fullPath)
	}
	rel, err := filepath.Rel(parentDir, cleanPath)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return "", fmt.Errorf("path %q is outside parent dir %q", cleanPath, parentDir)
	}
	return rel, nil
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
		return "", fmt.Errorf("invalid file staging_filename: %q", fileName)
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
