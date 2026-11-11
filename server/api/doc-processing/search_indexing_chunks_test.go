package docprocessing

import (
	"context"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildChunkRegistryRowsPreservesSource(t *testing.T) {
	chunks := []Chunk{{SeqNo: 1, Lines: []MarkedLine{
		{Mark: "o", Line: Line{LineNo: 3, PageNo: 1, LineType: "paragraph", Content: "context"}},
		{Mark: "n", Line: Line{LineNo: 4, PageNo: 2, LineType: "paragraph", Content: "original 中文 needle"}},
	}}}
	rows, err := buildChunkRegistryRows(InputRecord{ID: 42, FileName: "source.pdf"}, chunks)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	r := rows[0]
	if r.ArtifactID != "42_chk_1" || r.ArtifactType != "chunk" || !strings.Contains(r.SearchDocument, "original 中文 needle") || !strings.Contains(r.SearchDocument, "context") {
		t.Fatalf("wrong chunk: %+v", r)
	}
	var payload struct {
		ChunkSeqNo       int   `json:"chunk_seqno"`
		OverlapLineSpans []any `json:"overlap_line_spans"`
	}
	if err := json.Unmarshal(r.SemanticPayload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ChunkSeqNo != 1 || len(payload.OverlapLineSpans) != 1 || !strings.Contains(string(r.SourceLineSpans), "4") {
		t.Fatalf("lost provenance: %+v", r)
	}
}

func TestBackfillChunkSearchReportsMissingSavedArtifact(t *testing.T) {
	t.Setenv("SEARCH_SEMANTIC_ENABLED", "false")
	t.Setenv("ARTIFACT_DIR", t.TempDir())
	db, mock, _ := sqlmock.New()
	defer db.Close()
	source := filepath.Join(t.TempDir(), "source_test.txt")
	if err := os.WriteFile(source, []byte("1\t1\tparagraph\tF\t12\t[1,1,2,2]\ttext\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id FROM kb.inputs").WithArgs(int64(0), int64(0), 2).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectQuery("SELECT pg_try_advisory_lock").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"acquired"}).AddRow(true))
	mock.ExpectExec("INSERT INTO kb.chunk_search_state").WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT generation").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(0))
	mock.ExpectQuery("SELECT id,").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "parser_name", "result_filename", "staging_filename", "file_name", "status", "language"}).AddRow(42, "test", source, "source.pdf", "source.pdf", "[]", "en"))
	mock.ExpectExec("SELECT pg_advisory_unlock").WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
	res, err := BackfillChunkSearch(context.Background(), db, 0, 0, 1, &fakeLogger{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 || res.Indexed != 0 || res.NextRecordID != 42 || len(res.Errors) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSearchChunksRejectsMissingAndStaleArtifact(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ARTIFACT_DIR", root)
	rec := InputRecord{ID: 42, StagingFilename: "source.pdf", ParserName: "test"}
	lines := []Line{{LineNo: 1, PageNo: 1, Content: "text"}}
	if _, err := loadSearchChunks(rec, lines); err == nil {
		t.Fatal("missing artifact must not rebuild")
	}
	dir := filepath.Join(root, "0", "42")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "source_test.chunks"), []byte("overlap: []\nlines: [1, 2]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSearchChunks(rec, lines); err == nil {
		t.Fatal("missing source reference must fail")
	}
	if err := os.WriteFile(filepath.Join(dir, "source_test.chunks"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSearchChunks(rec, lines); err == nil {
		t.Fatal("empty artifact must not clear an existing index")
	}
}

func TestLoadSearchChunksPreservesEntriesWithoutBlankSeparators(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ARTIFACT_DIR", root)
	dir := filepath.Join(root, "0", "42")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "source_test.chunks"), []byte("overlap: []\nlines: [1]\noverlap: [1]\nlines: [2]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	chunks, err := loadSearchChunks(InputRecord{ID: 42, StagingFilename: "source.pdf", ParserName: "test"}, []Line{{LineNo: 1, Content: "first"}, {LineNo: 2, Content: "second"}})
	if err != nil || len(chunks) != 2 || chunks[1].SeqNo != 2 || len(chunks[0].Lines) != 1 {
		t.Fatalf("chunk boundaries lost: %+v %v", chunks, err)
	}
}

func TestChunkingPublishesSearchAndReportsIndexFailure(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fails], func(t *testing.T) {
			t.Setenv("SEARCH_SEMANTIC_ENABLED", "false")
			root := t.TempDir()
			source := filepath.Join(root, "source_test.txt")
			body := []byte("1\t1\tparagraph\tF\t12\t[1,1,2,2]\tcurrent source\n")
			if err := os.WriteFile(source, body, 0600); err != nil {
				t.Fatal(err)
			}
			rec := InputRecord{ID: 42, FileName: "source.pdf", ResultFilename: source, StagingFilename: "source.pdf", ParserName: "test", StatusRaw: "[]"}
			store := &fakeStore{rec: rec}
			db, mock, _ := sqlmock.New()
			defer db.Close()
			old := ApiTypes.ProjectDBHandle
			ApiTypes.ProjectDBHandle = db
			defer func() { ApiTypes.ProjectDBHandle = old }()
			mock.ExpectQuery("SELECT pg_try_advisory_lock").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"acquired"}).AddRow(true))
			mock.ExpectExec("INSERT INTO kb.chunk_search_state").WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT generation").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(0))
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT generation").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(0))
			mock.ExpectExec("DELETE FROM kb.search_artifacts").WithArgs("chunk", int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
			insert := mock.ExpectExec("INSERT INTO kb.search_artifacts")
			if fails {
				insert.WillReturnError(os.ErrPermission)
				mock.ExpectRollback()
			} else {
				insert.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE kb.chunk_search_state").WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			mock.ExpectExec("SELECT pg_advisory_unlock").WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
			svc := &FixedSizeChunkingService{Store: store, Extractor: &fakeSemanticExtractor{}, Logger: &fakeLogger{}, Now: time.Now, ChunkDir: root, ArtifactWebDir: root, ChunkSize: 300}
			err := svc.HandleInput(context.Background(), 42, "source_test.txt", body)
			if (err != nil) != fails {
				t.Fatalf("unexpected result: %v", err)
			}
			expected := "success"
			if fails {
				expected = "failed"
			}
			if !strings.Contains(store.updatedStatus, `"proc_status":"`+expected+`"`) {
				t.Fatalf("status: %s", store.updatedStatus)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestChunkSearchRejectsBufferFromBeforeSourceRewrite(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source_test.txt")
	if err := os.WriteFile(source, []byte("1\t1\tparagraph\tF\t12\t[1,1,2,2]\tnew source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rec := InputRecord{ID: 42, ResultFilename: source, ParserName: "test", StagingFilename: "source.pdf"}
	old := []Line{{LineNo: 1, PageNo: 1, LineType: "paragraph", Content: "old source"}}
	if err := verifyChunkSearchSource(rec, old); err == nil || !strings.Contains(err.Error(), "source changed") {
		t.Fatalf("accepted stale buffer: %v", err)
	}
	old[0].Content = "new source"
	if err := verifyChunkSearchSource(rec, old); err != nil {
		t.Fatal(err)
	}
}
