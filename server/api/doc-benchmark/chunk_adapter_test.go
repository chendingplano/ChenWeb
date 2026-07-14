package docbenchmark

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestChunkAdapterCapturesProductionRunRowAndReconcilesEachChunk(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "input_benchmark.chunks")
	if err := os.WriteFile(path, []byte("overlap: []\nlines: [1-4, 7]\n\noverlap: [7]\nlines: [8-9]"), 0o600); err != nil {
		t.Fatal(err)
	}
	const expectedChunkQuery = `SELECT id, chunking_method, chunking_size, overlap_percent, notes,
       overlap_lines, normal_lines, chunk_lines, create_time, update_time
FROM kb.chunks
WHERE source_record_id = $1
ORDER BY id ASC`
	mock.ExpectQuery(regexp.QuoteMeta(expectedChunkQuery)).WithArgs(int64(1203)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "chunking_method", "chunking_size", "overlap_percent", "notes", "overlap_lines", "normal_lines", "chunk_lines", "create_time", "update_time"}).
			AddRow(int64(9), "fixed", 1000, 20, "", `["[]","[7]"]`, `["[1-4, 7]","[8-9]"]`, `["a","b"]`, nil, nil),
	)
	a := ChunkAdapter{DB: db, ArtifactPath: func(int64) string { return path }}
	captured, err := a.Capture(context.Background(), 1203)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	actualAny, err := a.Reconcile(captured)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	actual := actualAny.(ChunkActual)
	if len(actual.Chunks) != 2 {
		t.Fatalf("chunks=%#v", actual.Chunks)
	}
	if got := actual.Chunks[0].NormalLines; !equalInts(got, []int{1, 2, 3, 4, 7}) {
		t.Fatalf("normal=%v", got)
	}
	if got := actual.Chunks[1].OverlapLines; !equalInts(got, []int{7}) {
		t.Fatalf("overlap=%v", got)
	}
	if actual.Chunks[0].Sequence != 1 || actual.Chunks[1].Sequence != 2 {
		t.Fatalf("sequences=%#v", actual.Chunks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestChunkAdapterRejectsRepresentationDisagreementAndMissingArtifact(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		write      bool
	}{
		{"disagreement", "overlap: []\nlines: [1-2]", true},
		{"missing", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			path := filepath.Join(t.TempDir(), "x.chunks")
			if tc.write {
				_ = os.WriteFile(path, []byte(tc.file), 0o600)
			}
			mock.ExpectQuery(`SELECT id, chunking_method, chunking_size, overlap_percent, notes,\s+overlap_lines, normal_lines, chunk_lines, create_time, update_time\s+FROM kb\.chunks\s+WHERE source_record_id = \$1\s+ORDER BY id ASC`).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "chunking_method", "chunking_size", "overlap_percent", "notes", "overlap_lines", "normal_lines", "chunk_lines", "create_time", "update_time"}).AddRow(1, "fixed", 1, 0, "", `["[]"]`, `["[1-3]"]`, `["x"]`, nil, nil))
			a := ChunkAdapter{DB: db, ArtifactPath: func(int64) string { return path }}
			v, err := a.Capture(context.Background(), 1)
			if tc.write && err == nil {
				_, err = a.Reconcile(v)
			}
			if !errors.Is(err, ErrInvalidOutput) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestChunkAdapterRejectsMalformedProductionRanges(t *testing.T) {
	for _, raw := range []string{`["[4-2]"]`, `["[1, 1]"]`, `["[0]"]`, `["[1-x]"]`} {
		t.Run(raw, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			path := filepath.Join(t.TempDir(), "x.chunks")
			_ = os.WriteFile(path, []byte("overlap: []\nlines: [1]"), 0o600)
			mock.ExpectQuery(`FROM kb\.chunks\s+WHERE source_record_id = \$1\s+ORDER BY id ASC`).WillReturnRows(sqlmock.NewRows([]string{"id", "chunking_method", "chunking_size", "overlap_percent", "notes", "overlap_lines", "normal_lines", "chunk_lines", "create_time", "update_time"}).AddRow(1, "fixed", 1, 0, "", `["[]"]`, raw, `["x"]`, nil, nil))
			_, err := (ChunkAdapter{DB: db, ArtifactPath: func(int64) string { return path }}).Capture(context.Background(), 1)
			if !errors.Is(err, ErrInvalidOutput) {
				t.Fatalf("raw=%s err=%v", raw, err)
			}
		})
	}
}

func TestChunksArtifactRequiresExactProductionGrammar(t *testing.T) {
	valid := "overlap: []\nlines: [1-2]\n\noverlap: [2]\nlines: [3]"
	if got, err := parseChunksFile([]byte(valid)); err != nil || len(got) != 2 {
		t.Fatalf("valid artifact: chunks=%d err=%v", len(got), err)
	}
	invalid := map[string]string{
		"empty":             "",
		"normal alias":      "overlap: []\nnormal: [1]",
		"reversed fields":   "lines: [1]\noverlap: []",
		"missing overlap":   "lines: [1]",
		"missing lines":     "overlap: []",
		"duplicate overlap": "overlap: []\noverlap: []\nlines: [1]",
		"duplicate lines":   "overlap: []\nlines: [1]\nlines: [2]",
		"malformed array":   "overlap: []\nlines: [1, x]",
	}
	for name, artifact := range invalid {
		t.Run(name, func(t *testing.T) {
			if _, err := parseChunksFile([]byte(artifact)); err == nil {
				t.Fatalf("accepted %q", artifact)
			}
		})
	}
}

func TestChunkRangesRejectUnboundedExpansionAndSourceOverflow(t *testing.T) {
	for _, raw := range []string{"[1-1000000000]", "[9223372036854775807]"} {
		if _, err := parseLineRanges(raw); err == nil {
			t.Fatalf("accepted dangerous range %s", raw)
		}
	}
	if _, err := parseLineRangesWithMax("[1-11]", 10); err == nil {
		t.Fatal("accepted line beyond source maximum")
	}
	if got, err := parseLineRangesWithMax("[1-10]", 10); err != nil || len(got) != 10 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestChunkAdapterRejectsLineBeyondKnownSource(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	path := filepath.Join(t.TempDir(), "x.chunks")
	_ = os.WriteFile(path, []byte("overlap: []\nlines: [1-4]"), 0o600)
	mock.ExpectQuery(`FROM kb\.chunks`).WillReturnRows(sqlmock.NewRows([]string{"id", "chunking_method", "chunking_size", "overlap_percent", "notes", "overlap_lines", "normal_lines", "chunk_lines", "create_time", "update_time"}).AddRow(1, "fixed", 1, 0, "", `["[]"]`, `["[1-4]"]`, `["x"]`, nil, nil))
	_, err := (ChunkAdapter{DB: db, ArtifactPath: func(int64) string { return path }, SourceMaxLine: 3}).Capture(context.Background(), 1)
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("err=%v", err)
	}
}
