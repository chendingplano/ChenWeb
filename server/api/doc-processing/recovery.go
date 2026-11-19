package docprocessing

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/llmrun"
	"github.com/nats-io/nats.go"
)

var ErrDocumentBusy = errors.New("document has an active processing lease")

// All processing shares this lock; recovery takes it exclusively to establish
// idleness across instances, including routing, slot release and off-peak waits.
const processingLeaseKey = "chenweb/doc-processing/active"

func releaseLease(conn *sql.Conn, queries ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, query := range queries {
		if _, err := conn.ExecContext(ctx, query); err != nil {
			// A pooled connection must never retain an advisory lock after cleanup fails.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			break
		}
	}
	_ = conn.Close()
}

func acquireDocumentLease(ctx context.Context, db *sql.DB, recordID int64, cancelOnLoss ...context.CancelFunc) (func(), bool, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	global := fmt.Sprintf("SELECT pg_advisory_lock_shared(hashtextextended('%s',0))", processingLeaseKey)
	if _, err = conn.ExecContext(ctx, global); err != nil {
		_ = conn.Close()
		return nil, false, err
	}
	unlockGlobal := fmt.Sprintf("SELECT pg_advisory_unlock_shared(hashtextextended('%s',0))", processingLeaseKey)
	key := fmt.Sprintf("chenweb/doc-processing/record/%d", recordID)
	var acquired bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", key).Scan(&acquired); err != nil || !acquired {
		releaseLease(conn, unlockGlobal)
		return nil, false, err
	}
	unlockRecord := fmt.Sprintf("SELECT pg_advisory_unlock(hashtextextended('%s',0))", key)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		if len(cancelOnLoss) == 0 {
			return
		}
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err := conn.PingContext(pingCtx)
				cancel()
				if err != nil {
					cancelOnLoss[0]()
					return
				}
			}
		}
	}()
	return func() { close(stop); <-done; releaseLease(conn, unlockRecord, unlockGlobal) }, true, nil
}

type RecoveryReport struct {
	Candidates    int     `json:"candidates"`
	Published     []int64 `json:"published"`
	MissingEvents []int64 `json:"missing_events"`
	Skipped       string  `json:"skipped,omitempty"`
}

type RecoverySource struct {
	Subject string
	Durable string
}

type RecoveryJetStream interface {
	StreamNameBySubject(string, ...nats.JSOpt) (string, error)
	ConsumerInfo(string, string, ...nats.JSOpt) (*nats.ConsumerInfo, error)
	StreamInfo(string, ...nats.JSOpt) (*nats.StreamInfo, error)
	GetMsg(string, uint64, ...nats.JSOpt) (*nats.RawStreamMsg, error)
	Publish(string, []byte, ...nats.PubOpt) (*nats.PubAck, error)
}

type RecoveryOptions struct {
	Sources  []RecoverySource
	Batch    int
	Grace    time.Duration
	Cooldown time.Duration
	MaxScan  uint64
}

