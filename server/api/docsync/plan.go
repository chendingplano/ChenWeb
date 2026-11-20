package docsync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/chendingplano/deepdoc/server/api/docsync/transfer"
	"github.com/chendingplano/deepdoc/server/api/pathutil"
	"github.com/lib/pq"
)

func (r *Runner) checkpoint(ctx context.Context, key string) (time.Time, error) {
	var val sql.NullTime
	err := r.DB.QueryRowContext(ctx, `SELECT last_cursor FROM kb.data_sync_state WHERE sync_item_id=$1`,
		"document-sync/"+r.Site+"/"+key).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	return val.Time, err
}

func (r *Runner) saveCheckpoint(ctx context.Context, key string, until time.Time) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO kb.data_sync_state(sync_item_id,last_cursor,last_synced_at)
		VALUES($1,$2,now()) ON CONFLICT(sync_item_id) DO UPDATE SET
		last_cursor=excluded.last_cursor,last_synced_at=now(),last_error=NULL`, "document-sync/"+r.Site+"/"+key, until)
	return err
}

func fetchPage(ctx context.Context, client *transfer.Client, request listRequest) ([]listing, error) {
	var buffer bytes.Buffer
	if err := client.List(ctx, request, &buffer); err != nil {
		return nil, err
	}
	var page []listing
	dec := json.NewDecoder(&buffer)
	for {
		var line listing
		err := dec.Decode(&line)
		if errors.Is(err, io.EOF) {
			return page, nil
		}
		if err != nil {
			return nil, err
		}
		page = append(page, line)
	}
}

func (r *Runner) plan(ctx context.Context, client *transfer.Client, runID int64) error {
	page, err := fetchPage(ctx, client, listRequest{})
	if err != nil {
		return err
	}
	if len(page) != 1 || page[0].Until.IsZero() {
		return errors.New("exporter did not supply a source clock")
	}
	until := page[0].Until
	checkpoint, err := r.checkpoint(ctx, "inputs")
	if err != nil {
		return err
	}
	weekly, err := r.checkpoint(ctx, "reconcile")
	if err != nil {
		return err
	}
	full := weekly.IsZero() || until.Sub(weekly) >= 7*24*time.Hour
	start := overlapping(checkpoint)
	if err := r.event(ctx, runID, "window_started", workItem{}, 0, map[string]any{"after": start, "until": until, "reconciliation": full}); err != nil {
		return err
	}
	vocabulary, err := r.checkpoint(ctx, "vocabulary")
	if err != nil {
		return err
	}
	var vocabRetry bool
	if err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM kb.doc_sync_queue WHERE site=$1 AND source_id=0 AND last_error IS NOT NULL)`,
		r.Site).Scan(&vocabRetry); err != nil {
		return err
	}
	vocab, err := fetchPage(ctx, client, listRequest{Until: until, Vocabulary: true, All: full || vocabRetry,
		After: cursor{Time: overlapping(vocabulary)}})
	if err != nil {
		return err
	}
	for _, line := range vocab {
		if err := r.observe(ctx, runID, line); err != nil {
			return err
		}
	}
	if err := r.saveCheckpoint(ctx, "vocabulary", until); err != nil {
		return err
	}
	req := listRequest{After: cursor{Time: start}, Until: until, All: full}
	for {
		page, err := fetchPage(ctx, client, req)
		if err != nil {
			return err
		}
		for _, line := range page {
			if err := r.observe(ctx, runID, line); err != nil {
				return err
			}
			if line.Cursor != nil {
				req.After = *line.Cursor
			}
		}
		if len(page) < pageSize {
			break
		}
		if page[len(page)-1].Cursor == nil {
			return errors.New("exporter page has no continuation cursor")
		}
	}
	if err := r.refreshDeferred(ctx, client, runID, until); err != nil {
		return err
	}
	if full {
		if err := r.saveCheckpoint(ctx, "reconcile", until); err != nil {
			return err
		}
	}
	if err := r.saveCheckpoint(ctx, "inputs", until); err != nil {
		return err
	}
	return r.event(ctx, runID, "window_queued", workItem{}, 0, map[string]any{"until": until})
}

