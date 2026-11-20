package docsync

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/chendingplano/deepdoc/server/api/docsync/transfer"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/idmanager"
)

type Runner struct {
	DB           *sql.DB
	IDs          *idmanager.Manager
	Logger       ApiTypes.JimoLogger
	SSH          transfer.SSHConfig
	Site         string
	RemotePhone  string
	Password     string
	StoreID      int64
	AdminURL     string
	ArtifactRoot string
	Inbox        string
	Window       Window
	MaxBytes     int64
	SegmentBytes int64
}

type pendingTransfer struct {
	Request  transfer.PackRequest
	Imported bool
}

const runSequence = "document_sync_run"

// Run is the common entry point for scheduled and manual runs.
func (r *Runner) Run(ctx context.Context) (returned error) {
	if r.DB == nil || r.IDs == nil || r.Logger == nil || r.Site == "" || r.StoreID <= 0 ||
		r.RemotePhone == "" || r.Password == "" || !filepath.IsAbs(r.Inbox) ||
		!filepath.IsAbs(r.ArtifactRoot) || r.SegmentBytes <= 0 || r.MaxBytes < r.SegmentBytes ||
		r.MaxBytes > math.MaxInt64/3 {
		return errors.New("invalid document sync configuration")
	}
	_, deadline := r.Window.bounds(time.Now())
	conn, err := r.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	const lock = "chenweb/document-sync/importer"
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1,0))`, lock); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return err
	}
	defer func() {
		unlock, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(unlock, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, lock); err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	if err := r.IDs.Provision(ctx, idmanager.Sequence{Name: runSequence, Desc: "Document sync runs"}); err != nil {
		return err
	}
	runID, err := r.IDs.Next(ctx, runSequence)
	if err != nil {
		return err
	}
	if err := r.event(ctx, runID, "run_started", workItem{}, 0, map[string]any{"window_end": deadline}); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		outcome := "run_finished"
		var detail map[string]any
		if returned != nil {
			outcome = "run_failed"
			detail = map[string]any{"error": returned.Error()}
			r.Logger.Error("document sync run failed", "run_id", runID, "error", returned)
		}
		returned = errors.Join(returned, r.event(cleanup, runID, outcome, workItem{}, 0, detail))
	}()
	if _, err := localRelativePath(r.ArtifactRoot); err != nil {
		return err
	}
	if err := os.MkdirAll(r.Inbox, 0o700); err != nil {
		return err
	}
	pending, err := r.pending(ctx)
	if err != nil {
		return err
	}
	outside := pending.Request.RunID == 0 && !time.Now().Before(deadline)
	var owner string
	if !outside {
		owner, err = resolveOwner(ctx, r.AdminURL, r.RemotePhone)
		if err != nil {
			return err
		}
		var active bool
		if err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM kb.knowledge_store WHERE id=$1 AND status='active')`,
			r.StoreID).Scan(&active); err != nil {
			return err
		}
		if !active {
			return errors.New("configured knowledge store is missing or inactive")
		}
	}
	if err := recoverFiles(ctx, r.DB, r.Inbox); err != nil {
		return err
	}
	if _, err := r.DB.ExecContext(ctx, `DELETE FROM kb.doc_sync_marks`); err != nil {
		return err
	}
	if outside {
		return r.event(ctx, runID, "outside_sync_window", workItem{}, 0, nil)
	}
	client, err := transfer.OpenSSH(r.SSH)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.Login(ctx, r.RemotePhone, r.Password); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := client.Logout(cleanup); err != nil {
			r.Logger.Warn("document sync remote logout failed", "run_id", runID, "error", err)
		}
	}()
	if err := r.event(ctx, runID, "remote_login", workItem{}, 0, map[string]any{"account": r.RemotePhone}); err != nil {
		return err
	}
	tried := make(map[workKey]bool)
	if pending.Request.RunID != 0 {
		if err := r.sync(ctx, client, runID, owner, pending, tried); err != nil {
			return err
		}
		if err := retrySSH(ctx, func() error { return client.Ack(ctx, pending.Request.RunID, "") }); err != nil {
			return err
		}
	}
	if !time.Now().Before(deadline) {
		return nil
	}
	if err := r.plan(ctx, client, runID); err != nil {
		return err
	}
	started := false
	for number := 1; time.Now().Before(deadline); number++ {
		work, err := r.queued(ctx, tried)
		if err != nil {
			return err
		}
		if len(work) == 0 {
			break
		}
		raw, err := json.Marshal(work)
		if err != nil {
			return err
		}
		if !time.Now().Before(deadline) {
			break
		}
		request := transfer.PackRequest{RunID: runID, TarNumber: number, MaxBytes: r.MaxBytes, SegmentBytes: r.SegmentBytes, Work: raw}
		encoded, err := json.Marshal(request)
		if err != nil {
			return err
		}
		if _, err := r.DB.ExecContext(ctx, `INSERT INTO kb.doc_sync_transfers(site,run_id,tar_number,request)
			VALUES($1,$2,$3,$4)`, r.Site, runID, number, string(encoded)); err != nil {
			return err
		}
		started = true
		if err := r.sync(ctx, client, runID, owner, pendingTransfer{Request: request}, tried); err != nil {
			return err
		}
	}
	if started {
		if err := retrySSH(ctx, func() error { return client.Ack(ctx, runID, "") }); err != nil {
			return err
		}
	}
	return r.rebuild(ctx, runID)
}

