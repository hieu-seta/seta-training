// Package logger sets up zerolog + Gin request middleware.
package logger

import (
	"io"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// New returns a zerolog.Logger writing JSON to w (default stderr).
// level: "debug", "info", "warn", "error".
func New(svc, level string, w io.Writer) zerolog.Logger {
	if w == nil {
		w = os.Stderr
	}
	lvl, err := zerolog.ParseLevel(level)
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}
	zerolog.TimeFieldFormat = time.RFC3339Nano
	return zerolog.New(w).Level(lvl).With().
		Timestamp().
		Str("svc", svc).
		Logger()
}

// GinMiddleware logs each HTTP request as one JSON line.
// Picks up req_id from ctx (set by the ReqID middleware) if present.
func GinMiddleware(l zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		ev := l.Info().
			Str("method", c.Request.Method).
			Str("path", c.FullPath()).
			Int("status", c.Writer.Status()).
			Dur("dur", time.Since(start)).
			Str("ip", c.ClientIP())
		if id := GetReqID(c); id != "" {
			ev = ev.Str("req_id", id)
		}
		ev.Msg("http")
	}
}
