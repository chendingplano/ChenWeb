package docprocessing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type exSummaryInput struct{ rec DocMetadataInputRecord }

func (s *exSummaryInput) GetInputRecord(context.Context, int64) (DocMetadataInputRecord, error) {
	return s.rec, nil
}
func (s *exSummaryInput) UpdateInputMetadata(_ context.Context, _ int64, u DocMetadataUpdate) error {
	s.rec.StatusRaw = u.StatusRaw
	return nil
}

type exSummarySink struct {
	calls     int
	algorithm string
	items     []ExtractSummary
	err       error
}

func (s *exSummarySink) Replace(ctx context.Context, _ int64, algorithm string, items []ExtractSummary, verify func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if verify != nil {
		if err := verify(); err != nil {
			return err
		}
	}
	if s.err != nil {
		return s.err
	}
	s.calls++
	s.algorithm = algorithm
	s.items = items
	return nil
}

type exSummaryRunner func(context.Context, SumyRequest) (SumyResponse, error)

func (r exSummaryRunner) Extract(ctx context.Context, q SumyRequest) (SumyResponse, error) {
	return r(ctx, q)
}

func exSummaryFixture(t *testing.T) (*ExtractSummariesProcessor, *exSummaryInput, *exSummarySink, context.Context) {
	t.Helper()
	in := &exSummaryInput{rec: DocMetadataInputRecord{ID: 42, SourceLanguage: "en", StatusRaw: `[{"operation":"chunking","proc_status":"success"}]`}}
	sink := &exSummarySink{}
	p := NewExtractSummariesProcessor(in, nil, nil)
	p.Store = sink
	p.ConfigErr = nil
	p.Config = SumyConfig{Python: "python3", Algorithm: "lexrank", SentenceCount: 3}
	p.Runner = exSummaryRunner(func(_ context.Context, q SumyRequest) (SumyResponse, error) {
		out := SumyResponse{Algorithm: q.Algorithm, Versions: map[string]string{"sumy": "0.13.0"}}
		for _, c := range q.Chunks {
			text := c.Lines[0].Content
			sentence := ExtractSummarySentence{Text: text, Spans: []ExtractSummarySpan{{Line: c.Lines[0].LineNumber, Start: 0, End: len([]rune(text))}}}
			out.Summaries = append(out.Summaries, ExtractSummary{SeqNo: c.SeqNo, Language: "en", Summary: text, Sentences: []ExtractSummarySentence{sentence}})
		}
		return out, nil
	})
	ctx, _ := withChunkBufferHolder(context.Background())
	storeChunksInContext(ctx, []Chunk{{SeqNo: 2, Lines: []MarkedLine{{Line: Line{LineNo: 7, Content: "Storage shall remain below 8°C.", LineType: "text"}}}}})
	return p, in, sink, ctx
}

func TestExtractSummariesPersistsPerChunkAndKeepsOtherStatus(t *testing.T) {
	p, in, sink, ctx := exSummaryFixture(t)
	if err := p.HandleEvent(ctx, []byte(`{"record_id":42}`)); err != nil {
		t.Fatal(err)
	}
	if sink.calls != 1 || sink.algorithm != "lexrank" || len(sink.items) != 1 || sink.items[0].SeqNo != 2 {
		t.Fatalf("sink=%+v", sink)
	}
	if !strings.Contains(in.rec.StatusRaw, `"algorithm":"lexrank"`) || !strings.Contains(in.rec.StatusRaw, `"operation":"chunking"`) || !strings.Contains(in.rec.StatusRaw, `"proc_status":"success"`) {
		t.Fatal(in.rec.StatusRaw)
	}
}

