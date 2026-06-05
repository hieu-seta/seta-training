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
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
	"github.com/hieu-seta/seta-training/pkg/logger"
	"github.com/hieu-seta/seta-training/pkg/pg"
	"github.com/hieu-seta/seta-training/services/auth/internal/config"
	"github.com/hieu-seta/seta-training/services/auth/internal/handler"
	"github.com/hieu-seta/seta-training/services/auth/internal/repo"
	"github.com/hieu-seta/seta-training/services/auth/internal/service"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New("auth-svc", cfg.LogLevel, os.Stderr)

	db, err := pg.Open(pg.Config{DSN: cfg.PostgresDSN, Schema: "auth"})
	if err != nil {
		log.Fatal().Err(err).Msg("pg open")
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
	if err := pingRedis(rdb, 3*time.Second); err != nil {
		log.Fatal().Err(err).Msg("redis ping")
	}

	signer := jwtauth.NewSigner(cfg.JWTSecret)
	users := repo.NewUserRepo(db)
	refresh := repo.NewRefreshRepo(rdb)
	svc := service.New(users, refresh, signer, service.Config{
		AccessTTL:  cfg.AccessTTL,
		RefreshTTL: cfg.RefreshTTL,
		BcryptCost: cfg.BcryptCost,
	})
	imp := service.NewImport(svc)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), logger.ReqID(), logger.GinMiddleware(log))
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	handler.New(svc, signer).Register(r)
	handler.NewImportHandler(imp, signer).Register(r)

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info().Str("port", cfg.HTTPPort).Msg("auth-svc listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("server died")
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	log.Info().Msg("shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = srv.Shutdown(shutCtx)
	_ = rdb.Close()
}

func pingRedis(rdb *redis.Client, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return rdb.Ping(ctx).Err()
}
