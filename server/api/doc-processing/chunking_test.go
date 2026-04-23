package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeStore struct {
	rec           InputRecord
	getErr        error
	insertedRun   ChunkRunRecord
	insertCalls   int
	updatedStatus string
	updatedError  *string
	updateCalls   int
}

func (f *fakeStore) GetInputRecord(_ context.Context, id int64) (InputRecord, error) {
	if f.getErr != nil {
		return InputRecord{}, f.getErr
	}
	if id != f.rec.ID {
		return InputRecord{}, errors.New("not found")
	}
	return f.rec, nil
}

func (f *fakeStore) InsertChunkRun(_ context.Context, rec ChunkRunRecord) error {
	f.insertedRun = rec
	f.insertCalls++
	return nil
}

func (f *fakeStore) UpdateInputStatus(_ context.Context, id int64, statusJSON string, errorMsg *string) error {
	if id != f.rec.ID {
		return errors.New("wrong id")
	}
	f.updateCalls++
	f.updatedStatus = statusJSON
	f.updatedError = errorMsg
	return nil
}

func TestBuildChunks_RespectsTableBlock(t *testing.T) {
	input := strings.Join([]string{
		"1	1	paragraph	TestFont	12	[0,0,1,1]	Intro",
		"2	1	table	TestFont	12	[0,0,1,1]	|A|B|",
		"3	1	table	TestFont	12	[0,0,1,1]	|1|2|",
		"4	1	paragraph	TestFont	12	[0,0,1,1]	Tail",
	}, "\n")
	lines, err := ParseInputLines([]byte(input))
	if err != nil {
		t.Fatalf("ParseInputLines: %v", err)
	}

	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: 45, OverlapPercent: 0})
	if err != nil {
		t.Fatalf("BuildChunks: %v", err)
	}
	chunkByLine := map[int]int{}
	for _, c := range chunks {
		for _, ml := range c.Lines {
			chunkByLine[ml.Line.LineNo] = c.SeqNo
		}
	}
	if chunkByLine[2] == 0 || chunkByLine[3] == 0 || chunkByLine[2] != chunkByLine[3] {
		t.Fatalf("expected table lines 2 and 3 to stay in same chunk, got line2=%d line3=%d", chunkByLine[2], chunkByLine[3])
	}
}

func TestBuildChunks_NonNumericListNotSplit(t *testing.T) {
	input := strings.Join([]string{
		"1	1	paragraph	TestFont	12	[0,0,1,1]	Intro",
		"2	1	list-item	TestFont	12	[0,0,1,1]	- item A",
		"3	1	list-item	TestFont	12	[0,0,1,1]	- item B",
		"4	1	paragraph	TestFont	12	[0,0,1,1]	Tail",
	}, "\n")
	lines, err := ParseInputLines([]byte(input))
	if err != nil {
		t.Fatalf("ParseInputLines: %v", err)
	}

	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: 45, OverlapPercent: 0})
	if err != nil {
		t.Fatalf("BuildChunks: %v", err)
	}
	chunkByLine := map[int]int{}
	for _, c := range chunks {
		for _, ml := range c.Lines {
			chunkByLine[ml.Line.LineNo] = c.SeqNo
		}
	}
	if chunkByLine[2] == 0 || chunkByLine[3] == 0 || chunkByLine[2] != chunkByLine[3] {
		t.Fatalf("expected non-numeric list lines 2 and 3 to stay in same chunk, got line2=%d line3=%d", chunkByLine[2], chunkByLine[3])
	}
}

func TestBuildChunks_LargeNumericListCanSplit(t *testing.T) {
	raw := []string{"1	1	paragraph	TestFont	12	[0,0,1,1]	Intro"}
	for i := 1; i <= 8; i++ {
		raw = append(raw, "2	1	list-item	TestFont	12	[0,0,1,1]	"+string(rune('0'+i))+". item")
	}
	input := strings.Join(raw, "\n")
	lines, err := ParseInputLines([]byte(input))
	if err != nil {
		t.Fatalf("ParseInputLines: %v", err)
	}

	chunks, err := BuildChunks(lines, ChunkOptions{ChunkSize: 40, OverlapPercent: 0})
	if err != nil {
		t.Fatalf("BuildChunks: %v", err)
	}
	if len(chunks) < 4 {
		t.Fatalf("expected large numeric list to be splittable, chunks=%d", len(chunks))
	}
}

