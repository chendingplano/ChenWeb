package fileconverters

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// Run against an isolated JetStream server, never a production stream:
// NATS_TEST_URL=nats://127.0.0.1:14222 go test -run TestJetStreamDurableDelivery .
func TestJetStreamDurableDelivery(t *testing.T) {
	url := os.Getenv("NATS_TEST_URL")
	if url == "" {
		t.Skip("set NATS_TEST_URL to an isolated JetStream server")
	}
	t.Setenv("DOC_PROCESSOR_ACK_WAIT_SEC", "1")
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	suffix := strings.ReplaceAll(nats.NewInbox(), ".", "-")
	stream := "health-test-" + suffix
	subject := "health-test." + suffix
	durable := "worker"
	if _, err := js.AddStream(&nats.StreamConfig{Name: stream, Subjects: []string{subject}}); err != nil {
		t.Fatal(err)
	}
	defer js.DeleteStream(stream)
	subscriber := &NATSSubscriber{conn: nc}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	exited := make(chan error, 1)
	var deliveries atomic.Int64
	go func() {
		exited <- subscriber.SubscribeConcurrent(ctx, subject, durable, 2, func(ctx context.Context, _ []byte) error {
			deliveries.Add(1)
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	waitForConsumer(t, js, stream, durable, func(i *nats.ConsumerInfo) bool { return i.PushBound })
	if _, err := js.Publish(subject, []byte(`{"record_id":400}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not start")
	}
	waitForConsumer(t, js, stream, durable, func(i *nats.ConsumerInfo) bool { return i.NumAckPending == 1 })
	// Exceed the configured ACK deadline. InProgress must prevent redelivery.
	time.Sleep(1500 * time.Millisecond)
	info, err := js.ConsumerInfo(stream, durable)
	if err != nil {
		t.Fatal(err)
	}
	if info.NumAckPending != 1 || info.NumRedelivered != 0 || deliveries.Load() != 1 {
		t.Fatalf("premature ACK or duplicate delivery: info=%+v deliveries=%d", info, deliveries.Load())
	}
	close(release)
	waitForConsumer(t, js, stream, durable, func(i *nats.ConsumerInfo) bool { return i.NumAckPending == 0 })
	cancel()
	select {
	case err := <-exited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subscription did not stop")
	}
	waitForConsumer(t, js, stream, durable, func(i *nats.ConsumerInfo) bool { return !i.PushBound })
	// This event is published while the process is stopped. Its durable must
	// still exist, and a restarted subscription must receive it.
	if _, err := js.Publish(subject, []byte(`{"record_id":401}`)); err != nil {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	received := make(chan struct{}, 1)
	exited2 := make(chan error, 1)
	go func() {
		exited2 <- subscriber.Subscribe(ctx2, subject, durable, func(context.Context, []byte) error { received <- struct{}{}; return nil })
	}()
	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("durable lost event during restart")
	}
	cancel2()
	select {
	case err := <-exited2:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second subscription did not stop")
	}
}

func waitForConsumer(t *testing.T, js nats.JetStreamContext, stream, durable string, condition func(*nats.ConsumerInfo) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		info, err := js.ConsumerInfo(stream, durable)
		if err == nil && condition(info) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	info, err := js.ConsumerInfo(stream, durable)
	t.Fatalf("consumer did not reach expected state: info=%+v err=%v", info, err)
}
