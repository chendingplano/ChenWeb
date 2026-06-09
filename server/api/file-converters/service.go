package fileconverters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/pathutil"
)

const statusTimeLayout = "20060102 15:04:05"
const DefaultLineFileGeneratedSubject = "kb.line-file-generated"

var ErrNotFound = errors.New("kb.inputs record not found")
var ErrMissingParsedSuccess = errors.New("kb.inputs record missing parsed success status")
var ErrResultFileNotFound = errors.New("kb.inputs result file not found")
var ErrUnsupportedParserName = errors.New("unsupported parser_name")
var ErrParserNotImplemented = errors.New("parser converter is not implemented")

type ConvertRequest struct {
	RecordID       int64  `json:"record_id"`
	ResultFilename string `json:"result_filename"`
	FileFormat     string `json:"file_format"`
	Type           string `json:"type"`
	Status         string `json:"status"`
	Force          *bool  `json:"force,omitempty"`
}

type InputRecord struct {
	ID             int64
	Type           string
	ParserName     string
	StatusRaw      string
	FileName       string
	ResultFilename string
}

type Store interface {
	GetInputRecord(ctx context.Context, id int64) (InputRecord, error)
	UpdateInputStatus(ctx context.Context, id int64, statusJSON string, errorMsg *string) error
}

type Publisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}

type LineFileGeneratedEvent struct {
	RecordID         int64  `json:"record_id"`
	Type             string `json:"type"`
	Status           string `json:"status"`
	FileFormat       string `json:"file_format"`
	ResultFilename   string `json:"result_filename"`
	LineFileFilename string `json:"line_file_filename"`
	Error            string `json:"error,omitempty"`
}

type Service struct {
	Store          Store
	Logger         *slog.Logger
	Now            func() time.Time
	Publisher      Publisher
	PublishSubject string
}

// Subscriber defines the minimal JetStream-facing contract this service needs.
// The concrete NATS/JetStream adapter can live outside this package.
type Subscriber interface {
	Subscribe(ctx context.Context, subject string, durable string, handler func(context.Context, []byte) error) error
}

func NewService(store Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		Store:          store,
		Logger:         logger,
		Now:            time.Now,
		PublishSubject: DefaultLineFileGeneratedSubject,
	}
}

func ParseRequest(payload []byte) (ConvertRequest, error) {
	// Use RawMessage for record_id so both JSON number (Python parser) and
	// JSON string (manual UI trigger) are accepted.
	var raw struct {
		RecordID       json.RawMessage `json:"record_id"`
		ResultFilename string          `json:"result_filename"`
		FileFormat     string          `json:"file_format"`
		Type           string          `json:"type"`
		Status         string          `json:"status"`
		Force          *bool           `json:"force,omitempty"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return ConvertRequest{}, fmt.Errorf("(MID_26041101) decode request: %w", err)
	}
	ridStr := strings.Trim(strings.TrimSpace(string(raw.RecordID)), `"`)
	rid, err := strconv.ParseInt(ridStr, 10, 64)
	if err != nil || rid <= 0 {
		return ConvertRequest{}, fmt.Errorf("(MID_26041102) invalid record_id: %s", raw.RecordID)
	}
	return ConvertRequest{
		RecordID:       rid,
		ResultFilename: raw.ResultFilename,
		FileFormat:     raw.FileFormat,
		Type:           raw.Type,
		Status:         raw.Status,
		Force:          raw.Force,
	}, nil
}

func shouldForceReprocess(req ConvertRequest) bool {
	if req.Force == nil {
		return true
	}
	return *req.Force
}

func docProcessorModeFromEnv() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("DOC_PROCESSOR_MODE")))
	if mode == "" {
		return "auto"
	}
	return mode
}

func shouldPublishLineFileGeneratedEvent() bool {
	return docProcessorModeFromEnv() == "auto"
}

func (s *Service) HandleMessage(ctx context.Context, payload []byte) error {
	req, err := ParseRequest(payload)
	if err != nil {
		return err
	}

	if req.Type != "" && !strings.EqualFold(strings.TrimSpace(req.Type), "pdf") {
		s.Logger.Info("ignored message by type", "type", req.Type, "record_id", req.RecordID)
		return nil
	}
	if req.Status != "" && !strings.EqualFold(strings.TrimSpace(req.Status), "success") {
		s.Logger.Info("ignored message by status", "status", req.Status, "record_id", req.RecordID)
		return nil
	}

	return s.HandleRequest(ctx, req)
}