func TestService_HandleInput_WritesChunksAndStatus(t *testing.T) {
	tmp := t.TempDir()
	input := strings.Join([]string{
		"1	1	paragraph	TestFont	12	[0,0,1,1]	Intro",
		"2	1	paragraph	TestFont	12	[0,0,1,1]	More",
		"3	2	paragraph	TestFont	12	[0,0,1,1]	End",
	}, "\n")

	st := &fakeStore{rec: InputRecord{ID: 7523, StatusRaw: "[]"}}
	svc := NewFixedSizeChunkingService(st, nil)
	svc.ChunkDir = tmp
	svc.ChunkSize = 80
	svc.OverlapPercent = 50

	if err := svc.HandleInput(context.Background(), 7523, "sample.txt", []byte(input)); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}

	if st.insertCalls != 1 {
		t.Fatalf("InsertChunkRun calls=%d, want 1", st.insertCalls)
	}
	if st.updateCalls != 1 {
		t.Fatalf("UpdateInputStatus calls=%d, want 1", st.updateCalls)
	}

	chunk1 := filepath.Join(tmp, "7", "7523", "chunk_0001")
	chunk2 := filepath.Join(tmp, "7", "7523", "chunk_0002")
	if _, err := os.Stat(chunk1); err != nil {
		t.Fatalf("missing chunk1: %v", err)
	}
	if _, err := os.Stat(chunk2); err != nil {
		t.Fatalf("missing chunk2: %v", err)
	}

	b2, err := os.ReadFile(chunk2)
	if err != nil {
		t.Fatalf("read chunk2: %v", err)
	}
	contents := []string{string(b2)}
	chunk3 := filepath.Join(tmp, "7", "7523", "chunk_0003")
	if b3, err := os.ReadFile(chunk3); err == nil {
		contents = append(contents, string(b3))
	}
	for _, content := range contents {
		for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "r ") && !strings.HasPrefix(line, "o ") {
				t.Fatalf("expected chunk line to start with mark prefix, got %q", line)
			}
		}
	}

	var status []map[string]any
	if err := json.Unmarshal([]byte(st.updatedStatus), &status); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if len(status) == 0 {
		t.Fatalf("expected status entry")
	}
	last := status[len(status)-1]
	if last["operation"] != "chunked" {
		t.Fatalf("operation=%v, want chunked", last["operation"])
	}
	if last["proc-status"] != "success" {
		t.Fatalf("proc-status=%v, want success", last["proc-status"])
	}
}

func TestService_HandleInput_MissingInputFilename(t *testing.T) {
	st := &fakeStore{rec: InputRecord{ID: 1001, StatusRaw: "[]"}}
	svc := NewFixedSizeChunkingService(st, nil)
	svc.ChunkDir = t.TempDir()
	svc.ChunkSize = 2
	svc.OverlapPercent = 0

	err := svc.HandleInput(context.Background(), 1001, "", []byte("1	1	paragraph	TestFont	12	[0,0,1,1]	x"))
	if err == nil {
		t.Fatalf("expected error when input_filename is empty")
	}
	if !strings.Contains(err.Error(), "missing input filename") {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.insertCalls != 0 {
		t.Fatalf("InsertChunkRun calls=%d, want 0", st.insertCalls)
	}
	if st.updateCalls != 1 {
		t.Fatalf("UpdateInputStatus calls=%d, want 1", st.updateCalls)
	}
	if st.updatedError == nil || !strings.Contains(*st.updatedError, "missing input filename") {
		t.Fatalf("expected persisted error for missing filename, got %v", st.updatedError)
	}
}

func TestNewService_UsesRequiredAndDefaultChunkEnv(t *testing.T) {
	t.Setenv("CHUNK_SIZE", "")
	t.Setenv("CHUNK_OVERLAP_PERCENT", "")
	t.Setenv("ARTIFACT_DIR", "")

	svc := NewFixedSizeChunkingService(&fakeStore{}, nil)
	if svc.ChunkSize != 300 {
		t.Fatalf("ChunkSize=%d, want 300", svc.ChunkSize)
	}
	if svc.OverlapPercent != 20 {
		t.Fatalf("OverlapPercent=%d, want 20", svc.OverlapPercent)
	}
	if svc.ChunkDir != "" {
		t.Fatalf("ChunkDir=%q, want empty when ARTIFACT_DIR is unset", svc.ChunkDir)
	}
}

func TestService_HandleInput_MissingChunkDir(t *testing.T) {
	st := &fakeStore{rec: InputRecord{ID: 2002, StatusRaw: "[]"}}
	svc := NewFixedSizeChunkingService(st, nil)
	svc.ChunkDir = ""
	svc.ChunkSize = 2
	svc.OverlapPercent = 0

	err := svc.HandleInput(context.Background(), 2002, "sample.txt", []byte("1	1	paragraph	TestFont	12	[0,0,1,1]	x"))
	if err == nil {
		t.Fatalf("expected error when ARTIFACT_DIR is empty")
	}
	if !strings.Contains(err.Error(), "missing ARTIFACT_DIR") {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.insertCalls != 0 {
		t.Fatalf("InsertChunkRun calls=%d, want 0", st.insertCalls)
	}
	if st.updateCalls != 1 {
		t.Fatalf("UpdateInputStatus calls=%d, want 1", st.updateCalls)
	}
	if st.updatedError == nil || !strings.Contains(*st.updatedError, "missing ARTIFACT_DIR") {
		t.Fatalf("expected persisted error for missing ARTIFACT_DIR, got %v", st.updatedError)
	}
}
