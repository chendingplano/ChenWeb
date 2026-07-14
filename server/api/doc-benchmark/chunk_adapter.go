package docbenchmark

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

const ChunkRowsQuery = `SELECT id, chunking_method, chunking_size, overlap_percent, notes,
       overlap_lines, normal_lines, chunk_lines, create_time, update_time
FROM kb.chunks
WHERE source_record_id = $1
ORDER BY id ASC`

type ChunkDBRow struct {
	ID             int64    `json:"id"`
	ChunkingMethod string   `json:"chunking_method"`
	ChunkingSize   any      `json:"chunking_size"`
	OverlapPercent any      `json:"overlap_percent"`
	Notes          any      `json:"notes"`
	OverlapLines   [][]int  `json:"overlap_lines"`
	NormalLines    [][]int  `json:"normal_lines"`
	ChunkLines     []string `json:"chunk_lines"`
	CreateTime     any      `json:"create_time"`
	UpdateTime     any      `json:"update_time"`
}
type ChunkCapture struct {
	Rows     []ChunkDBRow
	File     []byte
	FileName string
	Diff     map[string]any
}
type ChunkActual struct{ Chunks []ScoredChunk }
type ChunkAdapter struct {
	DB           *sql.DB
	ArtifactDir  string
	ArtifactPath func(int64) string
}

func (a ChunkAdapter) Processor() Processor { return ProcessorChunking }
func (a ChunkAdapter) AllowedOverrides() map[string]any {
	return map[string]any{"chunking": map[string]any{"size": true, "overlap_percent": true, "method": true}}
}
func (a ChunkAdapter) Applicable(e ExpectedOutput) bool { return e.Chunking != nil }
func (a ChunkAdapter) path(id int64) string {
	if a.ArtifactPath != nil {
		return a.ArtifactPath(id)
	}
	return a.ArtifactDir
}

func jsonBytes(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	var b []byte
	switch x := v.(type) {
	case []byte:
		b = x
	case string:
		b = []byte(x)
	default:
		return nil, fmt.Errorf("JSON value type %T", v)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil, nil
	}
	return b, nil
}

var lineRangeToken = regexp.MustCompile(`^([0-9]+)(?:-([0-9]+))?$`)

func parseLineRanges(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '[' || raw[len(raw)-1] != ']' {
		return nil, fmt.Errorf("invalid line range %q", raw)
	}
	inner := strings.TrimSpace(raw[1 : len(raw)-1])
	if inner == "" {
		return []int{}, nil
	}
	var out []int
	for _, part := range strings.Split(inner, ",") {
		token := strings.TrimSpace(part)
		m := lineRangeToken.FindStringSubmatch(token)
		if m == nil {
			return nil, fmt.Errorf("invalid line range token %q", token)
		}
		start, err := strconv.Atoi(m[1])
		if err != nil || start <= 0 {
			return nil, fmt.Errorf("invalid line number %q", m[1])
		}
		end := start
		if m[2] != "" {
			end, err = strconv.Atoi(m[2])
			if err != nil || end < start {
				return nil, fmt.Errorf("descending line range %q", token)
			}
		}
		for n := start; n <= end; n++ {
			if len(out) > 0 && n <= out[len(out)-1] {
				return nil, fmt.Errorf("duplicate or unordered line %d", n)
			}
			out = append(out, n)
		}
	}
	return out, nil
}

func parseProductionLineArrays(v any) ([][]int, error) {
	b, err := jsonBytes(v)
	if err != nil {
		return nil, err
	}
	var encoded []string
	if err := json.Unmarshal(b, &encoded); err != nil {
		return nil, err
	}
	out := make([][]int, len(encoded))
	for i, raw := range encoded {
		out[i], err = parseLineRanges(raw)
		if err != nil {
			return nil, fmt.Errorf("chunk %d: %w", i+1, err)
		}
	}
	return out, nil
}

