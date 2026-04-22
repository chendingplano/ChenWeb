package docprocessing

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const (
	DefaultChunkSize       = 300
	DefaultOverlapPercent  = 20
	defaultStatusTime      = "20060102 15:04:05"
	ChunkingMethodFixed    = "fix-size-chunking"
	numericSplitMultiplier = 3
)

var (
	numericSectionPattern  = regexp.MustCompile(`^\d+\.\d+\b`)
	numericListItemPattern = regexp.MustCompile(`^\d+[\.)]?\s+`)
	nonNumericListPattern  = regexp.MustCompile(`^([*\-•·—]+|[A-Za-z]\)|[（(][一二三四五六七八九十]+[）)])\s+`)
)

type InputRecord struct {
	ID        int64
	StatusRaw string
}

type ChunkRunRecord struct {
	SourceRecordID int64
	ChunkingMethod string
	ChunkingSize   int
	OverlapPercent int
	Notes          string
}

type Store interface {
	GetInputRecord(ctx context.Context, id int64) (InputRecord, error)
	InsertChunkRun(ctx context.Context, rec ChunkRunRecord) error
	UpdateInputStatus(ctx context.Context, id int64, statusJSON string, errorMsg *string) error
}

type FixedSizeChunkingService struct {
	Store          Store
	Logger         ApiTypes.JimoLogger
	Now            func() time.Time
	ChunkDir       string
	ChunkSize      int
	OverlapPercent int
}

type ChunkOptions struct {
	ChunkSize      int
	OverlapPercent int
}

type Line struct {
	Raw        string
	LineNo     int
	PageNo     int
	LineType   string
	Font       string
	FontSize   string
	Content    string
	Coordinate string
}

type MarkedLine struct {
	Line Line
	Mark string // r = regular, o = overlap
}

type Chunk struct {
	SeqNo int
	Lines []MarkedLine
}

type listKind int

const (
	listNone listKind = iota
	listNumeric
	listNonNumeric
)

type protectedBlock struct {
	start      int
	end        int
	splittable bool
}

func NewFixedSizeChunkingService(store Store, logger ApiTypes.JimoLogger) *FixedSizeChunkingService {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26041901")
	}
	return &FixedSizeChunkingService{
		Store:          store,
		Logger:         logger,
		Now:            time.Now,
		ChunkDir:       strings.TrimSpace(os.Getenv("CHUNK_DIR")),
		ChunkSize:      envInt("CHUNK_SIZE", DefaultChunkSize, 1),
		OverlapPercent: envInt("CHUNK_OVERLAP_PERCENT", DefaultOverlapPercent, 0),
	}
}

func (s *FixedSizeChunkingService) HandleInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error {
	if s.Store == nil {
		return errors.New("(MID_26042012) store is nil")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26042002) invalid record_id: %d", recordID)
	}

	start := s.Now()
	rec, err := s.Store.GetInputRecord(ctx, recordID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(s.ChunkDir) == "" {
		procErr := errors.New("(MID_26042003) missing CHUNK_DIR")
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}
	if strings.TrimSpace(inputFilename) == "" {
		procErr := errors.New("(MID_26042004) missing input filename")
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, procErr)
		return procErr
	}

	lines, err := ParseInputLines(inputFile)
	if err != nil {
		s.failAndPersist(ctx, rec, inputFilename, 0, 0, 0, start, err)
		return err
	}
	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: s.ChunkSize, OverlapPercent: s.OverlapPercent})
	if err != nil {
		s.failAndPersist(ctx, rec, inputFilename, uniquePages(lines), len(lines), 0, start, err)
		return err
	}

	if err := writeChunkFiles(s.ChunkDir, rec.ID, chunks); err != nil {
		s.failAndPersist(ctx, rec, inputFilename, uniquePages(lines), len(lines), 0, start, err)
		return err
	}

	if err := s.Store.InsertChunkRun(ctx, ChunkRunRecord{
		SourceRecordID: rec.ID,
		ChunkingMethod: ChunkingMethodFixed,
		ChunkingSize:   s.ChunkSize,
		OverlapPercent: s.OverlapPercent,
	}); err != nil {
		s.failAndPersist(ctx, rec, inputFilename, uniquePages(lines), len(lines), len(chunks), start, err)
		return err
	}

	statusRaw, err := appendChunkedStatus(rec.StatusRaw, chunkStatusParams{
		InputFilename: inputFilename,
		NumPages:      uniquePages(lines),
		NumLines:      len(lines),
		NumChunks:     len(chunks),
		Start:         start,
		DurationMs:    time.Since(start).Milliseconds(),
		ProcErr:       nil,
	})
	if err != nil {
		return err
	}

	if err := s.Store.UpdateInputStatus(ctx, rec.ID, statusRaw, nil); err != nil {
		return err
	}

	s.Logger.Info("chunking completed",
		"record_id", rec.ID,
		"chunk_dir", s.ChunkDir,
		"num_pages", uniquePages(lines),
		"num_lines", len(lines),
		"num_chunks", len(chunks),
	)
	return nil
}