func TestExtractSummariesRejectsIncompleteOrAlteredWorkerOutput(t *testing.T) {
	mutations := map[string]func(*SumyResponse){
		"missing":        func(r *SumyResponse) { r.Summaries = nil },
		"duplicate":      func(r *SumyResponse) { r.Summaries = append(r.Summaries, r.Summaries[0]) },
		"unknown":        func(r *SumyResponse) { r.Summaries[0].SeqNo = 99 },
		"algorithm":      func(r *SumyResponse) { r.Algorithm = "luhn" },
		"language":       func(r *SumyResponse) { r.Summaries[0].Language = "invented" },
		"wrong_language": func(r *SumyResponse) { r.Summaries[0].Language = "zh" },
		"text":           func(r *SumyResponse) { r.Summaries[0].Sentences[0].Text = "Temperature may exceed 80°C." },
		"span":           func(r *SumyResponse) { r.Summaries[0].Sentences[0].Spans[0].End = 999 },
		"summary":        func(r *SumyResponse) { r.Summaries[0].Summary = "invented" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			p, in, sink, ctx := exSummaryFixture(t)
			original := p.Runner
			p.Runner = exSummaryRunner(func(ctx context.Context, q SumyRequest) (SumyResponse, error) {
				r, e := original.Extract(ctx, q)
				mutate(&r)
				return r, e
			})
			if err := p.HandleEvent(ctx, []byte(`{"record_id":42}`)); err == nil {
				t.Fatal("expected failure")
			}
			if sink.calls != 0 {
				t.Fatal("replaced previous rows")
			}
			if !strings.Contains(in.rec.StatusRaw, `"proc_status":"failed"`) {
				t.Fatal(in.rec.StatusRaw)
			}
		})
	}
}

func TestExtractSummariesFailureAndStopPreserveRows(t *testing.T) {
	for _, stopped := range []bool{false, true} {
		t.Run(strings.ToUpper(strings.TrimSpace(map[bool]string{false: "failure", true: "stop"}[stopped])), func(t *testing.T) {
			p, in, sink, ctx := exSummaryFixture(t)
			ctx, cancel := context.WithCancelCause(ctx)
			p.Runner = exSummaryRunner(func(context.Context, SumyRequest) (SumyResponse, error) {
				if stopped {
					cancel(ErrPipelineStopped)
				}
				return SumyResponse{}, errors.New("worker failed")
			})
			err := p.HandleEvent(ctx, []byte(`{"record_id":42}`))
			if err == nil || sink.calls != 0 {
				t.Fatalf("error=%v calls=%d", err, sink.calls)
			}
			state := "failed"
			if stopped {
				state = "stopped"
				if !errors.Is(err, ErrPipelineStopped) {
					t.Fatal(err)
				}
			}
			if !strings.Contains(in.rec.StatusRaw, `"proc_status":"`+state+`"`) {
				t.Fatal(in.rec.StatusRaw)
			}
		})
	}
}

func TestExtractSummariesStoreFailureIsNotSuccess(t *testing.T) {
	p, in, sink, ctx := exSummaryFixture(t)
	sink.err = errors.New("database unavailable")
	if err := p.HandleEvent(ctx, []byte(`{"record_id":42}`)); err == nil {
		t.Fatal("expected persistence failure")
	}
	if !strings.Contains(in.rec.StatusRaw, `"proc_status":"failed"`) {
		t.Fatal(in.rec.StatusRaw)
	}
}

func TestExtractSummariesMissingChunkArtifactFails(t *testing.T) {
	p, _, sink, _ := exSummaryFixture(t)
	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":42}`)); err == nil {
		t.Fatal("missing source/chunks accepted")
	}
	if sink.calls != 0 {
		t.Fatal("deleted previous rows")
	}
}

func TestExtractSummariesChangedSourceOrEmptyBufferPreservesRows(t *testing.T) {
	t.Run("changed buffer", func(t *testing.T) {
		p, _, sink, ctx := exSummaryFixture(t)
		original := p.Runner
		p.Runner = exSummaryRunner(func(ctx context.Context, q SumyRequest) (SumyResponse, error) {
			r, e := original.Extract(ctx, q)
			storeChunksInContext(ctx, []Chunk{{SeqNo: 99}})
			return r, e
		})
		if err := p.HandleEvent(ctx, []byte(`{"record_id":42}`)); err == nil || sink.calls != 0 {
			t.Fatalf("error=%v calls=%d", err, sink.calls)
		}
	})
	t.Run("changed file", func(t *testing.T) {
		p, _, sink, ctx := exSummaryFixture(t)
		original := p.Runner
		path := filepath.Join(t.TempDir(), "input.txt")
		if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
		p.Runner = exSummaryRunner(func(ctx context.Context, q SumyRequest) (SumyResponse, error) {
			r, e := original.Extract(ctx, q)
			if err := os.WriteFile(path, []byte("updated"), 0600); err != nil {
				t.Fatal(err)
			}
			return r, e
		})
		payload := []byte(`{"record_id":42,"filename":"` + filepath.ToSlash(path) + `"}`)
		if err := p.HandleEvent(ctx, payload); err == nil || sink.calls != 0 {
			t.Fatalf("error=%v calls=%d", err, sink.calls)
		}
	})
	t.Run("empty buffer", func(t *testing.T) {
		p, _, sink, ctx := exSummaryFixture(t)
		storeChunksInContext(ctx, nil)
		if err := p.HandleEvent(ctx, []byte(`{"record_id":42}`)); err == nil || sink.calls != 0 {
			t.Fatalf("error=%v calls=%d", err, sink.calls)
		}
	})
}

