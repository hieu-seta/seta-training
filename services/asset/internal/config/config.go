// Package config loads asset-svc settings from env.
package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config = asset-svc runtime settings.
type Config struct {
	HTTPPort      string        `envconfig:"ASSET_HTTP_PORT" default:"8083"`
	PostgresDSN   string        `envconfig:"POSTGRES_DSN"    required:"true"`
	JWTSecret     string        `envconfig:"JWT_SECRET"      required:"true"`
	TeamSvcURL    string        `envconfig:"TEAM_SVC_URL"    default:"http://team-svc:8082"`
	TeamTimeout   time.Duration `envconfig:"TEAM_TIMEOUT"    default:"2s"`
	NATSURL       string        `envconfig:"NATS_URL"        default:""`
	RedisAddr     string        `envconfig:"REDIS_ADDR"      default:""`
	RedisPassword string        `envconfig:"REDIS_PASSWORD"  default:""`
	LogLevel      string        `envconfig:"LOG_LEVEL"       default:"info"`
}

// Load reads env vars into Config.
func Load() (*Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, err
	}
	return &c, nil
}
