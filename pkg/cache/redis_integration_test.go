//go:build integration

package cache_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/hieu-seta/seta-training/pkg/cache"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "redis:7-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
	}
	gc := testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true}
	if os.Getenv("TESTCONTAINERS_REUSE_ENABLE") == "true" {
		gc.Reuse = true
		gc.Name = "seta-it-redis-cache"
	}
	c, err := testcontainers.GenericContainer(ctx, gc)
	if err != nil {
		t.Fatalf("redis container: %v", err)
	}
	t.Cleanup(func() {
		if os.Getenv("TESTCONTAINERS_REUSE_ENABLE") != "true" {
			_ = c.Terminate(context.Background())
		}
	})
	ep, err := c.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: ep})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func TestRedisCache_SetGetDelCycle(t *testing.T) {
	rdb := startRedis(t)
	c := cache.NewRedis(rdb)
	ctx := context.Background()

	type meta struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}

	// miss before set
	var out meta
	if err := c.GetJSON(ctx, "it:k1", &out); !errors.Is(err, cache.ErrMiss) {
		t.Fatalf("want ErrMiss before set, got %v", err)
	}

	// set → get round-trips
	in := meta{Name: "alpha", N: 7}
	if err := c.SetJSON(ctx, "it:k1", in, time.Minute); err != nil {
		t.Fatalf("SetJSON: %v", err)
	}
	if err := c.GetJSON(ctx, "it:k1", &out); err != nil {
		t.Fatalf("GetJSON after set: %v", err)
	}
	if out != in {
		t.Fatalf("round-trip mismatch: got %+v want %+v", out, in)
	}

	// del → miss again; deleting absent keys is a no-op
	if err := c.Del(ctx, "it:k1", "it:never-existed"); err != nil {
		t.Fatalf("Del: %v", err)
	}
	if err := c.GetJSON(ctx, "it:k1", &out); !errors.Is(err, cache.ErrMiss) {
		t.Fatalf("want ErrMiss after del, got %v", err)
	}

	// del with no keys is a no-op
	if err := c.Del(ctx); err != nil {
		t.Fatalf("Del(): %v", err)
	}
}

func TestRedisCache_TTLExpires(t *testing.T) {
	rdb := startRedis(t)
	c := cache.NewRedis(rdb)
	ctx := context.Background()

	if err := c.SetJSON(ctx, "it:ttl", "v", 100*time.Millisecond); err != nil {
		t.Fatalf("SetJSON: %v", err)
	}
	var out string
	if err := c.GetJSON(ctx, "it:ttl", &out); err != nil {
		t.Fatalf("GetJSON before expiry: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := c.GetJSON(ctx, "it:ttl", &out); !errors.Is(err, cache.ErrMiss) {
		t.Fatalf("want ErrMiss after TTL, got %v", err)
	}
}

func TestRedisCache_SetJSON_MarshalError(t *testing.T) {
	rdb := startRedis(t)
	c := cache.NewRedis(rdb)

	// channels are not JSON-marshalable → error surfaces, nothing stored
	if err := c.SetJSON(context.Background(), "it:bad", make(chan int), time.Minute); err == nil {
		t.Fatal("want marshal error, got nil")
	}
}

func TestRedisCache_GetJSON_CorruptValue(t *testing.T) {
	rdb := startRedis(t)
	c := cache.NewRedis(rdb)
	ctx := context.Background()

	if err := rdb.Set(ctx, "it:corrupt", "{not-json", time.Minute).Err(); err != nil {
		t.Fatalf("raw set: %v", err)
	}
	var out struct{ X int }
	if err := c.GetJSON(ctx, "it:corrupt", &out); err == nil || errors.Is(err, cache.ErrMiss) {
		t.Fatalf("want unmarshal error (not miss), got %v", err)
	}
}

func TestRedisCache_OutageSurfacesError(t *testing.T) {
	// closed client ≈ Redis down: errors surface (callers fall back to repo).
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}) // nothing listens
	c := cache.NewRedis(rdb)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var out string
	if err := c.GetJSON(ctx, "it:down", &out); err == nil || errors.Is(err, cache.ErrMiss) {
		t.Fatalf("want transport error (not miss, not nil), got %v", err)
	}
	if err := c.SetJSON(ctx, "it:down", "v", time.Minute); err == nil {
		t.Fatal("want transport error on set, got nil")
	}
	if err := c.Del(ctx, "it:down"); err == nil {
		t.Fatal("want transport error on del, got nil")
	}
}