func (r *Runner) pending(ctx context.Context) (pendingTransfer, error) {
	var pending pendingTransfer
	var raw []byte
	err := r.DB.QueryRowContext(ctx, `SELECT request,imported FROM kb.doc_sync_transfers WHERE site=$1`,
		r.Site).Scan(&raw, &pending.Imported)
	if errors.Is(err, sql.ErrNoRows) {
		return pending, nil
	}
	if err != nil {
		return pending, err
	}
	return pending, json.Unmarshal(raw, &pending.Request)
}

func (r *Runner) sync(ctx context.Context, client *transfer.Client, runID int64, owner string,
	pending pendingTransfer, attempted map[workKey]bool) error {
	req := pending.Request
	name := fmt.Sprintf("%d-%04d.tar.gz", req.RunID, req.TarNumber)
	local := filepath.Join(r.Inbox, name)
	staging := local + ".staging"
	if !pending.Imported {
		index, err := packAndWait(ctx, client, req)
		if err != nil {
			return err
		}
		included, deferred, err := matchingWork(index, req)
		if err != nil {
			return err
		}
		if err := r.event(ctx, runID, "tar_packed", workItem{}, 0, map[string]any{"name": name, "bytes": index.Size, "included": len(included), "deferred": len(deferred)}); err != nil {
			return err
		}
		if err := client.Download(ctx, index, local); err != nil {
			return err
		}
		if err := r.event(ctx, runID, "tar_fetched", workItem{}, 0, map[string]any{"name": name, "bytes": index.Size}); err != nil {
			return err
		}
		if err := os.RemoveAll(staging); err != nil {
			return err
		}
		if err := os.Mkdir(staging, 0o700); err != nil {
			return err
		}
		m, err := unpackArchive(local, staging, req.RunID, req.TarNumber, included)
		if err != nil {
			return err
		}
		for _, item := range deferred {
			attempted[item.key()] = true
			if err := r.finishWork(ctx, item, errors.New(item.Issue)); err != nil {
				return err
			}
			if err := r.event(ctx, runID, "export_deferred", item, 0, map[string]any{"reason": item.Issue}); err != nil {
				return err
			}
		}
		if err := r.importItems(ctx, runID, owner, staging, m.Items, attempted); err != nil {
			return err
		}
		if err := r.event(ctx, runID, "tar_imported", workItem{}, 0, map[string]any{"name": name, "items": len(m.Items)}); err != nil {
			return err
		}
		if _, err := r.DB.ExecContext(ctx, `UPDATE kb.doc_sync_transfers SET imported=true WHERE site=$1`, r.Site); err != nil {
			return err
		}
	}
	if err := retrySSH(ctx, func() error { return client.Ack(ctx, req.RunID, name) }); err != nil {
		return err
	}
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := ignoreMissing(os.Remove(local)); err != nil {
		return err
	}
	_, err := r.DB.ExecContext(ctx, `DELETE FROM kb.doc_sync_transfers WHERE site=$1`, r.Site)
	return err
}

