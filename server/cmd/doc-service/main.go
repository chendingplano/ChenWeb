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
	"os/exec"
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
	statusTimeLayout        = "20060102 15:04:05"
)

type statusEntry map[string]any

type docParseJob struct {
	recordID        int64
	homePath        string
	homeDir         string // DATA_HOME_DIR root; used to compute relative paths for DB
	fileType        string
	stagingFilename string // kb.inputs.staging_filename; used to derive output file name
}

func maxDocGoroutines() int {
	v := strings.TrimSpace(os.Getenv("MAX_DOC_GOROUTINE"))
	if v == "" {
		return 4
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 4
	}
	return n
}

func decodeStatusEntries(raw string) []statusEntry {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return []statusEntry{}
	}
	var arr []statusEntry
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr
	}
	var one statusEntry
	if err := json.Unmarshal([]byte(raw), &one); err == nil {
		return []statusEntry{one}
	}
	return []statusEntry{}
}

func statusEntryAsString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

func appendParsedStatus(rawStatus string, start time.Time, durationMs int64, procErr error) (string, error) {
	entries := decodeStatusEntries(rawStatus)
	entry := statusEntry{
		"operation":  "parsed",
		"start_time": start.Format(statusTimeLayout),
		"ms-used":    durationMs,
	}
	if procErr == nil {
		entry["proc_status"] = "success"
	} else {
		entry["proc_status"] = "failed"
		entry["error"] = procErr.Error()
	}

	replaced := false
	dedup := make([]statusEntry, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(statusEntryAsString(e["operation"])))
		if op != "parsed" {
			dedup = append(dedup, e)
			continue
		}
		if !replaced {
			dedup = append(dedup, entry)
			replaced = true
		}
	}
	if !replaced {
		dedup = append(dedup, entry)
	}

	out, err := json.Marshal(dedup)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// sofficeBinary resolves the LibreOffice CLI binary. Checks SOFFICE_PATH env
// first, then PATH, then the macOS app bundle.
func sofficeBinary() (string, error) {
	if path := strings.TrimSpace(os.Getenv("SOFFICE_PATH")); path != "" {
		return path, nil
	}
	if path, err := exec.LookPath("soffice"); err == nil {
		return path, nil
	}
	const macPath = "/Applications/LibreOffice.app/Contents/MacOS/soffice"
	if _, err := os.Stat(macPath); err == nil {
		return macPath, nil
	}
	return "", fmt.Errorf("(CWB_DOCX_007) soffice not found; install LibreOffice or set SOFFICE_PATH")
}

// convertToPDF converts the file at srcPath (.doc or .docx) to PDF using LibreOffice
// headless, writing the output into the same directory alongside the original.
// Returns the PDF path, the trimmed soffice output, and any error.
// The context controls the subprocess lifetime — callers should supply a timeout.
func convertToPDF(ctx context.Context, srcPath string) (pdfPath string, cmdOut string, err error) {
	soffice, err := sofficeBinary()
	if err != nil {
		return "", "", err
	}
	outDir := filepath.Dir(srcPath)
	cmd := exec.CommandContext(ctx, soffice,
		"--headless", "--norestore", "--nofirststartwizard",
		"--convert-to", "pdf", "--outdir", outDir, srcPath)
	raw, runErr := cmd.CombinedOutput()
	cmdOut = strings.TrimSpace(string(raw))
	if runErr != nil {
		return "", cmdOut, fmt.Errorf("(CWB_DOCX_008) soffice convert failed: %w", runErr)
	}
	stem := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
	pdfPath = filepath.Join(outDir, stem+".pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		return "", cmdOut, fmt.Errorf("(CWB_DOCX_008) expected pdf output not found at %s: %w", pdfPath, err)
	}
	return pdfPath, cmdOut, nil
}