func TestExtractSummariesRejectsArtifactWithMissingLine(t *testing.T) {
	p, in, sink, _ := exSummaryFixture(t)
	dir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", dir)
	in.rec.ParserName = "mineru"
	in.rec.StagingFilename = "input.pdf"
	input := filepath.Join(dir, "input_mineru.txt")
	if err := os.WriteFile(input, []byte("1\t1\tparagraph\tfont\t12\t[]\tOne source sentence.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	recordDir := filepath.Join(dir, "0", "42")
	if err := os.MkdirAll(recordDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recordDir, "input_mineru.chunks"), []byte("overlap: []\nlines: [1,999]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.HandleEvent(context.Background(), []byte(`{"record_id":42,"filename":"`+filepath.ToSlash(input)+`"}`)); err == nil {
		t.Fatal("artifact with missing source line accepted")
	}
	if sink.calls != 0 {
		t.Fatal("replaced previous output using damaged artifact")
	}
}

func TestExtractSummariesConfigDefaultsAndRejectsInvalid(t *testing.T) {
	for _, key := range []string{"SUMY_PYTHON", "EXTRACT_SUMMARIES_ALGORITHM", "EXTRACT_SUMMARIES_SENTENCES", "EXTRACT_SUMMARIES_TIMEOUT"} {
		t.Setenv(key, "")
	}
	c, err := sumyConfigFromEnv()
	if err != nil || c.Algorithm != "lexrank" || c.SentenceCount != 3 {
		t.Fatalf("%+v %v", c, err)
	}
	for key, value := range map[string]string{"EXTRACT_SUMMARIES_ALGORITHM": "invented", "EXTRACT_SUMMARIES_SENTENCES": "0", "EXTRACT_SUMMARIES_TIMEOUT": "-1s"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, value)
			if _, err := sumyConfigFromEnv(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestExtractSummaryStoreScopesReplacementAndRollsBack(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT id FROM kb.inputs WHERE id = \$1 FOR UPDATE`).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
			mock.ExpectExec(`DELETE FROM kb.ex_summaries WHERE input_record_id = \$1 AND algorithm = \$2`).WithArgs(int64(42), "lexrank").WillReturnResult(sqlmock.NewResult(0, 1))
			insert := mock.ExpectExec(`INSERT INTO kb.ex_summaries`)
			if fail {
				insert.WillReturnError(errors.New("insert failed"))
				mock.ExpectRollback()
			} else {
				insert.WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			}
			s := ExtractSummarySQLStore{DB: db}
			err = s.Replace(context.Background(), 42, "lexrank", []ExtractSummary{{SeqNo: 1, Language: "en", Summary: "text"}}, nil)
			if (err != nil) != fail {
				t.Fatalf("error=%v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExtractSummaryStoreRejectsStaleSourceBeforeDelete(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM kb.inputs WHERE id = \$1 FOR UPDATE`).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectRollback()
	err := (ExtractSummarySQLStore{DB: db}).Replace(context.Background(), 42, "lexrank", nil, func() error { return errors.New("source changed") })
	if err == nil {
		t.Fatal("expected stale source rejection")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExtractSummariesIsLocalAndKeepsControllerStatusDetails(t *testing.T) {
	if processorUsesLLM("extract_summaries") {
		t.Fatal("local extraction must not wait for LLM off-peak window")
	}
	p, in, sink, ctx := exSummaryFixture(t)
	control := ControlService{InputStore: in}
	result := control.runSingleProcessorCollect(ctx, []byte(`{"record_id":42}`), p, 42)
	if result.err != nil || sink.calls != 1 {
		t.Fatalf("result=%+v calls=%d", result, sink.calls)
	}
	for _, want := range []string{`"algorithm":"lexrank"`, `"num_summaries":1`, `"progress":"100%"`} {
		if !strings.Contains(in.rec.StatusRaw, want) {
			t.Fatal(in.rec.StatusRaw)
		}
	}
}
