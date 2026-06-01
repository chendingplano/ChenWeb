package docprocessing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type fakePhaseService struct {
	chunkCalls   int
	topicCalls   int
	summaryCalls int
	logName      string
	modelNames   []string
	promptNames  []string
	extraInfo    map[string]any
}

func (f *fakePhaseService) HandleInput(_ context.Context, _ int64, _ string, _ []byte) error {
	f.chunkCalls++
	return nil
}

func (f *fakePhaseService) HandleGenerateTopicsInput(_ context.Context, _ int64, _ string, _ []byte) error {
	f.topicCalls++
	return nil
}

func (f *fakePhaseService) HandleGenerateSummariesInput(_ context.Context, _ int64, _ string, _ []byte) error {
	f.summaryCalls++
	return nil
}

func (f *fakePhaseService) LogName() string {
	return f.logName
}

func (f *fakePhaseService) DocProcModelNames() []string {
	return append([]string(nil), f.modelNames...)
}

func (f *fakePhaseService) DocProcPromptNames() []string {
	return append([]string(nil), f.promptNames...)
}

func (f *fakePhaseService) DocProcSummaryExtraInfo() map[string]any {
	if f.extraInfo == nil {
		return nil
	}
	out := make(map[string]any, len(f.extraInfo))
	for k, v := range f.extraInfo {
		out[k] = v
	}
	return out
}

