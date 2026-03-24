package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/chendingplano/shared/go/api/databaseutil"
	sharedgoose "github.com/chendingplano/shared/go/api/goose"
	"github.com/chendingplano/shared/go/api/loggerutil"
	pdfparser "github.com/chendingplano/shared/go/api/parsers/pdf-parser"
	"github.com/joho/godotenv"
)

func main() {
	var (
		configPath       string
		runOnce          bool
		migrateOnly      bool
		verifySchemaOnly bool
	)
	flag.StringVar(&configPath, "config", "../../../config.toml", "Path to config file")
	flag.BoolVar(&runOnce, "once", false, "Run exactly one polling cycle then exit")
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

	if err := databaseutil.InitDB(newCtx, ApiTypes.CommonConfig); err != nil {
		logger.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer databaseutil.CloseDatabase(ApiTypes.CommonConfig)

	if ApiTypes.ProjectDBHandle == nil {
		logger.Error("project DB handle is nil")
		os.Exit(1)
	}
	if ApiTypes.SharedDBHandle == nil {
		logger.Error("shared DB handle is nil")
		os.Exit(1)
	}

	migrateCfg := ApiTypes.CommonConfig.MigrationConfig
	if err := sharedgoose.RunProjectMigrations(ctx, logger, migrateCfg, ApiTypes.ProjectDBHandle); err != nil {
		logger.Error("project migrations failed", "error", err)
		os.Exit(1)
	}
	if err := sharedgoose.RunSharedMigrations(ctx, logger, migrateCfg, ApiTypes.SharedDBHandle); err != nil {
		logger.Error("shared migrations failed", "error", err)
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

	parserCfg := config.GetPDFParserConfig()
	if !parserCfg.Enabled {
		logger.Warn("pdf parser is disabled by config; exiting")
		return
	}

	svcCfg := pdfparser.Config{
		PollInterval:      time.Duration(parserCfg.PollIntervalSeconds) * time.Second,
		BatchSize:         parserCfg.BatchSize,
		StagingDir:        parserCfg.StagingDir,
		RepoDirs:          parserCfg.RepoDirs,
		BackupDir:         parserCfg.BackupDir,
		PythonBin:         parserCfg.PythonBin,
		PaddleOCRScript:   parserCfg.PaddleOCRScript,
		UsePaddleOCRVL:    parserCfg.UsePaddleOCRVL,
		DeleteFromStaging: parserCfg.DeleteFromStaging,
		WorkDir:           parserCfg.WorkDir,
	}

	svc, err := pdfparser.NewService(svcCfg)
	if err != nil {
		logger.Error("failed to initialize PDF parser service", "error", err)
		os.Exit(1)
	}

	if runOnce {
		if err := svc.ProcessOnce(context.Background(), logger); err != nil {
			logger.Error("PDF parser one-shot run failed", "error", err)
			os.Exit(1)
		}
		logger.Info("PDF parser one-shot run completed")
		return
	}

	stagingDir := firstNonEmpty(os.Getenv("DATA_STAGING_DIR"), parserCfg.StagingDir)
	backupDir := firstNonEmpty(os.Getenv("DATA_BACKUP_DIR"), parserCfg.BackupDir)
	homeDir := firstNonEmpty(os.Getenv("DATA_HOME_DIR"), firstRepoDir(parserCfg.RepoDirs))
	if strings.TrimSpace(stagingDir) == "" || strings.TrimSpace(backupDir) == "" || strings.TrimSpace(homeDir) == "" {
		logger.Error("staging thread directories are not configured",
			"DATA_STAGING_DIR", stagingDir,
			"DATA_BACKUP_DIR", backupDir,
			"DATA_HOME_DIR", homeDir,
		)
		os.Exit(1)
	}

	go runStagingThread(context.Background(), logger, ApiTypes.ProjectDBHandle, stagingDir, backupDir, homeDir)

	if err := svc.Run(context.Background(), logger); err != nil {
		logger.Error("PDF parser service failed", "error", err)
		os.Exit(1)
	}
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
	logger.Info("Process staging", "dir", stagingDir)
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
		homePath, err := uniquePath(homeDir, entry.Name())
		if err != nil {
			logger.Error("failed to allocate home path", "source", srcPath, "error", err)
			continue
		}

		if err := copyFile(srcPath, backupPath); err != nil {
			logger.Error("failed to backup staged file", "source", srcPath, "backup", backupPath, "error", err)
			continue
		}
		if err := os.Rename(srcPath, homePath); err != nil {
			logger.Error("failed to move staged file to home", "source", srcPath, "home", homePath, "error", err)
			continue
		}

		if err := insertStagedInputRecord(ctx, db, filepath.Base(homePath), homePath, backupPath); err != nil {
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
		)
	}

	return nil
}

func insertStagedInputRecord(ctx context.Context, db *sql.DB, name, homePath, backupPath string) error {
	status, err := json.Marshal([]any{})
	if err != nil {
		return fmt.Errorf("marshal default status failed: %w", err)
	}

	const stmt = `
INSERT INTO kb.inputs (
    name,
    type,
    file_name,
    backup_filename,
    status
) VALUES (
    $1,
    'pdf',
    $2,
    $3,
    $4::jsonb
)`
	_, err = db.ExecContext(ctx, stmt, name, homePath, backupPath, string(status))
	if err != nil {
		return fmt.Errorf("insert kb.inputs failed: %w", err)
	}
	return nil
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
