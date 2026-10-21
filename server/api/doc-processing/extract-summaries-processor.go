package docprocessing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type ExtractSummariesProcessor struct {
	InputStore DocMetadataStore
	Store      ExtractSummaryStore
	Runner     SumyRunner
	Config     SumyConfig
	ConfigErr  error
	Logger     ApiTypes.JimoLogger
	ProcLogger DocProcLogger
}

func NewExtractSummariesProcessor(inputStore DocMetadataStore, db *sql.DB, logger ApiTypes.JimoLogger) *ExtractSummariesProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("20261021-741")
	}
	c, err := sumyConfigFromEnv()
	return &ExtractSummariesProcessor{InputStore: inputStore, Store: ExtractSummarySQLStore{DB: db}, Runner: PythonSumyRunner{Config: c}, Config: c, ConfigErr: err, Logger: logger, ProcLogger: DocProcLogger{DB: db}}
}
func (p *ExtractSummariesProcessor) Name() string { return "extract_summaries" }

func (p *ExtractSummariesProcessor) HandleEvent(ctx context.Context, payload []byte) (procErr error) {
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return err
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}
	if p.InputStore == nil || p.Store == nil || p.Runner == nil {
		return fmt.Errorf("extract_summaries is not configured")
	}
	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		return err
	}
	start := time.Now()
	total, completed := 0, 0
	p.Logger.Info("extract summaries started", "record_id", rec.ID, "algorithm", p.Config.Algorithm)
	defer func() {
		if isCtxStopped(ctx) {
			procErr = ErrPipelineStopped
		} else if procErr == nil && ctx.Err() != nil {
			procErr = ctx.Err()
		}
		state := "success"
		if procErr != nil {
			state = "failed"
		}
		if errors.Is(procErr, ErrPipelineStopped) {
			state = "stopped"
		}
		finishCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := p.persistStatus(finishCtx, rec.ID, start, state, total, completed, procErr); err != nil {
			procErr = errors.Join(procErr, fmt.Errorf("persist extract summaries status: %w", err))
		}
		ms := time.Since(start).Milliseconds()
		extra := mustJSONString(map[string]any{"algorithm": p.Config.Algorithm, "sentence_count": p.Config.SentenceCount, "num_chunks": total, "num_summaries": completed, "proc_status": state}, map[string]any{})
		var errorText *string
		if procErr != nil {
			s := procErr.Error()
			errorText = &s
		}
		if p.ProcLogger.DB != nil {
			if err := p.ProcLogger.LogSummary(finishCtx, EntryTypeDocProcSummary, DocProcLogRecord{DocProcName: p.Name(), RecordID: &rec.ID, ExtraInfoJSON: &extra, Errors: errorText, MSUsed: &ms}, "20261021-742"); err != nil {
				p.Logger.Warn("extract summaries log write failed", "record_id", rec.ID, "error", err)
			}
		}
		p.Logger.Info("extract summaries finished", "record_id", rec.ID, "algorithm", p.Config.Algorithm, "proc_status", state, "num_summaries", completed, "ms_used", ms, "error", procErr)
	}()
	if p.ConfigErr != nil {
		return p.ConfigErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	chunks, verify, err := p.loadChunks(ctx, evt, rec)
	if err != nil {
		return err
	}
	total = len(chunks)
	request := SumyRequest{Algorithm: p.Config.Algorithm, SentenceCount: p.Config.SentenceCount, Language: rec.SourceLanguage, Chunks: sumyChunks(chunks)}
	if err := validateSumyRequest(request); err != nil {
		return err
	}
	if err := p.persistStatus(ctx, rec.ID, start, "active", total, 0, nil); err != nil {
		return err
	}
	response, err := p.Runner.Extract(ctx, request)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateSumyResponse(request, response); err != nil {
		return err
	}
	inputJSON, _ := json.Marshal(request)
	inputHash := sha256.Sum256(inputJSON)
	for i := range response.Summaries {
		config := map[string]any{"sentence_count": p.Config.SentenceCount, "versions": response.Versions, "input_sha256": hex.EncodeToString(inputHash[:]), "worker_version": "1.0.0"}
		if response.Summaries[i].RankingFallback != "" {
			config["ranking_fallback"] = response.Summaries[i].RankingFallback
		}
		response.Summaries[i].ConfigJSON = mustJSONString(config, map[string]any{})
	}
	sort.Slice(response.Summaries, func(i, j int) bool { return response.Summaries[i].SeqNo < response.Summaries[j].SeqNo })
	if err := p.Store.Replace(ctx, rec.ID, p.Config.Algorithm, response.Summaries, verify); err != nil {
		return err
	}
	completed = len(response.Summaries)
	return nil
}