// RecoverPendingEvents regenerates retained original events only when no worker
// holds a processing lease and both consumers have no queued/unacked delivery.
// New messages retain the original execution/user/options and carry a recovery
// marker so the worker rechecks eligibility under its per-document lease.
func RecoverPendingEvents(ctx context.Context, db *sql.DB, js RecoveryJetStream, opts RecoveryOptions) (RecoveryReport, error) {
	report := RecoveryReport{}
	if opts.Batch <= 0 {
		opts.Batch = 5
	}
	if opts.Grace <= 0 {
		opts.Grace = 2 * time.Minute
	}
	if opts.Cooldown <= 0 {
		opts.Cooldown = 5 * time.Minute
	}
	if opts.MaxScan == 0 {
		opts.MaxScan = 10000
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return report, err
	}
	var idle bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", processingLeaseKey).Scan(&idle); err != nil {
		_ = conn.Close()
		return report, err
	}
	if !idle {
		_ = conn.Close()
		report.Skipped = "workers active"
		return report, nil
	}
	defer releaseLease(conn, fmt.Sprintf("SELECT pg_advisory_unlock(hashtextextended('%s',0))", processingLeaseKey))

	sources := opts.Sources
	if len(sources) == 0 {
		sources = []RecoverySource{{"kb.line-file-generated", "doc-processor-line-file-generated"}, {DefaultEventSubject, "doc-processor"}}
	}

	type source struct {
		stream, subject string
		last, first     uint64
	}
	streams := []source{}
	for _, s := range sources {
		stream, err := js.StreamNameBySubject(s.Subject, nats.Context(ctx))
		if err != nil {
			return report, err
		}
		info, err := js.ConsumerInfo(stream, s.Durable, nats.Context(ctx))
		if err != nil {
			return report, fmt.Errorf("recovery requires consumer %s: %w", s.Durable, err)
		}
		if !info.PushBound {
			report.Skipped = "processor subscription unavailable"
			return report, nil
		}
		if info.NumAckPending > 0 || info.NumPending > 0 {
			report.Skipped = "delivery pending"
			return report, nil
		}
		si, err := js.StreamInfo(stream, nats.Context(ctx))
		if err != nil {
			return report, err
		}
		streams = append(streams, source{stream, s.Subject, si.State.LastSeq, si.State.FirstSeq})
	}
	rows, err := conn.QueryContext(ctx, `SELECT id FROM kb.inputs WHERE parse_state='parsed_success' AND pipeline_state IN ('pending','running') AND processing_mode IN ('auto','auto_offpeak') AND modify_time < now()-($1 * interval '1 second') ORDER BY id LIMIT 1000`, opts.Grace.Seconds())
	if err != nil {
		return report, err
	}
	candidates := map[int64]bool{}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return report, err
		}
		candidates[id] = true
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return report, err
	}
	report.Candidates = len(ids)
	if len(ids) == 0 {
		return report, nil
	}
	type original struct {
		subject        string
		payload        []byte
		at             time.Time
		recentRecovery bool
	}
	originals := map[int64]original{}
	for _, s := range streams {
		var scanned uint64
		for seq := s.last; seq >= s.first && seq > 0 && scanned < opts.MaxScan; seq-- {
			scanned++
			msg, err := js.GetMsg(s.stream, seq, nats.Context(ctx))
			if errors.Is(err, nats.ErrMsgNotFound) {
				continue
			}
			if err != nil {
				return report, err
			}
			if msg.Subject != s.subject {
				continue
			}
			evt, err := ParseLineFileGeneratedEvent(msg.Data)
			if err != nil || !candidates[evt.RecordID] {
				continue
			}
			var marker struct {
				Recovery bool `json:"doc_recovery"`
			}
			_ = json.Unmarshal(msg.Data, &marker)
			previous, exists := originals[evt.RecordID]
			if exists && !msg.Time.After(previous.at) {
				continue
			}
			payload := llmrun.QueuePayload(msg.Data, s.stream, seq)
			originals[evt.RecordID] = original{s.subject, payload, msg.Time, marker.Recovery && time.Since(msg.Time) < opts.Cooldown}
		}
	}
	for _, id := range ids {
		original, ok := originals[id]
		if !ok {
			report.MissingEvents = append(report.MissingEvents, id)
			continue
		}
		if original.recentRecovery {
			continue
		}
		evt, err := ParseLineFileGeneratedEvent(original.payload)
		if err != nil {
			return report, err
		}
		if strings.TrimSpace(evt.UserID) == "" || strings.TrimSpace(evt.Filename) == "" {
			report.MissingEvents = append(report.MissingEvents, id)
			continue
		}
		if _, err = os.Stat(evt.Filename); err != nil {
			report.MissingEvents = append(report.MissingEvents, id)
			continue
		}
		payload, err := recoveryPayload(original.payload)
		if err != nil {
			return report, err
		}
		// JetStream publish acknowledgement confirms persistence before releasing
		// the global lock; the next sweep sees the cooldown marker in the stream.
		if _, err = js.Publish(original.subject, payload, nats.Context(ctx)); err != nil {
			return report, err
		}
		report.Published = append(report.Published, id)
		if len(report.Published) >= opts.Batch {
			break
		}
	}
	return report, nil
}

func recoveryPayload(payload []byte) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}
	raw["doc_recovery"] = json.RawMessage("true")
	return json.Marshal(raw)
}

// ReportBusyDeliveryExhausted records a delivery that never acquired the lease.
// The lease owner remains responsible for the document's processing status.
func (s *ControlService) ReportBusyDeliveryExhausted(ctx context.Context, payload []byte, tries uint64) {
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil || evt.RecordID <= 0 {
		if s.Logger != nil {
			s.Logger.Error("(MID_26111801) busy-document retries exhausted but event has no record id (ChenWeb#42)", "tries", tries, "error", err)
		}
		return
	}
	msg := fmt.Sprintf("record_id=%d: processing lease still held after %d delivery attempts; acknowledging busy delivery", evt.RecordID, tries)
	if s.Logger != nil {
		s.Logger.Info("(MID_26111802) "+msg+" (ChenWeb#42)", "record_id", evt.RecordID, "tries", tries)
	}
	s.raiseRoutingAlarms(ctx, []RoutingAlarm{{
		Kind:     RoutingAlarmKindDocumentBusyMaxTries,
		Severity: RoutingAlarmSeverityError,
		Message:  msg,
		RecordID: evt.RecordID,
	}})
}
