package docreviews

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/nats-io/nats.go"
)

const DefaultDocReviewEventSubject = "kb.doc-review.start"
const defaultDocReviewStreamName = "doc-review-events"

var jsLogger = loggerutil.CreateDefaultLogger("DR16")

// docReviewEvent is the JetStream payload published when a review is accepted.
type docReviewEvent struct {
	RequestID int64 `json:"request_id"`
}

// ReviewPublisher publishes doc-review start events to JetStream.
type ReviewPublisher struct {
	conn    *nats.Conn
	js      nats.JetStreamContext
	subject string
}

var globalPublisher *ReviewPublisher

// InitPublisher initialises the package-level JetStream publisher.
// Called once at server startup; safe to call multiple times (idempotent).
func InitPublisher(natsURL string) error {
	natsURL = strings.TrimSpace(natsURL)
	if natsURL == "" {
		natsURL = "nats://127.0.0.1:4222"
	}

	opts := []nats.Option{}
	if tok := strings.TrimSpace(os.Getenv("NATS_TOKEN")); tok != "" {
		opts = append(opts, nats.Token(tok))
	} else {
		user := strings.TrimSpace(os.Getenv("NATS_USER"))
		pass := strings.TrimSpace(os.Getenv("NATS_PASS"))
		if pass == "" {
			pass = strings.TrimSpace(os.Getenv("NATS_PASSWORD"))
		}
		if user != "" || pass != "" {
			opts = append(opts, nats.UserInfo(user, pass))
		}
	}

	nc, err := nats.Connect(natsURL, opts...)
	if err != nil {
		return fmt.Errorf("(DR16_01) connect nats %s: %w", natsURL, err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return fmt.Errorf("(DR16_02) jetstream context: %w", err)
	}

	subject := strings.TrimSpace(os.Getenv("DOC_REVIEW_EVENT_SUBJECT"))
	if subject == "" {
		subject = DefaultDocReviewEventSubject
	}

	streamName := strings.TrimSpace(os.Getenv("DOC_REVIEW_EVENT_STREAM"))
	if streamName == "" {
		streamName = defaultDocReviewStreamName
	}

	if err := ensureDocReviewStream(js, streamName, subject); err != nil {
		nc.Close()
		return fmt.Errorf("(DR16_03) ensure stream: %w", err)
	}

	globalPublisher = &ReviewPublisher{conn: nc, js: js, subject: subject}
	jsLogger.Info("doc-review JetStream publisher ready", "subject", subject, "stream", streamName)
	return nil
}

// ClosePublisher drains and closes the package-level publisher connection.
func ClosePublisher() {
	if globalPublisher == nil || globalPublisher.conn == nil {
		return
	}
	_ = globalPublisher.conn.Drain()
	globalPublisher.conn.Close()
	globalPublisher = nil
}

// PublishReviewEvent publishes a doc-review start event for the given request ID.
// If the publisher is not initialised the call is a no-op (review falls back to
// being run inline by the caller).
func PublishReviewEvent(ctx context.Context, requestID int64) error {
	if globalPublisher == nil {
		return fmt.Errorf("(DR16_04) doc-review publisher not initialised")
	}
	payload, err := json.Marshal(docReviewEvent{RequestID: requestID})
	if err != nil {
		return fmt.Errorf("(DR16_05) marshal event: %w", err)
	}
	if _, err := globalPublisher.js.Publish(globalPublisher.subject, payload); err != nil {
		return fmt.Errorf("(DR16_06) publish event request_id=%d: %w", requestID, err)
	}
	jsLogger.Info("doc-review event published", "request_id", requestID, "subject", globalPublisher.subject)
	return nil
}

func ensureDocReviewStream(js nats.JetStreamContext, streamName, subject string) error {
	if info, err := js.StreamInfo(streamName); err == nil {
		for _, s := range info.Config.Subjects {
			if strings.TrimSpace(s) == subject {
				return nil
			}
		}
		cfg := info.Config
		cfg.Subjects = append(cfg.Subjects, subject)
		if _, err := js.UpdateStream(&cfg); err != nil {
			return fmt.Errorf("update stream %s: %w", streamName, err)
		}
		return nil
	}
	_, err := js.AddStream(&nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{subject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already in use") {
		return fmt.Errorf("add stream %s: %w", streamName, err)
	}
	return nil
}
