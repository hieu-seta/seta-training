package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds runtime settings loaded from env.
type Config struct {
	HTTPPort      string        `envconfig:"AUTH_HTTP_PORT" default:"8081"`
	PostgresDSN   string        `envconfig:"POSTGRES_DSN"   required:"true"`
	RedisAddr     string        `envconfig:"REDIS_ADDR"     default:"localhost:6379"`
	RedisPassword string        `envconfig:"REDIS_PASSWORD" default:""`
	JWTSecret     string        `envconfig:"JWT_SECRET"     required:"true"`
	AccessTTL     time.Duration `envconfig:"JWT_ACCESS_TTL"  default:"15m"`
	RefreshTTL    time.Duration `envconfig:"JWT_REFRESH_TTL" default:"168h"`
	BcryptCost    int           `envconfig:"BCRYPT_COST"     default:"12"`
	LogLevel      string        `envconfig:"LOG_LEVEL"       default:"info"`
}

// Load reads env vars into Config. Returns error on missing required.
func Load() (*Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, err
	}
	return &c, nil
}