func appendReroutedStatus(rawStatus string, start time.Time, note, originalRelPath, pdfRelPath string) (string, error) {
	entries := decodeStatusEntries(rawStatus)
	entry := statusEntry{
		"operation":          "docx-rerouted",
		"start_time":         start.Format(statusTimeLayout),
		"note":               note,
		"original_docx_path": originalRelPath,
		"pdf_path":           pdfRelPath,
	}
	entries = append(entries, entry)
	out, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func rerouteAsPDF(ctx context.Context, db *sql.DB, recordID int64, pdfRelPath, statusJSON string) error {
	const stmt = `
UPDATE kb.inputs
SET type = 'pdf',
    file_name = $1,
    result_filename = '',
    status = $2::jsonb,
    modify_time = NOW()
WHERE id = $3`
	_, err := db.ExecContext(ctx, stmt, pdfRelPath, statusJSON, recordID)
	return err
}

// setRecordParserName forces the per-record PDF parser backend (e.g. "mineru").
// Best-effort: the parser_name column is optional in some deployments, so callers
// should log — not fail — when this returns an error.
func setRecordParserName(ctx context.Context, db *sql.DB, recordID int64, parserName string) error {
	const stmt = `UPDATE kb.inputs SET parser_name = $1, modify_time = NOW() WHERE id = $2`
	_, err := db.ExecContext(ctx, stmt, parserName, recordID)
	return err
}

// rerouteToPDFPipeline converts job.homePath (.doc or .docx) to PDF, repoints the
// kb.inputs record at the PDF, optionally forces a PDF parser backend, and publishes
// a kb.pdf.staged event so the PDF/mineru pipeline takes over. note explains why the
// reroute happened (recorded in the docx-rerouted status entry).
//
// This is the shared path for both .doc files (which need layout-accurate pages,
// HTML tables, and image OCR that only the PDF pipeline provides) and image-only
// .docx files. Returns an error the caller should record as a parse failure.
func rerouteToPDFPipeline(ctx context.Context, logger ApiTypes.JimoLogger, db *sql.DB, publisher *stageEventPublisher, job docParseJob, rawStatus string, start time.Time, note, parserName string) error {
	convCtx, convCancel := context.WithTimeout(ctx, 2*time.Minute)
	logger.Info("docx worker: running soffice pdf conversion",
		"record_id", job.recordID,
		"soffice", func() string { p, _ := sofficeBinary(); return p }(),
		"src", job.homePath, "outdir", filepath.Dir(job.homePath))
	pdfPath, sofficeOut, convErr := convertToPDF(convCtx, job.homePath)
	convCancel()
	if sofficeOut != "" {
		logger.Info("docx worker: soffice output", "record_id", job.recordID, "output", sofficeOut)
	}
	if convErr != nil {
		return convErr
	}
	logger.Info("docx worker: converted to pdf; both files saved",
		"record_id", job.recordID, "src", job.homePath, "pdf", pdfPath)

	srcRelPath, err := relativePathFromHomeDir(job.homeDir, job.homePath)
	if err != nil {
		return fmt.Errorf("resolve source relative path: %w", err)
	}
	pdfRelPath, err := relativePathFromHomeDir(job.homeDir, pdfPath)
	if err != nil {
		return fmt.Errorf("resolve pdf relative path: %w", err)
	}

	updatedStatus, err := appendReroutedStatus(rawStatus, start, note, srcRelPath, pdfRelPath)
	if err != nil {
		return fmt.Errorf("build rerouted status: %w", err)
	}
	if err := rerouteAsPDF(ctx, db, job.recordID, pdfRelPath, updatedStatus); err != nil {
		return fmt.Errorf("reroute db update: %w", err)
	}
	// Force the requested parser (e.g. mineru for image OCR). Best-effort: the
	// parser_name column is optional, so a failure here only logs a warning.
	if parserName != "" {
		if err := setRecordParserName(ctx, db, job.recordID, parserName); err != nil {
			logger.Warn("docx worker: could not set parser_name (column may be absent); using pipeline default",
				"record_id", job.recordID, "parser_name", parserName, "error", err)
		}
	}

	if publisher == nil {
		logger.Warn("docx worker: publisher is nil, skipping pdf stage event",
			"record_id", job.recordID, "pdf", pdfPath)
		return nil
	}
	if err := publisher.Publish(stageEvent{
		RecordID:   job.recordID,
		Type:       "pdf",
		Status:     "success",
		Force:      true,
		FileFormat: "pdf",
		FileName:   pdfPath,
	}); err != nil {
		return fmt.Errorf("publish pdf stage event: %w", err)
	}
	logger.Info("docx worker: pdf stage event published",
		"record_id", job.recordID, "pdf", pdfPath, "subject", publisher.subject)
	return nil
}

func updateParsedStatus(ctx context.Context, db *sql.DB, recordID int64, resultFilename, statusJSON string) error {
	const stmt = `
UPDATE kb.inputs
SET result_filename = $1,
    status = $2::jsonb,
    modify_time = NOW()
WHERE id = $3`
	_, err := db.ExecContext(ctx, stmt, resultFilename, statusJSON, recordID)
	return err
}

// pdfPipelineParser is the PDF parser backend forced for documents rerouted from
// the doc pipeline. MinerU gives layout-accurate page numbers, HTML tables, image
// OCR, and formula recognition — none of which survive a flat text extraction.
const pdfPipelineParser = "mineru"

// runDocParseWorker processes staged .doc/.docx files. Both formats are converted to
// PDF and routed through the PDF+mineru pipeline. Direct .docx text extraction was
// dropped: .docx XML carries no rendered page layout (page numbers can't be recovered),
// and mineru natively produces HTML tables, image OCR, and formula LaTeX that a text
// extractor cannot. See doc-2026061003-docx-parser.md (Bug 3) for the full rationale.
func runDocParseWorker(ctx context.Context, logger ApiTypes.JimoLogger, db *sql.DB, jobs <-chan docParseJob, publisher *stageEventPublisher) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			start := time.Now()
			rawStatus := fetchRecordStatus(ctx, logger, db, job.recordID)

			ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(job.homePath)), ".")
			if ext == "" {
				ext = "doc"
			}
			note := fmt.Sprintf("%s: converted to pdf and routed to pdf+mineru pipeline", ext)

			logger.Info("docx worker: routing document to pdf+mineru pipeline",
				"record_id", job.recordID, "path", job.homePath, "ext", ext)
			if err := rerouteToPDFPipeline(ctx, logger, db, publisher, job, rawStatus, start,
				note, pdfPipelineParser); err != nil {
				logger.Error("docx worker: reroute to pdf+mineru failed",
					"record_id", job.recordID, "path", job.homePath, "error", err)
				recordParseFailure(ctx, logger, db, job.recordID, rawStatus, start, err)
			}
		}
	}
}

