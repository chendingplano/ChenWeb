package docbenchmark

import (
	"bufio"
	"bytes"
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
	Rows          []ChunkDBRow
	File          []byte
	FileName      string
	Diff          map[string]any
	SourceMaxLine int
}
type ChunkActual struct{ Chunks []ScoredChunk }
type ChunkAdapter struct {
	DB            *sql.DB
	ArtifactDir   string
	ArtifactPath  func(int64) string
	SourceMaxLine int
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

const maxExpandedLineEntries = 1_000_000
const maxProductionLineNumber = 10_000_000
const maxChunkCount = 10_000

type lineParseBudget struct{ expanded, chunks int }

func parseLineRanges(raw string) ([]int, error) {
	return parseLineRangesWithMax(raw, 0)
}

func parseLineRangesWithMax(raw string, sourceMax int) ([]int, error) {
	return parseLineRangesBudget(raw, sourceMax, &lineParseBudget{})
}

func parseLineRangesBudget(raw string, sourceMax int, budget *lineParseBudget) ([]int, error) {
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
		limit := maxProductionLineNumber
		if sourceMax > 0 && sourceMax < limit {
			limit = sourceMax
		}
		if end > limit {
			return nil, fmt.Errorf("line number %d exceeds maximum %d", end, limit)
		}
		count := end - start + 1
		if count > maxExpandedLineEntries || budget.expanded > maxExpandedLineEntries-count {
			return nil, fmt.Errorf("line range expands beyond %d entries", maxExpandedLineEntries)
		}
		budget.expanded += count
		for n := start; n <= end; n++ {
			if len(out) > 0 && n <= out[len(out)-1] {
				return nil, fmt.Errorf("duplicate or unordered line %d", n)
			}
			out = append(out, n)
		}
	}
	return out, nil
}

func parseProductionLineArrays(v any, sourceMax int) ([][]int, error) {
	return parseProductionLineArraysBudget(v, sourceMax, &lineParseBudget{}, true)
}