// Run starts message consumption from a JetStream subject.
// Message filtering is handled in HandleMessage:
// type must be "pdf" and status must be "success" when provided.
func (s *Service) Run(ctx context.Context, sub Subscriber, subject string, durable string) error {
	if sub == nil {
		return errors.New("subscriber is nil")
	}
	if strings.TrimSpace(subject) == "" {
		return errors.New("subject is empty")
	}
	return sub.Subscribe(ctx, subject, durable, s.HandleMessage)
}

func (s *Service) HandleRequest(ctx context.Context, req ConvertRequest) error {
	start := s.Now()
	var lineFilePaths []string
	var procErr error
	defer func() {
		status := "success"
		if procErr != nil {
			status = "failed"
		}
		s.Logger.Info("finished processing converter record",
			"record_id", req.RecordID,
			"status", status,
			"line_files", lineFilePaths,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}()

	rec, err := s.Store.GetInputRecord(ctx, req.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			procErr = fmt.Errorf("(MID_26041103) record_id=%d: %w", req.RecordID, ErrNotFound)
			return procErr
		}
		procErr = fmt.Errorf("(MID_26041104) load record %d: %w", req.RecordID, err)
		return procErr
	}
	if !shouldForceReprocess(req) && HasOperationSuccess(rec.StatusRaw, "converted") {
		s.Logger.Info("skip reprocess because converted already succeeded and force=false", "record_id", req.RecordID)
		return nil
	}

	lineFilePaths, procErr = s.convert(ctx, rec)

	durationMs := time.Since(start).Milliseconds()
	updatedStatus, statusErr := appendConvertedStatus(rec.StatusRaw, start, durationMs, procErr)
	if statusErr != nil {
		procErr = errors.Join(procErr, fmt.Errorf("(MID_26041105) build converted status: %w", statusErr))
		return procErr
	}

	var errorMsg *string
	if procErr != nil {
		errStr := procErr.Error()
		errorMsg = &errStr
	}
	if err := s.Store.UpdateInputStatus(ctx, rec.ID, updatedStatus, errorMsg); err != nil {
		procErr = errors.Join(procErr, fmt.Errorf("(MID_26041106) update record status: %w", err))
		return procErr
	}
	for _, lineFilePath := range lineFilePaths {
		if emitErr := s.emitLineFileGeneratedEvent(ctx, req.RecordID, lineFilePath); emitErr != nil {
			procErr = errors.Join(procErr, emitErr)
		}
	}
	return procErr
}

type parserJSONFile struct {
	path   string
	parser string
}

func findParserJSONs(dir, stem string) []parserJSONFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	prefix := strings.ToLower(stem) + "_"
	var out []parserJSONFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.EqualFold(filepath.Ext(name), ".json") {
			continue
		}
		base := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
		if !strings.HasPrefix(base, prefix) {
			continue
		}
		parserName := base[len(prefix):]
		if parserName == "" {
			continue
		}
		out = append(out, parserJSONFile{
			path:   filepath.Join(dir, name),
			parser: parserName,
		})
	}
	return out
}

func convertOneParserFile(inputPath, parserName string) (string, error) {
	switch parserName {
	case "opendata":
		return ConvertOpenDataFile(inputPath)
	case "mineru":
		return ConvertMineruFile(inputPath)
	case "paddleocr":
		return "", fmt.Errorf("(MID_26042601) parser_name=%q: %w", parserName, ErrParserNotImplemented)
	case "docline":
		return "", fmt.Errorf("(MID_26042603) parser_name=%q: %w", parserName, ErrParserNotImplemented)
	default:
		return "", fmt.Errorf("(MID_26041110) unsupported parser_name=%q: %w", parserName, ErrUnsupportedParserName)
	}
}

func (s *Service) convert(_ context.Context, rec InputRecord) ([]string, error) {
	if !strings.EqualFold(strings.TrimSpace(rec.Type), "pdf") {
		return nil, fmt.Errorf("(MID_26041107) record_id=%d type=%q is not pdf", rec.ID, rec.Type)
	}
	if !HasParsedSuccess(rec.StatusRaw) {
		return nil, fmt.Errorf("(MID_26041108) record_id=%d: %w", rec.ID, ErrMissingParsedSuccess)
	}

	resolvedFileName := pathutil.ResolveDataHomePath(rec.FileName)
	recordDir := filepath.Dir(resolvedFileName)
	stem := strings.TrimSuffix(filepath.Base(resolvedFileName), filepath.Ext(resolvedFileName))

	parserFiles := findParserJSONs(recordDir, stem)
	if len(parserFiles) == 0 {
		return nil, fmt.Errorf("(MID_26041111) record_id=%d stem=%q dir=%q: %w",
			rec.ID, stem, recordDir, ErrResultFileNotFound)
	}

	var outPaths []string
	var errs []error
	for _, pf := range parserFiles {
		out, err := convertOneParserFile(pf.path, pf.parser)
		if err != nil {
			errs = append(errs, err)
			s.Logger.Warn("parser conversion failed",
				"record_id", rec.ID, "parser", pf.parser, "error", err)
			continue
		}
		s.Logger.Info("converted parser output",
			"record_id", rec.ID, "parser", pf.parser, "line_file", out)
		outPaths = append(outPaths, out)
	}

	if len(outPaths) == 0 {
		return nil, errors.Join(errs...)
	}
	return outPaths, errors.Join(errs...)
}

func resolveOpenDataInputFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("(MID_26041114) opendata input path is empty")
	}
	if hasTopLevelKids(path) {
		return path, nil
	}

	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return path, nil
	}

	preferred := preferredOpenDataJSONName(path)
	type candidate struct {
		path     string
		score    int
		basename string
	}
	candidates := make([]candidate, 0, 8)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return path, nil
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.EqualFold(filepath.Ext(name), ".json") {
			continue
		}
		cPath := filepath.Join(dir, name)
		if cPath == path {
			continue
		}
		if !hasTopLevelKids(cPath) {
			continue
		}

		score := 0
		lowerName := strings.ToLower(name)
		if preferred != "" && strings.EqualFold(name, preferred) {
			score += 100
		}
		if strings.HasPrefix(lowerName, "parse_result_") {
			score -= 20
		}
		if strings.HasPrefix(lowerName, "ocr_rslt_") {
			score -= 10
		}
		candidates = append(candidates, candidate{
			path:     cPath,
			score:    score,
			basename: lowerName,
		})
	}
	if len(candidates) == 0 {
		return path, nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].basename < candidates[j].basename
	})
	return candidates[0].path, nil
}

func hasTopLevelKids(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	return streamingHasTopLevelArrayKey(f, "kids")
}

// streamingHasTopLevelArrayKey scans a JSON object stream to check whether the
// named key exists at the top level and its value is a JSON array. It reads
// only as far as the target key, so large files (>8 MB) are handled correctly.
func streamingHasTopLevelArrayKey(r io.Reader, targetKey string) bool {
	dec := json.NewDecoder(r)

	tok, err := dec.Token()
	if err != nil {
		return false
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return false
	}

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return false
		}
		key, ok := keyTok.(string)
		if !ok {
			return false
		}
		if key == targetKey {
			valTok, err := dec.Token()
			if err != nil {
				return false
			}
			delim, ok := valTok.(json.Delim)
			return ok && delim == '['
		}
		if err := skipJSONValue(dec); err != nil {
			return false
		}
	}
	return false
}

// skipJSONValue advances dec past the next complete JSON value. dec must have
// already consumed the opening delimiter for container types before calling.
func skipJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch v := tok.(type) {
	case json.Delim:
		switch v {
		case '{':
			for dec.More() {
				if err := skipJSONValue(dec); err != nil { // key
					return err
				}
				if err := skipJSONValue(dec); err != nil { // value
					return err
				}
			}
			_, err = dec.Token() // '}'
			return err
		case '[':
			for dec.More() {
				if err := skipJSONValue(dec); err != nil {
					return err
				}
			}
			_, err = dec.Token() // ']'
			return err
		}
	}
	return nil
}

func preferredOpenDataJSONName(path string) string {
	base := strings.ToLower(filepath.Base(path))
	if !strings.HasPrefix(base, "parse_result_") {
		return ""
	}

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	type parseResultMeta struct {
		SourcePDF string `json:"source_pdf"`
	}
	var meta parseResultMeta
	if err := json.NewDecoder(io.LimitReader(f, 4*1024*1024)).Decode(&meta); err != nil {
		return ""
	}
	sourcePDF := strings.TrimSpace(meta.SourcePDF)
	if sourcePDF == "" {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(sourcePDF), filepath.Ext(sourcePDF)) + ".json"
}

func (s *Service) emitLineFileGeneratedEvent(ctx context.Context, recordID int64, lineFilePath string) error {
	if s.Publisher == nil {
		return nil
	}
	if !shouldPublishLineFileGeneratedEvent() {
		s.Logger.Info("skip line-file-generated publish because DOC_PROCESSOR_MODE is not auto",
			"record_id", recordID,
			"doc_processor_mode", docProcessorModeFromEnv(),
		)
		return nil
	}
	subject := strings.TrimSpace(s.PublishSubject)
	if subject == "" {
		subject = DefaultLineFileGeneratedSubject
	}

	ev := LineFileGeneratedEvent{
		RecordID:         recordID,
		Type:             "pdf",
		Status:           "success",
		FileFormat:       "txt",
		LineFileFilename: strings.TrimSpace(lineFilePath),
	}

	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("(MID_26041112) encode line-file-generated event: %w", err)
	}
	if err := s.Publisher.Publish(ctx, subject, payload); err != nil {
		return fmt.Errorf("(MID_26041113) publish event subject=%s: %w", subject, err)
	}
	return nil
}

