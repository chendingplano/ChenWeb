package docprocessing

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const DefaultBlockingBlockSize = 8

// BlockLine is one output line from the blocking processor.
// Fields: flag (o/n), line_number, page_number, line_type, content.
// font, font_size, and coordinate from the input line file are dropped.
type BlockLine struct {
	Flag       string // "o" = overlap, "n" = normal
	LineNumber int
	PageNumber int
	LineType   string
	Content    string
}

// String serialises to the canonical output format:
// <flag>\t<line_number>\t<page_number>\t<line_type>\t<content>
func (l BlockLine) String() string {
	return strings.Join([]string{
		l.Flag,
		strconv.Itoa(l.LineNumber),
		strconv.Itoa(l.PageNumber),
		l.LineType,
		l.Content,
	}, "\t")
}

// Block is one blocking unit: up to one previous-overlap page + INPUT_BLOCK_SIZE
// core pages + up to one subsequent-overlap page.
type Block struct {
	Index int
	Lines []BlockLine
}

// BlockBuffer is the in-memory output of the blocking processor.
type BlockBuffer struct {
	Blocks []Block
}

// --- context-based block buffer sharing ---

type blockBufferCtxKey struct{}

type blockBufferHolder struct {
	mu     sync.Mutex
	buffer *BlockBuffer
}

// withBlockBufferHolder injects a mutable block buffer holder into ctx.
// The BlockingProcessor writes its result into the holder; downstream
// processors can retrieve it via BlockBufferFromContext.
func withBlockBufferHolder(ctx context.Context) (context.Context, *blockBufferHolder) {
	h := &blockBufferHolder{}
	return context.WithValue(ctx, blockBufferCtxKey{}, h), h
}

// BlockBufferFromContext returns the BlockBuffer populated by the
// BlockingProcessor, or nil if none is available in this context.
func BlockBufferFromContext(ctx context.Context) *BlockBuffer {
	h, _ := ctx.Value(blockBufferCtxKey{}).(*blockBufferHolder)
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.buffer
}

// --- BlockingProcessor ---

type BlockingProcessor struct {
	InputStore DocMetadataStore
	Logger     ApiTypes.JimoLogger
	BlockSize  int
}

func NewBlockingProcessor(store DocMetadataStore, logger ApiTypes.JimoLogger) *BlockingProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26050501")
	}
	return &BlockingProcessor{
		InputStore: store,
		Logger:     logger,
		BlockSize:  envInt("INPUT_BLOCK_SIZE", DefaultBlockingBlockSize, 1),
	}
}

func (p *BlockingProcessor) Name() string { return "blocking" }

func (p *BlockingProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26050502) parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}

	if p.InputStore == nil {
		return errors.New("(MID_26050503) input store is nil")
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("(MID_26050504) kb.inputs record not found: %d", evt.RecordID)
		}
		return fmt.Errorf("(MID_26050505) load kb.inputs record %d: %w", evt.RecordID, err)
	}

	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return fmt.Errorf("(MID_26050506) resolve input file path: %w", err)
	}

	body, err := os.ReadFile(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("(MID_26050507) input file not exist: %s", inputPath)
		}
		return fmt.Errorf("(MID_26050508) read input file: %w", err)
	}

	buf, err := buildBlocks(body, p.blockSize())
	if err != nil {
		return fmt.Errorf("(MID_26050509) build blocks: %w", err)
	}

	if h, ok := ctx.Value(blockBufferCtxKey{}).(*blockBufferHolder); ok {
		h.mu.Lock()
		h.buffer = buf
		h.mu.Unlock()
	}

	if p.Logger != nil {
		p.Logger.Info("blocking processor completed",
			"record_id", evt.RecordID,
			"input_file", inputPath,
			"num_blocks", len(buf.Blocks),
		)
	}
	return nil
}

func (p *BlockingProcessor) blockSize() int {
	if p.BlockSize >= 1 {
		return p.BlockSize
	}
	return DefaultBlockingBlockSize
}

// buildBlocks parses a line file and groups lines into overlapping page blocks.
// Each block has: 1 previous-overlap page (flag='o') + blockSize core pages
// (flag='n') + 1 subsequent-overlap page (flag='o').
func buildBlocks(body []byte, blockSize int) (*BlockBuffer, error) {
	if blockSize < 1 {
		blockSize = 1
	}

	linesByPage := make(map[int][]blockInputLine)
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	sc.Buffer(make([]byte, 1024), 16*1024*1024)
	for sc.Scan() {
		raw := sc.Text()
		if strings.TrimSpace(raw) == "" {
			continue
		}
		line, err := parseBlockInputLine(raw)
		if err != nil {
			continue // skip malformed lines
		}
		linesByPage[line.PageNumber] = append(linesByPage[line.PageNumber], line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("(MID_26050510) scan input: %w", err)
	}

	pages := make([]int, 0, len(linesByPage))
	for pg := range linesByPage {
		pages = append(pages, pg)
	}
	sort.Ints(pages)

	if len(pages) == 0 {
		return &BlockBuffer{}, nil
	}

	n := len(pages)
	blocks := make([]Block, 0, (n+blockSize-1)/blockSize)
	blockIdx := 1
	for start := 0; start < n; start += blockSize {
		end := start + blockSize
		if end > n {
			end = n
		}

		var lines []BlockLine

		// Previous-overlap page
		if start > 0 {
			for _, l := range linesByPage[pages[start-1]] {
				lines = append(lines, BlockLine{Flag: "o", LineNumber: l.LineNumber, PageNumber: l.PageNumber, LineType: l.LineType, Content: l.Content})
			}
		}

		// Core pages
		for _, pg := range pages[start:end] {
			for _, l := range linesByPage[pg] {
				lines = append(lines, BlockLine{Flag: "n", LineNumber: l.LineNumber, PageNumber: l.PageNumber, LineType: l.LineType, Content: l.Content})
			}
		}

		// Subsequent-overlap page
		if end < n {
			for _, l := range linesByPage[pages[end]] {
				lines = append(lines, BlockLine{Flag: "o", LineNumber: l.LineNumber, PageNumber: l.PageNumber, LineType: l.LineType, Content: l.Content})
			}
		}

		blocks = append(blocks, Block{Index: blockIdx, Lines: lines})
		blockIdx++
	}

	return &BlockBuffer{Blocks: blocks}, nil
}

type blockInputLine struct {
	LineNumber int
	PageNumber int
	LineType   string
	Content    string
}

// parseBlockInputLine parses a 7-field TAB-separated line file record and drops
// the font (field 3), font_size (field 4), and coordinate (field 5) fields.
func parseBlockInputLine(raw string) (blockInputLine, error) {
	fields := strings.Split(raw, "\t")
	if len(fields) != 7 {
		return blockInputLine{}, fmt.Errorf("(MID_26050511) expected 7 fields, got %d", len(fields))
	}
	lineNo, err := strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil || lineNo <= 0 {
		return blockInputLine{}, fmt.Errorf("(MID_26050512) invalid line_number: %q", fields[0])
	}
	pageNo, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil || pageNo <= 0 {
		return blockInputLine{}, fmt.Errorf("(MID_26050513) invalid page_number: %q", fields[1])
	}
	lineType := strings.TrimSpace(fields[2])
	if lineType == "" {
		return blockInputLine{}, fmt.Errorf("(MID_26050514) empty line_type")
	}
	return blockInputLine{
		LineNumber: lineNo,
		PageNumber: pageNo,
		LineType:   lineType,
		Content:    fields[6],
	}, nil
}
