package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/nats-io/nats.go"
)

type recoveryTestJS struct {
	messages  []*nats.RawStreamMsg
	pending   int
	bound     bool
	published [][]byte
	subjects  []string
}

func (j *recoveryTestJS) StreamNameBySubject(string, ...nats.JSOpt) (string, error) {
	return "events", nil
}
func (j *recoveryTestJS) ConsumerInfo(string, string, ...nats.JSOpt) (*nats.ConsumerInfo, error) {
	return &nats.ConsumerInfo{PushBound: j.bound, NumAckPending: j.pending}, nil
}
func (j *recoveryTestJS) StreamInfo(string, ...nats.JSOpt) (*nats.StreamInfo, error) {
	return &nats.StreamInfo{State: nats.StreamState{FirstSeq: 1, LastSeq: uint64(len(j.messages))}}, nil
}
func (j *recoveryTestJS) GetMsg(_ string, n uint64, _ ...nats.JSOpt) (*nats.RawStreamMsg, error) {
	if n == 0 || n > uint64(len(j.messages)) {
		return nil, nats.ErrMsgNotFound
	}
	return j.messages[n-1], nil
}
func (j *recoveryTestJS) Publish(s string, p []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	j.subjects = append(j.subjects, s)
	j.published = append(j.published, append([]byte(nil), p...))
	return &nats.PubAck{}, nil
}

func recoveryDB(t *testing.T, idle bool, candidates ...int64) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("SELECT pg_try_advisory_lock").WithArgs(processingLeaseKey).WillReturnRows(sqlmock.NewRows([]string{"acquired"}).AddRow(idle))
	if idle && candidates != nil {
		rows := sqlmock.NewRows([]string{"id"})
		for _, id := range candidates {
			rows.AddRow(id)
		}
		mock.ExpectQuery("SELECT id FROM kb.inputs WHERE parse_state='parsed_success' AND pipeline_state IN \\('pending','running'\\) AND processing_mode IN \\('auto','auto_offpeak'\\)").WithArgs(float64(120)).WillReturnRows(rows)
	}
	if idle {
		mock.ExpectExec("SELECT pg_advisory_unlock").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	return db, mock
}
func recoveryTestOptions() RecoveryOptions {
	return RecoveryOptions{Sources: []RecoverySource{{"kb.line-file-generated", "worker"}}}
}