func parseStringArray(v any) ([]string, error) {
	b, err := jsonBytes(v)
	if err != nil {
		return nil, err
	}
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a ChunkAdapter) Capture(ctx context.Context, id int64) (any, error) {
	if a.DB == nil {
		return nil, fmt.Errorf("nil database")
	}
	rows, err := a.DB.QueryContext(ctx, ChunkRowsQuery, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c := ChunkCapture{}
	for rows.Next() {
		var r ChunkDBRow
		var ov, nl, cl any
		if err := rows.Scan(&r.ID, &r.ChunkingMethod, &r.ChunkingSize, &r.OverlapPercent, &r.Notes, &ov, &nl, &cl, &r.CreateTime, &r.UpdateTime); err != nil {
			return nil, err
		}
		r.OverlapLines, err = parseProductionLineArrays(ov)
		if err != nil {
			return nil, fmt.Errorf("%w: overlap_lines: %v", ErrInvalidOutput, err)
		}
		r.NormalLines, err = parseProductionLineArrays(nl)
		if err != nil {
			return nil, fmt.Errorf("%w: normal_lines: %v", ErrInvalidOutput, err)
		}
		r.ChunkLines, err = parseStringArray(cl)
		if err != nil {
			return nil, fmt.Errorf("%w: chunk_lines: %v", ErrInvalidOutput, err)
		}
		c.Rows = append(c.Rows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(c.Rows) != 1 {
		return nil, fmt.Errorf("%w: want exactly one chunk row, got %d", ErrInvalidOutput, len(c.Rows))
	}
	if len(c.Rows[0].OverlapLines) != len(c.Rows[0].NormalLines) || len(c.Rows[0].OverlapLines) != len(c.Rows[0].ChunkLines) {
		return nil, fmt.Errorf("%w: inconsistent chunk arrays", ErrInvalidOutput)
	}
	p := a.path(id)
	if p == "" {
		return nil, fmt.Errorf("%w: missing chunks artifact", ErrInvalidOutput)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidOutput, err)
	}
	c.File = b
	c.FileName = p
	return c, nil
}

type artifactChunk struct{ overlap, normal []int }

func parseChunksFile(b []byte) ([]artifactChunk, error) {
	s := bufio.NewScanner(strings.NewReader(string(b)))
	var out []artifactChunk
	var overlap, normal []int
	field := 0
	flush := func() error {
		if overlap == nil && normal == nil {
			return nil
		}
		if field != 2 || overlap == nil || normal == nil {
			return fmt.Errorf("chunk missing overlap or lines")
		}
		out = append(out, artifactChunk{overlap: overlap, normal: normal})
		overlap, normal = nil, nil
		field = 0
		return nil
	}
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("malformed chunks line %q", line)
		}
		arr, err := parseLineRanges(strings.TrimSpace(v))
		if err != nil {
			return nil, err
		}
		switch strings.TrimSpace(k) {
		case "overlap":
			if field != 0 {
				return nil, fmt.Errorf("overlap field out of order")
			}
			overlap = arr
			field = 1
		case "lines":
			if field != 1 {
				return nil, fmt.Errorf("lines field out of order")
			}
			normal = arr
			field = 2
		default:
			return nil, fmt.Errorf("unknown chunks field %q", strings.TrimSpace(k))
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty chunks artifact")
	}
	return out, s.Err()
}
func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func (a ChunkAdapter) Reconcile(v any) (any, error) {
	c, ok := v.(ChunkCapture)
	if !ok {
		return nil, fmt.Errorf("%w: invalid capture", ErrInvalidOutput)
	}
	parsed, err := parseChunksFile(c.File)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidOutput, err)
	}
	row := c.Rows[0]
	if len(parsed) != len(row.NormalLines) {
		return nil, fmt.Errorf("%w: chunk count mismatch", ErrInvalidOutput)
	}
	out := ChunkActual{Chunks: make([]ScoredChunk, len(parsed))}
	for i, fileChunk := range parsed {
		if !equalInts(row.NormalLines[i], fileChunk.normal) || !equalInts(row.OverlapLines[i], fileChunk.overlap) {
			c.Diff = map[string]any{"chunk": i + 1, "db_overlap": row.OverlapLines[i], "file_overlap": fileChunk.overlap, "db_normal": row.NormalLines[i], "file_normal": fileChunk.normal}
			return c, fmt.Errorf("%w: representations disagree", ErrInvalidOutput)
		}
		out.Chunks[i] = ScoredChunk{Sequence: i + 1, OverlapLines: append([]int(nil), row.OverlapLines[i]...), NormalLines: append([]int(nil), row.NormalLines[i]...)}
	}
	return out, nil
}
func (a ChunkAdapter) Cleanup(ctx context.Context, id int64) error {
	if a.DB == nil {
		return fmt.Errorf("nil database")
	}
	_, err := a.DB.ExecContext(ctx, "DELETE FROM kb.chunks WHERE source_record_id = $1", id)
	return err
}