// fetchRecordStatus reads the current kb.inputs.status JSON for a record, falling
// back to an empty array when the row can't be read.
func fetchRecordStatus(ctx context.Context, logger ApiTypes.JimoLogger, db *sql.DB, recordID int64) string {
	var rawStatus string
	if err := db.QueryRowContext(ctx,
		`SELECT COALESCE(status::text, '[]') FROM kb.inputs WHERE id = $1`,
		recordID,
	).Scan(&rawStatus); err != nil {
		logger.Error("docx worker: fetch status failed", "record_id", recordID, "error", err)
		return "[]"
	}
	return rawStatus
}

// recordParseFailure appends a parsed/failed status entry for a record after a
// non-recoverable error (e.g. a failed PDF reroute).
func recordParseFailure(ctx context.Context, logger ApiTypes.JimoLogger, db *sql.DB, recordID int64, rawStatus string, start time.Time, cause error) {
	updatedStatus, err := appendParsedStatus(rawStatus, start, time.Since(start).Milliseconds(), cause)
	if err != nil {
		logger.Error("docx worker: build failed status failed", "record_id", recordID, "error", err)
		return
	}
	if err := updateParsedStatus(ctx, db, recordID, "", updatedStatus); err != nil {
		logger.Error("docx worker: update failed status failed", "record_id", recordID, "error", err)
	}
}

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

	numDocWorkers := maxDocGoroutines()
	docParseCh := make(chan docParseJob, numDocWorkers)
	for i := 0; i < numDocWorkers; i++ {
		go runDocParseWorker(ctx, logger, ApiTypes.ProjectDBHandle, docParseCh, publisher)
	}
	logger.Info("docx parse workers started", "count", numDocWorkers)

	go runStagingThread(ctx, logger, ApiTypes.ProjectDBHandle, stagingDir, backupDir, homeDir, publisher, docParseCh)
	logger.Info("staging thread started", "staging_dir", stagingDir, "backup_dir", backupDir, "home_dir", homeDir)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutdown signal received; stopping")
}

func runStagingThread(ctx context.Context, logger ApiTypes.JimoLogger, db *sql.DB, stagingDir, backupDir, homeDir string, publisher *stageEventPublisher, docParseCh chan docParseJob) {
	logger.Info("staging thread started",
		"fallback_interval", stagingFallbackInterval.String(),
		"watch_debounce", stagingWatchDebounce.String(),
		"staging_dir", stagingDir,
		"backup_dir", backupDir,
		"home_dir", homeDir,
	)

	process := func() error {
		return processStagingOnce(ctx, logger, db, stagingDir, backupDir, homeDir, publisher, docParseCh)
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

func processStagingOnce(ctx context.Context, logger ApiTypes.JimoLogger, db *sql.DB, stagingDir, backupDir, homeDir string, publisher *stageEventPublisher, docParseCh chan docParseJob) error {
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

		if strings.EqualFold(fileType, "doc") || strings.EqualFold(fileType, "docx") {
			if docParseCh != nil {
				select {
				case docParseCh <- docParseJob{recordID: recordID, homePath: homePath, homeDir: homeDir, fileType: fileType, stagingFilename: entry.Name()}:
				default:
					logger.Warn("docx parse queue full; dropping job",
						"record_id", recordID, "path", homePath, "type", fileType)
				}
			}
			continue
		}

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