func TestRecoverySkipsActiveWorkersAndUnfinishedDelivery(t *testing.T) {
	for _, tc := range []struct {
		name    string
		idle    bool
		pending int
		bound   bool
		want    string
	}{{"lease", false, 0, true, "workers active"}, {"delivery", true, 1, true, "delivery pending"}, {"unbound", true, 0, false, "processor subscription unavailable"}} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := recoveryDB(t, tc.idle)
			js := &recoveryTestJS{pending: tc.pending, bound: tc.bound}
			r, err := RecoverPendingEvents(context.Background(), db, js, recoveryTestOptions())
			if err != nil || r.Skipped != tc.want || len(js.published) != 0 {
				t.Fatalf("report=%+v err=%v", r, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecoveryPreservesOriginalExecutionAndOptions(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(filename, []byte("lines"), 0600); err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{"record_id": float64(400), "line_file_filename": filename, "user_id": "user-1", "llm_execution_key": "original-run", "doc_processors": []any{"extract_metrics"}, "pipeline_override": "custom", "force": false}
	payload, _ := json.Marshal(raw)
	db, mock := recoveryDB(t, true, 400)
	js := &recoveryTestJS{bound: true, messages: []*nats.RawStreamMsg{{Subject: "kb.line-file-generated", Data: payload, Time: time.Now().Add(-time.Hour)}}}
	r, err := RecoverPendingEvents(context.Background(), db, js, recoveryTestOptions())
	if err != nil || !reflect.DeepEqual(r.Published, []int64{400}) {
		t.Fatalf("report=%+v err=%v", r, err)
	}
	var got map[string]any
	if err := json.Unmarshal(js.published[0], &got); err != nil {
		t.Fatal(err)
	}
	delete(got, "doc_recovery")
	if !reflect.DeepEqual(got, raw) {
		t.Fatalf("changed original payload: got=%v want=%v", got, raw)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryCooldownUsesPersistedStreamMarker(t *testing.T) {
	db, mock := recoveryDB(t, true, 400)
	js := &recoveryTestJS{bound: true, messages: []*nats.RawStreamMsg{{Subject: "kb.line-file-generated", Data: []byte(`{"record_id":400,"doc_recovery":true}`), Time: time.Now()}}}
	r, err := RecoverPendingEvents(context.Background(), db, js, recoveryTestOptions())
	if err != nil || len(r.Published) != 0 || len(r.MissingEvents) != 0 {
		t.Fatalf("report=%+v err=%v", r, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryReportsMissingEventInsteadOfInventingExecution(t *testing.T) {
	db, mock := recoveryDB(t, true, 400)
	js := &recoveryTestJS{bound: true}
	r, err := RecoverPendingEvents(context.Background(), db, js, recoveryTestOptions())
	if err != nil || !reflect.DeepEqual(r.MissingEvents, []int64{400}) || len(js.published) != 0 {
		t.Fatalf("report=%+v err=%v", r, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentLeaseRefusesConcurrentProcessing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("SELECT pg_advisory_lock_shared").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT pg_try_advisory_lock").WithArgs("chenweb/doc-processing/record/400").WillReturnRows(sqlmock.NewRows([]string{"acquired"}).AddRow(false))
	mock.ExpectExec("SELECT pg_advisory_unlock_shared").WillReturnResult(sqlmock.NewResult(0, 1))
	release, acquired, err := acquireDocumentLease(context.Background(), db, 400)
	if err != nil || acquired || release != nil {
		t.Fatalf("acquired=%v err=%v", acquired, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSynchronousJetStreamHandlerDoesNotFinishBeforePipeline(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "input.txt")
	writeLineFile(t, filename)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var finished sync.WaitGroup
	finished.Add(1)
	p := &blockingConcurrencyProcessor{name: "probe", started: started, release: release, done: &finished}
	svc := &ControlService{Processors: []Processor{p}, InputStore: &fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: 1, ParserName: "opendata", ResultFilename: "result.json", StatusRaw: `[]`}}, MaxDocProcessPipelines: 1, Now: time.Now}
	result := make(chan error, 1)
	go func() {
		result <- svc.HandleJetStreamEventSync(context.Background(), DefaultEventSubject, []byte(`{"record_id":1,"user_id":"test-user","filename":"`+filename+`"}`))
	}()
	waitForProcessorStarts(t, started, 1)
	if svc.InFlightPipelines() != 1 {
		t.Fatal("active pipeline not reported")
	}
	select {
	case err := <-result:
		t.Fatalf("handler returned before work completed: %v", err)
	default:
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if svc.InFlightPipelines() != 0 {
		t.Fatal("pipeline remains active")
	}
}

func TestSynchronousJetStreamHandlerReturnsCancellation(t *testing.T) {
	svc := &ControlService{MaxDocProcessPipelines: 1}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := svc.HandleJetStreamEventSync(ctx, DefaultEventSubject, []byte(`{}`))
	// Cancellation must never turn a rejected dispatch into success (and ACK).
	if err == nil {
		t.Fatal("cancelled dispatch returned success")
	}
	if !errors.Is(err, context.Canceled) {
		t.Logf("dispatch rejected before work: %v", err)
	}
}

func TestRuntimeHealthRevalidatesOnlyLatchedFailures(t *testing.T) {
	calls := 0
	failure := errors.New("required configuration unavailable")
	svc := &ControlService{newRunProcessors: func() ([]Processor, Processor, error) { calls++; return nil, nil, failure }}
	if err := svc.CheckRuntimeHealth(); err != nil || calls != 0 {
		t.Fatalf("healthy worker unexpectedly rebuilt graph: calls=%d err=%v", calls, err)
	}
	svc.recordRuntimeFault(failure)
	if err := svc.CheckRuntimeHealth(); !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("fault lost: calls=%d err=%v", calls, err)
	}
	failure = nil
	if err := svc.CheckRuntimeHealth(); err != nil || calls != 2 {
		t.Fatalf("repaired graph did not clear fault: calls=%d err=%v", calls, err)
	}
	if err := svc.CheckRuntimeHealth(); err != nil || calls != 2 {
		t.Fatalf("healthy graph revalidated: calls=%d err=%v", calls, err)
	}
}

func TestRuntimeHealthDoesNotClearNewerConcurrentFailure(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	old := errors.New("old config problem")
	newer := errors.New("new config problem")
	svc := &ControlService{newRunProcessors: func() ([]Processor, Processor, error) { close(started); <-finish; return nil, nil, nil }}
	svc.recordRuntimeFault(old)
	result := make(chan error, 1)
	go func() { result <- svc.CheckRuntimeHealth() }()
	<-started
	svc.recordRuntimeFault(newer)
	close(finish)
	if err := <-result; !errors.Is(err, newer) {
		t.Fatalf("cleared newer fault: %v", err)
	}
}

func TestBusyDeliveryExhaustionLeavesLeaseOwnerStatusAlone(t *testing.T) {
	store := &fakeDocMetadataStore{rec: DocMetadataInputRecord{ID: 469, StatusRaw: "[]"}}
	alarms := &recordingAlarmWriter{}
	service := &ControlService{InputStore: store, RoutingAlarms: alarms}
	service.ReportBusyDeliveryExhausted(context.Background(), []byte(`{"record_id":469}`), 20)

	if len(alarms.alarms) != 1 || alarms.alarms[0].Kind != RoutingAlarmKindDocumentBusyMaxTries ||
		alarms.alarms[0].RecordID != 469 {
		t.Fatalf("expected a busy-delivery alarm for record 469, got %+v", alarms.alarms)
	}
	if updates := store.statusUpdates(); len(updates) != 0 {
		t.Fatalf("exhausted duplicate updated the lease owner's status: %+v", updates)
	}
}
