//go:build integration

// Package testutil boots Postgres + NATS containers for audit-worker integration tests.
package testutil

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/hieu-seta/seta-training/pkg/pg"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	natsmod "github.com/testcontainers/testcontainers-go/modules/nats"
	pgmod "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

// StartPG returns a *gorm.DB scoped to the audit schema, with migrations applied.
func StartPG(t *testing.T) *gorm.DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	opts := []testcontainers.ContainerCustomizer{
		pgmod.WithDatabase("seta_test"),
		pgmod.WithUsername("seta"),
		pgmod.WithPassword("seta_test_pw"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60 * time.Second)),
	}
	if os.Getenv("TESTCONTAINERS_REUSE_ENABLE") == "true" {
		opts = append(opts, testcontainers.WithReuseByName("seta-it-pg-audit"))
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
		t.Fatalf("dsn: %v", err)
	}
	db, err := pg.Open(pg.Config{DSN: dsn, Schema: "audit"})
	if err != nil {
		t.Fatalf("gorm: %v", err)
	}
	ApplyMigration(t, db)
	return db
}

// ApplyMigration runs migrations/audit/0001_init.up.sql inline so the test
// doesn't depend on the migrator binary or filesystem paths.
func ApplyMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmt := `
CREATE SCHEMA IF NOT EXISTS audit;
SET search_path TO audit;
CREATE TABLE IF NOT EXISTS events (
    id          UUID PRIMARY KEY,
    subject     TEXT NOT NULL,
    idem_key    TEXT NOT NULL UNIQUE,
    occurred_at TIMESTAMPTZ NOT NULL,
    actor_uid   UUID,
    payload     JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS events_occurred_idx ON events(occurred_at DESC);
CREATE INDEX IF NOT EXISTS events_subj_occurred_idx ON events(subject, occurred_at DESC);
`
	if err := db.Exec(stmt).Error; err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// StartNATS boots a JetStream-enabled NATS container and returns a raw Conn +
// JetStream context. Container is terminated on test cleanup unless
// TESTCONTAINERS_REUSE_ENABLE=true.
func StartNATS(t *testing.T) (*nats.Conn, jetstream.JetStream) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	opts := []testcontainers.ContainerCustomizer{
		testcontainers.WithCmd("-js", "-DV"),
	}
	if os.Getenv("TESTCONTAINERS_REUSE_ENABLE") == "true" {
		opts = append(opts, testcontainers.WithReuseByName("seta-it-nats-audit"))
	}
	c, err := natsmod.Run(ctx, "nats:2.10-alpine", opts...)
	if err != nil {
		t.Fatalf("nats container: %v", err)
	}
	t.Cleanup(func() {
		if os.Getenv("TESTCONTAINERS_REUSE_ENABLE") != "true" {
			_ = c.Terminate(context.Background())
		}
	})
	url, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	t.Cleanup(func() { _ = nc.Drain() })
	return nc, js
}

// CountEvents returns the number of rows in audit.events.
func CountEvents(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM audit.events").Scan(&n).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// CountEventsBySubject returns the row count filtered by subject.
func CountEventsBySubject(t *testing.T, db *gorm.DB, subject string) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM audit.events WHERE subject = ?", subject).Scan(&n).Error; err != nil {
		t.Fatalf("count by subject: %v", err)
	}
	return n
}
