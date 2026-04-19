package fileconverters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/nats-io/nats.go"
)

type NATSSubscriber struct {
	conn   *nats.Conn
	logger ApiTypes.JimoLogger
}

var invalidConsumerNameCharsRE = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func shouldAckOnHandlerError(err error) bool {
	return errors.Is(err, ErrMissingParsedSuccess) ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrResultFileNotFound) ||
		errors.Is(err, ErrUnsupportedParserName) ||
		errors.Is(err, ErrParserNotImplemented)
}

func envEnabled(keys ...string) bool {
	for _, key := range keys {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if v == "1" || v == "true" || v == "yes" || v == "y" || v == "on" {
			return true
		}
	}
	return false
}

func envEnabledDefaultTrue(keys ...string) bool {
	for _, key := range keys {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if v == "" {
			continue
		}
		if v == "0" || v == "false" || v == "no" || v == "n" || v == "off" {
			return false
		}
		if v == "1" || v == "true" || v == "yes" || v == "y" || v == "on" {
			return true
		}
	}
	return true
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

func normalizeConsumerName(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	normalized := invalidConsumerNameCharsRE.ReplaceAllString(v, "-")
	normalized = strings.Trim(normalized, "-_")
	if normalized == "" {
		return ""
	}
	return normalized
}

func NewNATSSubscriber(url string) (*NATSSubscriber, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, fmt.Errorf("nats url is empty")
	}

	logger := loggerutil.CreateDefaultLogger("MID_26041201")

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

	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	return &NATSSubscriber{
		conn:   nc,
		logger: logger,
	}, nil
}

func (s *NATSSubscriber) Close() {
	if s.conn != nil {
		s.conn.Drain()
		s.conn.Close()
	}
}

func (s *NATSSubscriber) Publish(ctx context.Context, subject string, payload []byte) error {
	if s.conn == nil {
		return fmt.Errorf("nats connection is nil")
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return fmt.Errorf("publish subject is empty")
	}
	if err := s.conn.Publish(subject, payload); err != nil {
		return err
	}
	if ctx == nil {
		return s.conn.Flush()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		return s.conn.Flush()
	}
	return s.conn.FlushWithContext(ctx)
}

