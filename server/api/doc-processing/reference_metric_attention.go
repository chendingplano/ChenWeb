package docprocessing

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chendingplano/deepdoc/server/api/attention"
)

func metricReferenceAttention(m RefMention, out ResolveOutcome) attention.Report {
	key := firstNonEmptyTrimmed(m.MentionKey, m.RefID)
	report := attention.Report{
		DedupeKey:     fmt.Sprintf("metric:%d:%s", m.InputRecordID, key),
		Title:         firstNonEmptyTrimmed(m.CitedRaw, m.TargetContent, m.RefID),
		InputRecordID: m.InputRecordID,
	}
	reason := out.State
	switch out.State {
	case metricRefPending:
		if m.R2State == refR2NotHeld || m.R2State == refR2EditionMis {
			report.ItemType = attention.TypeMissingReferenceDocument
			reason = m.R2State
		}
	case metricRefNoMatch, metricRefNeedsReview:
		report.ItemType = attention.TypeUnresolvedReference
		if len(out.Candidates) == 0 {
			report.ItemType = attention.TypeReferenceMetricNoMatch
		}
		if len(out.Notes) > 0 {
			reason = out.Notes[0]
		}
	}
	report.Details = map[string]any{
		"family":            "metric",
		"mention_key":       key,
		"ref_id":            m.RefID,
		"cited_raw":         m.CitedRaw,
		"source_line_spans": m.SourceLineSpans,
		"target_content":    m.TargetContent,
		"target_locator":    m.TargetLocator,
		"metric_name":       firstNonEmptyTrimmed(m.MetricName, m.MetricNameEn),
		"holding_state":     m.R2State,
		"target_record_id":  out.TargetInputID,
		"state":             out.State,
		"reason":            reason,
		"candidate_count":   len(out.Candidates),
		"notes":             out.Notes,
	}
	return report
}

func syncMetricReferenceAttention(ctx context.Context, tx *sql.Tx, m RefMention, out ResolveOutcome) error {
	report := metricReferenceAttention(m, out)
	// Older unresolved-reference items included the target ID after the mention
	// key. Retire them when this citation is reconciled under the stable key.
	if _, err := tx.ExecContext(ctx, `
UPDATE kb.attention_items
SET status = 'resolved', resolved_by = 'metric-reference-resolver', resolved_at = NOW(),
    resolution = 'metric reference outcome changed', modify_time = NOW()
WHERE input_record_id = $1 AND status = 'open' AND details->>'family' = 'metric'
  AND ((dedupe_key = $2 AND item_type <> $3)
       OR (item_type = $4 AND starts_with(dedupe_key, $2 || ':')))`,
		m.InputRecordID, report.DedupeKey, report.ItemType, attention.TypeUnresolvedReference); err != nil {
		return fmt.Errorf("close stale metric reference attention items: %w", err)
	}
	if report.ItemType == "" {
		return nil
	}
	if _, err := (attention.SQLStore{}).RecordTx(ctx, tx, report); err != nil {
		return fmt.Errorf("record metric reference attention item: %w", err)
	}
	return nil
}

func closeRemovedMetricReferenceAttention(ctx context.Context, tx *sql.Tx, recordID int64) error {
	_, err := tx.ExecContext(ctx, `
UPDATE kb.attention_items a
SET status = 'resolved', resolved_by = 'metric-reference-resolver', resolved_at = NOW(),
    resolution = 'metric citation removed', modify_time = NOW()
WHERE a.input_record_id = $1 AND a.status = 'open' AND a.details->>'family' = 'metric'
  AND NOT EXISTS (
    SELECT 1 FROM kb.ref_mentions m
    WHERE m.input_record_id = $1 AND NOT m.internal AND m.target_content IN ('criterion', 'criteria_set')
      AND COALESCE(NULLIF(m.mention_key, ''), m.ref_id) =
          COALESCE(a.details->>'mention_key', split_part(a.dedupe_key, ':', 3))
  )`, recordID)
	return err
}
