package docprocessing

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProcessor struct {
	name   string
	calls  *[]string
	retErr error
}

func (f fakeProcessor) Name() string { return f.name }

func (f fakeProcessor) HandleEvent(_ context.Context, _ []byte) error {
	if f.calls != nil {
		*f.calls = append(*f.calls, f.name)
	}
	return f.retErr
}

func TestControlService_UsesOperationOrder(t *testing.T) {
	got := make([]string, 0, 3)
	svc := &ControlService{
		Processors: []Processor{
			fakeProcessor{name: "chunking", calls: &got},
			fakeProcessor{name: "extract_doc_metadata", calls: &got},
			fakeProcessor{name: "extract_metrics", calls: &got},
		},
	}

	payload := []byte(`{"record_id":"1","operation":["extract_metrics","chunking"]}`)
	svc.HandleEvent(context.Background(), payload)

	want := []string{"extract_metrics", "chunking"}
	if len(got) != len(want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestControlService_DefaultsToConfiguredOrder(t *testing.T) {
	got := make([]string, 0, 3)
	svc := &ControlService{
		Processors: []Processor{
			fakeProcessor{name: "chunking", calls: &got},
			fakeProcessor{name: "extract_doc_metadata", calls: &got},
			fakeProcessor{name: "extract_metrics", calls: &got},
		},
	}

	svc.HandleEvent(context.Background(), []byte(`{"record_id":"1"}`))

	want := []string{"chunking", "extract_doc_metadata", "extract_metrics"}
	if len(got) != len(want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

type fakeEventStore struct {
	insertErr error
}

func (f fakeEventStore) InsertEvent(_ context.Context, _ EventRecord) error {
	return f.insertErr
}

func (f fakeEventStore) UpsertConsumedStatus(_ context.Context, _ string, _ time.Time, _ int64, _ error) error {
	return nil
}

func TestControlService_HandleJetStreamEvent_DoesNotFailWhenEventInsertFails(t *testing.T) {
	svc := &ControlService{
		EventStore: fakeEventStore{insertErr: errors.New(`pq: relation "kb.events" does not exist (42P01)`)},
	}
	err := svc.HandleJetStreamEvent(context.Background(), DefaultEventSubject, []byte(`{"record_id":"1"}`))
	if err != nil {
		t.Fatalf("HandleJetStreamEvent() error = %v, want nil", err)
	}
}
