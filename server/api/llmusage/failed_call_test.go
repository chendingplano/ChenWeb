package llmusage

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRecordFailedCallWritesOneRowPerTry(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	chunk := 2
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.failed_llm_calls`)).
		WithArgs(int64(825), int64(0), int64(77), "compliance", "review_compliance_judgement", "prompt-judgement-v1.md", 1, "model-a",
			"model-a", "model-b", "check failed: item 1", `{"verdicts":[]}`, FailedCallRecovered, &chunk).
		WillReturnResult(sqlmock.NewResult(1, 1))

	RecordFailedCall(context.Background(), db, nil, FailedCall{
		RecordID: 825, ReviewRunID: 77, Aspect: "compliance", Operation: "review_compliance_judgement",
		Prompt: "prompt-judgement-v1.md", Attempt: 1, Model: "model-a", PrimaryModel: "model-a", FallbackModel: "model-b",
		Error: "check failed: item 1", RawResponse: `{"verdicts":[]}`, Outcome: FailedCallRecovered, ChunkIndex: &chunk,
	})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
