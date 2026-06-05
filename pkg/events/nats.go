package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// JS wraps a NATS connection + JetStream context.
type JS struct {
	nc *nats.Conn
	js jetstream.JetStream
}

// Connect dials NATS, gets a JetStream context, ensures streams exist.
func Connect(ctx context.Context, url string) (*JS, error) {
	nc, err := nats.Connect(url, nats.Timeout(5*time.Second), nats.RetryOnFailedConnect(true), nats.MaxReconnects(5))
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		_ = nc.Drain()
		return nil, fmt.Errorf("jetstream ctx: %w", err)
	}
	if err := EnsureStreams(ctx, js); err != nil {
		_ = nc.Drain()
		return nil, err
	}
	return &JS{nc: nc, js: js}, nil
}

// Close drains the underlying connection.
func (j *JS) Close() {
	if j == nil || j.nc == nil {
		return
	}
	_ = j.nc.Drain()
}

// Publisher returns a Publisher backed by JetStream.
func (j *JS) Publisher() Publisher { return &jsPublisher{js: j.js} }

// EnsureStreams declares the two streams idempotently.
// 7d retention, 5m dedup window. File storage.
func EnsureStreams(ctx context.Context, js jetstream.JetStream) error {
	streams := []jetstream.StreamConfig{
		{
			Name:       StreamActivity,
			Subjects:   []string{SubjTeamActivityAll},
			Retention:  jetstream.LimitsPolicy,
			MaxAge:     7 * 24 * time.Hour,
			Storage:    jetstream.FileStorage,
			Duplicates: 5 * time.Minute,
			Discard:    jetstream.DiscardOld,
		},
		{
			Name:       StreamAssets,
			Subjects:   []string{SubjAssetAll},
			Retention:  jetstream.LimitsPolicy,
			MaxAge:     7 * 24 * time.Hour,
			Storage:    jetstream.FileStorage,
			Duplicates: 5 * time.Minute,
			Discard:    jetstream.DiscardOld,
		},
	}
	for _, cfg := range streams {
		if err := upsertStream(ctx, js, cfg); err != nil {
			return err
		}
	}
	return nil
}

func upsertStream(ctx context.Context, js jetstream.JetStream, cfg jetstream.StreamConfig) error {
	_, err := js.CreateStream(ctx, cfg)
	if err == nil {
		return nil
	}
	if errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		_, uerr := js.UpdateStream(ctx, cfg)
		return uerr
	}
	return fmt.Errorf("stream %s: %w", cfg.Name, err)
}

type jsPublisher struct{ js jetstream.JetStream }

// Publish marshals payload, attaches Nats-Msg-Id, publishes synchronously.
func (p *jsPublisher) Publish(ctx context.Context, subject string, payload any, msgID string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	msg := &nats.Msg{
		Subject: subject,
		Data:    body,
		Header:  nats.Header{},
	}
	if msgID != "" {
		msg.Header.Set("Nats-Msg-Id", msgID)
	}
	_, err = p.js.PublishMsg(ctx, msg)
	return err
}