const BenchmarkInputFilename = "benchmark-input.pdf"

type SeedInputRequest struct {
	AttemptID, Workspace, TenantID, Title, ParserName, ResultFilename, Status string
	StoreID                                                                   int64
	Case                                                                      DatasetCase
}

type SeededInput struct {
	ID                                                    int64
	ParserName, StagingFilename, ResultFilename, FileName string
}

const seedInputQuery = `INSERT INTO kb.inputs (tenant_id, ks_store_id, type, title, parser_name, staging_filename, result_filename, file_name, status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb) RETURNING id`

func SeedInput(ctx context.Context, db *sql.DB, req SeedInputRequest) (SeededInput, error) {
	if db == nil {
		return SeededInput{}, fmt.Errorf("nil database")
	}
	if req.Status == "" {
		req.Status = "[]"
	}
	if req.AttemptID == "" || req.Workspace == "" || req.StoreID <= 0 {
		return SeededInput{}, fmt.Errorf("invalid seed input request")
	}
	if req.ParserName == "" {
		req.ParserName = "benchmark"
	}
	if req.Title == "" {
		req.Title = req.Case.CaseID
	}
	if err := os.MkdirAll(req.Workspace, 0o700); err != nil {
		return SeededInput{}, err
	}
	stagingMetadata := filepath.Join(req.Workspace, BenchmarkInputFilename)
	resultMetadata := req.ResultFilename
	if resultMetadata == "" {
		resultMetadata = filepath.Join(req.Workspace, "result.txt")
	}
	linePath, err := docprocessing.ResolveInputFilePath(docprocessing.LineFileGeneratedEvent{}, resultMetadata, req.ParserName, stagingMetadata)
	if err != nil {
		return SeededInput{}, err
	}
	if err := os.WriteFile(linePath, req.Case.InputBytes, 0o600); err != nil {
		return SeededInput{}, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(linePath)
		}
	}()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return SeededInput{}, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, seedInputQuery, req.TenantID, req.StoreID, "pdf", req.Title, req.ParserName, stagingMetadata, linePath, BenchmarkInputFilename, req.Status).Scan(&id)
	if err != nil {
		return SeededInput{}, err
	}
	if err = bindInputOwnershipTx(ctx, tx, req.AttemptID, id); err != nil {
		return SeededInput{}, err
	}
	if err = tx.Commit(); err != nil {
		return SeededInput{}, err
	}
	ok = true
	return SeededInput{ID: id, ParserName: req.ParserName, StagingFilename: stagingMetadata, ResultFilename: linePath, FileName: BenchmarkInputFilename}, nil
}

func ProductionArtifactPath(artifactDir string, recordID int64, stagingFilename, parserName, extension string) (string, error) {
	if strings.TrimSpace(artifactDir) == "" || recordID <= 0 {
		return "", fmt.Errorf("invalid artifact location")
	}
	if extension != ".chunks" && extension != ".metrics" {
		return "", fmt.Errorf("unsupported artifact extension %q", extension)
	}
	root := strings.TrimSuffix(filepath.Base(strings.TrimSpace(stagingFilename)), filepath.Ext(strings.TrimSpace(stagingFilename)))
	parser := strings.TrimSpace(parserName)
	if root == "" || parser == "" {
		return "", fmt.Errorf("invalid artifact filename inputs")
	}
	return filepath.Join(artifactDir, strconv.FormatInt(recordID/1000, 10), strconv.FormatInt(recordID, 10), root+"_"+parser+extension), nil
}
