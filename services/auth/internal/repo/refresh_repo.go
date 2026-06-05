package repo

import (
	"context"
	"errors"
	"time"

	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/redis/go-redis/v9"
)

// RefreshRepo stores opaque refresh tokens (jti → uid + family) in Redis.
// Key shape:
//
//	refresh:{jti}        = "uid:family"      TTL = refresh ttl
//	refresh-fam:{family} = SET of jtis        TTL = refresh ttl + grace
type RefreshRepo interface {
	Store(ctx context.Context, jti, uid, family string, ttl time.Duration) error
	Lookup(ctx context.Context, jti string) (uid, family string, err error)
	Delete(ctx context.Context, jti string) error
	DeleteFamily(ctx context.Context, family string) error
}

type redisRefresh struct{ c *redis.Client }

// NewRefreshRepo wraps a redis client.
func NewRefreshRepo(c *redis.Client) RefreshRepo { return &redisRefresh{c: c} }

const (
	keyToken  = "refresh:"
	keyFamily = "refresh-fam:"
	famGrace  = 5 * time.Minute
)

func (r *redisRefresh) Store(ctx context.Context, jti, uid, family string, ttl time.Duration) error {
	pipe := r.c.TxPipeline()
	pipe.Set(ctx, keyToken+jti, uid+":"+family, ttl)
	pipe.SAdd(ctx, keyFamily+family, jti)
	pipe.Expire(ctx, keyFamily+family, ttl+famGrace)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *redisRefresh) Lookup(ctx context.Context, jti string) (uid, family string, err error) {
	v, err := r.c.Get(ctx, keyToken+jti).Result()
	if errors.Is(err, redis.Nil) {
		return "", "", httpx.ErrUnauthd
	}
	if err != nil {
		return "", "", err
	}
	for i := 0; i < len(v); i++ {
		if v[i] == ':' {
			return v[:i], v[i+1:], nil
		}
	}
	return "", "", httpx.ErrInternal
}

func (r *redisRefresh) Delete(ctx context.Context, jti string) error {
	return r.c.Del(ctx, keyToken+jti).Err()
}

func (r *redisRefresh) DeleteFamily(ctx context.Context, family string) error {
	jtis, err := r.c.SMembers(ctx, keyFamily+family).Result()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(jtis)+1)
	for _, j := range jtis {
		keys = append(keys, keyToken+j)
	}
	keys = append(keys, keyFamily+family)
	if len(keys) == 0 {
		return nil
	}
	return r.c.Del(ctx, keys...).Err()
}