func resolveInputFile(req ConvertRequest, rec InputRecord) (string, error) {
	candidates := make([]string, 0, 16)
	if strings.TrimSpace(req.ResultFilename) != "" {
		candidates = append(candidates, pathutil.ResolveDataHomePath(req.ResultFilename))
	}
	if strings.TrimSpace(rec.ResultFilename) != "" {
		candidates = append(candidates, pathutil.ResolveDataHomePath(rec.ResultFilename))
	}

	baseDir := filepath.Dir(pathutil.ResolveDataHomePath(rec.FileName))
	for _, name := range []string{strings.TrimSpace(req.ResultFilename), strings.TrimSpace(rec.ResultFilename)} {
		if name == "" || filepath.IsAbs(name) || baseDir == "." || baseDir == "" {
			continue
		}
		candidates = append(candidates, filepath.Join(baseDir, name))
	}
	if baseDir != "." && baseDir != "" {
		// Fallback candidates when result_filename is empty/missing.
		candidates = append(candidates, filepath.Join(baseDir, fmt.Sprintf("parse_result_%d.json", rec.ID)))
		if base := strings.TrimSpace(filepath.Base(rec.FileName)); base != "" {
			candidates = append(candidates, filepath.Join(baseDir, strings.TrimSuffix(base, filepath.Ext(base))+".json"))
		}
	}
	if baseDir != "." && baseDir != "" {
		if entries, err := os.ReadDir(baseDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
					candidates = append(candidates, filepath.Join(baseDir, entry.Name()))
				}
			}
		}
	}

	seen := map[string]struct{}{}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	return "", fmt.Errorf("(MID_26041111) record_id=%d requested_result=%q: %w", rec.ID, req.ResultFilename, ErrResultFileNotFound)
}

type statusEntry map[string]any

func decodeStatus(raw string) []statusEntry {
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

func HasParsedSuccess(rawStatus string) bool {
	return HasOperationSuccess(rawStatus, "parsed")
}

func HasOperationSuccess(rawStatus string, operation string) bool {
	wantOp := strings.ToLower(strings.TrimSpace(operation))
	if wantOp == "" {
		return false
	}
	entries := decodeStatus(rawStatus)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != wantOp {
			continue
		}
		proc := strings.ToLower(strings.TrimSpace(asString(e["proc-status"])))
		if proc == "success" {
			return true
		}
		if proc == "" {
			legacy := strings.ToLower(strings.TrimSpace(asString(e["status"])))
			if legacy == "success" {
				return true
			}
		}
	}
	return false
}

func appendConvertedStatus(raw string, start time.Time, durationMs int64, procErr error) (string, error) {
	entries := decodeStatus(raw)
	entry := statusEntry{
		"operation":  "converted",
		"start_time": start.Format(statusTimeLayout),
		"ms-used":    durationMs,
	}
	if procErr == nil {
		entry["proc-status"] = "success"
	} else {
		entry["proc-status"] = "failed"
		entry["error"] = procErr.Error()
	}

	replaced := false
	dedup := make([]statusEntry, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "converted" {
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

	entries = dedup
	out, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

type SQLStore struct {
	DB *sql.DB
}

func (s SQLStore) GetInputRecord(ctx context.Context, id int64) (InputRecord, error) {
	const stmt = `
SELECT id,
       COALESCE(type, ''),
       COALESCE(parser_name, ''),
       COALESCE(status::text, '[]'),
       COALESCE(file_name, ''),
       COALESCE(result_filename, '')
FROM kb.inputs
WHERE id = $1`

	var rec InputRecord
	err := s.DB.QueryRowContext(ctx, stmt, id).Scan(
		&rec.ID,
		&rec.Type,
		&rec.ParserName,
		&rec.StatusRaw,
		&rec.FileName,
		&rec.ResultFilename,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return InputRecord{}, ErrNotFound
		}
		return InputRecord{}, err
	}
	return rec, nil
}

func (s SQLStore) UpdateInputStatus(ctx context.Context, id int64, statusJSON string, errorMsg *string) error {
	const stmt = `
UPDATE kb.inputs
SET status = $1::jsonb,
    error_msg = $2,
    modify_time = NOW()
WHERE id = $3`
	_, err := s.DB.ExecContext(ctx, stmt, statusJSON, errorMsg, id)
	return err
}