func (s *NATSSubscriber) Subscribe(ctx context.Context, subject string, durable string, handler func(context.Context, []byte) error) error {
	if s.conn == nil {
		return fmt.Errorf("nats connection is nil")
	}
	js, err := s.conn.JetStream()
	if err != nil {
		return fmt.Errorf("jetstream context: %w", err)
	}

	subject = strings.TrimSpace(subject)
	durable = strings.TrimSpace(durable)
	if subject == "" {
		return fmt.Errorf("subject is empty")
	}
	normalizedDurable := normalizeConsumerName(durable)
	if durable != "" && normalizedDurable == "" {
		return fmt.Errorf("durable consumer name %q is invalid after normalization", durable)
	}
	if durable != normalizedDurable && normalizedDurable != "" {
		s.logger.Info("normalized invalid durable consumer name",
			"original", durable,
			"normalized", normalizedDurable,
			"subject", subject,
		)
	}

	opts := []nats.SubOpt{
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.DeliverNew(),
	}
	if normalizedDurable != "" {
		opts = append(opts, nats.Durable(normalizedDurable))
	}

	cb := func(msg *nats.Msg) {
		if err := handler(ctx, msg.Data); err != nil {
			s.logger.Error("(MID_26041120) converter message failed", "subject", msg.Subject, "error", err)
			if shouldAckOnHandlerError(err) {
				_ = msg.Ack()
				return
			}
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
	}

	sub, err := js.Subscribe(subject, cb, opts...)
	if err != nil && normalizedDurable != "" {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "already bound to a subscription") || strings.Contains(msg, "without a deliver group") {
			altDurable := normalizeConsumerName(fmt.Sprintf("%s-%d", normalizedDurable, time.Now().UnixNano()%1_000_000))
			if altDurable != "" && altDurable != normalizedDurable {
				s.logger.Info("retry subscribe with fallback durable consumer name",
					"subject", subject,
					"original_durable", normalizedDurable,
					"fallback_durable", altDurable,
				)
				altOpts := []nats.SubOpt{
					nats.ManualAck(),
					nats.AckExplicit(),
					nats.DeliverNew(),
					nats.Durable(altDurable),
				}
				sub, err = js.Subscribe(subject, cb, altOpts...)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("subscribe subject=%s: %w", subject, err)
	}
	defer sub.Unsubscribe()

	<-ctx.Done()
	return nil
}

func (s *NATSSubscriber) EnsureStream(streamName string, subject string) error {
	if s.conn == nil {
		return fmt.Errorf("nats connection is nil")
	}
	streamName = normalizeStreamName(streamName, "pdf-parsed-events")
	subject = strings.TrimSpace(subject)
	if streamName == "" {
		return nil
	}
	if subject == "" {
		return fmt.Errorf("subject is empty")
	}

	js, err := s.conn.JetStream()
	if err != nil {
		return fmt.Errorf("jetstream context: %w", err)
	}
	autoRecreate := envEnabledDefaultTrue(
		"PARSER_RESULT_CONVERTER_AUTO_RECREATE_STREAM",
		"PDF_RESULT_CONVERTER_AUTO_RECREATE_STREAM",
		"PDF_CONVERTER_AUTO_RECREATE_STREAM",
	)

	ownerStream, err := js.StreamNameBySubject(subject)
	if err == nil && strings.TrimSpace(ownerStream) != "" {
		info, infoErr := js.StreamInfo(ownerStream)
		if infoErr != nil {
			return fmt.Errorf("stream info for subject owner %s: %w", ownerStream, infoErr)
		}
		if info != nil && info.Config.Retention == nats.WorkQueuePolicy {
			if autoRecreate {
				if err := recreateSubjectOwnerStreamAsLimits(js, ownerStream, subject, info.Config); err != nil {
					return err
				}
				return nil
			}
			return fmt.Errorf("subject %s belongs to stream %s with WorkQueuePolicy; recreate it with LimitsPolicy to use DeliverNew", subject, ownerStream)
		}
		return nil
	}
	if err != nil && !errors.Is(err, nats.ErrNoMatchingStream) {
		return fmt.Errorf("lookup stream by subject %s: %w", subject, err)
	}

	if info, err := js.StreamInfo(streamName); err == nil {
		if info != nil && info.Config.Retention == nats.WorkQueuePolicy {
			if autoRecreate {
				if err := recreateSubjectOwnerStreamAsLimits(js, streamName, subject, info.Config); err != nil {
					return err
				}
				return nil
			}
			return fmt.Errorf("stream %s uses WorkQueuePolicy; recreate it with LimitsPolicy to use DeliverNew", streamName)
		}
		return nil
	}

	_, err = js.AddStream(&nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{subject},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	})
	if err != nil && !isNonFatalEnsureStreamErr(err) {
		return fmt.Errorf("add stream %s: %w", streamName, err)
	}
	return nil
}

func recreateSubjectOwnerStreamAsLimits(js nats.JetStreamContext, streamName string, subject string, cfg nats.StreamConfig) error {
	streamName = strings.TrimSpace(streamName)
	if streamName == "" {
		return fmt.Errorf("stream name is empty for auto-recreate")
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return fmt.Errorf("subject is empty for auto-recreate")
	}

	if len(cfg.Subjects) != 1 || strings.TrimSpace(cfg.Subjects[0]) != subject {
		return fmt.Errorf("refusing auto-recreate for stream %s: expected exactly one subject %q, got %v", streamName, subject, cfg.Subjects)
	}

	if err := js.DeleteStream(streamName); err != nil {
		return fmt.Errorf("delete stream %s for auto-recreate: %w", streamName, err)
	}

	cfg.Name = streamName
	cfg.Retention = nats.LimitsPolicy
	if strings.TrimSpace(cfg.Subjects[0]) != subject {
		cfg.Subjects = []string{subject}
	}

	if _, err := js.AddStream(&cfg); err != nil {
		return fmt.Errorf("recreate stream %s with LimitsPolicy: %w", streamName, err)
	}
	return nil
}

func isNonFatalEnsureStreamErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// Non-fatal cases:
	// 1) Target stream already exists.
	return strings.Contains(msg, "already in use")
}