func parseProductionLineArraysBudget(v any, sourceMax int, budget *lineParseBudget, countChunks bool) ([][]int, error) {
	b, err := jsonBytes(v)
	if err != nil {
		return nil, err
	}
	var encoded []string
	if err := json.Unmarshal(b, &encoded); err != nil {
		return nil, err
	}
	if len(encoded) > maxChunkCount || (countChunks && budget.chunks > maxChunkCount-len(encoded)) {
		return nil, fmt.Errorf("chunk count exceeds %d", maxChunkCount)
	}
	if countChunks {
		budget.chunks += len(encoded)
	}
	out := make([][]int, len(encoded))
	for i, raw := range encoded {
		out[i], err = parseLineRangesBudget(raw, sourceMax, budget)
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
		budget := &lineParseBudget{}
		if err := rows.Scan(&r.ID, &r.ChunkingMethod, &r.ChunkingSize, &r.OverlapPercent, &r.Notes, &ov, &nl, &cl, &r.CreateTime, &r.UpdateTime); err != nil {
			return nil, err
		}
		r.OverlapLines, err = parseProductionLineArraysBudget(ov, a.SourceMaxLine, budget, true)
		if err != nil {
			return nil, fmt.Errorf("%w: overlap_lines: %v", ErrInvalidOutput, err)
		}
		r.NormalLines, err = parseProductionLineArraysBudget(nl, a.SourceMaxLine, budget, false)
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
	c.SourceMaxLine = a.SourceMaxLine
	return c, nil
}

type artifactChunk struct{ overlap, normal []int }

func parseChunksFile(b []byte) ([]artifactChunk, error) { return parseChunksFileWithMax(b, 0) }
func parseChunksFileWithMax(b []byte, sourceMax int) ([]artifactChunk, error) {
	s := bufio.NewScanner(strings.NewReader(string(b)))
	var out []artifactChunk
	var overlap, normal []int
	field := 0
	budget := &lineParseBudget{}
	flush := func() error {
		if overlap == nil && normal == nil {
			return nil
		}
		if field != 2 || overlap == nil || normal == nil {
			return fmt.Errorf("chunk missing overlap or lines")
		}
		out = append(out, artifactChunk{overlap: overlap, normal: normal})
		budget.chunks++
		if budget.chunks > maxChunkCount {
			return fmt.Errorf("chunk count exceeds %d", maxChunkCount)
		}
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
		arr, err := parseLineRangesBudget(strings.TrimSpace(v), sourceMax, budget)
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
	parsed, err := parseChunksFileWithMax(c.File, c.SourceMaxLine)
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

func safeLeaf(s string) bool {
	return s != "" && s != "." && s != ".." && filepath.Base(s) == s && !strings.ContainsAny(s, `/\`)
}

func canonicalSeedRoot(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, e := filepath.EvalSymlinks(abs); e == nil {
		abs = resolved
	}
	return filepath.Clean(abs), nil
}

func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func rejectSymlinkPath(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil || !pathWithin(root, target) {
		return fmt.Errorf("path escapes root")
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path component %q", current)
		}
	}
	return nil
}

func writeNewFileDurable(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".benchmark-input-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Link(tmpName, path); err != nil {
		return fmt.Errorf("stage input without overwrite: %w", err)
	}
	if dir, e := os.Open(filepath.Dir(path)); e == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func SeedInput(ctx context.Context, db *sql.DB, req SeedInputRequest) (SeededInput, error) {
	if !safeLeaf(req.ParserName) {
		return SeededInput{}, fmt.Errorf("invalid parser name %q", req.ParserName)
	}
	workspace, err := canonicalSeedRoot(req.Workspace)
	if err != nil {
		return SeededInput{}, err
	}
	if req.ResultFilename != "" {
		candidate := req.ResultFilename
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(workspace, candidate)
		}
		candidate, err = filepath.Abs(candidate)
		if err != nil || !pathWithin(workspace, filepath.Clean(candidate)) {
			return SeededInput{}, fmt.Errorf("result filename escapes workspace")
		}
		if err := rejectSymlinkPath(workspace, filepath.Clean(candidate)); err != nil {
			return SeededInput{}, err
		}
	}
	if db == nil {
		return SeededInput{}, fmt.Errorf("nil database")
	}
	if req.Status == "" {
		req.Status = "[]"
	}
	if req.AttemptID == "" || req.Workspace == "" || req.StoreID <= 0 {
		return SeededInput{}, fmt.Errorf("invalid seed input request")
	}
	if req.Title == "" {
		req.Title = req.Case.CaseID
	}
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		return SeededInput{}, err
	}
	stagingMetadata := filepath.Join(workspace, BenchmarkInputFilename)
	resultMetadata := req.ResultFilename
	if resultMetadata == "" {
		resultMetadata = filepath.Join(workspace, "result.txt")
	}
	linePath, err := docprocessing.ResolveInputFilePath(docprocessing.LineFileGeneratedEvent{}, resultMetadata, req.ParserName, stagingMetadata)
	if err != nil {
		return SeededInput{}, err
	}
	linePath, err = filepath.Abs(linePath)
	if err != nil || !pathWithin(workspace, filepath.Clean(linePath)) {
		return SeededInput{}, fmt.Errorf("resolved line path escapes workspace")
	}
	if err := rejectSymlinkPath(workspace, filepath.Clean(linePath)); err != nil {
		return SeededInput{}, err
	}
	created := false
	commitAttempted := false
	ok := false
	defer func() {
		if !ok && created && !commitAttempted {
			_ = os.Remove(linePath)
		}
	}()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return SeededInput{}, err
	}
	defer tx.Rollback()
	var owned sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT input_record_id FROM kb.benchmark_workspaces WHERE execution_attempt_id=$1 FOR UPDATE`, req.AttemptID).Scan(&owned); err != nil {
		return SeededInput{}, err
	}
	if owned.Valid {
		var tenant, parser, staging, result, fileName, status string
		var storeID int64
		if err = tx.QueryRowContext(ctx, `SELECT tenant_id, ks_store_id, parser_name, staging_filename, result_filename, file_name, status::text FROM kb.inputs WHERE id=$1`, owned.Int64).Scan(&tenant, &storeID, &parser, &staging, &result, &fileName, &status); err != nil {
			return SeededInput{}, err
		}
		if tenant != req.TenantID || storeID != req.StoreID || parser != req.ParserName || staging != stagingMetadata || result != linePath || fileName != BenchmarkInputFilename {
			return SeededInput{}, fmt.Errorf("seed input retry metadata conflict")
		}
		existing, readErr := os.ReadFile(linePath)
		if readErr != nil || !bytes.Equal(existing, req.Case.InputBytes) {
			return SeededInput{}, fmt.Errorf("seed input retry bytes conflict")
		}
		if err = tx.Commit(); err != nil {
			return SeededInput{}, err
		}
		ok = true
		return SeededInput{ID: owned.Int64, ParserName: parser, StagingFilename: staging, ResultFilename: result, FileName: fileName}, nil
	}
	if existing, readErr := os.ReadFile(linePath); readErr == nil {
		if !bytes.Equal(existing, req.Case.InputBytes) {
			return SeededInput{}, fmt.Errorf("staged orphan bytes conflict")
		}
	} else if os.IsNotExist(readErr) {
		if err = writeNewFileDurable(linePath, req.Case.InputBytes); err != nil {
			return SeededInput{}, err
		}
		created = true
	} else {
		return SeededInput{}, readErr
	}
	var id int64
	err = tx.QueryRowContext(ctx, seedInputQuery, req.TenantID, req.StoreID, "pdf", req.Title, req.ParserName, stagingMetadata, linePath, BenchmarkInputFilename, req.Status).Scan(&id)
	if err != nil {
		return SeededInput{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE kb.benchmark_workspaces SET input_record_id=$2 WHERE execution_attempt_id=$1 AND input_record_id IS NULL`, req.AttemptID, id)
	if err != nil {
		return SeededInput{}, err
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		return SeededInput{}, fmt.Errorf("benchmark: workspace input bind lost race")
	}
	var snapshot sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT input_record_id_snapshot FROM kb.benchmark_case_attempts WHERE id=$1 AND kind='execution' FOR UPDATE`, req.AttemptID).Scan(&snapshot); err != nil {
		return SeededInput{}, err
	}
	if snapshot.Valid && snapshot.Int64 != id {
		return SeededInput{}, fmt.Errorf("attempt input snapshot conflict")
	}
	if !snapshot.Valid {
		result, err = tx.ExecContext(ctx, `UPDATE kb.benchmark_case_attempts SET input_record_id_snapshot=$2 WHERE id=$1 AND kind='execution' AND input_record_id_snapshot IS NULL`, req.AttemptID, id)
		if err != nil {
			return SeededInput{}, err
		}
		if n, e := result.RowsAffected(); e != nil || n != 1 {
			return SeededInput{}, fmt.Errorf("benchmark: attempt input bind lost race")
		}
	}
	commitAttempted = true
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
	if !safeLeaf(parserName) {
		return "", fmt.Errorf("invalid parser name %q", parserName)
	}
	rootDir, err := canonicalSeedRoot(artifactDir)
	if err != nil {
		return "", err
	}
	root := strings.TrimSuffix(filepath.Base(strings.TrimSpace(stagingFilename)), filepath.Ext(strings.TrimSpace(stagingFilename)))
	parser := strings.TrimSpace(parserName)
	if root == "" || parser == "" {
		return "", fmt.Errorf("invalid artifact filename inputs")
	}
	target := filepath.Join(rootDir, strconv.FormatInt(recordID/1000, 10), strconv.FormatInt(recordID, 10), root+"_"+parser+extension)
	if !pathWithin(rootDir, target) {
		return "", fmt.Errorf("artifact path escapes artifact directory")
	}
	if err := rejectSymlinkPath(rootDir, target); err != nil {
		return "", err
	}
	return target, nil
}
