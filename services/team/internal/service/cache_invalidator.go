package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/cache"
	"github.com/hieu-seta/seta-training/pkg/events"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
)

// CacheInvalidator subscribes to team.activity.* and deletes affected cache keys.
type CacheInvalidator struct {
	js    jetstream.JetStream
	cache cache.Cache
	log   zerolog.Logger
	stop  func()
}

// NewCacheInvalidator builds an invalidator. Call Start to begin consuming.
func NewCacheInvalidator(js jetstream.JetStream, c cache.Cache, log zerolog.Logger) *CacheInvalidator {
	return &CacheInvalidator{js: js, cache: c, log: log}
}

// Start subscribes via an ephemeral consumer (no replay across restarts — TTL self-heals).
func (i *CacheInvalidator) Start(ctx context.Context) error {
	s, err := i.js.Stream(ctx, events.StreamActivity)
	if err != nil {
		return err
	}
	cons, err := s.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubject: events.SubjTeamActivityAll,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       10 * time.Second,
		DeliverPolicy: jetstream.DeliverNewPolicy, // only new events
		MaxAckPending: 64,
	})
	if err != nil {
		return err
	}
	cc, err := cons.Consume(i.handle)
	if err != nil {
		return err
	}
	i.stop = cc.Stop
	i.log.Info().Msg("team cache invalidator started")
	return nil
}

// Stop drains in-flight.
func (i *CacheInvalidator) Stop() {
	if i.stop != nil {
		i.stop()
	}
}

func (i *CacheInvalidator) handle(msg jetstream.Msg) {
	defer func() { _ = msg.Ack() }()
	var p struct {
		TeamID uuid.UUID `json:"team_id"`
		UserID uuid.UUID `json:"user_id"`
	}
	if err := json.Unmarshal(msg.Data(), &p); err != nil {
		i.log.Warn().Err(err).Str("subject", msg.Subject()).Msg("cache-inv: bad payload")
		return
	}
	keys := []string{
		cache.TeamMembersKey(p.TeamID),
		cache.TeamManagersKey(p.TeamID),
	}
	// On member/manager mutations, also blow the affected user's ManagersOf cache (asset-svc reads this).
	if p.UserID != uuid.Nil {
		keys = append(keys, cache.ManagersOfKey(p.UserID))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := i.cache.Del(ctx, keys...); err != nil && !errors.Is(err, cache.ErrMiss) {
		i.log.Warn().Err(err).Strs("keys", keys).Msg("cache-inv: del failed")
		return
	}
	i.log.Debug().Strs("keys", keys).Str("subject", msg.Subject()).Msg("cache invalidated")
}
