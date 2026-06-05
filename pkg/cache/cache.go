// Package cache wraps go-redis with typed JSON helpers + a Noop fallback.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrMiss signals a cache miss. Callers should fall through to the data source.
var ErrMiss = errors.New("cache miss")

// Cache is the port. Reads return ErrMiss on absence.
type Cache interface {
	GetJSON(ctx context.Context, key string, out any) error
	SetJSON(ctx context.Context, key string, val any, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
}

// Redis-backed implementation.
type redisCache struct{ c *redis.Client }

// NewRedis builds a Redis-backed Cache.
func NewRedis(c *redis.Client) Cache { return &redisCache{c: c} }

func (r *redisCache) GetJSON(ctx context.Context, key string, out any) error {
	v, err := r.c.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return ErrMiss
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(v, out)
}

func (r *redisCache) SetJSON(ctx context.Context, key string, val any, ttl time.Duration) error {
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return r.c.Set(ctx, key, b, ttl).Err()
}

func (r *redisCache) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return r.c.Del(ctx, keys...).Err()
}

// Noop is a safe stand-in when Redis is unavailable.
// GetJSON always returns ErrMiss → callers fall back to the data source.
type Noop struct{}

// GetJSON always misses.
func (Noop) GetJSON(_ context.Context, _ string, _ any) error { return ErrMiss }

// SetJSON swallows.
func (Noop) SetJSON(_ context.Context, _ string, _ any, _ time.Duration) error { return nil }

// Del swallows.
func (Noop) Del(_ context.Context, _ ...string) error { return nil }
