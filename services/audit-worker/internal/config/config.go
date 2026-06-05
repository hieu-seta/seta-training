// Package config loads audit-worker settings from env.
package config

import "github.com/kelseyhightower/envconfig"

// Config = audit-worker runtime settings.
type Config struct {
	PostgresDSN string `envconfig:"POSTGRES_DSN" required:"true"`
	NATSURL     string `envconfig:"NATS_URL"     required:"true"`
	LogLevel    string `envconfig:"LOG_LEVEL"    default:"info"`
}

// Load reads env vars into Config.
func Load() (*Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, err
	}
	return &c, nil
}
