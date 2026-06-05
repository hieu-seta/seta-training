// Package consumer subscribes to JetStream streams and forwards to AuditService.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hieu-seta/seta-training/pkg/events"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/service"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
)

// Consumer manages durable JS consumers for ACTIVITY + ASSETS streams.
type Consumer struct {
	js        jetstream.JetStream
	audit     *service.AuditService
	log       zerolog.Logger
	consumers []jetstream.Consumer
	stops     []func()
}

// New builds a Consumer. Streams must already exist (caller invokes events.EnsureStreams).
func New(js jetstream.JetStream, audit *service.AuditService, log zerolog.Logger) *Consumer {
	return &Consumer{js: js, audit: audit, log: log}
}

// Start subscribes both consumers + dispatches messages. Returns on first setup error.
// Use Stop to drain in-flight messages.
func (c *Consumer) Start(ctx context.Context) error {
	for _, spec := range []struct {
		stream, name, filter string
	}{
		{events.StreamActivity, "audit-activity", events.SubjTeamActivityAll},
		{events.StreamAssets, "audit-assets", events.SubjAssetAll},
	} {
		s, err := c.js.Stream(ctx, spec.stream)
		if err != nil {
			return fmt.Errorf("get stream %s: %w", spec.stream, err)
		}
		cons, err := s.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			Durable:       spec.name,
			FilterSubject: spec.filter,
			AckPolicy:     jetstream.AckExplicitPolicy,
			AckWait:       30 * time.Second,
			MaxDeliver:    5,
			MaxAckPending: 256,
			DeliverPolicy: jetstream.DeliverAllPolicy,
		})
		if err != nil {
			return fmt.Errorf("create consumer %s: %w", spec.name, err)
		}
		ctxConsumer, err := cons.Consume(c.handle)
		if err != nil {
			return fmt.Errorf("consume %s: %w", spec.name, err)
		}
		c.consumers = append(c.consumers, cons)
		c.stops = append(c.stops, ctxConsumer.Stop)
		c.log.Info().Str("stream", spec.stream).Str("consumer", spec.name).Msg("consumer started")
	}
	return nil
}

// Stop drains in-flight delivery + stops the consumers.
func (c *Consumer) Stop() {
	for _, s := range c.stops {
		s()
	}
}

func (c *Consumer) handle(msg jetstream.Msg) {
	subject := msg.Subject()
	msgID := msg.Headers().Get("Nats-Msg-Id")
	reqID := peekRequestID(msg.Data())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	inserted, err := c.audit.Record(ctx, subject, msgID, msg.Data())
	if err != nil {
		c.log.Error().Err(err).Str("subject", subject).Str("msg_id", msgID).Str("req_id", reqID).Msg("record failed — NAK")
		if nakErr := msg.Nak(); nakErr != nil && !errors.Is(nakErr, context.Canceled) {
			c.log.Error().Err(nakErr).Msg("nak failed")
		}
		return
	}
	if ackErr := msg.Ack(); ackErr != nil {
		c.log.Error().Err(ackErr).Str("req_id", reqID).Msg("ack failed")
		return
	}
	c.log.Debug().Str("subject", subject).Str("msg_id", msgID).Str("req_id", reqID).Bool("inserted", inserted).Msg("audit recorded")
}

// peekRequestID extracts the envelope's request_id field without committing to
// a full unmarshal — keeps the consumer tolerant of payload shape drift.
func peekRequestID(data []byte) string {
	var env struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return ""
	}
	return env.RequestID
}
