package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hieu-seta/seta-training/pkg/authclient"
	"github.com/hieu-seta/seta-training/pkg/cache"
	"github.com/hieu-seta/seta-training/pkg/events"
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
	"github.com/hieu-seta/seta-training/pkg/logger"
	"github.com/hieu-seta/seta-training/pkg/pg"
	"github.com/hieu-seta/seta-training/services/team/internal/config"
	"github.com/hieu-seta/seta-training/services/team/internal/handler"
	"github.com/hieu-seta/seta-training/services/team/internal/repo"
	"github.com/hieu-seta/seta-training/services/team/internal/service"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New("team-svc", cfg.LogLevel, os.Stderr)

	db, err := pg.Open(pg.Config{DSN: cfg.PostgresDSN, Schema: "team"})
	if err != nil {
		log.Fatal().Err(err).Msg("pg open")
	}

	signer := jwtauth.NewSigner(cfg.JWTSecret)
	auth := authclient.New(cfg.AuthSvcURL, cfg.AuthTimeout)
	teamRepo := repo.New(db)

	// Events: best-effort. Failure to connect logs + falls back to Noop so the svc still boots.
	var pub events.Publisher = events.Noop{}
	var js jetstream.JetStream
	var nc *nats.Conn
	if cfg.NATSURL != "" {
		natsCtx, ncancel := context.WithTimeout(context.Background(), 10*time.Second)
		jsConn, jerr := events.Connect(natsCtx, cfg.NATSURL)
		ncancel()
		if jerr != nil {
			log.Error().Err(jerr).Msg("nats connect failed — publishing disabled")
		} else {
			pub = jsConn.Publisher()
			defer jsConn.Close()
			// Separate raw connection for the cache invalidator consumer (different lifecycle).
			rawCtx, rawCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer rawCancel()
			nc2, err := nats.Connect(cfg.NATSURL, nats.Timeout(5*time.Second))
			if err != nil {
				log.Error().Err(err).Msg("nats raw connect for invalidator failed")
			} else {
				nc = nc2
				js, _ = jetstream.New(nc, jetstream.WithDefaultTimeout(5*time.Second))
			}
			_ = rawCtx
			log.Info().Str("url", cfg.NATSURL).Msg("nats connected, streams ready")
		}
	}

	// Cache: best-effort. Failure → Noop, svc still serves reads (slower).
	var c cache.Cache = cache.Noop{}
	if cfg.RedisAddr != "" {
		rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
		pingCtx, pcancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := rdb.Ping(pingCtx).Err(); err != nil {
			log.Error().Err(err).Msg("redis ping failed — caching disabled")
		} else {
			c = cache.NewRedis(rdb)
			defer func() { _ = rdb.Close() }()
			log.Info().Str("addr", cfg.RedisAddr).Msg("redis connected")
		}
		pcancel()
	}

	svc := service.New(teamRepo, auth, pub).WithCache(c)

	// Cache invalidator: subscribes to team.activity.* and Dels stale keys.
	if js != nil {
		inv := service.NewCacheInvalidator(js, c, log)
		invCtx, invCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := inv.Start(invCtx); err != nil {
			log.Error().Err(err).Msg("cache invalidator failed to start")
		} else {
			defer inv.Stop()
		}
		invCancel()
	}
	_ = nc // referenced for lifecycle clarity; closed via jsConn.Close()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), logger.ReqID(), logger.GinMiddleware(log))
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	handler.New(svc, signer).Register(r)

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Info().Str("port", cfg.HTTPPort).Msg("team-svc listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("server died")
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	log.Info().Msg("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}