func (s *FixedSizeChunkingService) failAndPersist(
	ctx context.Context,
	rec InputRecord,
	inputFilename string,
	numPages int,
	numLines int,
	numChunks int,
	start time.Time,
	procErr error,
) {
	statusRaw, err := appendChunkedStatus(rec.StatusRaw, chunkStatusParams{
		InputFilename: inputFilename,
		NumPages:      numPages,
		NumLines:      numLines,
		NumChunks:     numChunks,
		Start:         start,
		DurationMs:    time.Since(start).Milliseconds(),
		ProcErr:       procErr,
	})
	if err != nil {
		s.Logger.Error("failed building chunk status", "record_id", rec.ID, "error", err)
		return
	}
	errMsg := strings.TrimSpace(procErr.Error())
	if updateErr := s.Store.UpdateInputStatus(ctx, rec.ID, statusRaw, &errMsg); updateErr != nil {
		s.Logger.Error("failed persisting chunk failure status", "record_id", rec.ID, "error", updateErr)
		return
	}
	s.Logger.Error("chunking failed", "record_id", rec.ID, "error", procErr)
}

func ParseInputLines(input []byte) ([]Line, error) {
	sc := bufio.NewScanner(strings.NewReader(string(input)))
	sc.Buffer(make([]byte, 1024), 16*1024*1024)

	out := make([]Line, 0, 128)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		parsed, err := parseLine(raw)
		if err != nil {
			return nil, fmt.Errorf("(MID_26042005) line %d: %w", lineNo, err)
		}
		out = append(out, parsed)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseLine(raw string) (Line, error) {
	fields := strings.Split(raw, "\t")
	if len(fields) != 7 {
		return Line{}, fmt.Errorf("(MID_26042006) invalid input line format, line:%s", raw)
	}
	lineNo, err := strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil {
		return Line{}, fmt.Errorf("(MID_26042007) invalid line number: %w", err)
	}
	pageNo, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil {
		return Line{}, fmt.Errorf("(MID_26042008) invalid page number: %w", err)
	}
	lineType := strings.ToLower(strings.TrimSpace(fields[2]))
	font := strings.TrimSpace(fields[3])
	fontSize := strings.TrimSpace(fields[4])
	coordinate := strings.TrimSpace(fields[5])
	content := strings.TrimSpace(fields[6])
	if lineType == "" || font == "" || fontSize == "" || coordinate == "" {
		return Line{}, fmt.Errorf("(MID_26042006) invalid input line format, line:%s", raw)
	}
	return Line{
		Raw:        raw,
		LineNo:     lineNo,
		PageNo:     pageNo,
		LineType:   lineType,
		Font:       font,
		FontSize:   fontSize,
		Content:    content,
		Coordinate: coordinate,
	}, nil
}

func BuildChunks(lines []Line, opts ChunkOptions) ([]Chunk, error) {
	if opts.ChunkSize <= 0 {
		return nil, errors.New("(MID_26042009) chunk size must be positive")
	}
	if len(lines) == 0 {
		return []Chunk{}, nil
	}
	overlap := opts.OverlapPercent
	if overlap < 0 {
		overlap = 0
	}
	if overlap > 99 {
		overlap = 99
	}

	blocks := buildProtectedBlocks(lines, opts.ChunkSize)

	chunks := make([]Chunk, 0, max(1, len(lines)/2))
	start := 0
	prevEnd := 0
	seq := 1
	for start < len(lines) {
		target := findTargetByBytes(lines, start, opts.ChunkSize)
		end := adjustChunkEnd(start, target, len(lines), blocks)
		if end <= start {
			end = target
			if end <= start {
				end = start + 1
			}
		}

		c := Chunk{SeqNo: seq, Lines: make([]MarkedLine, 0, end-start)}
		for i := start; i < end; i++ {
			mark := "r"
			if i < prevEnd {
				mark = "o"
			}
			c.Lines = append(c.Lines, MarkedLine{Line: lines[i], Mark: mark})
		}
		chunks = append(chunks, c)

		if end >= len(lines) {
			break
		}
		prevEnd = end
		overlapLines := (end - start) * overlap / 100
		if overlapLines >= (end - start) {
			overlapLines = end - start - 1
		}
		if overlapLines < 0 {
			overlapLines = 0
		}
		nextStart := end - overlapLines
		if nextStart <= start {
			nextStart = start + 1
		}
		start = nextStart
		seq++
	}

	return chunks, nil
}

func findTargetByBytes(lines []Line, start int, targetBytes int) int {
	if start >= len(lines) {
		return len(lines)
	}
	if targetBytes <= 0 {
		return min(start+1, len(lines))
	}
	size := 0
	for i := start; i < len(lines); i++ {
		size += lineRawByteSize(lines[i])
		if size >= targetBytes {
			return i + 1
		}
	}
	return len(lines)
}

func writeChunkFiles(chunkDir string, recordID int64, chunks []Chunk) error {
	if strings.TrimSpace(chunkDir) == "" {
		return errors.New("(MID_26042010) chunk dir is empty")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26042011) invalid record id: %d", recordID)
	}
	groupID := recordID / 1000
	targetDir := filepath.Join(chunkDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	for _, c := range chunks {
		path := filepath.Join(targetDir, fmt.Sprintf("chunk_%04d", c.SeqNo))
		var b strings.Builder
		for _, ml := range c.Lines {
			b.WriteString(formatMarkedLine(ml))
			b.WriteByte('\n')
		}
		if err := os.WriteFile(path, []byte(strings.TrimRight(b.String(), "\n")), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func formatMarkedLine(ml MarkedLine) string {
	base := lineRawForChunking(ml.Line)
	mark := strings.TrimSpace(ml.Mark)
	if mark == "" {
		mark = "r"
	}
	return mark + " " + base
}

func lineRawForChunking(line Line) string {
	base := strings.TrimSpace(line.Raw)
	if base != "" {
		return base
	}
	return fmt.Sprintf("%d %d %s %s %s", line.LineNo, line.PageNo, line.LineType, line.Content, line.Coordinate)
}

func lineRawByteSize(line Line) int {
	return len([]byte(lineRawForChunking(line)))
}

func buildProtectedBlocks(lines []Line, chunkSizeBytes int) []*protectedBlock {
	out := make([]*protectedBlock, len(lines))

	for i := 0; i < len(lines); i++ {
		if out[i] != nil {
			continue
		}
		lt := strings.ToLower(strings.TrimSpace(lines[i].LineType))

		if isTableType(lt) || isFormulaType(lt) {
			j := i + 1
			for j < len(lines) {
				nlt := strings.ToLower(strings.TrimSpace(lines[j].LineType))
				if nlt != lt {
					break
				}
				j++
			}
			markBlock(out, &protectedBlock{start: i, end: j, splittable: false})
			i = j - 1
			continue
		}

		if lt != "list-item" {
			continue
		}

		kind := listKindForLine(lines[i])
		if kind == listNone {
			continue
		}

		j := i + 1
		for j < len(lines) && strings.ToLower(strings.TrimSpace(lines[j].LineType)) == "list-item" && listKindForLine(lines[j]) == kind {
			j++
		}

		if kind == listNonNumeric {
			markBlock(out, &protectedBlock{start: i, end: j, splittable: false})
			i = j - 1
			continue
		}

		if kind == listNumeric {
			if j-i >= 2 {
				blockBytes := 0
				for k := i; k < j; k++ {
					blockBytes += lineRawByteSize(lines[k])
				}
				splittable := chunkSizeBytes > 0 && blockBytes >= numericSplitMultiplier*chunkSizeBytes
				markBlock(out, &protectedBlock{start: i, end: j, splittable: splittable})
			}
			i = j - 1
		}
	}

	return out
}

func markBlock(index []*protectedBlock, block *protectedBlock) {
	for i := block.start; i < block.end; i++ {
		index[i] = block
	}
}

func adjustChunkEnd(start int, target int, total int, blockIndex []*protectedBlock) int {
	cut := target
	seen := map[int]struct{}{}

	for {
		if cut <= start || cut >= total {
			return cut
		}
		if _, ok := seen[cut]; ok {
			return cut
		}
		seen[cut] = struct{}{}

		b := splitUnsplittableBlock(cut, blockIndex)
		if b == nil {
			return cut
		}

		before := b.start
		after := b.end
		if before <= start {
			cut = after
			continue
		}
		if after >= total {
			cut = before
			continue
		}

		if absInt(target-before) < absInt(after-target) {
			cut = before
		} else {
			cut = after
		}
	}
}

func splitUnsplittableBlock(cut int, blockIndex []*protectedBlock) *protectedBlock {
	if cut <= 0 || cut >= len(blockIndex) {
		return nil
	}
	left := blockIndex[cut-1]
	right := blockIndex[cut]
	if left == nil || right == nil || left != right {
		return nil
	}
	if left.splittable {
		return nil
	}
	return left
}

func listKindForLine(line Line) listKind {
	if strings.ToLower(strings.TrimSpace(line.LineType)) != "list-item" {
		return listNone
	}
	content := strings.TrimSpace(line.Content)
	if content == "" {
		return listNone
	}
	if numericSectionPattern.MatchString(content) {
		return listNone
	}
	if numericListItemPattern.MatchString(content) {
		return listNumeric
	}
	if nonNumericListPattern.MatchString(content) {
		return listNonNumeric
	}
	return listNonNumeric
}

func isTableType(lineType string) bool {
	return strings.Contains(lineType, "table")
}

func isFormulaType(lineType string) bool {
	return strings.Contains(lineType, "formula")
}

type chunkStatusParams struct {
	InputFilename string
	NumPages      int
	NumLines      int
	NumChunks     int
	Start         time.Time
	DurationMs    int64
	ProcErr       error
}

func appendChunkedStatus(raw string, p chunkStatusParams) (string, error) {
	entries := decodeStatus(raw)
	entry := map[string]any{
		"operation":      "chunked",
		"input_filename": strings.TrimSpace(p.InputFilename),
		"num_pages":      p.NumPages,
		"num_lines":      p.NumLines,
		"num_chunks":     p.NumChunks,
		"start_time":     p.Start.Format(defaultStatusTime),
		"ms-used":        p.DurationMs,
	}
	if p.ProcErr == nil {
		entry["proc-status"] = "success"
	} else {
		entry["proc-status"] = "failed"
		entry["error"] = p.ProcErr.Error()
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "chunked" {
			out = append(out, e)
			continue
		}
		if !replaced {
			out = append(out, entry)
			replaced = true
		}
	}
	if !replaced {
		out = append(out, entry)
	}

	bs, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func decodeStatus(raw string) []map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return []map[string]any{}
	}

	var arr []map[string]any
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr
	}

	var one map[string]any
	if err := json.Unmarshal([]byte(raw), &one); err == nil {
		return []map[string]any{one}
	}

	return []map[string]any{}
}

func uniquePages(lines []Line) int {
	if len(lines) == 0 {
		return 0
	}
	seen := map[int]struct{}{}
	for _, l := range lines {
		if l.PageNo <= 0 {
			continue
		}
		seen[l.PageNo] = struct{}{}
	}
	return len(seen)
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

func envInt(key string, fallback int, min int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	return n
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
