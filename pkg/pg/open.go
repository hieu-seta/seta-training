// Package pg opens GORM connections w/ per-svc schema isolation.
package pg

import (
	"fmt"
	"net/url"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config = minimal DSN-style settings.
type Config struct {
	DSN    string // postgres://user:pw@host:port/db?sslmode=disable
	Schema string // e.g. "auth", "team", "asset", "audit"
	Debug  bool
}

// Open connects + sets search_path to the svc schema.
// Caller owns the *gorm.DB (close via sql.DB.Close()).
func Open(cfg Config) (*gorm.DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("pg: empty DSN")
	}
	if cfg.Schema == "" {
		return nil, fmt.Errorf("pg: empty schema")
	}
	lvl := logger.Warn
	if cfg.Debug {
		lvl = logger.Info
	}
	// Bake search_path into the DSN so every pooled connection inherits it.
	// Without this, GORM's pool gives each goroutine a fresh conn whose search_path defaults to "$user,public".
	dsn, err := withSearchPath(cfg.DSN, cfg.Schema)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(lvl),
		NamingStrategy: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("pg: gorm open: %w", err)
	}
	// Ensure schema exists (idempotent).
	if err := db.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %q", cfg.Schema)).Error; err != nil {
		return nil, fmt.Errorf("pg: create schema: %w", err)
	}
	return db, nil
}

// withSearchPath rewrites a postgres:// URL to append `?options=-csearch_path=<schema>`.
// Works for both URL-style DSNs and bare key=value DSNs.
func withSearchPath(dsn, schema string) (string, error) {
	schema = strings.TrimSpace(schema)
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", fmt.Errorf("pg: parse DSN: %w", err)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	// key=value style
	return dsn + " search_path=" + schema, nil
}
