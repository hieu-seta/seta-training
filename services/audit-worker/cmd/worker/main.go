package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hieu-seta/seta-training/pkg/events"
	"github.com/hieu-seta/seta-training/pkg/logger"
	"github.com/hieu-seta/seta-training/pkg/pg"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/config"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/consumer"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/repo"
	"github.com/hieu-seta/seta-training/services/audit-worker/internal/service"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
)

func main() {
	if err := run(); err != nil {
		// Use stderr directly — logger may not be ready yet.
		fmt.Fprintf(os.Stderr, "audit-worker fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() (retErr error) {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logger.New("audit-worker", cfg.LogLevel, os.Stderr)

	db, err := pg.Open(pg.Config{DSN: cfg.PostgresDSN, Schema: "audit"})
	if err != nil {
		return fmt.Errorf("pg open: %w", err)
	}

	nc, err := nats.Connect(cfg.NATSURL, nats.Timeout(5*time.Second), nats.MaxReconnects(-1))
	if err != nil {
		return fmt.Errorf("nats connect: %w", err)
	}
	defer func() { _ = nc.Drain() }()

	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("jetstream ctx: %w", err)
	}
	if err := events.EnsureStreams(context.Background(), js); err != nil {
		return fmt.Errorf("ensure streams: %w", err)
	}

	auditRepo := repo.New(db)
	auditSvc := service.New(auditRepo)
	cons := consumer.New(js, auditSvc, log)

	if err := startConsumer(cons, log); err != nil {
		return err
	}

	waitForSignal(log)
	drainConsumer(cons, log)
	return nil
}

func startConsumer(cons *consumer.Consumer, log zerolog.Logger) error {
	startCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := cons.Start(startCtx); err != nil {
		return fmt.Errorf("consumer start: %w", err)
	}
	log.Info().Msg("audit-worker running")
	return nil
}

func waitForSignal(log zerolog.Logger) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	log.Info().Msg("shutting down — draining consumers (10s max)")
}

func drainConsumer(cons *consumer.Consumer, log zerolog.Logger) {
	done := make(chan struct{})
	go func() { cons.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		log.Warn().Msg("drain timeout — exiting")
	}
}
