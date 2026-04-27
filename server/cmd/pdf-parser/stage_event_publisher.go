package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/nats-io/nats.go"
)

type stageEvent struct {
	RecordID   int64  `json:"record_id"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Force      bool   `json:"force,omitempty"`
	FileFormat string `json:"file_format"`
	FileName   string `json:"file_name"`
}

type stageEventPublisher struct {
	conn    *nats.Conn
	js      nats.JetStreamContext
	subject string
}

func normalizeStreamName(raw string, fallback string) string {
	v := strings.TrimSpace(raw)
	if v == "" || strings.EqualFold(v, "true") {
		return fallback
	}
	if strings.EqualFold(v, "false") || strings.EqualFold(v, "none") || v == "0" {
		return ""
	}
	return v
}

func subjectExists(subjects []string, subject string) bool {
	for _, s := range subjects {
		if strings.TrimSpace(s) == strings.TrimSpace(subject) {
			return true
		}
	}
	return false
}

func newStageEventPublisher(logger ApiTypes.JimoLogger) (*stageEventPublisher, error) {
	natsURL := strings.TrimSpace(os.Getenv("NATS_URL"))
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
		return nil, fmt.Errorf("(MID_26042701) connect nats: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("(MID_26042702) jetstream context: %w", err)
	}

	subject := strings.TrimSpace(os.Getenv("PDF_STAGE_EVENT_SUBJECT"))
	if subject == "" {
		subject = "kb.pdf.staged"
	}

	streamName := normalizeStreamName(os.Getenv("PDF_STAGE_EVENT_STREAM"), "pdf-stage-events")
	if streamName != "" {
		if info, err := js.StreamInfo(streamName); err == nil {
			cfg := info.Config
			if !subjectExists(cfg.Subjects, subject) {
				cfg.Subjects = append(cfg.Subjects, subject)
				if _, err := js.UpdateStream(&cfg); err != nil {
					nc.Close()
					return nil, fmt.Errorf("(MID_26042703) update stream %s subjects: %w", streamName, err)
				}
				logger.Info("updated stream to include stage subject", "stream", streamName, "subject", subject)
			}
		} else {
			if _, err := js.AddStream(&nats.StreamConfig{
				Name:      streamName,
				Subjects:  []string{subject},
				Retention: nats.WorkQueuePolicy,
				Storage:   nats.FileStorage,
			}); err != nil {
				nc.Close()
				return nil, fmt.Errorf("(MID_26042704) add stream %s: %w", streamName, err)
			}
			logger.Info("created jetstream stream for stage events", "stream", streamName, "subject", subject)
		}
	}

	return &stageEventPublisher{conn: nc, js: js, subject: subject}, nil
}

func (p *stageEventPublisher) Publish(evt stageEvent) error {
	if p == nil || p.js == nil {
		return fmt.Errorf("(MID_26042705) publisher is not initialized")
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("(MID_26042706) marshal stage event: %w", err)
	}
	if _, err := p.js.Publish(p.subject, payload); err != nil {
		return fmt.Errorf("(MID_26042707) publish stage event: %w", err)
	}
	return nil
}

func (p *stageEventPublisher) Close() {
	if p == nil || p.conn == nil {
		return
	}
	_ = p.conn.Drain()
	p.conn.Close()
}