func sumyChunks(chunks []Chunk) []SumyChunk {
	out := make([]SumyChunk, 0, len(chunks))
	for _, c := range chunks {
		item := SumyChunk{SeqNo: c.SeqNo, Lines: []SumyLine{}}
		for _, marked := range c.Lines {
			line := marked.Line
			item.Lines = append(item.Lines, SumyLine{LineNumber: line.LineNo, Content: line.Content, LineType: line.LineType})
		}
		out = append(out, item)
	}
	return out
}

func validateSumyRequest(request SumyRequest) error {
	if len(request.Chunks) == 0 {
		return fmt.Errorf("extract_summaries: no chunks; run chunking first")
	}
	seen := map[int]bool{}
	for _, chunk := range request.Chunks {
		if chunk.SeqNo <= 0 || seen[chunk.SeqNo] {
			return fmt.Errorf("invalid or duplicate chunk sequence %d", chunk.SeqNo)
		}
		seen[chunk.SeqNo] = true
		lines := map[int]bool{}
		for _, line := range chunk.Lines {
			if line.LineNumber <= 0 || lines[line.LineNumber] {
				return fmt.Errorf("invalid or duplicate source line in chunk %d", chunk.SeqNo)
			}
			lines[line.LineNumber] = true
		}
	}
	return nil
}

func sumyImageLine(kind string) bool {
	return kind == "image" || kind == "table-image" || kind == "equation-image"
}

func validateSumyResponse(request SumyRequest, response SumyResponse) error {
	if response.Algorithm != request.Algorithm || strings.TrimSpace(response.Versions["sumy"]) == "" {
		return fmt.Errorf("Sumy algorithm/version mismatch")
	}
	if len(response.Summaries) != len(request.Chunks) {
		return fmt.Errorf("Sumy returned %d summaries for %d chunks", len(response.Summaries), len(request.Chunks))
	}
	chunks := map[int]SumyChunk{}
	for _, c := range request.Chunks {
		chunks[c.SeqNo] = c
	}
	seen := map[int]bool{}
	for _, item := range response.Summaries {
		chunk, ok := chunks[item.SeqNo]
		if !ok || seen[item.SeqNo] {
			return fmt.Errorf("unexpected or duplicate Sumy chunk %d", item.SeqNo)
		}
		seen[item.SeqNo] = true
		if item.Language != "en" && item.Language != "zh" {
			return fmt.Errorf("unsupported Sumy result language %q", item.Language)
		}
		docLanguage := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(request.Language), "_", "-"))
		if ((docLanguage == "en" || docLanguage == "english" || strings.HasPrefix(docLanguage, "en-")) && item.Language != "en") ||
			((docLanguage == "zh" || docLanguage == "chinese" || strings.HasPrefix(docLanguage, "zh-")) && item.Language != "zh") {
			return fmt.Errorf("Sumy result language differs from document language")
		}
		if len(item.Sentences) > request.SentenceCount {
			return fmt.Errorf("Sumy exceeded sentence budget")
		}
		lineMap := map[int]SumyLine{}
		order := map[int]int{}
		hasText := false
		for i, line := range chunk.Lines {
			lineMap[line.LineNumber] = line
			order[line.LineNumber] = i
			if !sumyImageLine(line.LineType) && strings.TrimSpace(line.Content) != "" {
				hasText = true
			}
		}
		if hasText && len(item.Sentences) == 0 {
			return fmt.Errorf("Sumy returned empty extraction for textual chunk %d", chunk.SeqNo)
		}
		texts := []string{}
		lastOrder, lastEnd := -1, 0
		for _, sentence := range item.Sentences {
			if len(sentence.Spans) == 0 || strings.TrimSpace(sentence.Text) == "" {
				return fmt.Errorf("Sumy returned empty sentence evidence")
			}
			pieces := []string{}
			for _, span := range sentence.Spans {
				line, ok := lineMap[span.Line]
				runes := []rune(line.Content)
				index := order[span.Line]
				if !ok || sumyImageLine(line.LineType) || span.Start < 0 || span.Start >= span.End || span.End > len(runes) {
					return fmt.Errorf("Sumy returned invalid source span")
				}
				if index < lastOrder || (index == lastOrder && span.Start < lastEnd) {
					return fmt.Errorf("Sumy returned repeated or reordered evidence")
				}
				lastOrder, lastEnd = index, span.End
				pieces = append(pieces, string(runes[span.Start:span.End]))
			}
			if strings.Join(strings.Fields(sentence.Text), " ") != strings.Join(strings.Fields(strings.Join(pieces, " ")), " ") {
				return fmt.Errorf("Sumy sentence differs from cited source")
			}
			texts = append(texts, sentence.Text)
		}
		if item.Summary != strings.Join(texts, "\n") {
			return fmt.Errorf("Sumy summary differs from its selected sentences")
		}
	}
	return nil
}

