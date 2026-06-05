package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/cache"
	"github.com/hieu-seta/seta-training/pkg/teamclient"
	"golang.org/x/sync/singleflight"
)

// CachedTeamClient wraps a teamclient.TeamClient w/ a 5m cache + singleflight.
// Invalidator (cache_invalidator.go) Dels per uid on team.activity events.
type CachedTeamClient struct {
	inner teamclient.TeamClient
	cache cache.Cache
	sf    singleflight.Group
	ttl   time.Duration
}

// NewCachedTeamClient builds a cache-fronted teamclient.
func NewCachedTeamClient(inner teamclient.TeamClient, c cache.Cache) *CachedTeamClient {
	if c == nil {
		c = cache.Noop{}
	}
	return &CachedTeamClient{inner: inner, cache: c, ttl: 5 * time.Minute}
}

// ManagersOf checks cache first; on miss calls inner + caches; concurrent misses collapse via singleflight.
func (c *CachedTeamClient) ManagersOf(ctx context.Context, uid uuid.UUID) ([]uuid.UUID, error) {
	key := cache.ManagersOfKey(uid)
	var hit []uuid.UUID
	if err := c.cache.GetJSON(ctx, key, &hit); err == nil {
		return hit, nil
	}
	v, err, _ := c.sf.Do(key, func() (any, error) {
		mgrs, ierr := c.inner.ManagersOf(ctx, uid)
		if ierr != nil {
			return nil, ierr
		}
		_ = c.cache.SetJSON(ctx, key, mgrs, c.ttl)
		return mgrs, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]uuid.UUID), nil
}
