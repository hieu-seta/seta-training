//go:build integration

// Package testutil boots Postgres for team-svc integration tests.
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

// StartPG returns (gorm DB w/ schema=team).
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
		opts = append(opts, testcontainers.WithReuseByName("seta-it-pg-team"))
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
	db, err := pg.Open(pg.Config{DSN: dsn, Schema: "team"})
	if err != nil {
		t.Fatalf("gorm: %v", err)
	}
	ApplyMigration(t, db)
	return db
}

// ApplyMigration runs migrations/team/0001_init.up.sql inline.
func ApplyMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmt := `
CREATE SCHEMA IF NOT EXISTS team;
CREATE TABLE IF NOT EXISTS team.teams (
    id         UUID PRIMARY KEY,
    name       TEXT NOT NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS team.team_members (
    team_id  UUID NOT NULL REFERENCES team.teams(id) ON DELETE CASCADE,
    user_id  UUID NOT NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);
CREATE TABLE IF NOT EXISTS team.team_managers (
    team_id  UUID NOT NULL REFERENCES team.teams(id) ON DELETE CASCADE,
    user_id  UUID NOT NULL,
    is_main  BOOLEAN NOT NULL DEFAULT false,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS team_managers_one_main_idx ON team.team_managers(team_id) WHERE is_main;
`
	if err := db.Exec(stmt).Error; err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// Truncate wipes team tables.
func Truncate(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec("TRUNCATE team.team_members, team.team_managers, team.teams RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
}