func TestGenerateTopicsProcessor_PrefersTopicPhaseHandler(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_81_opendata.txt")
	if err := os.WriteFile(lineFile, []byte("1\t1\tparagraph\tF\t12\t[1,1,2,2]\tok\n"), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	service := &fakePhaseService{}
	processor := NewGenerateTopicsProcessor(&fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              81,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_81.json"),
		StagingFilename: filepath.Join(tmp, "stdGk_3032174.pdf"),
		StatusRaw:       "[]",
	}}, service, nil)

	if err := processor.HandleEvent(context.Background(), []byte(`{"record_id":"81","line_file_filename":"`+lineFile+`"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if service.topicCalls != 1 {
		t.Fatalf("topicCalls=%d, want 1", service.topicCalls)
	}
	if service.chunkCalls != 0 {
		t.Fatalf("chunkCalls=%d, want 0", service.chunkCalls)
	}
}

func TestGenerateSummariesProcessor_PrefersSummaryPhaseHandler(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_81_opendata.txt")
	if err := os.WriteFile(lineFile, []byte("1\t1\tparagraph\tF\t12\t[1,1,2,2]\tok\n"), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	service := &fakePhaseService{}
	processor := NewGenerateSummariesProcessor(&fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              81,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_81.json"),
		StagingFilename: filepath.Join(tmp, "stdGk_3032174.pdf"),
		StatusRaw:       "[]",
	}}, service, nil)

	if err := processor.HandleEvent(context.Background(), []byte(`{"record_id":"81","line_file_filename":"`+lineFile+`"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if service.summaryCalls != 1 {
		t.Fatalf("summaryCalls=%d, want 1", service.summaryCalls)
	}
	if service.chunkCalls != 0 {
		t.Fatalf("chunkCalls=%d, want 0", service.chunkCalls)
	}
}

func TestGenerateTopicsProcessor_WritesSummaryLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	procProgress := "100%"
	activityName := "extract_topics"
	mock.ExpectExec("INSERT INTO kb\\.doc_proc_logs").
		WithArgs(
			"extract topics",
			"generate_topics",
			"{}",
			nil,
			int64(81),
			&procProgress,
			"extract_topics_finish",
			nil,
			nil,
			&activityName,
			nil,
			nil,
			"{\"total_chunks\":4,\"total_lines\":20,\"total_time_ms\":1500,\"total_topics\":6}",
			int64(1500),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO kb\\.doc_proc_logs").
		WithArgs(
			nil,
			"generate_topics",
			"{topic-model,topic-embed-model}",
			"topic-prompt",
			int64(81),
			nil,
			"doc_proc_summary",
			nil,
			nil,
			nil,
			nil,
			nil,
			jsonWithFieldMatcher{field: "topics_generated", want: float64(6)},
			int64(1500),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_81_opendata.txt")
	if err := os.WriteFile(lineFile, []byte("1\t1\tparagraph\tF\t12\t[1,1,2,2]\tok\n"), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	service := &fakePhaseService{
		modelNames:  []string{"topic-model", "topic-embed-model"},
		promptNames: []string{"topic-prompt"},
		extraInfo: map[string]any{
			"proc_progress":    "100%",
			"total_chunks":     4,
			"total_lines":      20,
			"total_topics":     6,
			"topics_generated": 6,
		},
	}
	processor := NewGenerateTopicsProcessor(&fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              81,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_81.json"),
		StagingFilename: filepath.Join(tmp, "stdGk_3032174.pdf"),
		StatusRaw:       "[]",
	}}, service, nil)
	times := []time.Time{
		time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 27, 12, 0, 1, 500000000, time.UTC),
	}
	idx := 0
	processor.Now = func() time.Time {
		if idx >= len(times) {
			return times[len(times)-1]
		}
		v := times[idx]
		idx++
		return v
	}

	if err := processor.HandleEvent(context.Background(), []byte(`{"record_id":"81","line_file_filename":"`+lineFile+`"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestGenerateSummariesProcessor_WritesSummaryLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	activityName := "generate_summary"
	procProgress := "100% (3/3)"
	mock.ExpectExec("INSERT INTO kb\\.doc_proc_logs").
		WithArgs(
			"generate summary",
			"generate_summary",
			"{summary-model,summary-embed-model}",
			"summary-prompt",
			int64(81),
			&procProgress,
			"generate_summary_finish",
			nil,
			nil,
			&activityName,
			nil,
			nil,
			"{\"num_summaries\":3,\"total_lines\":20,\"total_time_ms\":800}",
			int64(800),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO kb\\.doc_proc_logs").
		WithArgs(
			nil,
			"generate_summaries",
			"{summary-model,summary-embed-model}",
			"summary-prompt",
			int64(81),
			&procProgress,
			"doc_proc_summary",
			nil,
			nil,
			nil,
			nil,
			nil,
			jsonWithFieldMatcher{field: "summaries_generated", want: float64(3)},
			int64(800),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_81_opendata.txt")
	if err := os.WriteFile(lineFile, []byte("1\t1\tparagraph\tF\t12\t[1,1,2,2]\tok\n"), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	service := &fakePhaseService{
		modelNames:  []string{"summary-model", "summary-embed-model"},
		promptNames: []string{"summary-prompt"},
		extraInfo: map[string]any{
			"total_chunks":        2,
			"total_lines":         20,
			"num_summaries":       3,
			"proc_progress":       "100% (3/3)",
			"summaries_generated": 3,
		},
	}
	processor := NewGenerateSummariesProcessor(&fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              81,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_81.json"),
		StagingFilename: filepath.Join(tmp, "stdGk_3032174.pdf"),
		StatusRaw:       "[]",
	}}, service, nil)
	times := []time.Time{
		time.Date(2026, 5, 27, 13, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 27, 13, 0, 0, 800000000, time.UTC),
	}
	idx := 0
	processor.Now = func() time.Time {
		if idx >= len(times) {
			return times[len(times)-1]
		}
		v := times[idx]
		idx++
		return v
	}

	if err := processor.HandleEvent(context.Background(), []byte(`{"record_id":"81","line_file_filename":"`+lineFile+`"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestChunkingProcessor_WritesMethodSpecificSummaryLog(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectExec("INSERT INTO kb\\.doc_proc_logs").
		WithArgs(
			nil,
			"topic_chunking",
			"{topic-model,summary-model,topic-embed-model}",
			"topic-prompt,summary-prompt",
			int64(81),
			nil,
			"doc_proc_summary",
			nil,
			nil,
			nil,
			nil,
			nil,
			"{\"total_chunks\":4}",
			int64(900),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "ocr_rslt_81_opendata.txt")
	if err := os.WriteFile(lineFile, []byte("1\t1\tparagraph\tF\t12\t[1,1,2,2]\tok\n"), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	service := &fakePhaseService{
		logName:     "topic_chunking",
		modelNames:  []string{"topic-model", "summary-model", "topic-embed-model"},
		promptNames: []string{"topic-prompt", "summary-prompt"},
		extraInfo: map[string]any{
			"total_chunks": 4,
		},
	}
	processor := NewChunkingProcessor(&fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              81,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(tmp, "ocr_rslt_81.json"),
		StagingFilename: filepath.Join(tmp, "stdGk_3032174.pdf"),
		StatusRaw:       "[]",
	}}, service, nil)
	times := []time.Time{
		time.Date(2026, 5, 27, 14, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 27, 14, 0, 0, 900000000, time.UTC),
	}
	idx := 0
	processor.Now = func() time.Time {
		if idx >= len(times) {
			return times[len(times)-1]
		}
		v := times[idx]
		idx++
		return v
	}

	if err := processor.HandleEvent(context.Background(), []byte(`{"record_id":"81","line_file_filename":"`+lineFile+`"}`)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
