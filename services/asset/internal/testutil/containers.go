//go:build integration

// Package testutil boots Postgres for asset-svc integration tests.
package testutil

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/hieu-seta/seta-training/pkg/pg"
	"github.com/testcontainers/testcontainers-go"
	pgmod "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

// StartPG returns a gorm DB w/ schema=asset + migrations applied.
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
		opts = append(opts, testcontainers.WithReuseByName("seta-it-pg-asset"))
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
	db, err := pg.Open(pg.Config{DSN: dsn, Schema: "asset"})
	if err != nil {
		t.Fatalf("gorm: %v", err)
	}
	ApplyMigration(t, db)
	return db
}

// ApplyMigration applies asset/0001_init.up.sql inline.
func ApplyMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmt := `
CREATE SCHEMA IF NOT EXISTS asset;
CREATE TABLE IF NOT EXISTS asset.folders (
    id UUID PRIMARY KEY, owner_id UUID NOT NULL, name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS asset.notes (
    id UUID PRIMARY KEY,
    folder_id UUID NOT NULL REFERENCES asset.folders(id) ON DELETE CASCADE,
    owner_id UUID NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS asset.folder_shares (
    folder_id UUID NOT NULL REFERENCES asset.folders(id) ON DELETE CASCADE,
    user_id UUID NOT NULL, access TEXT NOT NULL CHECK (access IN ('read','write')),
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (folder_id, user_id)
);
CREATE TABLE IF NOT EXISTS asset.note_shares (
    note_id UUID NOT NULL REFERENCES asset.notes(id) ON DELETE CASCADE,
    user_id UUID NOT NULL, access TEXT NOT NULL CHECK (access IN ('read','write')),
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (note_id, user_id)
);
`
	if err := db.Exec(stmt).Error; err != nil {
		t.Fatalf("migrate: %v", err)
	}
}