// Source/chunk files are checked immediately before replacement, under the input
// row lock. The files themselves are not transactionally managed by PostgreSQL.
func (p *ExtractSummariesProcessor) loadChunks(ctx context.Context, evt LineFileGeneratedEvent, rec DocMetadataInputRecord) ([]Chunk, func() error, error) {
	var chunks []Chunk
	files := map[string][32]byte{}
	inputPath, pathErr := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName) + ".chunks"
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if buf := ChunkBufferFromContext(ctx); buf != nil {
		chunks = buf.Chunks
		// Snapshot files when present; in-memory-only callers still have a stable
		// buffer digest and do not need to materialize a line file.
		paths := []string{}
		if pathErr == nil {
			paths = append(paths, inputPath)
		}
		if dir, err := buildRecordArtifactDir(artifactDir, rec.ID); err == nil {
			paths = append(paths, filepath.Join(dir, artifactBase))
		}
		for _, path := range paths {
			body, err := os.ReadFile(path)
			if err == nil {
				files[path] = sha256.Sum256(body)
			} else if !os.IsNotExist(err) {
				return nil, nil, err
			}
		}
	} else {
		if pathErr != nil {
			return nil, nil, pathErr
		}
		body, err := os.ReadFile(inputPath)
		if err != nil {
			return nil, nil, err
		}
		files[inputPath] = sha256.Sum256(body)
		lines, err := ParseInputLinesIncludingTOC(body)
		if err != nil {
			return nil, nil, err
		}
		dir, err := buildRecordArtifactDir(artifactDir, rec.ID)
		if err != nil {
			return nil, nil, err
		}
		path := filepath.Join(dir, artifactBase)
		artifact, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		files[path] = sha256.Sum256(artifact)
		if err := validateExtractChunkReferences(artifact, lines); err != nil {
			return nil, nil, err
		}
		chunks, err = loadChunksFromArtifactFile(artifactDir, rec.ID, artifactBase, lines)
		if err != nil {
			return nil, nil, err
		}
	}
	if len(chunks) == 0 {
		return nil, nil, fmt.Errorf("extract_summaries: missing or empty chunk output")
	}
	original, _ := json.Marshal(sumyChunks(chunks))
	expected := sha256.Sum256(original)
	verify := func() error {
		for path, expected := range files {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if sha256.Sum256(body) != expected {
				return fmt.Errorf("extract_summaries source changed during extraction")
			}
		}
		if buf := ChunkBufferFromContext(ctx); buf != nil {
			current, _ := json.Marshal(sumyChunks(buf.Chunks))
			if sha256.Sum256(current) != expected {
				return fmt.Errorf("extract_summaries chunk buffer changed during extraction")
			}
		}
		return ctx.Err()
	}
	return chunks, verify, nil
}

// Other chunk consumers intentionally tolerate legacy artifacts with missing
// line IDs. Exact extraction must reject them instead of losing evidence.
func validateExtractChunkReferences(artifact []byte, lines []Line) error {
	known := make(map[int]bool, len(lines))
	for _, line := range lines {
		known[line.LineNo] = true
	}
	for _, raw := range strings.Split(string(artifact), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		key, value, ok := strings.Cut(raw, ":")
		if !ok || (key != "lines" && key != "overlap") {
			return fmt.Errorf("invalid extract-summary chunk artifact")
		}
		numbers, err := parseLineNumberRanges(strings.TrimSpace(value))
		if err != nil {
			return err
		}
		for _, number := range numbers {
			if !known[number] {
				return fmt.Errorf("extract-summary chunk artifact references missing source line %d", number)
			}
		}
	}
	return nil
}

func (p *ExtractSummariesProcessor) persistStatus(ctx context.Context, recordID int64, start time.Time, state string, total, completed int, procErr error) error {
	return updateInputStatusAtomic(ctx, p.InputStore, recordID, func(raw string) (DocMetadataUpdate, error) {
		progress := "0%"
		if state == "success" {
			progress = "100%"
		}
		updated, err := upsertProcessorRuntimeStatus(raw, start, p.Name(), state, progress)
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		entries := decodeDocMetaStatus(updated)
		for _, entry := range entries {
			if asString(entry["operation"]) == p.Name() {
				entry["record_id"] = strconv.FormatInt(recordID, 10)
				entry["algorithm"] = p.Config.Algorithm
				entry["sentence_count"] = p.Config.SentenceCount
				entry["num_chunks"] = total
				entry["num_summaries"] = completed
				entry["ms_used"] = time.Since(start).Milliseconds()
				if procErr != nil {
					entry["error"] = procErr.Error()
				}
			}
		}
		body, err := json.Marshal(entries)
		return DocMetadataUpdate{StatusRaw: string(body)}, err
	})
}
