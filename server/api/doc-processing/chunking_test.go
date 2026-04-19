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
		"1 1 paragraph Intro [0,0,1,1]",
		"2 1 table |A|B| [0,0,1,1]",
		"3 1 table |1|2| [0,0,1,1]",
		"4 1 paragraph Tail [0,0,1,1]",
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
		"1 1 paragraph Intro [0,0,1,1]",
		"2 1 list-item - item A [0,0,1,1]",
		"3 1 list-item - item B [0,0,1,1]",
		"4 1 paragraph Tail [0,0,1,1]",
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
	raw := []string{"1 1 paragraph Intro [0,0,1,1]"}
	for i := 1; i <= 8; i++ {
		raw = append(raw, "2 1 list-item "+string(rune('0'+i))+". item [0,0,1,1]")
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
		"1 1 paragraph Intro [0,0,1,1]",
		"2 1 paragraph More [0,0,1,1]",
		"3 2 paragraph End [0,0,1,1]",
	}, "\n")

	st := &fakeStore{rec: InputRecord{ID: 7523, StatusRaw: "[]"}}
	svc := NewService(st, nil)
	svc.ChunkDir = tmp
	svc.ChunkSize = 40
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
	lines := strings.Split(strings.TrimSpace(string(b2)), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "o ") {
		t.Fatalf("expected overlap mark 'o' as prefix in chunk2 first line, got: %s", string(b2))
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
	svc := NewService(st, nil)
	svc.ChunkDir = t.TempDir()
	svc.ChunkSize = 2
	svc.OverlapPercent = 0

	err := svc.HandleInput(context.Background(), 1001, "", []byte("1 1 paragraph x [0,0,1,1]"))
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
	t.Setenv("CHUNK_DIR", "")

	svc := NewService(&fakeStore{}, nil)
	if svc.ChunkSize != 300 {
		t.Fatalf("ChunkSize=%d, want 300", svc.ChunkSize)
	}
	if svc.OverlapPercent != 20 {
		t.Fatalf("OverlapPercent=%d, want 20", svc.OverlapPercent)
	}
	if svc.ChunkDir != "" {
		t.Fatalf("ChunkDir=%q, want empty when CHUNK_DIR is unset", svc.ChunkDir)
	}
}

func TestService_HandleInput_MissingChunkDir(t *testing.T) {
	st := &fakeStore{rec: InputRecord{ID: 2002, StatusRaw: "[]"}}
	svc := NewService(st, nil)
	svc.ChunkDir = ""
	svc.ChunkSize = 2
	svc.OverlapPercent = 0

	err := svc.HandleInput(context.Background(), 2002, "sample.txt", []byte("1 1 paragraph x [0,0,1,1]"))
	if err == nil {
		t.Fatalf("expected error when CHUNK_DIR is empty")
	}
	if !strings.Contains(err.Error(), "missing CHUNK_DIR") {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.insertCalls != 0 {
		t.Fatalf("InsertChunkRun calls=%d, want 0", st.insertCalls)
	}
	if st.updateCalls != 1 {
		t.Fatalf("UpdateInputStatus calls=%d, want 1", st.updateCalls)
	}
	if st.updatedError == nil || !strings.Contains(*st.updatedError, "missing CHUNK_DIR") {
		t.Fatalf("expected persisted error for missing CHUNK_DIR, got %v", st.updatedError)
	}
}
