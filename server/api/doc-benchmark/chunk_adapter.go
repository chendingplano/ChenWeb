package docbenchmark

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const ChunkRowsQuery = `SELECT id, chunking_method, chunking_size, overlap_percent, notes,
       overlap_lines, normal_lines, chunk_lines, create_time, update_time
FROM kb.chunks
WHERE source_record_id = $1
ORDER BY id ASC`

type ChunkDBRow struct {
	ID             int64  `json:"id"`
	ChunkingMethod string `json:"chunking_method"`
	ChunkingSize   any    `json:"chunking_size"`
	OverlapPercent any    `json:"overlap_percent"`
	Notes          any    `json:"notes"`
	OverlapLines   []int  `json:"overlap_lines"`
	NormalLines    []int  `json:"normal_lines"`
	ChunkLines     []int  `json:"chunk_lines"`
	CreateTime     any    `json:"create_time"`
	UpdateTime     any    `json:"update_time"`
}
type ChunkCapture struct {
	Rows     []ChunkDBRow
	File     []byte
	FileName string
	Diff     map[string]any
}
type ChunkActual struct{ Chunks []ChunkDBRow }
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

func parseLineArray(v any) ([]int, error) {
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
		return nil, fmt.Errorf("line array type %T", v)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil, nil
	}
	var out []int
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
		r.OverlapLines, err = parseLineArray(ov)
		if err != nil {
			return nil, err
		}
		r.NormalLines, err = parseLineArray(nl)
		if err != nil {
			return nil, err
		}
		r.ChunkLines, err = parseLineArray(cl)
		if err != nil {
			return nil, err
		}
		c.Rows = append(c.Rows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(c.Rows) != 1 {
		return nil, fmt.Errorf("%w: want exactly one chunk row, got %d", ErrInvalidOutput, len(c.Rows))
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

func parseChunksFile(b []byte) ([][]int, error) {
	s := bufio.NewScanner(strings.NewReader(string(b)))
	var out [][]int
	var overlap, normal []int
	flush := func() {
		if overlap != nil || normal != nil {
			x := append([]int{}, normal...)
			x = append(x, overlap...)
			out = append(out, x)
			overlap = nil
			normal = nil
		}
	}
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			flush()
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		arr, err := parseLineArray(strings.TrimSpace(v))
		if err != nil {
			return nil, err
		}
		switch strings.TrimSpace(k) {
		case "overlap":
			overlap = arr
		case "lines", "normal":
			normal = arr
		}
	}
	flush()
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
	if len(parsed) != len(c.Rows) {
		return nil, fmt.Errorf("%w: chunk count mismatch", ErrInvalidOutput)
	}
	for i, r := range c.Rows {
		want := append(append([]int{}, r.NormalLines...), r.OverlapLines...)
		if !equalInts(want, parsed[i]) {
			c.Diff = map[string]any{"chunk": i, "db": want, "file": parsed[i]}
			return c, fmt.Errorf("%w: representations disagree", ErrInvalidOutput)
		}
	}
	return ChunkActual{Chunks: c.Rows}, nil
}
func (a ChunkAdapter) Cleanup(ctx context.Context, id int64) error {
	if a.DB == nil {
		return fmt.Errorf("nil database")
	}
	_, err := a.DB.ExecContext(ctx, "DELETE FROM kb.chunks WHERE source_record_id = $1", id)
	return err
}

type SeedInputRequest struct {
	TenantID, StoreID, ParserName, StagingFilename, ResultFilename, FileName string
	Status                                                                   string
	Body                                                                     []byte
}

func SeedInput(ctx context.Context, db *sql.DB, req SeedInputRequest) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("nil database")
	}
	if req.Status == "" {
		req.Status = "[]"
	}
	var id int64
	err := db.QueryRowContext(ctx, `INSERT INTO kb.inputs (tenant_id, store_id, parser_name, staging_filename, result_filename, file_name, status, input_bytes) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, req.TenantID, req.StoreID, req.ParserName, req.StagingFilename, req.ResultFilename, req.FileName, req.Status, req.Body).Scan(&id)
	return id, err
}

var _ = strconv.Itoa