func overlapping(checkpoint time.Time) time.Time {
	if checkpoint.IsZero() {
		return checkpoint
	}
	return checkpoint.Add(-checkpointOverlap)
}

func (r *Runner) refreshDeferred(ctx context.Context, client *transfer.Client, runID int64, upper time.Time) error {
	rows, err := r.DB.QueryContext(ctx, `SELECT DISTINCT source_id FROM kb.doc_sync_queue
		WHERE site=$1 AND source_id<>0 AND last_error IS NOT NULL AND (item->>'observed_at')::timestamptz<$2 ORDER BY source_id`,
		r.Site, upper)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for group := range slices.Chunk(ids, pageSize) {
		page, err := fetchPage(ctx, client, listRequest{Until: upper, IDs: group})
		if err != nil {
			return err
		}
		for _, line := range page {
			if err := r.observe(ctx, runID, line); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Runner) observe(ctx context.Context, runID int64, line listing) error {
	if line.Issue != "" {
		if line.Cursor == nil {
			return errors.New("source issue lacks input ID")
		}
		work := workItem{SourceID: line.Cursor.ID, Kind: documentItem, Hash: line.Hash}
		if _, err := r.DB.ExecContext(ctx, `DELETE FROM kb.doc_sync_queue WHERE site=$1 AND source_id=$2`, r.Site, line.Cursor.ID); err != nil {
			return err
		}
		return r.event(ctx, runID, "source_document_skipped", work, 0, map[string]any{"reason": line.Issue})
	}
	if len(line.Items) == 0 {
		return nil
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var kinds []string
	changed := make(map[string]bool)
	outputChanged := make(map[string]bool)
	if line.Cursor != nil {
		var mapped bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM kb.doc_sync_inputs WHERE site=$1 AND source_id=$2 AND md5=$3)`,
			r.Site, line.Cursor.ID, line.Hash).Scan(&mapped); err != nil {
			return err
		}
		if !mapped {
			if _, err := tx.ExecContext(ctx, `DELETE FROM kb.doc_sync_applied WHERE site=$1 AND source_id=$2`, r.Site, line.Cursor.ID); err != nil {
				return err
			}
		}
	}
	for _, item := range line.Items {
		kinds = append(kinds, item.Kind)
		applied, err := r.version(ctx, tx, item.SourceID, item.Kind)
		if err != nil {
			return err
		}
		if !applied.same(item) {
			changed[item.Kind] = true
		}
		outputChanged[item.Kind] = applied.Digest != item.Digest || item.Issue != ""
		for dep, expected := range item.Requires {
			if expected == "*" {
				continue
			}
			version, err := r.version(ctx, tx, item.SourceID, dep)
			if err != nil {
				return err
			}
			if version.Digest != expected {
				changed[item.Kind] = true
			}
		}
	}
	// A dependency can invalidate a child whose portable payload is identical.
	// Include all descendants so their references are remapped after replacement.
	for range len(line.Items) {
		propagated := false
		for _, item := range line.Items {
			if outputChanged[item.Kind] {
				continue
			}
			for dep, expected := range item.Requires {
				if expected != "*" && outputChanged[dep] {
					changed[item.Kind] = true
					outputChanged[item.Kind] = true
					propagated = true
					break
				}
			}
		}
		if !propagated {
			break
		}
	}

	for _, item := range line.Items {
		if !changed[item.Kind] {
			if _, err := tx.ExecContext(ctx, `DELETE FROM kb.doc_sync_queue WHERE site=$1 AND source_id=$2 AND processor=$3`, r.Site, item.SourceID, item.Kind); err != nil {
				return err
			}
			continue
		}
		if item.Issue != "" {
			if err := r.eventWith(ctx, tx, runID, "item_waiting_for_review", item, 0, map[string]any{"reason": item.Issue}); err != nil {
				return err
			}
		}
		encoded, err := json.Marshal(item)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO kb.doc_sync_queue(site,source_id,processor,item,last_error)
			VALUES($1,$2,$3,$4,NULLIF($5,'')) ON CONFLICT(site,source_id,processor) DO UPDATE SET
			item=excluded.item,last_error=excluded.last_error`, r.Site, item.SourceID, item.Kind, string(encoded), item.Issue)
		if err != nil {
			return err
		}
	}
	if line.Cursor != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM kb.doc_sync_queue WHERE site=$1 AND source_id=$2 AND NOT (processor=ANY($3))`,
			r.Site, line.Cursor.ID, pq.Array(kinds)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Runner) queued(ctx context.Context, tried map[workKey]bool) ([]workItem, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT item FROM kb.doc_sync_queue WHERE site=$1
		AND coalesce(item->>'issue','')='' ORDER BY id`, r.Site)
	if err != nil {
		return nil, err
	}
	var work []workItem
	priority := make(map[int64]int)
	for rows.Next() {
		var raw []byte
		var item workItem
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			rows.Close()
			return nil, err
		}
		if !tried[item.key()] {
			if _, exists := priority[item.SourceID]; !exists {
				priority[item.SourceID] = len(priority) + 1
			}
			work = append(work, item)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	order := make(map[string]int)
	order[VocabularyProcessor], order[documentItem] = 0, 1
	for i, kind := range AllContracts() {
		if kind != VocabularyProcessor {
			order[kind] = i + 2
		}
	}
	order[pipelineItem] = len(order) + 2
	priority[0] = 0 // Vocabulary comes before any document that refers to it.
	slices.SortStableFunc(work, func(a, b workItem) int {
		if a.SourceID != b.SourceID {
			return priority[a.SourceID] - priority[b.SourceID]
		}
		return order[a.Kind] - order[b.Kind]
	})
	const maxWorkItems = 2500
	if len(work) > maxWorkItems {
		work = work[:maxWorkItems]
	}
	for i := range work {
		item := &work[i]
		item.Held, err = r.heldFiles(ctx, *item)
		if err != nil {
			return nil, err
		}
	}
	return work, nil
}

type workKey struct {
	id   int64
	kind string
}

func (w workItem) key() workKey { return workKey{w.SourceID, w.Kind} }

func (r *Runner) heldFiles(ctx context.Context, item workItem) (map[string]string, error) {
	version, err := r.version(ctx, r.DB, item.SourceID, item.Kind)
	if err != nil {
		return nil, err
	}
	held := make(map[string]string)
	for name, old := range version.Files {
		if artifactNeedsIDRewrite(item.Kind, name) {
			continue
		}
		f, err := describeFile(name, old.Path, "")
		if err == nil && f.SHA256 == old.SHA256 {
			held[name] = f.SHA256
		}
	}
	if item.Kind == documentItem && held["original"] == "" {
		d, err := r.resolveInput(ctx, r.DB, item)
		if err == nil && d.ID != 0 {
			f, err := describeFile("original", pathutil.ResolveDataHomePath(d.Original), item.Hash)
			if err == nil {
				held["original"] = f.SHA256
			}
		}
	}
	if len(held) == 0 {
		return nil, nil
	}
	return held, nil
}

func (r *Runner) finishWork(ctx context.Context, item workItem, result error) error {
	if result == nil {
		_, err := r.DB.ExecContext(ctx, `DELETE FROM kb.doc_sync_queue WHERE site=$1 AND source_id=$2 AND processor=$3
			AND item->>'digest'=$4 AND (item->>'observed_at')::timestamptz=$5`, r.Site, item.SourceID, item.Kind, item.Digest, item.SeenAt)
		return err
	}
	_, err := r.DB.ExecContext(ctx, `UPDATE kb.doc_sync_queue SET last_error=$4 WHERE site=$1 AND source_id=$2 AND processor=$3`,
		r.Site, item.SourceID, item.Kind, fmt.Sprint(result))
	return err
}