func matchingWork(index transfer.Index, req transfer.PackRequest) (included, deferred []workItem, err error) {
	var requested []workItem
	if err = json.Unmarshal(req.Work, &requested); err != nil {
		return
	}
	if err = json.Unmarshal(index.Included, &included); err != nil {
		return
	}
	if err = json.Unmarshal(index.Deferred, &deferred); err != nil {
		return
	}
	valid := make(map[workKey]workItem)
	for _, item := range requested {
		valid[item.key()] = item
	}
	for _, item := range append(included, deferred...) {
		orig, found := valid[item.key()]
		if !found || !sameWork(orig, item) {
			return nil, nil, errors.New("exporter returned an unrequested item")
		}
		delete(valid, item.key())
	}
	if len(included)+len(deferred) == 0 {
		return nil, nil, errors.New("exporter made no progress packing requested work")
	}
	return
}

func packAndWait(ctx context.Context, client *transfer.Client, req transfer.PackRequest) (transfer.Index, error) {
	started := false
	for {
		var index transfer.Index
		err := retrySSH(ctx, func() error {
			var err error
			index, err = client.Status(ctx, req.RunID)
			return err
		})
		if err != nil {
			return index, err
		}
		switch index.State {
		case transfer.StateFailed:
			if started {
				return index, fmt.Errorf("production exporter could not pack archive: %s", index.Error)
			}
			fallthrough
		case transfer.StateMissing:
			if err := retrySSH(ctx, func() error { return client.Pack(ctx, req) }); err != nil {
				return index, err
			}
			started = true
		case transfer.StateReady:
			if index.TarNumber != req.TarNumber {
				return index, errors.New("exporter tar number does not match current transfer")
			}
			return index, nil
		case transfer.StatePacking:
			started = true
		default:
			return index, errors.New("unknown exporter pack state")
		}
		if err := wait(ctx, 5*time.Second); err != nil {
			return index, err
		}
	}
}

func retrySSH(ctx context.Context, action func() error) error {
	for backoff := 30 * time.Second; ; backoff = min(backoff*2, 10*time.Minute) {
		err := action()
		if !errors.Is(err, transfer.ErrSSHConnection) {
			return err
		}
		if err := wait(ctx, backoff); err != nil {
			return err
		}
	}
}

func (r *Runner) importItems(ctx context.Context, runID int64, owner, staging string, items []manifestItem, tried map[workKey]bool) error {
	var retry []manifestItem
	for pass := 0; pass < 2; pass++ {
		for _, item := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			work := item.Work
			tried[work.key()] = true
			applied, err := r.apply(ctx, owner, staging, item)
			if err != nil {
				retry = append(retry, item)
			}
			if err := r.finishWork(ctx, work, err); err != nil {
				return err
			}
			event := "item_applied"
			detail := map[string]any{"rows": applied.Rows, "files": applied.Files, "removed_processors": applied.Removed,
				"inserted": applied.Inserted, "converted": applied.Converted, "conflicts": applied.Conflicts}
			if applied.Skipped {
				event = "item_already_applied"
			}
			if err != nil {
				event = "item_deferred"
				detail["reason"] = err.Error()
				if errors.Is(err, errProcessing) {
					detail["stop_requested"] = true
				}
			}
			if err := r.event(ctx, runID, event, work, applied.InputID, detail); err != nil {
				return err
			}
		}
		items, retry = retry, nil
	}
	return nil
}

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (r *Runner) event(ctx context.Context, runID int64, name string, item workItem, localID int64, detail map[string]any) error {
	return r.eventWith(ctx, r.DB, runID, name, item, localID, detail)
}

func (r *Runner) eventWith(ctx context.Context, exec executor, runID int64, name string, item workItem, localID int64, detail map[string]any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `INSERT INTO kb.doc_sync_events(site,run_id,event,source_id,input_id,processor,detail)
		VALUES($1,$2,$3,NULLIF($4,0),NULLIF($5,0),NULLIF($6,''),$7::jsonb)`,
		r.Site, runID, name, item.SourceID, localID, item.Kind, string(raw))
	if err != nil {
		return fmt.Errorf("record document sync %s: %w", name, err)
	}
	r.Logger.Info("document sync", "run_id", runID, "event", name, "source_id", item.SourceID, "local_id", localID, "processor", item.Kind)
	return nil
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func SiteKey(config transfer.SSHConfig) string {
	// Stable destination identifier for the checkpoint and mapping.
	return config.User + "@" + net.JoinHostPort(config.Host, fmt.Sprint(config.Port))
}
