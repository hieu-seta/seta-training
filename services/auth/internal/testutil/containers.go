//go:build integration

// Package testutil spins up Postgres + Redis containers for integration tests.
// Reuse=true locally (TESTCONTAINERS_REUSE_ENABLE=true), false in CI.
package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hieu-seta/seta-training/pkg/pg"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	pgmod "github.com/testcontainers/testcontainers-go/modules/postgres"
	redismod "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

func startPG(ctx context.Context, t *testing.T) string {
	t.Helper()
	opts := []testcontainers.ContainerCustomizer{
		pgmod.WithDatabase("seta_test"),
		pgmod.WithUsername("seta"),
		pgmod.WithPassword("seta_test_pw"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60 * time.Second)),
	}
	if os.Getenv("TESTCONTAINERS_REUSE_ENABLE") == "true" {
		opts = append(opts, testcontainers.WithReuseByName("seta-it-pg"))
	}
	c, err := pgmod.Run(ctx, "postgres:16-alpine", opts...)
	if err != nil {
		t.Fatalf("pg start: %v", err)
	}
	t.Cleanup(func() {
		if os.Getenv("TESTCONTAINERS_REUSE_ENABLE") != "true" {
			_ = c.Terminate(context.Background())
		}
	})
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("pg dsn: %v", err)
	}
	return dsn
}

func startRedis(ctx context.Context, t *testing.T) string {
	t.Helper()
	opts := []testcontainers.ContainerCustomizer{
		testcontainers.WithWaitStrategy(wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second)),
	}
	if os.Getenv("TESTCONTAINERS_REUSE_ENABLE") == "true" {
		opts = append(opts, testcontainers.WithReuseByName("seta-it-redis"))
	}
	c, err := redismod.Run(ctx, "redis:7-alpine", opts...)
	if err != nil {
		t.Fatalf("redis start: %v", err)
	}
	t.Cleanup(func() {
		if os.Getenv("TESTCONTAINERS_REUSE_ENABLE") != "true" {
			_ = c.Terminate(context.Background())
		}
	})
	addr, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("redis addr: %v", err)
	}
	return addr
}

// Stack returns (gorm DB w/ schema=auth, redis client, cleanup).
func Stack(t *testing.T, schema string) (*gorm.DB, *redis.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	dsn := startPG(ctx, t)
	db, err := pg.Open(pg.Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}

	addr := startRedis(ctx, t)
	// redis-go expects host:port — strip scheme.
	if len(addr) > 8 && addr[:8] == "redis://" {
		addr = addr[8:]
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}

	t.Cleanup(func() { _ = rdb.Close() })
	return db, rdb
}

// ApplyAuthMigration runs the SQL from migrations/auth/0001_init.up.sql.
// Hand-rolled (no migrate.New URL plumbing in tests).
func ApplyAuthMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmt := `
CREATE SCHEMA IF NOT EXISTS auth;
CREATE TABLE IF NOT EXISTS auth.users (
    id              UUID PRIMARY KEY,
    username        TEXT NOT NULL,
    email           TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('manager','member')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS users_role_idx ON auth.users(role);
`
	if err := db.Exec(stmt).Error; err != nil {
		t.Fatalf("apply migration: %v", err)
	}
}

// TruncateAll wipes auth tables between tests.
func TruncateAll(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec("TRUNCATE auth.users RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// FlushRedis wipes all keys (test-only).
func FlushRedis(t *testing.T, rdb *redis.Client) {
	t.Helper()
	if err := rdb.FlushAll(context.Background()).Err(); err != nil {
		t.Fatalf("flush redis: %v", err)
	}
}

var _ = fmt.Sprintf
