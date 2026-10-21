package docprocessing

import (
	"context"
	"database/sql"
	"fmt"
)

type ExtractSummaryStore interface {
	Replace(context.Context, int64, string, []ExtractSummary, func() error) error
}
type ExtractSummarySQLStore struct{ DB *sql.DB }

func (s ExtractSummarySQLStore) Replace(ctx context.Context, recordID int64, algorithm string, items []ExtractSummary, verify func() error) error {
	if s.DB == nil {
		return fmt.Errorf("extract summaries database is nil")
	}
	if recordID <= 0 || !validSumyAlgorithm(algorithm) {
		return fmt.Errorf("invalid extract summary record or algorithm")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lockedID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM kb.inputs WHERE id = $1 FOR UPDATE`, recordID).Scan(&lockedID); err != nil {
		return err
	}
	if verify != nil {
		if err := verify(); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.ex_summaries WHERE input_record_id = $1 AND algorithm = $2`, recordID, algorithm); err != nil {
		return err
	}
	const insert = `INSERT INTO kb.ex_summaries (
summary_id,input_record_id,algorithm,summary_level,summary_seq_no,
source_line_spans,source_sentence_spans,summary_text,language,extraction_config
) VALUES ($1,$2,$3,0,$4,$5::jsonb,$6::jsonb,$7,$8,$9::jsonb)`
	for _, item := range items {
		lineNos := []int{}
		for _, sentence := range item.Sentences {
			for _, span := range sentence.Spans {
				lineNos = append(lineNos, span.Line)
			}
		}
		config := item.ConfigJSON
		if config == "" {
			config = "{}"
		}
		if _, err := tx.ExecContext(ctx, insert, fmt.Sprintf("%d_exsum_%s_0_%d", recordID, algorithm, item.SeqNo), recordID, algorithm, item.SeqNo,
			mustJSONString(lineRangesFromNumbers(lineNos), []string{}), mustJSONString(item.Sentences, []ExtractSummarySentence{}), item.Summary, item.Language, config); err != nil {
			return err
		}
	}
	return tx.Commit()
}
